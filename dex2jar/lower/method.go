package lower

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

type methodLowerer struct {
	file     *model.File
	class    *model.Class
	encoded  *model.EncodedMethod
	method   *ir.Method
	builder  *classfilebuilder.Class
	code     *classfilebuilder.Code
	homeBase uint16

	labels             map[uint32]*classfilebuilder.Label
	addressLabels      map[uint32]*classfilebuilder.Label
	index              int
	endLabel           *classfilebuilder.Label
	allocationByNew    map[int]ir.Allocation
	allocationByCtor   map[int]ir.Allocation
	allocationAliasOps map[int]bool
	registerDescriptor []string
	// homeSlots maps a DEX register to the JVM local where its value starts.
	// Registers that can hold a category-2 value reserve a second slot.
	homeSlots []uint16
	// homeLimit is the number of local slots the register homes occupy.
	homeLimit uint16
	// handlerTrampolines maps a handler address whose block never binds the
	// caught exception to the stub that discards it.
	handlerTrampolines map[uint32]*classfilebuilder.Label

	// delegationIndex marks this-constructor delegation; field stores that DEX
	// permits before it are emitted immediately after it.
	delegationIndex int
	deferredStores  []int
}

func (l *methodLowerer) lower() error {
	if l.code == nil {
		return methodDiagnostic(l.method.Context, nil, "method", fmt.Errorf("concrete method has no builder"))
	}
	if err := l.assignHomes(); err != nil {
		return methodDiagnostic(l.method.Context, nil, "locals", err)
	}
	l.labels = make(map[uint32]*classfilebuilder.Label, len(l.method.Blocks))
	for _, block := range l.method.Blocks {
		l.labels[block.Start] = l.code.NewLabel()
	}
	l.registerDescriptor = make([]string, len(l.method.RegisterTypes))
	for register, category := range l.method.RegisterTypes {
		l.registerDescriptor[register] = category.Descriptor
	}
	l.delegationIndex = -1
	l.allocationAliasOps = make(map[int]bool)
	for _, allocation := range l.method.Allocations {
		l.allocationByNew[allocation.NewInstruction] = allocation
		l.allocationByCtor[allocation.Constructor] = allocation
		if allocation.Fused {
			for index := allocation.NewInstruction + 1; index < allocation.Constructor; index++ {
				l.allocationAliasOps[index] = true
			}
		}
		if err := l.validateAllocationCFG(allocation); err != nil {
			return err
		}
	}
	if err := l.validateMoveResults(); err != nil {
		return err
	}
	if err := l.validateInvokes(); err != nil {
		return err
	}
	if err := l.validateDefiniteAssignments(); err != nil {
		return err
	}
	if err := l.validateReferenceUses(); err != nil {
		return err
	}
	if err := l.emitPrologue(); err != nil {
		return err
	}
	l.prepareAddressLabels()
	deferred := make(map[int]bool, len(l.deferredStores))
	for _, index := range l.deferredStores {
		deferred[index] = true
	}
	for _, block := range l.method.Blocks {
		l.code.Mark(l.labels[block.Start])
		for _, index := range block.Instructions {
			instruction := &l.method.Instructions[index]
			// A deferred store still owns its address: debug data may reference
			// it, and an unmarked label would fail class assembly.
			if label := l.addressLabels[instruction.Offset]; label != nil && label != l.labels[block.Start] {
				l.code.Mark(label)
			}
			if deferred[index] {
				continue
			}
			if err := l.emitInstruction(index); err != nil {
				return err
			}
			if index == l.delegationIndex {
				for _, store := range l.deferredStores {
					// The deferred store keeps its own DEX position, so its
					// category state is the one recorded for that instruction.
					previous := l.index
					if err := l.emitInstruction(store); err != nil {
						return err
					}
					l.index = previous
				}
			}
		}
	}
	if l.endLabel != nil {
		l.code.Mark(l.endLabel)
	}
	l.emitHandlerTrampolines()
	if err := l.emitExceptionTable(); err != nil {
		return err
	}
	if err := l.emitDebug(); err != nil {
		return err
	}
	l.code.SetMaxLocals(l.homeBase + l.homeLimit)
	return nil
}

// prepareAddressLabels binds a JVM label to every DEX address that debug data
// refers to, so line numbers and local ranges survive branch widening.
// assignHomes gives every DEX register a JVM local. A register that ever holds
// a category-2 value reserves two consecutive slots so no other register can
// occupy the second half of a long or double.
func (l *methodLowerer) assignHomes() error {
	registerCount := int(l.method.Code.RegistersSize)
	widths := make([]int, registerCount)
	for register := range widths {
		widths[register] = 1
	}
	for register, category := range l.method.RegisterTypes {
		if register < registerCount && category.Kind.Words() == 2 {
			widths[register] = 2
		}
	}
	if stride := l.method.CategoryStride; stride > 0 {
		for offset, category := range l.method.Categories {
			if category.Kind.Words() == 2 {
				if register := offset % stride; register < registerCount {
					widths[register] = 2
				}
			}
		}
	}
	l.homeSlots = make([]uint16, registerCount)
	next := 0
	for register, width := range widths {
		l.homeSlots[register] = uint16(next)
		next += width
	}
	if int(l.homeBase)+next > math.MaxUint16 {
		return fmt.Errorf("incoming arguments and DEX register homes require %d locals", int(l.homeBase)+next)
	}
	l.homeLimit = uint16(next)
	return nil
}

func (l *methodLowerer) prepareAddressLabels() {
	l.addressLabels = make(map[uint32]*classfilebuilder.Label, len(l.method.Blocks))
	for _, block := range l.method.Blocks {
		l.addressLabels[block.Start] = l.labels[block.Start]
	}
	boundary := make(map[uint32]bool, len(l.method.Instructions)+1)
	for index := range l.method.Instructions {
		boundary[l.method.Instructions[index].Offset] = true
	}
	codeUnits := uint32(len(l.method.Code.Instructions))
	need := func(address uint32) {
		if _, ok := l.addressLabels[address]; ok {
			return
		}
		if address != codeUnits && !boundary[address] {
			return
		}
		l.addressLabels[address] = l.code.NewLabel()
	}
	// Exception table boundaries are needed even without debug information.
	for _, try := range l.method.Code.Tries {
		need(try.StartAddress)
		need(try.StartAddress + uint32(try.InstructionCount))
	}
	if debug := l.method.Code.Debug; debug != nil {
		for _, position := range debug.Positions {
			need(position.Address)
		}
		for _, local := range debug.Locals {
			need(local.Start)
			need(local.End)
		}
	}
	need(codeUnits)
	if label, ok := l.addressLabels[codeUnits]; ok {
		l.endLabel = label
	}
}

// emitExceptionTable records every DEX try item as JVM exception-table
// entries. Handler order is preserved: typed handlers first, then catch-all.
func (l *methodLowerer) emitExceptionTable() error {
	for _, try := range l.method.Code.Tries {
		start := l.addressLabels[try.StartAddress]
		end := l.addressLabels[try.StartAddress+uint32(try.InstructionCount)]
		if start == nil || end == nil {
			return methodDiagnostic(l.method.Context, nil, "try", fmt.Errorf("try item [%d,%d) has no bound labels", try.StartAddress, try.StartAddress+uint32(try.InstructionCount)))
		}
		emit := func(address uint32, catchType string) error {
			handler := l.labels[address]
			if handler == nil {
				return methodDiagnostic(l.method.Context, nil, "try", fmt.Errorf("handler address %d is not a basic block", address))
			}
			// A handler that does not begin with move-exception never binds the
			// caught exception. The JVM still pushes it, so enter through a
			// trampoline that discards it before the shared block runs.
			if trampoline, ok := l.handlerTrampolines[address]; ok {
				handler = trampoline
			}
			class := ""
			if catchType != "" {
				name, err := objectInternalName(catchType)
				if err != nil {
					return methodDiagnostic(l.method.Context, nil, "try", fmt.Errorf("handler type %q: %w", catchType, err))
				}
				class = name
			}
			l.code.Catch(start, end, handler, class)
			return nil
		}
		for _, handler := range try.Handler.Handlers {
			if err := emit(handler.Address, handler.Type); err != nil {
				return err
			}
		}
		if try.Handler.CatchAllAddress != nil {
			if err := emit(*try.Handler.CatchAllAddress, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// emitHandlerTrampolines appends one stub per handler that never binds the
// caught exception. DEX makes the exception available only through
// move-exception, so a handler without one discards it; the JVM dispatcher
// pushes it unconditionally, and the stub pops it before the shared handler
// block runs.
func (l *methodLowerer) emitHandlerTrampolines() {
	l.handlerTrampolines = make(map[uint32]*classfilebuilder.Label)
	first := make(map[uint32]byte, len(l.method.Blocks))
	for _, block := range l.method.Blocks {
		if len(block.Instructions) > 0 {
			first[block.Start] = l.method.Instructions[block.Instructions[0]].Opcode
		}
	}
	for _, try := range l.method.Code.Tries {
		addresses := make([]uint32, 0, len(try.Handler.Handlers)+1)
		for _, handler := range try.Handler.Handlers {
			addresses = append(addresses, handler.Address)
		}
		if try.Handler.CatchAllAddress != nil {
			addresses = append(addresses, *try.Handler.CatchAllAddress)
		}
		for _, address := range addresses {
			if _, ok := l.handlerTrampolines[address]; ok {
				continue
			}
			opcode, ok := first[address]
			if !ok {
				// Missing handler blocks are reported by emitExceptionTable.
				continue
			}
			if opcode == 0x0d {
				continue
			}
			label := l.code.NewLabel()
			l.code.Mark(label).Emit(classfilebuilder.POP)
			l.code.Branch(classfilebuilder.GOTO, l.labels[address])
			l.handlerTrampolines[address] = label
		}
	}
}

// emitDebug records line numbers, local variable names and parameter names from
// the original DEX debug_info. Positions that cannot be anchored to a real
// instruction boundary are dropped rather than misattributed.
func (l *methodLowerer) emitDebug() error {
	debug := l.method.Code.Debug
	if debug == nil {
		return nil
	}
	for _, position := range debug.Positions {
		label := l.addressLabels[position.Address]
		if label == nil || position.Line == 0 {
			// R8 emits line 0 for synthetic code; a JVM LineNumberTable cannot
			// represent it, so the entry is dropped instead of failing the class.
			continue
		}
		l.code.Line(label, uint16(position.Line))
	}
	codeUnits := uint32(len(l.method.Code.Instructions))
	startLabel := l.addressLabels[0]
	// The JVM rejects duplicate LocalVariableTable entries for the same slot and
	// range, so an explicitly described range keeps only that entry.
	type localRange struct {
		register   uint16
		start, end *classfilebuilder.Label
	}
	described := make(map[localRange]bool)
	for _, local := range debug.Locals {
		if local.Name == "" || local.Type == "" || local.Start >= local.End || local.End > codeUnits {
			continue
		}
		if local.Type == "J" || local.Type == "D" || local.Type == "F" {
			continue
		}
		start, end := l.addressLabels[local.Start], l.addressLabels[local.End]
		if start == nil || end == nil {
			continue
		}
		l.code.Local(start, end, local.Name, local.Type, l.home(local.Register))
		described[localRange{register: local.Register, start: start, end: end}] = true
	}
	parameterRegister := l.method.Code.RegistersSize - l.method.Code.InsSize
	if l.encoded.AccessFlags&dexStatic == 0 {
		parameterRegister++
	}
	if l.endLabel == nil || startLabel == nil {
		return nil
	}
	for index, name := range debug.ParameterNames {
		if name == "" {
			return nil
		}
		register := int(parameterRegister) + index
		if register >= int(l.method.Code.RegistersSize) {
			return nil
		}
		// Without a proven descriptor the parameter name is dropped rather than
		// described as java.lang.Object, which would invent a type.
		descriptor := l.registerDescriptor[register]
		if descriptor == "" {
			continue
		}
		key := localRange{register: uint16(register), start: startLabel, end: l.endLabel}
		if described[key] {
			continue
		}
		described[key] = true
		l.code.Local(startLabel, l.endLabel, name, descriptor, l.home(uint16(register)))
	}
	return nil
}

func (l *methodLowerer) validateAllocationCFG(allocation ir.Allocation) error {
	newBlock, ctorBlock := -1, -1
	for _, block := range l.method.Blocks {
		for _, index := range block.Instructions {
			if index == allocation.NewInstruction {
				newBlock = block.ID
			}
			if index == allocation.Constructor {
				ctorBlock = block.ID
			}
		}
	}
	instruction := &l.method.Instructions[allocation.NewInstruction]
	if newBlock < 0 || newBlock != ctorBlock {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("new-instance and constructor must remain in one basic block"))
	}
	constructor := l.method.Instructions[allocation.Constructor]
	for _, argument := range constructor.Regs[1:] {
		for _, alias := range allocation.AliasRegisters {
			if argument == alias {
				return methodDiagnostic(l.method.Context, &constructor, "", fmt.Errorf("uninitialized allocation is also a constructor argument"))
			}
		}
	}
	return nil
}

func (l *methodLowerer) validateMoveResults() error {
	branchTargets := make(map[uint32]bool)
	for _, instruction := range l.method.Instructions {
		if instruction.Target >= 0 {
			branchTargets[uint32(instruction.Target)] = true
		}
	}
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if instruction.Opcode == 0x0a || instruction.Opcode == 0x0c {
			if branchTargets[instruction.Offset] {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("control flow may not enter at move-result"))
			}
		}
	}
	return nil
}

func (l *methodLowerer) emitPrologue() error {
	parameterRegister := l.method.Code.RegistersSize - l.method.Code.InsSize
	jvmLocal := uint16(0)
	if l.encoded.AccessFlags&dexStatic == 0 {
		l.code.Load(classfilebuilder.RefLocal, jvmLocal).Store(classfilebuilder.RefLocal, l.home(parameterRegister))
		parameterRegister++
		jvmLocal++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		kind, err := fieldKind(descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "prologue", err)
		}
		words := uint16(kind.Words())
		local := localKind(kind)
		l.code.Load(local, jvmLocal).Store(local, l.home(parameterRegister))
		parameterRegister += words
		jvmLocal += words
	}
	return nil
}

func (l *methodLowerer) emitInstruction(index int) error {
	l.index = index
	instruction := &l.method.Instructions[index]
	if allocation, ok := l.allocationByNew[index]; ok {
		if allocation.Fused {
			return nil
		}
		return l.emitAllocationNew(instruction, allocation)
	}
	if l.allocationAliasOps[index] {
		return nil
	}
	if allocation, ok := l.allocationByCtor[index]; ok {
		if allocation.Fused {
			return l.emitAllocationFused(instruction, allocation)
		}
		return l.emitAllocationCtor(instruction, allocation)
	}
	switch {
	case instruction.Opcode == 0x00:
		l.code.Emit(classfilebuilder.NOP)
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09:
		return l.emitMove(instruction)
	case instruction.Opcode >= 0x0a && instruction.Opcode <= 0x0c:
		// The preceding invoke stores directly into the stable register home.
	case instruction.Opcode == 0x0d:
		// The JVM pushes the caught exception at handler entry; store it.
		l.store(instruction.Regs[0])
	case instruction.Opcode == 0x0e:
		l.code.Emit(classfilebuilder.RETURN)
	case instruction.Opcode == 0x0f:
		kind, err := ir.DescriptorCategory(l.method.Proto.ReturnType)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.load(instruction.Regs[0])
		if kind == ir.Float {
			l.code.Emit(classfilebuilder.FRETURN)
		} else {
			l.code.Emit(classfilebuilder.IRETURN)
		}
	case instruction.Opcode == 0x10:
		return l.emitWideReturn(instruction)
	case instruction.Opcode == 0x11:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.ARETURN)
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15:
		if err := l.pushDexInt(instruction.Regs[0], int32(instruction.Literal)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.store(instruction.Regs[0])
	case instruction.Opcode >= 0x16 && instruction.Opcode <= 0x19:
		if err := l.pushDexWide(instruction.Regs[0], instruction.Literal); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.store(instruction.Regs[0])
	case instruction.Opcode == 0x1a || instruction.Opcode == 0x1b:
		constant, err := l.builder.Pool().String(l.file.Strings[instruction.Index])
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.LDC(constant)
		l.store(instruction.Regs[0])
	case instruction.Opcode == 0x1c:
		if err := l.emitConstClass(instruction); err != nil {
			return err
		}
	case instruction.Opcode == 0x1d:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.MONITORENTER)
	case instruction.Opcode == 0x1e:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.MONITOREXIT)
	case instruction.Opcode == 0x1f:
		return l.emitTypeInstruction(instruction, classfilebuilder.CHECKCAST)
	case instruction.Opcode == 0x20:
		return l.emitTypeInstruction(instruction, classfilebuilder.INSTANCEOF)
	case instruction.Opcode == 0x21:
		l.load(instruction.Regs[1]).Emit(classfilebuilder.ARRAYLENGTH)
		l.store(instruction.Regs[0])
	case instruction.Opcode == 0x23:
		return l.emitNewArray(instruction)
	case instruction.Opcode >= 0x24 && instruction.Opcode <= 0x26:
		if instruction.Opcode == 0x26 {
			return l.emitFillArrayData(instruction)
		}
		return l.emitFilledNewArray(index, instruction)
	case instruction.Opcode == 0x27:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.ATHROW)
	case instruction.Opcode >= 0x28 && instruction.Opcode <= 0x2a:
		l.code.Branch(classfilebuilder.GOTO, l.labels[uint32(instruction.Target)])
	case instruction.Opcode == 0x2b || instruction.Opcode == 0x2c:
		return l.emitSwitch(instruction)
	case instruction.Opcode >= 0x2d && instruction.Opcode <= 0x31:
		return l.emitCompare(instruction)
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x3d:
		return l.emitIf(instruction)
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x51:
		return l.emitArray(instruction)
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d:
		return l.emitField(instruction)
	case isInvoke(instruction.Opcode):
		return l.emitInvoke(index, instruction)
	case instruction.Opcode >= 0x7b && instruction.Opcode <= 0x8f:
		return l.emitUnary(instruction)
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0xaf || instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xcf || instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		return l.emitArithmetic(instruction)
	default:
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("lowering has no implementation for validated opcode"))
	}
	return nil
}

func (l *methodLowerer) emitMove(instruction *ir.Instruction) error {
	kind := l.method.CategoryAt(l.index, instruction.Regs[0]).Kind
	if definition, ok := l.method.DefinitionAt(l.index); ok && definition.Register == int32(instruction.Regs[0]) {
		kind = definition.Category.Kind
	}
	if kind == ir.Unknown {
		kind = l.method.CategoryAt(l.index, instruction.Regs[1]).Kind
	}
	if kind == ir.Unknown {
		return nil
	}
	switch {
	case instruction.Opcode <= 0x03:
		if kind != ir.Int && kind != ir.Float && kind != ir.Bits32 && kind != ir.Null {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("move opcode does not match stable register category %s", kind))
		}
	case instruction.Opcode <= 0x06:
		if !isWideKind(kind) {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("move-wide opcode does not match stable register category %s", kind))
		}
	default:
		if !isReferenceKind(kind) {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("move-object opcode does not match stable register category %s", kind))
		}
	}
	l.load(instruction.Regs[1])
	l.store(instruction.Regs[0])
	if isReferenceKind(kind) {
		l.registerDescriptor[instruction.Regs[0]] = l.registerDescriptor[instruction.Regs[1]]
	}
	return nil
}

// emitWideReturn returns the category-2 value the method prototype declares.
func (l *methodLowerer) emitWideReturn(instruction *ir.Instruction) error {
	kind, err := ir.DescriptorCategory(l.method.Proto.ReturnType)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.load(instruction.Regs[0])
	switch kind {
	case ir.Long:
		l.code.Emit(classfilebuilder.LRETURN)
	case ir.Double:
		l.code.Emit(classfilebuilder.DRETURN)
	default:
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("return-wide requires a long or double return type, not %q", l.method.Proto.ReturnType))
	}
	return nil
}

func (l *methodLowerer) emitConstClass(instruction *ir.Instruction) error {
	descriptor := l.file.Types[instruction.Index].Descriptor
	if owner, ok := primitiveClassOwner(descriptor); ok {
		constant, err := l.builder.Pool().Fieldref(owner, "TYPE", "Ljava/lang/Class;")
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.CP(classfilebuilder.GETSTATIC, constant)
	} else {
		name, err := ir.InternalName(descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		constant, err := l.builder.Pool().Class(name)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.LDC(constant)
	}
	l.store(instruction.Regs[0])
	l.registerDescriptor[instruction.Regs[0]] = "Ljava/lang/Class;"
	return nil
}

func primitiveClassOwner(descriptor string) (string, bool) {
	owner, ok := map[string]string{
		"V": "java/lang/Void", "Z": "java/lang/Boolean", "B": "java/lang/Byte",
		"C": "java/lang/Character", "S": "java/lang/Short", "I": "java/lang/Integer",
	}[descriptor]
	return owner, ok
}

func (l *methodLowerer) emitTypeInstruction(instruction *ir.Instruction, opcode classfilebuilder.Opcode) error {
	descriptor := l.file.Types[instruction.Index].Descriptor
	name, err := ir.InternalName(descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	constant, err := l.builder.Pool().Class(name)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	source := instruction.Regs[0]
	if opcode == classfilebuilder.INSTANCEOF {
		source = instruction.Regs[1]
	}
	l.load(source)
	l.code.CP(opcode, constant)
	l.store(instruction.Regs[0])
	if opcode == classfilebuilder.CHECKCAST {
		l.registerDescriptor[instruction.Regs[0]] = descriptor
	}
	return nil
}

func (l *methodLowerer) emitNewArray(instruction *ir.Instruction) error {
	descriptor := l.file.Types[instruction.Index].Descriptor
	if !strings.HasPrefix(descriptor, "[") || len(descriptor) < 2 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("new-array type %q is not an array", descriptor))
	}
	l.load(instruction.Regs[1])
	component := descriptor[1:]
	if atype, ok := primitiveArrayType(component); ok {
		l.code.Byte(classfilebuilder.NEWARRAY, atype)
	} else {
		if len(component) == 1 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unsupported primitive array component %q", component))
		}
		name, err := ir.InternalName(component)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unsupported array component: %w", err))
		}
		constant, err := l.builder.Pool().Class(name)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.CP(classfilebuilder.ANEWARRAY, constant)
	}
	l.store(instruction.Regs[0])
	l.registerDescriptor[instruction.Regs[0]] = descriptor
	return nil
}

// primitiveArrayType maps a one-character DEX primitive descriptor to the JVM
// NEWARRAY atype.
func primitiveArrayType(component string) (int, bool) {
	atypes := map[string]int{"Z": 4, "C": 5, "F": 6, "D": 7, "B": 8, "S": 9, "I": 10, "J": 11}
	atype, ok := atypes[component]
	return atype, ok
}

// arrayElementOpcode maps a DEX element opcode and the array component
// descriptor to the JVM element access. DEX shares aget/aput between int and
// float values, so the component type selects the JVM opcode.
func arrayElementOpcode(opcode byte, component string) (classfilebuilder.Opcode, error) {
	stores := opcode >= 0x4b
	var load, store classfilebuilder.Opcode
	switch component {
	case "I":
		load, store = classfilebuilder.IALOAD, classfilebuilder.IASTORE
	case "F":
		load, store = classfilebuilder.FALOAD, classfilebuilder.FASTORE
	case "J":
		load, store = classfilebuilder.LALOAD, classfilebuilder.LASTORE
	case "D":
		load, store = classfilebuilder.DALOAD, classfilebuilder.DASTORE
	case "Z", "B":
		load, store = classfilebuilder.BALOAD, classfilebuilder.BASTORE
	case "C":
		load, store = classfilebuilder.CALOAD, classfilebuilder.CASTORE
	case "S":
		load, store = classfilebuilder.SALOAD, classfilebuilder.SASTORE
	default:
		if component == "" || component[0] != 'L' && component[0] != '[' {
			return 0, fmt.Errorf("unsupported array component %q", component)
		}
		load, store = classfilebuilder.AALOAD, classfilebuilder.AASTORE
	}
	if stores {
		return store, nil
	}
	return load, nil
}

// emitFillArrayData materializes a primitive array literal by storing every
// payload element. The array register's flow type selects the element store,
// and the payload width must agree with it.
func (l *methodLowerer) emitFillArrayData(instruction *ir.Instruction) error {
	array := l.method.CategoryAt(l.index, instruction.Regs[0])
	if len(array.Descriptor) < 2 || array.Descriptor[0] != '[' {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("fill-array-data register v%d has no array type", instruction.Regs[0]))
	}
	component := array.Descriptor[1:]
	width, store, err := fillArrayShape(component)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if int(instruction.DataWidth) != width || len(instruction.Data)%width != 0 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("fill-array-data payload does not match element type %q", component))
	}
	l.load(instruction.Regs[0])
	for begin := 0; begin+width <= len(instruction.Data); begin += width {
		l.code.Emit(classfilebuilder.DUP)
		if err := l.pushInt(int32(begin / width)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		if err := l.pushFillElement(component, instruction.Data[begin:begin+width]); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.Emit(store)
	}
	l.code.Emit(classfilebuilder.POP)
	return nil
}

// fillArrayShape maps a fill-array-data component to its element width and the
// JVM array store.
func fillArrayShape(component string) (int, classfilebuilder.Opcode, error) {
	switch component {
	case "Z", "B":
		return 1, classfilebuilder.BASTORE, nil
	case "C":
		return 2, classfilebuilder.CASTORE, nil
	case "S":
		return 2, classfilebuilder.SASTORE, nil
	case "I":
		return 4, classfilebuilder.IASTORE, nil
	case "F":
		return 4, classfilebuilder.FASTORE, nil
	case "J":
		return 8, classfilebuilder.LASTORE, nil
	case "D":
		return 8, classfilebuilder.DASTORE, nil
	default:
		return 0, 0, fmt.Errorf("fill-array-data requires a primitive array, not %q", component)
	}
}

// pushFillElement pushes one little-endian payload element as the JVM value the
// element store expects.
func (l *methodLowerer) pushFillElement(component string, data []byte) error {
	switch component {
	case "Z":
		return l.pushInt(int32(data[0]))
	case "B":
		return l.pushInt(int32(int8(data[0])))
	case "C":
		return l.pushInt(int32(binary.LittleEndian.Uint16(data)))
	case "S":
		return l.pushInt(int32(int16(binary.LittleEndian.Uint16(data))))
	case "I":
		return l.pushInt(int32(binary.LittleEndian.Uint32(data)))
	case "F":
		constant, err := l.builder.Pool().Float(binary.LittleEndian.Uint32(data))
		if err != nil {
			return err
		}
		l.code.LDC(constant)
		return nil
	case "J":
		constant, err := l.builder.Pool().Long(int64(binary.LittleEndian.Uint64(data)))
		if err != nil {
			return err
		}
		l.code.LDC2(constant)
		return nil
	default:
		constant, err := l.builder.Pool().Double(binary.LittleEndian.Uint64(data))
		if err != nil {
			return err
		}
		l.code.LDC2(constant)
		return nil
	}
}

// emitFilledNewArray builds an array literal. The DEX result register is only
// assigned by the following move-result-object, which lowering consumes here.
func (l *methodLowerer) emitFilledNewArray(index int, instruction *ir.Instruction) error {
	descriptor := l.file.Types[instruction.Index].Descriptor
	if len(descriptor) < 2 || descriptor[0] != '[' {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("filled-new-array type %q is not an array", descriptor))
	}
	component := descriptor[1:]
	storeOp, atype, className, err := filledArrayShape(component)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if err := l.pushInt(int32(len(instruction.Regs))); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if atype != 0 {
		l.code.Byte(classfilebuilder.NEWARRAY, atype)
	} else {
		constant, err := l.builder.Pool().Class(className)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.CP(classfilebuilder.ANEWARRAY, constant)
	}
	for offset, register := range instruction.Regs {
		l.code.Emit(classfilebuilder.DUP)
		if err := l.pushInt(int32(offset)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.load(register)
		l.code.Emit(storeOp)
	}
	next := index + 1
	if next >= len(l.method.Instructions) || l.method.Instructions[next].Opcode != 0x0c {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("filled-new-array must be followed by move-result-object"))
	}
	l.storeAt(l.method.Instructions[next].Regs[0], next)
	return nil
}

// filledArrayShape maps an array component descriptor to its construction and
// element-store form.
func filledArrayShape(component string) (classfilebuilder.Opcode, int, string, error) {
	if atype, ok := primitiveArrayType(component); ok {
		stores := map[string]classfilebuilder.Opcode{
			"I": classfilebuilder.IASTORE, "Z": classfilebuilder.BASTORE, "B": classfilebuilder.BASTORE,
			"C": classfilebuilder.CASTORE, "S": classfilebuilder.SASTORE,
		}
		return stores[component], atype, "", nil
	}
	if component == "J" || component == "D" || component == "F" {
		return 0, 0, "", fmt.Errorf("unsupported filled array component %q", component)
	}
	name, err := ir.InternalName(component)
	if err != nil {
		return 0, 0, "", fmt.Errorf("unsupported array component: %w", err)
	}
	return classfilebuilder.AASTORE, 0, name, nil
}

func (l *methodLowerer) emitArray(instruction *ir.Instruction) error {
	// The element type comes from the array register's flow type; DEX does not
	// encode it in the instruction.
	array := l.method.CategoryAt(l.index, instruction.Regs[1])
	if len(array.Descriptor) < 2 || array.Descriptor[0] != '[' {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("array register v%d has no array type", instruction.Regs[1]))
	}
	op, err := arrayElementOpcode(instruction.Opcode, array.Descriptor[1:])
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.load(instruction.Regs[1])
	l.load(instruction.Regs[2])
	if instruction.Opcode <= 0x4a {
		l.code.Emit(op)
		l.store(instruction.Regs[0])
		return nil
	}
	l.load(instruction.Regs[0])
	l.code.Emit(op)
	return nil
}

// emitAllocationNew allocates at the original new-instance position and keeps
// the uninitialized reference in its register home. Instructions that d8 emits
// between allocation and initialization then keep their original order.
func (l *methodLowerer) emitAllocationNew(instruction *ir.Instruction, allocation ir.Allocation) error {
	name, err := objectInternalName(allocation.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	classIndex, err := l.builder.Pool().Class(name)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.CP(classfilebuilder.NEW, classIndex)
	l.store(instruction.Regs[0])
	return nil
}

// emitAllocationFused emits allocation and initialization as one sequence. This
// keeps the historical, more readable shape whenever nothing sits between them.
func (l *methodLowerer) emitAllocationFused(instruction *ir.Instruction, allocation ir.Allocation) error {
	name, err := objectInternalName(allocation.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	classIndex, err := l.builder.Pool().Class(name)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	callee := l.file.Methods[instruction.Index]
	proto := l.file.Protos[callee.ProtoIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	methodIndex, err := l.builder.Pool().Methodref(name, "<init>", descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.CP(classfilebuilder.NEW, classIndex).Emit(classfilebuilder.DUP)
	if err := l.loadArguments(instruction.Regs, proto, 1); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.CP(classfilebuilder.INVOKESPECIAL, methodIndex)
	first := allocation.AliasRegisters[0]
	l.store(first)
	for _, register := range allocation.AliasRegisters[1:] {
		l.load(first)
		l.store(register)
	}
	return nil
}

// emitAllocationCtor performs the deferred constructor call on the receiver that
// still holds the uninitialized reference.
func (l *methodLowerer) emitAllocationCtor(instruction *ir.Instruction, allocation ir.Allocation) error {
	name, err := objectInternalName(allocation.Descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	callee := l.file.Methods[instruction.Index]
	proto := l.file.Protos[callee.ProtoIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	methodIndex, err := l.builder.Pool().Methodref(name, "<init>", descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.load(instruction.Regs[0])
	if err := l.loadArguments(instruction.Regs, proto, 1); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.CP(classfilebuilder.INVOKESPECIAL, methodIndex)
	return nil
}

// loadArguments loads the argument registers of an invoke. The DEX register
// list holds one entry per word, so a long or double argument lists both of its
// registers; only the first one is a JVM value.
func (l *methodLowerer) loadArguments(registers []uint16, proto model.Proto, position int) error {
	for _, descriptor := range proto.Parameters {
		if position >= len(registers) {
			return nil
		}
		kind, err := ir.DescriptorCategory(descriptor)
		if err != nil {
			return err
		}
		l.load(registers[position])
		position += kind.Words()
	}
	return nil
}

// emitSwitch lowers a packed or sparse DEX switch. DEX switches have no
// explicit default case: a value that matches no key falls through to the
// instruction after the switch, which is the JVM default target.
func (l *methodLowerer) emitSwitch(instruction *ir.Instruction) error {
	defaultTarget := l.labels[instruction.End()]
	if defaultTarget == nil {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("switch has no fall-through block"))
	}
	targets := make([]*classfilebuilder.Label, len(instruction.Targets))
	for index, target := range instruction.Targets {
		targets[index] = l.labels[uint32(target)]
		if targets[index] == nil {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("switch target %d is not a basic block", target))
		}
	}
	if kind := l.method.CategoryAt(l.index, instruction.Regs[0]).Kind; !isIntKind(kind) && kind != ir.Unknown {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("switch selector has category %s, want int", kind))
	}
	l.load(instruction.Regs[0])
	if len(targets) == 0 {
		// A table with no cases can only fall through.
		l.code.Branch(classfilebuilder.GOTO, defaultTarget)
		return nil
	}
	if instruction.Opcode == 0x2b {
		// A packed switch's keys are consecutive starting at first_key.
		l.code.TableSwitch(instruction.FirstKey, defaultTarget, targets...)
		return nil
	}
	l.code.LookupSwitch(defaultTarget, instruction.Keys, targets...)
	return nil
}

// emitCompare lowers float/double/long comparisons, which all produce -1, 0 or
// 1 in an int register.
func (l *methodLowerer) emitCompare(instruction *ir.Instruction) error {
	opcodes := map[byte]classfilebuilder.Opcode{
		0x2d: classfilebuilder.FCMPL,
		0x2e: classfilebuilder.FCMPG,
		0x2f: classfilebuilder.DCMPL,
		0x30: classfilebuilder.DCMPG,
		0x31: classfilebuilder.LCMP,
	}
	opcode, ok := opcodes[instruction.Opcode]
	if !ok {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unsupported comparison opcode %#x", instruction.Opcode))
	}
	l.load(instruction.Regs[1])
	l.load(instruction.Regs[2])
	l.code.Emit(opcode)
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitIf(instruction *ir.Instruction) error {
	var opcode classfilebuilder.Opcode
	if instruction.Opcode <= 0x37 {
		left := l.method.CategoryAt(l.index, instruction.Regs[0]).Kind
		right := l.method.CategoryAt(l.index, instruction.Regs[1]).Kind
		leftReference, rightReference := isReferenceKind(left), isReferenceKind(right)
		if leftReference != rightReference {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("comparison mixes reference and integer register homes"))
		}
		if leftReference && instruction.Opcode > 0x33 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("ordered reference comparison is invalid"))
		}
		l.load(instruction.Regs[0])
		l.load(instruction.Regs[1])
		if leftReference {
			opcode = classfilebuilder.IF_ACMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
		} else {
			if !isIntKind(left) || !isIntKind(right) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("comparison requires int or reference registers, got %s and %s", left, right))
			}
			opcode = classfilebuilder.IF_ICMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
		}
	} else {
		kind := l.method.CategoryAt(l.index, instruction.Regs[0]).Kind
		l.load(instruction.Regs[0])
		if isReferenceKind(kind) {
			if instruction.Opcode == 0x38 {
				opcode = classfilebuilder.IFNULL
			} else if instruction.Opcode == 0x39 {
				opcode = classfilebuilder.IFNONNULL
			} else {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("ordered reference comparison is invalid"))
			}
		} else {
			if !isIntKind(kind) && kind != ir.Unknown {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("branch requires an int or reference register, got %s", kind))
			}
			opcode = classfilebuilder.IFEQ + classfilebuilder.Opcode(instruction.Opcode-0x38)
		}
	}
	l.code.Branch(opcode, l.labels[uint32(instruction.Target)])
	return nil
}

func (l *methodLowerer) emitUnary(instruction *ir.Instruction) error {
	opcodes := map[byte]classfilebuilder.Opcode{
		0x7b: classfilebuilder.INEG,
		0x7d: classfilebuilder.LNEG,
		0x7f: classfilebuilder.FNEG,
		0x80: classfilebuilder.DNEG,
		0x81: classfilebuilder.I2L,
		0x82: classfilebuilder.I2F,
		0x83: classfilebuilder.I2D,
		0x84: classfilebuilder.L2I,
		0x85: classfilebuilder.L2F,
		0x86: classfilebuilder.L2D,
		0x87: classfilebuilder.F2I,
		0x88: classfilebuilder.F2L,
		0x89: classfilebuilder.F2D,
		0x8a: classfilebuilder.D2I,
		0x8b: classfilebuilder.D2L,
		0x8c: classfilebuilder.D2F,
		0x8d: classfilebuilder.I2B,
		0x8e: classfilebuilder.I2C,
		0x8f: classfilebuilder.I2S,
	}
	l.load(instruction.Regs[1])
	switch instruction.Opcode {
	case 0x7c:
		// not-int is x ^ -1; the JVM has no dedicated opcode.
		l.code.Emit(classfilebuilder.ICONST_M1).Emit(classfilebuilder.IXOR)
	case 0x7e:
		constant, err := l.builder.Pool().Long(-1)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.LDC2(constant).Emit(classfilebuilder.LXOR)
	default:
		opcode, ok := opcodes[instruction.Opcode]
		if !ok {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unsupported unary opcode %#x", instruction.Opcode))
		}
		l.code.Emit(opcode)
	}
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitArithmetic(instruction *ir.Instruction) error {
	base := instruction.Opcode
	if base >= 0xb0 && base <= 0xcf {
		base -= 0x20
	}
	if base >= 0xd0 {
		return l.emitLiteralArithmetic(instruction)
	}
	opcodes := []classfilebuilder.Opcode{
		classfilebuilder.IADD, classfilebuilder.ISUB, classfilebuilder.IMUL, classfilebuilder.IDIV, classfilebuilder.IREM,
		classfilebuilder.IAND, classfilebuilder.IOR, classfilebuilder.IXOR, classfilebuilder.ISHL, classfilebuilder.ISHR, classfilebuilder.IUSHR,
		classfilebuilder.LADD, classfilebuilder.LSUB, classfilebuilder.LMUL, classfilebuilder.LDIV, classfilebuilder.LREM,
		classfilebuilder.LAND, classfilebuilder.LOR, classfilebuilder.LXOR, classfilebuilder.LSHL, classfilebuilder.LSHR, classfilebuilder.LUSHR,
		classfilebuilder.FADD, classfilebuilder.FSUB, classfilebuilder.FMUL, classfilebuilder.FDIV, classfilebuilder.FREM,
		classfilebuilder.DADD, classfilebuilder.DSUB, classfilebuilder.DMUL, classfilebuilder.DDIV, classfilebuilder.DREM,
	}
	index := int(base - 0x90)
	if index < 0 || index >= len(opcodes) {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("unsupported arithmetic opcode %#x", instruction.Opcode))
	}
	if instruction.Opcode < 0xb0 {
		l.load(instruction.Regs[1])
		l.load(instruction.Regs[2])
	} else {
		l.load(instruction.Regs[0])
		l.load(instruction.Regs[1])
	}
	l.code.Emit(opcodes[index])
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitLiteralArithmetic(instruction *ir.Instruction) error {
	var opIndex byte
	if instruction.Opcode <= 0xd7 {
		opIndex = instruction.Opcode - 0xd0
	} else {
		opIndex = instruction.Opcode - 0xd8
	}
	opcodes := []classfilebuilder.Opcode{
		classfilebuilder.IADD, classfilebuilder.ISUB, classfilebuilder.IMUL, classfilebuilder.IDIV, classfilebuilder.IREM,
		classfilebuilder.IAND, classfilebuilder.IOR, classfilebuilder.IXOR, classfilebuilder.ISHL, classfilebuilder.ISHR, classfilebuilder.IUSHR,
	}
	if opIndex == 1 {
		if err := l.pushInt(int32(instruction.Literal)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.load(instruction.Regs[1])
	} else {
		l.load(instruction.Regs[1])
		if err := l.pushInt(int32(instruction.Literal)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
	}
	l.code.Emit(opcodes[opIndex])
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitField(instruction *ir.Instruction) error {
	field := l.file.Fields[instruction.Index]
	owner, err := objectInternalName(field.Class)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	kind, err := fieldKind(field.Type)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if err := validateFieldOpcode(instruction.Opcode, field.Type, kind); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	constant, err := l.builder.Pool().Fieldref(owner, field.Name, field.Type)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	switch {
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x58:
		l.load(instruction.Regs[1])
		l.code.CP(classfilebuilder.GETFIELD, constant)
		l.store(instruction.Regs[0])
		if kind == ir.Reference {
			l.registerDescriptor[instruction.Regs[0]] = field.Type
		}
	case instruction.Opcode >= 0x59 && instruction.Opcode <= 0x5f:
		l.load(instruction.Regs[1])
		l.load(instruction.Regs[0])
		l.code.CP(classfilebuilder.PUTFIELD, constant)
	case instruction.Opcode >= 0x60 && instruction.Opcode <= 0x66:
		l.code.CP(classfilebuilder.GETSTATIC, constant)
		l.store(instruction.Regs[0])
		if kind == ir.Reference {
			l.registerDescriptor[instruction.Regs[0]] = field.Type
		}
	case instruction.Opcode >= 0x67 && instruction.Opcode <= 0x6d:
		l.load(instruction.Regs[0])
		l.code.CP(classfilebuilder.PUTSTATIC, constant)
	}
	return nil
}

func validateFieldOpcode(opcode byte, descriptor string, kind ir.CategoryKind) error {
	family := opcode
	if family >= 0x59 && family <= 0x5f {
		family -= 7
	} else if family >= 0x60 && family <= 0x66 {
		family -= 14
	} else if family >= 0x67 && family <= 0x6d {
		family -= 21
	}
	switch family {
	case 0x52:
		// iget/iput and sget/sput share one opcode for int and float values.
		if descriptor != "I" && descriptor != "F" {
			return fmt.Errorf("32-bit field opcode does not match descriptor %q", descriptor)
		}
		if kind != ir.Int && kind != ir.Float {
			return fmt.Errorf("32-bit field opcode does not match register category %s", kind)
		}
	case 0x53:
		if descriptor != "J" && descriptor != "D" {
			return fmt.Errorf("wide field opcode does not match descriptor %q", descriptor)
		}
		if kind != ir.Long && kind != ir.Double {
			return fmt.Errorf("wide field opcode does not match register category %s", kind)
		}
	case 0x54:
		// An allocation is reference-typed even while the verifier still tracks
		// it as uninitialized; allocation validation guarantees the constructor
		// runs before any such use.
		if kind != ir.Reference && kind != ir.Uninitialized {
			return fmt.Errorf("object field opcode does not match descriptor %q", descriptor)
		}
	case 0x55:
		if descriptor != "Z" {
			return fmt.Errorf("boolean field opcode does not match descriptor %q", descriptor)
		}
	case 0x56:
		if descriptor != "B" {
			return fmt.Errorf("byte field opcode does not match descriptor %q", descriptor)
		}
	case 0x57:
		if descriptor != "C" {
			return fmt.Errorf("char field opcode does not match descriptor %q", descriptor)
		}
	case 0x58:
		if descriptor != "S" {
			return fmt.Errorf("short field opcode does not match descriptor %q", descriptor)
		}
	default:
		return fmt.Errorf("unsupported field opcode %#x", opcode)
	}
	return nil
}

func (l *methodLowerer) emitInvoke(index int, instruction *ir.Instruction) error {
	callee := l.file.Methods[instruction.Index]
	proto := l.file.Protos[callee.ProtoIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	owner, err := memberOwnerName(callee.Class)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	position := 0
	if !isStaticInvoke(instruction.Opcode) {
		l.load(instruction.Regs[0])
		position = 1
	}
	if err := l.loadArguments(instruction.Regs, proto, position); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if isInterfaceInvoke(instruction.Opcode) {
		constant, err := l.builder.Pool().InterfaceMethodref(owner, callee.Name, descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.InvokeInterface(constant)
	} else {
		constant, err := l.builder.Pool().Methodref(owner, callee.Name, descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		opcode := classfilebuilder.INVOKEVIRTUAL
		if isStaticInvoke(instruction.Opcode) {
			opcode = classfilebuilder.INVOKESTATIC
		} else if isSpecialInvoke(instruction.Opcode) {
			opcode = classfilebuilder.INVOKESPECIAL
		}
		l.code.CP(opcode, constant)
	}
	result, err := ir.DescriptorCategory(proto.ReturnType)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if result == ir.Unknown {
		return nil
	}
	if index+1 < len(l.method.Instructions) {
		next := l.method.Instructions[index+1]
		if next.Opcode == 0x0a || next.Opcode == 0x0b || next.Opcode == 0x0c {
			// The move-result instruction owns the destination category; the
			// invoke itself defines nothing, so the pre-invoke category of that
			// register is unrelated to the returned value.
			l.storeAt(next.Regs[0], index+1)
			if result == ir.Reference {
				l.registerDescriptor[next.Regs[0]] = proto.ReturnType
			}
			return nil
		}
	}
	if result.Words() == 2 {
		l.code.Emit(classfilebuilder.POP2)
	} else {
		l.code.Emit(classfilebuilder.POP)
	}
	return nil
}

// definitionCategory returns the category an instruction writes into the
// register, falling back to the category the register carried before it.
func (l *methodLowerer) definitionCategory(register uint16) ir.Category {
	if definition, ok := l.method.DefinitionAt(l.index); ok && definition.Register == int32(register) {
		return definition.Category
	}
	return l.method.CategoryAt(l.index, register)
}

func (l *methodLowerer) pushDexInt(register uint16, value int32) error {
	// The constant types the register it defines, so the definition category
	// decides how the raw 32-bit pattern is materialized.
	category := l.definitionCategory(register)
	switch {
	case category.Kind == ir.Float:
		constant, err := l.builder.Pool().Float(uint32(value))
		if err != nil {
			return err
		}
		l.code.LDC(constant)
		return nil
	case isReferenceKind(category.Kind):
		if value != 0 {
			return fmt.Errorf("integer constant %d cannot be stored in %s register home", value, category.Kind)
		}
		l.code.Emit(classfilebuilder.ACONST_NULL)
		return nil
	case isWideKind(category.Kind):
		return fmt.Errorf("32-bit constant cannot be stored in %s register home", category.Kind)
	}
	return l.pushInt(value)
}

// pushDexWide materializes a 64-bit constant. A value whose interpretation was
// never resolved keeps its raw long bits.
func (l *methodLowerer) pushDexWide(register uint16, literal int64) error {
	category := l.definitionCategory(register)
	switch category.Kind {
	case ir.Double:
		constant, err := l.builder.Pool().Double(uint64(literal))
		if err != nil {
			return err
		}
		l.code.LDC2(constant)
		return nil
	case ir.Long, ir.Bits64, ir.Unknown:
		constant, err := l.builder.Pool().Long(literal)
		if err != nil {
			return err
		}
		l.code.LDC2(constant)
		return nil
	default:
		return fmt.Errorf("64-bit constant cannot be stored in %s register home", category.Kind)
	}
}

func (l *methodLowerer) pushInt(value int32) error {
	switch value {
	case -1:
		l.code.Emit(classfilebuilder.ICONST_M1)
	case 0:
		l.code.Emit(classfilebuilder.ICONST_0)
	case 1:
		l.code.Emit(classfilebuilder.ICONST_1)
	case 2:
		l.code.Emit(classfilebuilder.ICONST_2)
	case 3:
		l.code.Emit(classfilebuilder.ICONST_3)
	case 4:
		l.code.Emit(classfilebuilder.ICONST_4)
	case 5:
		l.code.Emit(classfilebuilder.ICONST_5)
	default:
		if value >= math.MinInt8 && value <= math.MaxInt8 {
			l.code.Byte(classfilebuilder.BIPUSH, int(value))
		} else if value >= math.MinInt16 && value <= math.MaxInt16 {
			l.code.Short(classfilebuilder.SIPUSH, int(value))
		} else {
			constant, err := l.builder.Pool().Integer(value)
			if err != nil {
				return err
			}
			l.code.LDC(constant)
		}
	}
	return nil
}

func (l *methodLowerer) home(register uint16) uint16 {
	if int(register) >= len(l.homeSlots) {
		return l.homeBase + register
	}
	return l.homeBase + l.homeSlots[register]
}

func (l *methodLowerer) load(register uint16) *classfilebuilder.Code {
	return l.code.Load(localKind(l.method.CategoryAt(l.index, register).Kind), l.home(register))
}

func (l *methodLowerer) store(register uint16) *classfilebuilder.Code {
	return l.storeAt(register, l.index)
}

// storeAt stores using the category defined by the given instruction, which is
// how an invoke or filled-new-array writes its move-result destination.
func (l *methodLowerer) storeAt(register uint16, index int) *classfilebuilder.Code {
	category := l.method.CategoryAt(index, register)
	if definition, ok := l.method.DefinitionAt(index); ok && definition.Register == int32(register) {
		category = definition.Category
	}
	return l.code.Store(localKind(category.Kind), l.home(register))
}

func localKind(kind ir.CategoryKind) classfilebuilder.LocalKind {
	switch kind {
	case ir.Long:
		return classfilebuilder.LongLocal
	case ir.Double:
		return classfilebuilder.DoubleLocal
	case ir.Float:
		return classfilebuilder.FloatLocal
	}
	if isReferenceKind(kind) {
		return classfilebuilder.RefLocal
	}
	if kind == ir.Bits64 {
		// An unresolved constant keeps its raw 64-bit pattern.
		return classfilebuilder.LongLocal
	}
	return classfilebuilder.IntLocal
}

func isReferenceKind(kind ir.CategoryKind) bool {
	return kind == ir.Reference || kind == ir.Null || kind == ir.Uninitialized
}

// isWideKind reports whether the category occupies two registers.
func isWideKind(kind ir.CategoryKind) bool {
	return kind == ir.Long || kind == ir.Double || kind == ir.Bits64
}

// isIntKind reports whether the category is a 32-bit integer value.
func isIntKind(kind ir.CategoryKind) bool {
	return kind == ir.Int || kind == ir.Bits32
}

func isInvoke(opcode byte) bool {
	return opcode >= 0x6e && opcode <= 0x72 || opcode >= 0x74 && opcode <= 0x78
}

func isStaticInvoke(opcode byte) bool    { return opcode == 0x71 || opcode == 0x77 }
func isDirectInvoke(opcode byte) bool    { return opcode == 0x70 || opcode == 0x76 }
func isInterfaceInvoke(opcode byte) bool { return opcode == 0x72 || opcode == 0x78 }
func isSpecialInvoke(opcode byte) bool {
	return opcode == 0x6f || opcode == 0x70 || opcode == 0x75 || opcode == 0x76
}
