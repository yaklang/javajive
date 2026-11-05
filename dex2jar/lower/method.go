package lower

import (
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
	allocationByNew    map[int]ir.Allocation
	allocationByCtor   map[int]ir.Allocation
	allocationAliasOps map[int]bool
	registerDescriptor []string
}

func (l *methodLowerer) lower() error {
	if l.code == nil {
		return methodDiagnostic(l.method.Context, nil, "method", fmt.Errorf("concrete method has no Code builder"))
	}
	if int(l.homeBase)+int(l.method.Code.RegistersSize) > math.MaxUint16 {
		return methodDiagnostic(l.method.Context, nil, "locals", fmt.Errorf("incoming arguments and DEX register homes require %d locals", int(l.homeBase)+int(l.method.Code.RegistersSize)))
	}
	l.labels = make(map[uint32]*classfilebuilder.Label, len(l.method.Blocks))
	for _, block := range l.method.Blocks {
		l.labels[block.Start] = l.code.NewLabel()
	}
	l.registerDescriptor = make([]string, len(l.method.RegisterTypes))
	for register, category := range l.method.RegisterTypes {
		l.registerDescriptor[register] = category.Descriptor
	}
	l.allocationAliasOps = make(map[int]bool)
	for _, allocation := range l.method.Allocations {
		l.allocationByNew[allocation.NewInstruction] = allocation
		l.allocationByCtor[allocation.Constructor] = allocation
		for index := allocation.NewInstruction + 1; index < allocation.Constructor; index++ {
			l.allocationAliasOps[index] = true
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
	for _, block := range l.method.Blocks {
		l.code.Mark(l.labels[block.Start])
		for _, index := range block.Instructions {
			if err := l.emitInstruction(index); err != nil {
				return err
			}
		}
	}
	l.code.SetMaxLocals(l.homeBase + l.method.Code.RegistersSize)
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
		local := localKind(kind)
		l.code.Load(local, jvmLocal).Store(local, l.home(parameterRegister))
		parameterRegister++
		jvmLocal++
	}
	return nil
}

func (l *methodLowerer) emitInstruction(index int) error {
	instruction := &l.method.Instructions[index]
	if _, ok := l.allocationByNew[index]; ok || l.allocationAliasOps[index] {
		return nil
	}
	if allocation, ok := l.allocationByCtor[index]; ok {
		return l.emitAllocation(instruction, allocation)
	}
	switch {
	case instruction.Opcode == 0x00:
		l.code.Emit(classfilebuilder.NOP)
	case instruction.Opcode >= 0x01 && instruction.Opcode <= 0x03 || instruction.Opcode >= 0x07 && instruction.Opcode <= 0x09:
		return l.emitMove(instruction)
	case instruction.Opcode == 0x0a || instruction.Opcode == 0x0c:
		// The preceding invoke stores directly into the stable register home.
	case instruction.Opcode == 0x0e:
		l.code.Emit(classfilebuilder.RETURN)
	case instruction.Opcode == 0x0f:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.IRETURN)
	case instruction.Opcode == 0x11:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.ARETURN)
	case instruction.Opcode >= 0x12 && instruction.Opcode <= 0x15:
		if err := l.pushDexInt(instruction.Regs[0], int32(instruction.Literal)); err != nil {
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
	case instruction.Opcode == 0x1f:
		return l.emitTypeInstruction(instruction, classfilebuilder.CHECKCAST)
	case instruction.Opcode == 0x20:
		return l.emitTypeInstruction(instruction, classfilebuilder.INSTANCEOF)
	case instruction.Opcode == 0x21:
		l.load(instruction.Regs[1]).Emit(classfilebuilder.ARRAYLENGTH)
		l.store(instruction.Regs[0])
	case instruction.Opcode == 0x23:
		return l.emitNewArray(instruction)
	case instruction.Opcode == 0x27:
		l.load(instruction.Regs[0]).Emit(classfilebuilder.ATHROW)
	case instruction.Opcode >= 0x28 && instruction.Opcode <= 0x2a:
		l.code.Branch(classfilebuilder.GOTO, l.labels[uint32(instruction.Target)])
	case instruction.Opcode >= 0x32 && instruction.Opcode <= 0x3d:
		return l.emitIf(instruction)
	case instruction.Opcode >= 0x44 && instruction.Opcode <= 0x51:
		return l.emitArray(instruction)
	case instruction.Opcode >= 0x52 && instruction.Opcode <= 0x6d:
		return l.emitField(instruction)
	case isInvoke(instruction.Opcode):
		return l.emitInvoke(index, instruction)
	case instruction.Opcode == 0x7b || instruction.Opcode == 0x7c || instruction.Opcode >= 0x8d && instruction.Opcode <= 0x8f:
		return l.emitUnary(instruction)
	case instruction.Opcode >= 0x90 && instruction.Opcode <= 0x9a || instruction.Opcode >= 0xb0 && instruction.Opcode <= 0xba || instruction.Opcode >= 0xd0 && instruction.Opcode <= 0xe2:
		return l.emitArithmetic(instruction)
	default:
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("lowering has no implementation for validated opcode"))
	}
	return nil
}

func (l *methodLowerer) emitMove(instruction *ir.Instruction) error {
	kind := l.method.RegisterTypes[instruction.Regs[0]].Kind
	if kind == ir.Unknown {
		kind = l.method.RegisterTypes[instruction.Regs[1]].Kind
	}
	if kind == ir.Unknown {
		return nil
	}
	if instruction.Opcode <= 0x03 && kind != ir.Int || instruction.Opcode >= 0x07 && kind != ir.Reference && kind != ir.Null && kind != ir.Uninitialized {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("move opcode does not match stable register category %s", kind))
	}
	l.load(instruction.Regs[1])
	l.store(instruction.Regs[0])
	l.registerDescriptor[instruction.Regs[0]] = l.registerDescriptor[instruction.Regs[1]]
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
	if component == "I" {
		l.code.Byte(classfilebuilder.NEWARRAY, 10)
	} else {
		if len(component) == 1 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("only int and reference arrays are supported"))
		}
		name, err := ir.InternalName(component)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("only int and reference arrays are supported: %w", err))
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

func (l *methodLowerer) emitArray(instruction *ir.Instruction) error {
	switch instruction.Opcode {
	case 0x44, 0x46:
		l.load(instruction.Regs[1])
		l.load(instruction.Regs[2])
		if instruction.Opcode == 0x44 {
			l.code.Emit(classfilebuilder.IALOAD)
		} else {
			l.code.Emit(classfilebuilder.AALOAD)
		}
		l.store(instruction.Regs[0])
	case 0x4b, 0x4d:
		l.load(instruction.Regs[1])
		l.load(instruction.Regs[2])
		l.load(instruction.Regs[0])
		if instruction.Opcode == 0x4b {
			l.code.Emit(classfilebuilder.IASTORE)
		} else {
			l.code.Emit(classfilebuilder.AASTORE)
		}
	default:
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("only int and reference array elements are supported"))
	}
	return nil
}

func (l *methodLowerer) emitAllocation(instruction *ir.Instruction, allocation ir.Allocation) error {
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
	for _, register := range instruction.Regs[1:] {
		l.load(register)
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

func (l *methodLowerer) emitIf(instruction *ir.Instruction) error {
	var opcode classfilebuilder.Opcode
	if instruction.Opcode <= 0x37 {
		left := l.method.RegisterTypes[instruction.Regs[0]].Kind
		right := l.method.RegisterTypes[instruction.Regs[1]].Kind
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
			opcode = classfilebuilder.IF_ICMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
		}
	} else {
		kind := l.method.RegisterTypes[instruction.Regs[0]].Kind
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
			opcode = classfilebuilder.IFEQ + classfilebuilder.Opcode(instruction.Opcode-0x38)
		}
	}
	l.code.Branch(opcode, l.labels[uint32(instruction.Target)])
	return nil
}

func (l *methodLowerer) emitUnary(instruction *ir.Instruction) error {
	l.load(instruction.Regs[1])
	opcodes := map[byte]classfilebuilder.Opcode{
		0x7b: classfilebuilder.INEG,
		0x7c: classfilebuilder.ICONST_M1,
		0x8d: classfilebuilder.I2B,
		0x8e: classfilebuilder.I2C,
		0x8f: classfilebuilder.I2S,
	}
	if instruction.Opcode == 0x7c {
		l.code.Emit(classfilebuilder.ICONST_M1).Emit(classfilebuilder.IXOR)
	} else {
		l.code.Emit(opcodes[instruction.Opcode])
	}
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitArithmetic(instruction *ir.Instruction) error {
	base := instruction.Opcode
	if base >= 0xb0 && base <= 0xba {
		base -= 0x20
	}
	if base >= 0xd0 {
		return l.emitLiteralArithmetic(instruction)
	}
	opcodes := []classfilebuilder.Opcode{
		classfilebuilder.IADD, classfilebuilder.ISUB, classfilebuilder.IMUL, classfilebuilder.IDIV, classfilebuilder.IREM,
		classfilebuilder.IAND, classfilebuilder.IOR, classfilebuilder.IXOR, classfilebuilder.ISHL, classfilebuilder.ISHR, classfilebuilder.IUSHR,
	}
	if instruction.Opcode < 0xb0 {
		l.load(instruction.Regs[1])
		l.load(instruction.Regs[2])
	} else {
		l.load(instruction.Regs[0])
		l.load(instruction.Regs[1])
	}
	l.code.Emit(opcodes[base-0x90])
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
	want := map[byte]string{0x52: "I", 0x54: "reference", 0x55: "Z", 0x56: "B", 0x57: "C", 0x58: "S"}[family]
	if want == "reference" {
		if kind != ir.Reference {
			return fmt.Errorf("object field opcode does not match descriptor %q", descriptor)
		}
		return nil
	}
	if want == "" || descriptor != want {
		return fmt.Errorf("field opcode does not match descriptor %q", descriptor)
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
	owner, err := ir.InternalName(callee.Class)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	for _, register := range instruction.Regs {
		l.load(register)
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
		if next.Opcode == 0x0a || next.Opcode == 0x0c {
			l.store(next.Regs[0])
			if result == ir.Reference {
				l.registerDescriptor[next.Regs[0]] = proto.ReturnType
			}
			return nil
		}
	}
	l.code.Emit(classfilebuilder.POP)
	return nil
}

func (l *methodLowerer) pushDexInt(register uint16, value int32) error {
	kind := l.method.RegisterTypes[register].Kind
	if value == 0 && isReferenceKind(kind) {
		l.code.Emit(classfilebuilder.ACONST_NULL)
		return nil
	}
	if kind != ir.Int && kind != ir.Unknown {
		return fmt.Errorf("integer constant cannot be stored in %s register home", kind)
	}
	return l.pushInt(value)
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

func (l *methodLowerer) home(register uint16) uint16 { return l.homeBase + register }

func (l *methodLowerer) load(register uint16) *classfilebuilder.Code {
	return l.code.Load(localKind(l.method.RegisterTypes[register].Kind), l.home(register))
}

func (l *methodLowerer) store(register uint16) *classfilebuilder.Code {
	return l.code.Store(localKind(l.method.RegisterTypes[register].Kind), l.home(register))
}

func localKind(kind ir.CategoryKind) classfilebuilder.LocalKind {
	if isReferenceKind(kind) {
		return classfilebuilder.RefLocal
	}
	return classfilebuilder.IntLocal
}

func isReferenceKind(kind ir.CategoryKind) bool {
	return kind == ir.Reference || kind == ir.Null || kind == ir.Uninitialized
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
