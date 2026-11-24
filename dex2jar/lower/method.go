package lower

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/internal/jdecenv"
)

type polySlotKey struct {
	reg  uint16
	kind classfilebuilder.LocalKind
}

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
	// directParamSlots maps a read-only incoming DEX parameter register directly
	// to its incoming JVM argument local (< homeBase), avoiding prologue copies.
	// Entries >= 0 bypass homeSlots and emitPrologue stores.
	directParamSlots []int
	// paramPrologueSlot maps a non-direct web-split incoming parameter register to
	// its allocated entry-web JVM local slot.
	paramPrologueSlot []int
	webSplitting      bool
	webPinned         []bool
	hasLegacyHome     []bool
	webDefs           []defKindInfo
	ufParent          []int
	webSlots          []uint16
	webAssigned       []bool
	instDefID         map[regUseKey]int
	useWebDef         map[regUseKey]int
	entryDefByReg     []int
	webReachIn        []defState
	webPolySlots      map[webPolyKey]uint16
	webPolyMask       map[int]uint8
	webZeroKind       map[int]ir.CategoryKind
	webSingleZeroKind map[int]ir.CategoryKind
	pinnedZeroKind    map[uint16]ir.CategoryKind
	refIn             []referenceState
	// allocationNewLabels maps an Allocation.ID to the JVM bytecode label right
	// before NEW so StackMapTable frames can emit ITEM_Uninitialized offsets.
	allocationNewLabels map[uint32]*classfilebuilder.Label
	// polySlots maps (register, LocalKind) to companion JVM locals when a
	// polymorphic zero constant (const 0) is used across multiple categories.
	polySlots      map[polySlotKey]uint16
	registerWidths []int
	// homeLimit is the number of local slots the register homes occupy.
	homeLimit uint16
	// tempLimit is the number of extra local slots used to capture values that
	// are stored to fields after constructor delegation.
	tempLimit uint16
	// hierarchy is the caller-provided application class index, if any.
	hierarchy map[string]*model.Class
	// handlerTrampolines maps a handler address whose block never binds the
	// caught exception to the stub that discards it.
	handlerTrampolines map[uint32]*classfilebuilder.Label

	// delegationIndex marks a single-site this-constructor delegation; field
	// stores that DEX permits before delegation are tracked per delegation site.
	delegationIndex          int
	delegationSites          map[int]bool
	deferredByDelegation     map[int][]int
	deferredStores           []deferredStore
	deferredInsn             map[int]bool
	labelOp                  map[*classfilebuilder.Label]int
	insnStartLabel           []*classfilebuilder.Label
	insnThrowEndLabel        []*classfilebuilder.Label
	insnEndLabel             []*classfilebuilder.Label
	canonicalClinit          *canonicalClinitPlan
	canonicalClinitConsts    map[int]*canonicalEnumConstSpan
	canonicalClinitConstSkip map[int]bool
	fusedNoLocals            map[int]bool
}

func (l *methodLowerer) lower() error {
	if l.code == nil {
		return methodDiagnostic(l.method.Context, nil, "method", fmt.Errorf("concrete method has no builder"))
	}
	if err := l.assignHomes(); err != nil {
		return methodDiagnostic(l.method.Context, nil, "locals", err)
	}
	l.index = -1
	l.labelOp = make(map[*classfilebuilder.Label]int, len(l.method.Instructions)*3+len(l.method.Blocks)+16)
	l.insnStartLabel = make([]*classfilebuilder.Label, len(l.method.Instructions))
	l.insnThrowEndLabel = make([]*classfilebuilder.Label, len(l.method.Instructions))
	l.insnEndLabel = make([]*classfilebuilder.Label, len(l.method.Instructions))
	l.labels = make(map[uint32]*classfilebuilder.Label, len(l.method.Blocks))
	for _, block := range l.method.Blocks {
		l.labels[block.Start] = l.code.NewLabel()
	}
	l.allocationNewLabels = make(map[uint32]*classfilebuilder.Label)
	l.registerDescriptor = make([]string, len(l.method.RegisterTypes))
	for register, category := range l.method.RegisterTypes {
		l.registerDescriptor[register] = category.Descriptor
	}
	l.delegationIndex = -1
	l.delegationSites = nil
	l.deferredByDelegation = nil
	l.allocationAliasOps = make(map[int]bool)
	for _, allocation := range l.method.Allocations {
		l.allocationByNew[allocation.NewInstruction] = allocation
		for _, site := range allocation.Constructors {
			l.allocationByCtor[site] = allocation
		}
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
	if handled, err := l.tryEmitCanonicalEnumMethod(); err != nil {
		return err
	} else if handled {
		return nil
	}
	if err := l.emitPrologue(); err != nil {
		return err
	}
	l.prepareAddressLabels()
	if err := l.assignDeferredTemps(); err != nil {
		return err
	}
	deferred := make(map[int]*deferredStore, len(l.deferredStores))
	l.deferredInsn = make(map[int]bool, len(l.deferredStores))
	for index := range l.deferredStores {
		deferred[l.deferredStores[index].index] = &l.deferredStores[index]
		l.deferredInsn[l.deferredStores[index].index] = true
	}
	l.canonicalClinit = l.detectCanonicalEnumClinitBacking()
	l.detectCanonicalEnumClinitConstSpans()
	for _, block := range l.method.Blocks {
		l.markLabel(l.labels[block.Start])
		if !block.Reachable {
			for _, index := range block.Instructions {
				instruction := &l.method.Instructions[index]
				startLabel := l.addressLabels[instruction.Offset]
				if startLabel != nil && startLabel != l.labels[block.Start] {
					l.markLabel(startLabel)
				} else if startLabel == nil {
					startLabel = l.code.NewLabel()
					l.markLabel(startLabel)
				}
				l.insnStartLabel[index] = startLabel
				l.insnEndLabel[index] = startLabel
			}
			continue
		}
		for _, index := range block.Instructions {
			instruction := &l.method.Instructions[index]
			// A deferred store still owns its address: debug data may reference
			// it, and an unmarked label would fail class assembly.
			startLabel := l.addressLabels[instruction.Offset]
			if startLabel != nil && startLabel != l.labels[block.Start] {
				l.markLabel(startLabel)
			} else if startLabel == nil {
				startLabel = l.code.NewLabel()
				l.markLabel(startLabel)
			}
			l.insnStartLabel[index] = startLabel
			if span := l.canonicalClinitConsts[index]; span != nil {
				if err := l.emitCanonicalEnumConstSpan(span); err != nil {
					return err
				}
			} else if l.canonicalClinitConstSkip[index] {
				// Fused into canonical enum constant allocation span.
			} else if l.canonicalClinit != nil && index == l.canonicalClinit.startIdx {
				if err := l.emitCanonicalEnumClinitPacket(); err != nil {
					return err
				}
			} else if l.canonicalClinit != nil && l.canonicalClinit.skipSet[index] {
				// Fused into canonical enum backing packet at startIdx.
			} else if store, ok := deferred[index]; ok {
				l.captureDeferredValue(store)
			} else {
				if err := l.emitInstruction(index); err != nil {
					return err
				}
				if l.isDelegationSite(index) {
					positions := l.deferredByDelegation[index]
					if len(positions) == 0 && index == l.delegationIndex && len(l.delegationSites) <= 1 {
						for position := range l.deferredStores {
							if err := l.emitDeferredStore(&l.deferredStores[position]); err != nil {
								return err
							}
						}
					} else {
						for _, position := range positions {
							if err := l.emitDeferredStore(&l.deferredStores[position]); err != nil {
								return err
							}
						}
					}
				}
			}
			endLabel := l.code.NewLabel()
			l.markLabel(endLabel)
			l.insnEndLabel[index] = endLabel
		}
	}
	l.index = -1
	if l.endLabel != nil {
		l.markLabel(l.endLabel)
	}
	l.emitHandlerTrampolines()
	var smState *stackMapState
	if len(l.method.Code.Tries) > 0 || l.builder.Version() >= classfilebuilder.VersionJava7 {
		state, err := l.computeStackMapFrames()
		if err != nil {
			if l.builder.Version() >= classfilebuilder.VersionJava7 {
				return err
			}
		} else {
			smState = state
		}
	}
	if err := l.emitExceptionTable(smState); err != nil {
		return err
	}
	if l.builder.Version() >= classfilebuilder.VersionJava7 {
		if err := l.emitStackMapTable(smState); err != nil {
			return err
		}
	}
	if err := l.emitDebug(); err != nil {
		return err
	}
	l.code.SetMaxLocals(l.homeBase + l.homeLimit + l.tempLimit)
	return nil
}

func (l *methodLowerer) markLabel(label *classfilebuilder.Label) *classfilebuilder.Code {
	if l.labelOp != nil && label != nil {
		l.labelOp[label] = l.code.InstructionCount()
	}
	return l.code.Mark(label)
}

func (l *methodLowerer) markThrowEnd() {
	if l.index < 0 || l.index >= len(l.insnThrowEndLabel) {
		return
	}
	if l.insnThrowEndLabel[l.index] != nil || !ir.MayThrow(l.method.Instructions[l.index].Opcode) {
		return
	}
	startLabel := l.insnStartLabel[l.index]
	if startLabel == nil {
		return
	}
	startOp, ok := l.labelOp[startLabel]
	if !ok || l.code.InstructionCount() <= startOp {
		return
	}
	end := l.code.NewLabel()
	l.markLabel(end)
	l.insnThrowEndLabel[l.index] = end
}

// prepareAddressLabels binds a JVM label to every DEX address that debug data
// refers to, so line numbers and local ranges survive branch widening.
// assignHomesLegacy gives every DEX register a JVM local. A register that ever holds
// a category-2 value reserves two consecutive slots so no other register can
// occupy the second half of a long or double.
func (l *methodLowerer) assignHomesLegacy() error {
	l.webSplitting = false
	l.polySlots = nil
	l.webPolySlots = nil
	l.webPolyMask = nil
	l.webZeroKind = nil
	l.webSingleZeroKind = nil
	l.pinnedZeroKind = nil
	l.paramPrologueSlot = nil
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

	written := make([]bool, registerCount)
	for idx := range l.method.Instructions {
		_, defs := l.method.EffectsAt(idx)
		for _, reg := range defs {
			if int(reg) < registerCount {
				written[reg] = true
			}
		}
		if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 && int(def.Register) < registerCount {
			written[def.Register] = true
		}
	}
	for _, allocation := range l.method.Allocations {
		if allocation.NewInstruction >= 0 && allocation.NewInstruction < len(l.method.Instructions) {
			regs := l.method.Instructions[allocation.NewInstruction].Regs
			if len(regs) > 0 && int(regs[0]) < registerCount {
				written[regs[0]] = true
			}
		}
		for _, alias := range allocation.AliasRegisters {
			if int(alias) < registerCount {
				written[alias] = true
			}
		}
	}

	l.directParamSlots = make([]int, registerCount)
	for index := range l.directParamSlots {
		l.directParamSlots[index] = -1
	}
	skipHome := make([]bool, registerCount)
	parameterRegister := int(l.method.Code.RegistersSize) - int(l.method.Code.InsSize)
	jvmLocal := 0
	if l.encoded.AccessFlags&dexStatic == 0 {
		if parameterRegister >= 0 && parameterRegister < registerCount && !written[parameterRegister] && widths[parameterRegister] == 1 {
			l.directParamSlots[parameterRegister] = jvmLocal
			skipHome[parameterRegister] = true
		}
		parameterRegister++
		jvmLocal++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		words := descriptorWords(descriptor)
		if parameterRegister >= 0 && parameterRegister+words <= registerCount && widths[parameterRegister] == words {
			dirty := written[parameterRegister]
			if words == 2 && (written[parameterRegister+1] || widths[parameterRegister+1] != 1) {
				dirty = true
			}
			if !dirty {
				l.directParamSlots[parameterRegister] = jvmLocal
				skipHome[parameterRegister] = true
				if words == 2 {
					skipHome[parameterRegister+1] = true
				}
			}
		}
		parameterRegister += words
		jvmLocal += words
	}

	l.registerWidths = widths
	l.homeSlots = make([]uint16, registerCount)
	next := 0
	for register, width := range widths {
		if skipHome[register] {
			continue
		}
		l.homeSlots[register] = uint16(next)
		next += width
	}
	regNarrowMask := make([]uint8, registerCount)
	regHasWide := make([]bool, registerCount)
	incomingReg := registerCount - int(l.method.Code.InsSize)
	if l.encoded.AccessFlags&dexStatic == 0 {
		if incomingReg >= 0 && incomingReg < registerCount {
			regNarrowMask[incomingReg] |= ir.PolyZeroRef
		}
		incomingReg++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		if incomingReg >= 0 && incomingReg < registerCount {
			if fk, fkErr := fieldKind(descriptor); fkErr == nil {
				if bit, wide := categoryMaskBit(fk, 0, descriptor); bit != 0 || wide {
					regNarrowMask[incomingReg] |= bit
					if wide {
						regHasWide[incomingReg] = true
					}
				}
			}
		}
		incomingReg += descriptorWords(descriptor)
	}
	for _, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		for _, idx := range block.Instructions {
			_, effDefs := l.method.EffectsAt(idx)
			var defReg int = -1
			if len(effDefs) > 0 {
				defReg = int(effDefs[0])
			} else if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 {
				defReg = int(def.Register)
			}
			if defReg >= 0 && defReg < registerCount {
				cat := l.definitionCategoryAt(idx, uint16(defReg))
				if bit, wide := categoryMaskBit(cat.Kind, cat.Allocation, cat.Descriptor); bit != 0 || wide {
					regNarrowMask[defReg] |= bit
					if wide {
						regHasWide[defReg] = true
					}
				}
			}
			uses, _ := l.method.EffectsAt(idx)
			for _, u := range uses {
				if int(u) >= registerCount {
					continue
				}
				cat := l.method.CategoryAt(idx, u)
				if cat.Kind == ir.WideUpper {
					continue
				}
				if bit, wide := categoryMaskBit(cat.Kind, cat.Allocation, cat.Descriptor); bit != 0 || wide {
					regNarrowMask[u] |= bit
					if wide {
						regHasWide[u] = true
					}
				}
			}
		}
	}
	for register := 0; register < registerCount; register++ {
		if skipHome[register] {
			continue
		}
		var mask uint8
		if register < len(l.method.PolyZeroRegisters) {
			mask = l.method.PolyZeroRegisters[register]
		}
		if hasMultipleNarrowBits(regNarrowMask[register]) || (regNarrowMask[register] != 0 && regHasWide[register]) {
			mask |= regNarrowMask[register]
		}
		if !hasMultipleNarrowBits(regNarrowMask[register]) && !regHasWide[register] && (register >= len(l.method.PolyZeroRegisters) || l.method.PolyZeroRegisters[register] == 0) {
			if regNarrowMask[register] == ir.PolyZeroInt || regNarrowMask[register] == ir.PolyZeroFloat {
				if l.pinnedZeroKind == nil {
					l.pinnedZeroKind = make(map[uint16]ir.CategoryKind)
				}
				k := ir.Int
				if regNarrowMask[register] == ir.PolyZeroFloat {
					k = ir.Float
				}
				l.pinnedZeroKind[uint16(register)] = k
			}
		}
		if mask == 0 {
			continue
		}
		if l.polySlots == nil {
			l.polySlots = make(map[polySlotKey]uint16)
		}
		first := widths[register] == 1 && !regHasWide[register]
		for _, entry := range []struct {
			bit  uint8
			kind classfilebuilder.LocalKind
		}{
			{ir.PolyZeroRef, classfilebuilder.RefLocal},
			{ir.PolyZeroInt, classfilebuilder.IntLocal},
			{ir.PolyZeroFloat, classfilebuilder.FloatLocal},
		} {
			if mask&entry.bit == 0 {
				continue
			}
			var slot uint16
			if first {
				slot = l.home(uint16(register))
				first = false
			} else {
				if int(l.homeBase)+next > math.MaxUint16 {
					return fmt.Errorf("incoming arguments and DEX register homes require %d locals", int(l.homeBase)+next)
				}
				slot = uint16(int(l.homeBase) + next)
				next++
			}
			l.polySlots[polySlotKey{reg: uint16(register), kind: entry.kind}] = slot
		}
	}
	for _, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		for _, idx := range block.Instructions {
			insn := &l.method.Instructions[idx]
			if insn.Opcode < 0x12 || insn.Opcode > 0x15 || insn.Literal != 0 || len(insn.Regs) == 0 {
				continue
			}
			r := insn.Regs[0]
			if k, ok := l.pinnedZeroKind[r]; ok {
				defCat := l.definitionCategoryAt(idx, r)
				if defCat.Kind == ir.Null && defCat.Allocation == 0 {
					if l.webZeroKind == nil {
						l.webZeroKind = make(map[int]ir.CategoryKind)
					}
					l.webZeroKind[idx] = k
				}
			}
		}
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

// emitExceptionTable records narrowed JVM exception-table entries covering only
// throwing instructions while coalescing safe intermediate non-throwing ranges
// whose stackmap frames stay compatible with every active catch handler.
func (l *methodLowerer) emitExceptionTable(state *stackMapState) error {
	for _, try := range l.method.Code.Tries {
		tryStart := try.StartAddress
		tryEnd := try.StartAddress + uint32(try.InstructionCount)
		if l.addressLabels[tryStart] == nil || l.addressLabels[tryEnd] == nil {
			return methodDiagnostic(l.method.Context, nil, "try", fmt.Errorf("try item [%d,%d) has no bound labels", tryStart, tryEnd))
		}
		type resolvedHandler struct {
			handler *classfilebuilder.Label
			class   string
		}
		resolved := make([]resolvedHandler, 0, len(try.Handler.Handlers)+1)
		resolve := func(address uint32, catchType string) error {
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
			resolved = append(resolved, resolvedHandler{handler: handler, class: class})
			return nil
		}
		for _, handler := range try.Handler.Handlers {
			if err := resolve(handler.Address, handler.Type); err != nil {
				return err
			}
		}
		if try.Handler.CatchAllAddress != nil {
			if err := resolve(*try.Handler.CatchAllAddress, ""); err != nil {
				return err
			}
		}

		var handlerFrames []slotFrame
		canCoalesce := state != nil
		if canCoalesce {
			collectFrame := func(addr uint32) {
				if hb, ok := state.blockByStart[addr]; ok && state.visited[hb] {
					handlerFrames = append(handlerFrames, state.in[hb])
				} else {
					canCoalesce = false
				}
			}
			for _, handler := range try.Handler.Handlers {
				collectFrame(handler.Address)
			}
			if try.Handler.CatchAllAddress != nil {
				collectFrame(*try.Handler.CatchAllAddress)
			}
			if len(handlerFrames) == 0 {
				canCoalesce = false
			}
		}
		compatibleFrame := func(frame slotFrame) bool {
			if !canCoalesce || frame == nil {
				return false
			}
			for _, hf := range handlerFrames {
				if !l.slotFrameCompatibleWithHandler(frame, hf) {
					return false
				}
			}
			return true
		}

		type labelRun struct {
			start, end *classfilebuilder.Label
		}
		var runs []labelRun
		var curStart, curEnd *classfilebuilder.Label
		hasOpen := false
		flush := func() {
			if hasOpen {
				runs = append(runs, labelRun{start: curStart, end: curEnd})
				hasOpen = false
			}
		}
		for blockIdx, block := range l.method.Blocks {
			var curFrame slotFrame
			blockValid := canCoalesce && state.visited[blockIdx]
			if blockValid {
				curFrame = cloneSlotFrame(state.in[blockIdx])
				if hasOpen && !compatibleFrame(curFrame) {
					flush()
				}
			} else {
				flush()
			}
			for _, index := range block.Instructions {
				insn := &l.method.Instructions[index]
				inTry := insn.Offset >= tryStart && insn.Offset < tryEnd
				if !inTry {
					flush()
					if blockValid {
						l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
					}
					continue
				}
				alloc, isNew := l.allocationByNew[index]
				protected := ir.MayThrow(insn.Opcode) &&
					!l.allocationAliasOps[index] &&
					!(isNew && alloc.Fused) &&
					!l.deferredInsn[index]
				if !protected {
					if hasOpen {
						if !blockValid || !compatibleFrame(curFrame) {
							flush()
						} else {
							l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
							if !compatibleFrame(curFrame) {
								flush()
							}
							continue
						}
					}
					if blockValid {
						l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
					}
					continue
				}
				s := l.insnStartLabel[index]
				e := l.insnThrowEndLabel[index]
				if e == nil {
					e = l.insnEndLabel[index]
				}
				if s == nil || e == nil {
					flush()
					if blockValid {
						l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
					}
					continue
				}
				sOp, okS := l.labelOp[s]
				eOp, okE := l.labelOp[e]
				if !okS || !okE || sOp >= eOp {
					flush()
					if blockValid {
						l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
					}
					continue
				}
				if hasOpen && (l.labelOp[curEnd] == sOp || compatibleFrame(curFrame)) {
					curEnd = e
				} else {
					flush()
					curStart, curEnd = s, e
					hasOpen = true
				}
				if blockValid {
					l.transferSlotFrame(blockIdx, index, curFrame, state.deferred, state.catchClassByAddr)
					if hasOpen && !compatibleFrame(curFrame) {
						flush()
					}
				} else {
					flush()
				}
			}
		}
		flush()

		for _, r := range runs {
			for _, rh := range resolved {
				l.code.Catch(r.start, r.end, rh.handler, rh.class)
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
		if !block.Reachable || len(block.Instructions) == 0 {
			continue
		}
		first[block.Start] = l.method.Instructions[block.Instructions[0]].Opcode
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
			l.markLabel(label).Emit(classfilebuilder.POP)
			l.code.Branch(classfilebuilder.GOTO, l.labels[address])
			l.handlerTrampolines[address] = label
		}
	}
}

// emitDebug records line numbers, local variable names and parameter names from
// the original DEX debug_info. Positions that cannot be anchored strictly inside
// emitted JVM code are dropped rather than misattributed or failing assembly.
func (l *methodLowerer) emitDebug() error {
	debug := l.method.Code.Debug
	if debug == nil {
		return nil
	}
	codeUnits := uint32(len(l.method.Code.Instructions))
	totalOps := l.code.InstructionCount()
	for _, position := range debug.Positions {
		if position.Line == 0 || position.Address >= codeUnits {
			continue
		}
		label := l.addressLabels[position.Address]
		if label == nil {
			// R8 emits line 0 or out-of-range positions for synthetic code; a
			// JVM LineNumberTable requires start_pc < code_length.
			continue
		}
		if opIdx, ok := l.labelOp[label]; !ok || opIdx >= totalOps {
			continue
		}
		l.code.Line(label, uint16(position.Line))
	}
	startLabel := l.addressLabels[0]
	// The JVM rejects duplicate LocalVariableTable entries for the same slot and
	// range, so an explicitly described range keeps only that entry.
	type localRange struct {
		slot       uint16
		start, end *classfilebuilder.Label
	}
	described := make(map[localRange]bool)
	for _, local := range debug.Locals {
		if local.Register >= l.method.Code.RegistersSize || local.Name == "" || local.Type == "" || local.Start >= local.End || local.End > codeUnits {
			continue
		}
		kind, err := ir.DescriptorCategory(local.Type)
		if err != nil {
			continue
		}
		if kind.Words() == 2 && (int(local.Register) >= len(l.registerWidths) || l.registerWidths[local.Register] < 2) {
			continue
		}
		start, end := l.addressLabels[local.Start], l.addressLabels[local.End]
		if start == nil || end == nil {
			continue
		}
		startOp, okS := l.labelOp[start]
		endOp, okE := l.labelOp[end]
		if !okS || !okE || startOp >= endOp {
			continue
		}
		slot, ok := l.debugWebSlot(local.Register, local.Start, local.End, kind)
		if !ok {
			continue
		}
		key := localRange{slot: slot, start: start, end: end}
		if described[key] {
			continue
		}
		l.code.Local(start, end, local.Name, local.Type, slot)
		described[key] = true
	}
	parameterRegisters := debugParameterRegisters(l.method.Code, l.method.Proto, l.encoded.AccessFlags)
	if l.endLabel == nil || startLabel == nil {
		return nil
	}
	startOp, okS := l.labelOp[startLabel]
	endOp, okE := l.labelOp[l.endLabel]
	if !okS || !okE || startOp >= endOp {
		return nil
	}
	for index, name := range debug.ParameterNames {
		if index >= len(parameterRegisters) {
			break
		}
		descriptor := l.method.Proto.Parameters[index]
		register := parameterRegisters[index]
		if name == "" || register >= l.method.Code.RegistersSize {
			continue
		}
		pk := classfilebuilder.RefLocal
		if k, err := fieldKind(descriptor); err == nil {
			pk = localKind(k)
		}
		slot := l.prologueStoreSlot(register, pk)
		if l.directParamSlots[register] >= 0 {
			slot = uint16(l.directParamSlots[register])
		} else if l.webSplitting && !l.webPinned[register] && l.paramPrologueSlot[register] < 0 {
			continue
		}
		key := localRange{slot: slot, start: startLabel, end: l.endLabel}
		if described[key] {
			continue
		}
		described[key] = true
		l.code.Local(startLabel, l.endLabel, name, descriptor, slot)
	}
	return nil
}

func debugParameterRegisters(code *model.Code, proto model.Proto, accessFlags uint32) []uint16 {
	if code == nil {
		return nil
	}
	register := code.RegistersSize - code.InsSize
	if accessFlags&dexStatic == 0 {
		register++
	}
	out := make([]uint16, len(proto.Parameters))
	for index, descriptor := range proto.Parameters {
		out[index] = register
		register += uint16(descriptorWords(descriptor))
	}
	return out
}

func (l *methodLowerer) validateAllocationCFG(allocation ir.Allocation) error {
	for _, site := range allocation.Constructors {
		constructor := &l.method.Instructions[site]
		if !allocation.Fused {
			// The uninitialized object may travel through other blocks before its
			// constructor runs; the receiver must still be exactly this allocation.
			category := l.method.CategoryAt(site, constructor.Regs[0])
			if category.Kind != ir.Uninitialized || category.Allocation != allocation.ID {
				return methodDiagnostic(l.method.Context, constructor, "", fmt.Errorf("constructor receiver v%d does not hold the uninitialized allocation", constructor.Regs[0]))
			}
		}
		for _, argument := range constructor.Regs[1:] {
			for _, alias := range allocation.AliasRegisters {
				if argument == alias {
					return methodDiagnostic(l.method.Context, constructor, "", fmt.Errorf("uninitialized allocation is also a constructor argument"))
				}
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

func (l *methodLowerer) prologueStoreSlot(register uint16, kind classfilebuilder.LocalKind) uint16 {
	if l.webSplitting && int(register) < len(l.paramPrologueSlot) && l.paramPrologueSlot[register] >= 0 {
		if kind != classfilebuilder.LongLocal && kind != classfilebuilder.DoubleLocal &&
			int(register) < len(l.entryDefByReg) && l.entryDefByReg[register] >= 0 {
			if slot, ok := l.webPolySlots[webPolyKey{root: l.ufFindRoot(l.entryDefByReg[register]), kind: kind}]; ok {
				return slot
			}
		}
		return uint16(l.paramPrologueSlot[register])
	}
	return l.slotFor(register, kind)
}

func (l *methodLowerer) emitPrologue() error {
	parameterRegister := l.method.Code.RegistersSize - l.method.Code.InsSize
	jvmLocal := uint16(0)
	if l.encoded.AccessFlags&dexStatic == 0 {
		if int(parameterRegister) >= len(l.directParamSlots) || l.directParamSlots[parameterRegister] < 0 {
			l.code.Load(classfilebuilder.RefLocal, jvmLocal).Store(classfilebuilder.RefLocal, l.prologueStoreSlot(parameterRegister, classfilebuilder.RefLocal))
		}
		parameterRegister++
		jvmLocal++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		kind, err := fieldKind(descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, nil, "prologue", err)
		}
		words := uint16(kind.Words())
		if int(parameterRegister) >= len(l.directParamSlots) || l.directParamSlots[parameterRegister] < 0 {
			local := localKind(kind)
			l.code.Load(local, jvmLocal).Store(local, l.prologueStoreSlot(parameterRegister, local))
		}
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
		l.loadAs(instruction.Regs[0], localKind(kind))
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
		rawDef := l.definitionCategoryAt(l.index, instruction.Regs[0])
		defCat := l.definitionCategory(instruction.Regs[0])
		if instruction.Literal == 0 && (defCat.Kind == ir.Null || rawDef.Kind == ir.Null) {
			stored := false
			for _, entry := range []struct {
				lk classfilebuilder.LocalKind
				op classfilebuilder.Opcode
			}{
				{classfilebuilder.RefLocal, classfilebuilder.ACONST_NULL},
				{classfilebuilder.IntLocal, classfilebuilder.ICONST_0},
				{classfilebuilder.FloatLocal, classfilebuilder.FCONST_0},
			} {
				if slot, ok := l.slotIfCompanion(l.index, instruction.Regs[0], entry.lk, true); ok {
					l.code.Emit(entry.op).Store(entry.lk, slot)
					stored = true
				}
			}
			if stored {
				return nil
			}
		} else if defCat.Kind == ir.Bits32 || rawDef.Kind == ir.Bits32 {
			stored := false
			if slot, ok := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.IntLocal, true); ok {
				if err := l.pushInt(int32(instruction.Literal)); err != nil {
					return methodDiagnostic(l.method.Context, instruction, "", err)
				}
				l.code.Store(classfilebuilder.IntLocal, slot)
				stored = true
			}
			if slot, ok := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.FloatLocal, true); ok {
				constant, err := l.builder.Pool().Float(uint32(int32(instruction.Literal)))
				if err != nil {
					return methodDiagnostic(l.method.Context, instruction, "", err)
				}
				l.code.LDC(constant).Store(classfilebuilder.FloatLocal, slot)
				stored = true
			}
			if stored {
				return nil
			}
		}
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
	case instruction.Opcode == 0xfa || instruction.Opcode == 0xfb:
		return l.emitInvokePolymorphic(index, instruction)
	case instruction.Opcode == 0xfc || instruction.Opcode == 0xfd:
		return l.emitInvokeCustom(index, instruction)
	case instruction.Opcode == 0xfe:
		return l.emitConstMethodHandle(instruction)
	case instruction.Opcode == 0xff:
		return l.emitConstMethodType(instruction)
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
	kind := l.resolvedDefCategoryAt(l.index, instruction.Regs[0]).Kind
	if kind == ir.Unknown {
		kind = l.resolvedUseCategoryAt(l.index, instruction.Regs[1]).Kind
	}
	if kind == ir.Unknown {
		return nil
	}
	switch {
	case instruction.Opcode <= 0x03:
		// A plain move copies one 32-bit register; d8 also uses it for a
		// reference alias, so any single-width category is valid.
		if kind == ir.Long || kind == ir.Double || kind == ir.Bits64 || kind == ir.WideUpper || kind == ir.Unknown {
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
	if instruction.Opcode <= 0x03 {
		if rawSrc := l.method.CategoryAt(l.index, instruction.Regs[1]); rawSrc.Kind == ir.Null {
			dst, src := instruction.Regs[0], instruction.Regs[1]
			hasDstPoly := false
			for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.RefLocal, classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
				if _, ok := l.slotIfCompanion(l.index, dst, lk, true); ok {
					hasDstPoly = true
					break
				}
			}
			srcResolvedLK := localKind(l.resolvedUseCategoryAt(l.index, src).Kind)
			if hasDstPoly {
				for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.RefLocal, classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
					dstSlot, ok := l.slotIfCompanion(l.index, dst, lk, true)
					if !ok {
						continue
					}
					if srcSlot, srcOK := l.slotIfCompanion(l.index, src, lk, false); srcOK {
						l.code.Load(lk, srcSlot).Store(lk, dstSlot)
					} else if srcResolvedLK == lk && rawSrc.Allocation != ir.PolyZeroAllocation {
						l.code.Load(lk, l.loadSlotAt(l.index, src, lk)).Store(lk, dstSlot)
					} else {
						l.code.Emit(zeroOpcodeForLocalKind(lk)).Store(lk, dstSlot)
					}
				}
				l.registerDescriptor[dst] = l.registerDescriptor[src]
				return nil
			}
			dstLK := localKind(kind)
			dstSlot := l.storeSlotAt(l.index, dst, dstLK)
			if srcSlot, srcOK := l.slotIfCompanion(l.index, src, dstLK, false); srcOK {
				l.code.Load(dstLK, srcSlot).Store(dstLK, dstSlot)
			} else if srcResolvedLK == dstLK && rawSrc.Allocation != ir.PolyZeroAllocation {
				l.code.Load(dstLK, l.loadSlotAt(l.index, src, dstLK)).Store(dstLK, dstSlot)
			} else {
				l.code.Emit(zeroOpcodeForLocalKind(dstLK)).Store(dstLK, dstSlot)
			}
			if isReferenceKind(kind) {
				l.registerDescriptor[dst] = l.registerDescriptor[src]
			}
			return nil
		} else if rawSrc.Kind == ir.Bits32 {
			dst, src := instruction.Regs[0], instruction.Regs[1]
			hasDstPoly := false
			for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
				if _, ok := l.slotIfCompanion(l.index, dst, lk, true); ok {
					hasDstPoly = true
					break
				}
			}
			srcResolvedLK := localKind(l.resolvedUseCategoryAt(l.index, src).Kind)
			if hasDstPoly {
				for _, lk := range []classfilebuilder.LocalKind{classfilebuilder.IntLocal, classfilebuilder.FloatLocal} {
					dstSlot, ok := l.slotIfCompanion(l.index, dst, lk, true)
					if !ok {
						continue
					}
					if err := l.emitBits32Copy(instruction, src, dstSlot, lk, srcResolvedLK); err != nil {
						return err
					}
				}
				return nil
			}
			dstLK := localKind(kind)
			dstSlot := l.storeSlotAt(l.index, dst, dstLK)
			return l.emitBits32Copy(instruction, src, dstSlot, dstLK, srcResolvedLK)
		}
	}
	l.load(instruction.Regs[1])
	l.store(instruction.Regs[0])
	if isReferenceKind(kind) {
		l.registerDescriptor[instruction.Regs[0]] = l.registerDescriptor[instruction.Regs[1]]
	}
	return nil
}

func (l *methodLowerer) emitBits32Copy(instruction *ir.Instruction, src, dstSlot uint16, dstLK, srcResolvedLK classfilebuilder.LocalKind) error {
	if srcSlot, srcOK := l.slotIfCompanion(l.index, src, dstLK, false); srcOK {
		l.code.Load(dstLK, srcSlot).Store(dstLK, dstSlot)
		return nil
	}
	if srcResolvedLK == dstLK {
		l.code.Load(dstLK, l.loadSlotAt(l.index, src, dstLK)).Store(dstLK, dstSlot)
		return nil
	}
	if dstLK == classfilebuilder.FloatLocal {
		srcSlot, ok := l.slotIfCompanion(l.index, src, classfilebuilder.IntLocal, false)
		if !ok && srcResolvedLK == classfilebuilder.IntLocal {
			srcSlot = l.loadSlotAt(l.index, src, classfilebuilder.IntLocal)
			ok = true
		}
		if ok {
			conv, err := l.builder.Pool().Methodref("java/lang/Float", "intBitsToFloat", "(I)F")
			if err != nil {
				return methodDiagnostic(l.method.Context, instruction, "", err)
			}
			l.code.Load(classfilebuilder.IntLocal, srcSlot).CP(classfilebuilder.INVOKESTATIC, conv).Store(classfilebuilder.FloatLocal, dstSlot)
			return nil
		}
	} else if dstLK == classfilebuilder.IntLocal {
		srcSlot, ok := l.slotIfCompanion(l.index, src, classfilebuilder.FloatLocal, false)
		if !ok && srcResolvedLK == classfilebuilder.FloatLocal {
			srcSlot = l.loadSlotAt(l.index, src, classfilebuilder.FloatLocal)
			ok = true
		}
		if ok {
			conv, err := l.builder.Pool().Methodref("java/lang/Float", "floatToRawIntBits", "(F)I")
			if err != nil {
				return methodDiagnostic(l.method.Context, instruction, "", err)
			}
			l.code.Load(classfilebuilder.FloatLocal, srcSlot).CP(classfilebuilder.INVOKESTATIC, conv).Store(classfilebuilder.IntLocal, dstSlot)
			return nil
		}
	}
	return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("cannot copy 32-bit value from v%d (%d) to %d", src, srcResolvedLK, dstLK))
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
	l.loadAs(instruction.Regs[1], classfilebuilder.IntLocal)
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

// arrayElementOpcode maps a DEX element opcode to the JVM access. The opcode
// fixes the access width and reference-ness; the tracked component only picks
// int versus float and long versus double.
func arrayElementOpcode(opcode byte, component string) (classfilebuilder.Opcode, error) {
	stores := opcode >= 0x4b
	switch opcode {
	case 0x44, 0x4b:
		if component == "F" {
			if stores {
				return classfilebuilder.FASTORE, nil
			}
			return classfilebuilder.FALOAD, nil
		}
		if stores {
			return classfilebuilder.IASTORE, nil
		}
		return classfilebuilder.IALOAD, nil
	case 0x45, 0x4c:
		if component == "D" {
			if stores {
				return classfilebuilder.DASTORE, nil
			}
			return classfilebuilder.DALOAD, nil
		}
		if stores {
			return classfilebuilder.LASTORE, nil
		}
		return classfilebuilder.LALOAD, nil
	case 0x46, 0x4d:
		if stores {
			return classfilebuilder.AASTORE, nil
		}
		return classfilebuilder.AALOAD, nil
	case 0x47, 0x48, 0x4e, 0x4f:
		if stores {
			return classfilebuilder.BASTORE, nil
		}
		return classfilebuilder.BALOAD, nil
	case 0x49, 0x50:
		if stores {
			return classfilebuilder.CASTORE, nil
		}
		return classfilebuilder.CALOAD, nil
	case 0x4a, 0x51:
		if stores {
			return classfilebuilder.SASTORE, nil
		}
		return classfilebuilder.SALOAD, nil
	default:
		return 0, fmt.Errorf("unsupported array element opcode %#x", opcode)
	}
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
	elemKind := classfilebuilder.IntLocal
	if storeOp == classfilebuilder.FASTORE {
		elemKind = classfilebuilder.FloatLocal
	} else if storeOp == classfilebuilder.AASTORE {
		elemKind = classfilebuilder.RefLocal
	}
	for offset, register := range instruction.Regs {
		l.code.Emit(classfilebuilder.DUP)
		if err := l.pushInt(int32(offset)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.loadAs(register, elemKind)
		l.code.Emit(storeOp)
	}
	next := index + 1
	if next >= len(l.method.Instructions) || l.method.Instructions[next].Opcode != 0x0c {
		l.markThrowEnd()
		l.code.Emit(classfilebuilder.POP)
		return nil
	}
	l.markThrowEnd()
	l.storeAt(l.method.Instructions[next].Regs[0], next)
	return nil
}

// filledArrayShape maps an array component descriptor to its construction and
// element-store form.
func filledArrayShape(component string) (classfilebuilder.Opcode, int, string, error) {
	if component == "J" || component == "D" {
		return 0, 0, "", fmt.Errorf("unsupported filled array component %q", component)
	}
	if atype, ok := primitiveArrayType(component); ok {
		stores := map[string]classfilebuilder.Opcode{
			"I": classfilebuilder.IASTORE, "Z": classfilebuilder.BASTORE, "B": classfilebuilder.BASTORE,
			"C": classfilebuilder.CASTORE, "S": classfilebuilder.SASTORE, "F": classfilebuilder.FASTORE,
		}
		return stores[component], atype, "", nil
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
	component := ""
	if len(array.Descriptor) >= 2 && array.Descriptor[0] == '[' {
		component = array.Descriptor[1:]
	} else if array.Kind != ir.Null && !(array.Kind == ir.Reference && array.Descriptor == "") {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("array register v%d has no array type", instruction.Regs[1]))
	}
	op, err := arrayElementOpcode(instruction.Opcode, component)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.loadAs(instruction.Regs[1], classfilebuilder.RefLocal)
	l.loadAs(instruction.Regs[2], classfilebuilder.IntLocal)
	if instruction.Opcode <= 0x4a {
		l.code.Emit(op)
		l.store(instruction.Regs[0])
		return nil
	}
	valueKind := classfilebuilder.IntLocal
	switch op {
	case classfilebuilder.FASTORE:
		valueKind = classfilebuilder.FloatLocal
	case classfilebuilder.LASTORE:
		valueKind = classfilebuilder.LongLocal
	case classfilebuilder.DASTORE:
		valueKind = classfilebuilder.DoubleLocal
	case classfilebuilder.AASTORE:
		valueKind = classfilebuilder.RefLocal
	}
	l.loadAs(instruction.Regs[0], valueKind)
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
	newLabel := l.code.NewLabel()
	l.markLabel(newLabel)
	if l.allocationNewLabels != nil {
		l.allocationNewLabels[allocation.ID] = newLabel
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
	for i := 0; i < instruction.ExtraNullArgs; i++ {
		l.code.Emit(classfilebuilder.ACONST_NULL)
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
	for i := 0; i < instruction.ExtraNullArgs; i++ {
		l.code.Emit(classfilebuilder.ACONST_NULL)
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
		l.loadAs(registers[position], localKind(kind))
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
	if kind := l.method.CategoryAt(l.index, instruction.Regs[0]).Kind; !isIntKind(kind) && kind != ir.Null && kind != ir.Unknown {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("switch selector has category %s, want int", kind))
	}
	l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
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
	operandKind := classfilebuilder.FloatLocal
	if instruction.Opcode == 0x2f || instruction.Opcode == 0x30 {
		operandKind = classfilebuilder.DoubleLocal
	} else if instruction.Opcode == 0x31 {
		operandKind = classfilebuilder.LongLocal
	}
	l.loadAs(instruction.Regs[1], operandKind)
	l.loadAs(instruction.Regs[2], operandKind)
	l.code.Emit(opcode)
	l.store(instruction.Regs[0])
	return nil
}

func (l *methodLowerer) emitIf(instruction *ir.Instruction) error {
	var opcode classfilebuilder.Opcode
	if instruction.Opcode <= 0x37 {
		left := l.resolvedUseCategoryAt(l.index, instruction.Regs[0]).Kind
		right := l.resolvedUseCategoryAt(l.index, instruction.Regs[1]).Kind
		leftNonNullRef := isReferenceKind(left) && left != ir.Null
		rightNonNullRef := isReferenceKind(right) && right != ir.Null
		if (leftNonNullRef || rightNonNullRef) && instruction.Opcode > 0x33 {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("ordered reference comparison is invalid"))
		}
		if (leftNonNullRef && isIntKind(right)) || (rightNonNullRef && isIntKind(left)) {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("comparison mixes reference and integer register homes"))
		}
		if instruction.Opcode >= 0x34 || isIntKind(left) || isIntKind(right) {
			if (!isIntKind(left) && left != ir.Null) || (!isIntKind(right) && right != ir.Null) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("comparison requires int or reference registers, got %s and %s", left, right))
			}
			l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
			l.loadAs(instruction.Regs[1], classfilebuilder.IntLocal)
			opcode = classfilebuilder.IF_ICMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
		} else {
			if !isReferenceKind(left) || !isReferenceKind(right) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("comparison requires int or reference registers, got %s and %s", left, right))
			}
			_, hasRef0 := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.RefLocal, false)
			_, hasRef1 := l.slotIfCompanion(l.index, instruction.Regs[1], classfilebuilder.RefLocal, false)
			_, hasInt0 := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.IntLocal, false)
			_, hasInt1 := l.slotIfCompanion(l.index, instruction.Regs[1], classfilebuilder.IntLocal, false)
			if left == ir.Null && right == ir.Null && (hasInt0 || hasInt1) && !hasRef0 && !hasRef1 {
				l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
				l.loadAs(instruction.Regs[1], classfilebuilder.IntLocal)
				opcode = classfilebuilder.IF_ICMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
			} else {
				l.loadAs(instruction.Regs[0], classfilebuilder.RefLocal)
				l.loadAs(instruction.Regs[1], classfilebuilder.RefLocal)
				opcode = classfilebuilder.IF_ACMPEQ + classfilebuilder.Opcode(instruction.Opcode-0x32)
			}
		}
	} else {
		kind := l.resolvedUseCategoryAt(l.index, instruction.Regs[0]).Kind
		if instruction.Opcode >= 0x3a {
			if isReferenceKind(kind) && kind != ir.Null {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("ordered reference comparison is invalid"))
			}
			if !isIntKind(kind) && kind != ir.Null && kind != ir.Unknown {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("branch requires an int or reference register, got %s", kind))
			}
			l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
			opcode = classfilebuilder.IFEQ + classfilebuilder.Opcode(instruction.Opcode-0x38)
		} else if kind == ir.Null {
			_, hasRef := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.RefLocal, false)
			_, hasInt := l.slotIfCompanion(l.index, instruction.Regs[0], classfilebuilder.IntLocal, false)
			if hasInt && !hasRef {
				l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
				opcode = classfilebuilder.IFEQ + classfilebuilder.Opcode(instruction.Opcode-0x38)
			} else {
				l.loadAs(instruction.Regs[0], classfilebuilder.RefLocal)
				if instruction.Opcode == 0x38 {
					opcode = classfilebuilder.IFNULL
				} else {
					opcode = classfilebuilder.IFNONNULL
				}
			}
		} else if isReferenceKind(kind) {
			l.loadAs(instruction.Regs[0], classfilebuilder.RefLocal)
			if instruction.Opcode == 0x38 {
				opcode = classfilebuilder.IFNULL
			} else {
				opcode = classfilebuilder.IFNONNULL
			}
		} else {
			if !isIntKind(kind) && kind != ir.Unknown {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("branch requires an int or reference register, got %s", kind))
			}
			l.loadAs(instruction.Regs[0], classfilebuilder.IntLocal)
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
	inputKind := classfilebuilder.IntLocal
	switch instruction.Opcode {
	case 0x7f, 0x87, 0x88, 0x89:
		inputKind = classfilebuilder.FloatLocal
	case 0x7d, 0x7e, 0x84, 0x85, 0x86:
		inputKind = classfilebuilder.LongLocal
	case 0x80, 0x8a, 0x8b, 0x8c:
		inputKind = classfilebuilder.DoubleLocal
	}
	l.loadAs(instruction.Regs[1], inputKind)
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
	leftKind, rightKind := classfilebuilder.IntLocal, classfilebuilder.IntLocal
	switch {
	case base >= 0x9b && base <= 0xa2:
		leftKind, rightKind = classfilebuilder.LongLocal, classfilebuilder.LongLocal
	case base >= 0xa3 && base <= 0xa5:
		leftKind, rightKind = classfilebuilder.LongLocal, classfilebuilder.IntLocal
	case base >= 0xa6 && base <= 0xaa:
		leftKind, rightKind = classfilebuilder.FloatLocal, classfilebuilder.FloatLocal
	case base >= 0xab && base <= 0xaf:
		leftKind, rightKind = classfilebuilder.DoubleLocal, classfilebuilder.DoubleLocal
	}
	if instruction.Opcode < 0xb0 {
		l.loadAs(instruction.Regs[1], leftKind)
		l.loadAs(instruction.Regs[2], rightKind)
	} else {
		l.loadAs(instruction.Regs[0], leftKind)
		l.loadAs(instruction.Regs[1], rightKind)
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
	if opIndex == 0 && instruction.Regs[0] == instruction.Regs[1] &&
		instruction.Literal >= math.MinInt16 && instruction.Literal <= math.MaxInt16 {
		readSlot := l.loadSlotAt(l.index, instruction.Regs[0], classfilebuilder.IntLocal)
		writeSlot := l.storeSlotAt(l.index, instruction.Regs[0], classfilebuilder.IntLocal)
		if readSlot == writeSlot {
			l.code.IInc(writeSlot, int16(instruction.Literal))
			return nil
		}
	}
	opcodes := []classfilebuilder.Opcode{
		classfilebuilder.IADD, classfilebuilder.ISUB, classfilebuilder.IMUL, classfilebuilder.IDIV, classfilebuilder.IREM,
		classfilebuilder.IAND, classfilebuilder.IOR, classfilebuilder.IXOR, classfilebuilder.ISHL, classfilebuilder.ISHR, classfilebuilder.IUSHR,
	}
	if opIndex == 1 {
		if err := l.pushInt(int32(instruction.Literal)); err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.loadAs(instruction.Regs[1], classfilebuilder.IntLocal)
	} else {
		l.loadAs(instruction.Regs[1], classfilebuilder.IntLocal)
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
		l.loadAs(instruction.Regs[0], localKind(kind))
		l.code.CP(classfilebuilder.PUTFIELD, constant)
	case instruction.Opcode >= 0x60 && instruction.Opcode <= 0x66:
		l.code.CP(classfilebuilder.GETSTATIC, constant)
		l.store(instruction.Regs[0])
		if kind == ir.Reference {
			l.registerDescriptor[instruction.Regs[0]] = field.Type
		}
	case instruction.Opcode >= 0x67 && instruction.Opcode <= 0x6d:
		l.loadAs(instruction.Regs[0], localKind(kind))
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
	targetDesc := callee.Class
	useInterfaceRef := false
	if instruction.Opcode == 0x6f || instruction.Opcode == 0x75 {
		if resolved, isIface, ok := resolveInvokeSuperTarget(l.class, callee.Class, l.classIndex()); ok {
			targetDesc = resolved
			useInterfaceRef = isIface
		}
	} else if (instruction.Opcode == 0x70 || instruction.Opcode == 0x76) && callee.Name != "<init>" && l.class.AccessFlags&dexInterface != 0 {
		useInterfaceRef = true
	} else if isStaticInvoke(instruction.Opcode) {
		if callee.Class == l.class.Descriptor {
			useInterfaceRef = l.class.AccessFlags&dexInterface != 0
		} else if target, ok := l.classIndex()[callee.Class]; ok && target.AccessFlags&dexInterface != 0 {
			useInterfaceRef = true
		}
	}
	owner, err := memberOwnerName(targetDesc)
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
	for i := 0; i < instruction.ExtraNullArgs; i++ {
		l.code.Emit(classfilebuilder.ACONST_NULL)
	}
	if isInterfaceInvoke(instruction.Opcode) {
		constant, err := l.builder.Pool().InterfaceMethodref(owner, callee.Name, descriptor)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		l.code.InvokeInterface(constant)
	} else {
		var constant uint16
		if useInterfaceRef {
			constant, err = l.builder.Pool().InterfaceMethodref(owner, callee.Name, descriptor)
		} else {
			constant, err = l.builder.Pool().Methodref(owner, callee.Name, descriptor)
		}
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
	return l.emitInvokeResult(index, instruction, proto.ReturnType)
}

func (l *methodLowerer) emitInvokeResult(index int, instruction *ir.Instruction, returnType string) error {
	result, err := ir.DescriptorCategory(returnType)
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
			l.markThrowEnd()
			l.storeAt(next.Regs[0], index+1)
			if result == ir.Reference {
				l.registerDescriptor[next.Regs[0]] = returnType
			}
			return nil
		}
	}
	l.markThrowEnd()
	if result.Words() == 2 {
		l.code.Emit(classfilebuilder.POP2)
	} else {
		l.code.Emit(classfilebuilder.POP)
	}
	return nil
}

func (l *methodLowerer) emitInvokePolymorphic(index int, instruction *ir.Instruction) error {
	callee := l.file.Methods[instruction.Index]
	proto := l.file.Protos[instruction.ProtoIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	owner, err := memberOwnerName(callee.Class)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.load(instruction.Regs[0])
	if err := l.loadArguments(instruction.Regs, proto, 1); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	constant, err := l.builder.Pool().Methodref(owner, callee.Name, descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.CP(classfilebuilder.INVOKEVIRTUAL, constant)
	return l.emitInvokeResult(index, instruction, proto.ReturnType)
}

func (l *methodLowerer) emitInvokeCustom(index int, instruction *ir.Instruction) error {
	cs := l.file.CallSites[instruction.Index]
	proto := l.file.Protos[cs.MethodTypeIndex]
	descriptor, err := methodDescriptor(proto)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	bsmHandle, err := l.lowerMethodHandle(cs.BootstrapMethodHandle)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	bsmArgs := make([]uint16, 0, len(cs.Arguments))
	for _, arg := range cs.Arguments {
		cpArg, err := l.lowerBootstrapArg(arg)
		if err != nil {
			return methodDiagnostic(l.method.Context, instruction, "", err)
		}
		bsmArgs = append(bsmArgs, cpArg)
	}
	bsmIndex, err := l.builder.AddBootstrapMethod(bsmHandle, bsmArgs...)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	indyCP, err := l.builder.Pool().InvokeDynamic(bsmIndex, cs.MethodName, descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if err := l.loadArguments(instruction.Regs, proto, 0); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.InvokeDynamic(indyCP)
	return l.emitInvokeResult(index, instruction, proto.ReturnType)
}

func (l *methodLowerer) emitConstMethodHandle(instruction *ir.Instruction) error {
	cpIndex, err := l.lowerMethodHandle(instruction.Index)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.LDC(cpIndex)
	l.store(instruction.Regs[0])
	l.registerDescriptor[instruction.Regs[0]] = "Ljava/lang/invoke/MethodHandle;"
	return nil
}

func (l *methodLowerer) emitConstMethodType(instruction *ir.Instruction) error {
	descriptor, err := methodDescriptor(l.file.Protos[instruction.Index])
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	cpIndex, err := l.builder.Pool().MethodType(descriptor)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	l.code.LDC(cpIndex)
	l.store(instruction.Regs[0])
	l.registerDescriptor[instruction.Regs[0]] = "Ljava/lang/invoke/MethodType;"
	return nil
}

func (l *methodLowerer) lowerMethodHandle(mhIndex uint32) (uint16, error) {
	if int(mhIndex) >= len(l.file.MethodHandles) {
		return 0, fmt.Errorf("method handle index %d out of range", mhIndex)
	}
	mh := l.file.MethodHandles[mhIndex]
	pool := l.builder.Pool()
	switch mh.Kind {
	case model.MethodHandleStaticPut, model.MethodHandleStaticGet,
		model.MethodHandleInstancePut, model.MethodHandleInstanceGet:
		if int(mh.TargetIndex) >= len(l.file.Fields) {
			return 0, fmt.Errorf("method handle %d field index %d out of range", mhIndex, mh.TargetIndex)
		}
		field := l.file.Fields[mh.TargetIndex]
		owner, err := objectInternalName(field.Class)
		if err != nil {
			return 0, err
		}
		fieldRef, err := pool.Fieldref(owner, field.Name, field.Type)
		if err != nil {
			return 0, err
		}
		kindMap := [...]byte{
			model.MethodHandleStaticPut:   classfilebuilder.RefPutStatic,
			model.MethodHandleStaticGet:   classfilebuilder.RefGetStatic,
			model.MethodHandleInstancePut: classfilebuilder.RefPutField,
			model.MethodHandleInstanceGet: classfilebuilder.RefGetField,
		}
		return pool.MethodHandle(kindMap[mh.Kind], fieldRef)

	case model.MethodHandleInvokeStatic, model.MethodHandleInvokeInstance,
		model.MethodHandleInvokeConstructor, model.MethodHandleInvokeDirect,
		model.MethodHandleInvokeInterface:
		if int(mh.TargetIndex) >= len(l.file.Methods) {
			return 0, fmt.Errorf("method handle %d method index %d out of range", mhIndex, mh.TargetIndex)
		}
		callee := l.file.Methods[mh.TargetIndex]
		proto := l.file.Protos[callee.ProtoIndex]
		owner, err := memberOwnerName(callee.Class)
		if err != nil {
			return 0, err
		}
		desc, err := methodDescriptor(proto)
		if err != nil {
			return 0, err
		}
		idx := l.classIndex()
		targetClass, ok := idx[callee.Class]
		isIface := mh.Kind == model.MethodHandleInvokeInterface ||
			((mh.Kind == model.MethodHandleInvokeStatic || mh.Kind == model.MethodHandleInvokeDirect) && ok && targetClass.AccessFlags&dexInterface != 0)
		var memberRef uint16
		if isIface {
			memberRef, err = pool.InterfaceMethodref(owner, callee.Name, desc)
		} else {
			memberRef, err = pool.Methodref(owner, callee.Name, desc)
		}
		if err != nil {
			return 0, err
		}
		var refKind byte
		switch mh.Kind {
		case model.MethodHandleInvokeStatic:
			refKind = classfilebuilder.RefInvokeStatic
		case model.MethodHandleInvokeInstance:
			refKind = classfilebuilder.RefInvokeVirtual
		case model.MethodHandleInvokeConstructor:
			refKind = classfilebuilder.RefNewInvokeSpecial
		case model.MethodHandleInvokeDirect:
			refKind = classfilebuilder.RefInvokeSpecial
		case model.MethodHandleInvokeInterface:
			refKind = classfilebuilder.RefInvokeInterface
		}
		return pool.MethodHandle(refKind, memberRef)

	default:
		return 0, fmt.Errorf("unsupported method handle kind %#x", mh.Kind)
	}
}

func (l *methodLowerer) lowerBootstrapArg(value model.EncodedValue) (uint16, error) {
	pool := l.builder.Pool()
	switch value.Type {
	case model.ValueByte, model.ValueShort, model.ValueInt:
		return pool.Integer(int32(value.Signed))
	case model.ValueChar:
		return pool.Integer(int32(value.Unsigned))
	case model.ValueBoolean:
		v := int32(0)
		if value.Boolean {
			v = 1
		}
		return pool.Integer(v)
	case model.ValueLong:
		return pool.Long(value.Signed)
	case model.ValueFloat:
		return pool.Float(uint32(value.Unsigned))
	case model.ValueDouble:
		return pool.Double(value.Unsigned)
	case model.ValueString:
		return pool.String(l.file.Strings[value.Unsigned])
	case model.ValueTypeRef:
		name, err := ir.InternalName(l.file.Types[value.Unsigned].Descriptor)
		if err != nil {
			return 0, err
		}
		return pool.Class(name)
	case model.ValueMethodType:
		desc, err := methodDescriptor(l.file.Protos[value.Unsigned])
		if err != nil {
			return 0, err
		}
		return pool.MethodType(desc)
	case model.ValueMethodHandle:
		return l.lowerMethodHandle(uint32(value.Unsigned))
	default:
		return 0, fmt.Errorf("unsupported bootstrap argument type %#x", value.Type)
	}
}

func (l *methodLowerer) resolvedDefCategoryAt(index int, register uint16) ir.Category {
	category := l.definitionCategoryAt(index, register)
	if (category.Kind == ir.Null && category.Allocation == 0) || category.Kind == ir.Bits32 {
		if k, ok := l.webZeroKind[index]; ok && (category.Kind != ir.Bits32 || k == ir.Int || k == ir.Float) {
			return ir.Category{Kind: k}
		}
		if k, ok := l.resolvedZeroKindForUse(index, register, true); ok && (category.Kind != ir.Bits32 || k == ir.Int || k == ir.Float) {
			return ir.Category{Kind: k}
		}
	}
	return category
}

func (l *methodLowerer) resolvedUseCategoryAt(index int, register uint16) ir.Category {
	category := l.method.CategoryAt(index, register)
	if (category.Kind == ir.Null && category.Allocation == 0) || category.Kind == ir.Bits32 {
		if k, ok := l.resolvedZeroKindForUse(index, register, false); ok && (category.Kind != ir.Bits32 || k == ir.Int || k == ir.Float) {
			return ir.Category{Kind: k}
		}
	}
	return category
}

func (l *methodLowerer) resolvedZeroKindForUse(index int, register uint16, isDef bool) (ir.CategoryKind, bool) {
	if l.webSplitting && int(register) < len(l.webPinned) && !l.webPinned[register] {
		var d int
		var ok bool
		if isDef {
			d, ok = l.instDefID[regUseKey{index: index, reg: register}]
		} else {
			d, ok = l.useWebDef[regUseKey{index: index, reg: register}]
		}
		if ok {
			if k, exists := l.webSingleZeroKind[l.ufFindRoot(d)]; exists {
				return k, true
			}
		}
		return ir.Unknown, false
	}
	if k, exists := l.pinnedZeroKind[register]; exists {
		return k, true
	}
	return ir.Unknown, false
}

func (l *methodLowerer) slotIfCompanion(index int, register uint16, lk classfilebuilder.LocalKind, isDef bool) (uint16, bool) {
	if l.webSplitting && int(register) < len(l.webPinned) && !l.webPinned[register] &&
		lk != classfilebuilder.LongLocal && lk != classfilebuilder.DoubleLocal {
		var d int
		var ok bool
		if isDef {
			d, ok = l.instDefID[regUseKey{index: index, reg: register}]
		} else {
			d, ok = l.useWebDef[regUseKey{index: index, reg: register}]
		}
		if ok {
			slot, exists := l.webPolySlots[webPolyKey{root: l.ufFindRoot(d), kind: lk}]
			return slot, exists
		}
	}
	slot, ok := l.polySlots[polySlotKey{reg: register, kind: lk}]
	return slot, ok
}

// definitionCategory returns the category an instruction writes into the
// register, falling back to the category the register carried before it.
func (l *methodLowerer) definitionCategory(register uint16) ir.Category {
	return l.resolvedDefCategoryAt(l.index, register)
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
	if int(register) < len(l.directParamSlots) && l.directParamSlots[register] >= 0 {
		return uint16(l.directParamSlots[register])
	}
	if int(register) >= len(l.homeSlots) {
		return l.homeBase + register
	}
	return l.homeBase + l.homeSlots[register]
}

func (l *methodLowerer) slotFor(register uint16, kind classfilebuilder.LocalKind) uint16 {
	if slot, ok := l.polySlots[polySlotKey{reg: register, kind: kind}]; ok {
		return slot
	}
	return l.home(register)
}

func (l *methodLowerer) loadAs(register uint16, expected classfilebuilder.LocalKind) *classfilebuilder.Code {
	if rawCat := l.method.CategoryAt(l.index, register); rawCat.Kind == ir.Null {
		if slot, ok := l.slotIfCompanion(l.index, register, expected, false); ok {
			return l.code.Load(expected, slot)
		}
		resolved := l.resolvedUseCategoryAt(l.index, register).Kind
		if localKind(resolved) == expected && rawCat.Allocation != ir.PolyZeroAllocation {
			return l.code.Load(expected, l.loadSlotAt(l.index, register, expected))
		}
		return l.code.Emit(zeroOpcodeForLocalKind(expected))
	} else if rawCat.Kind == ir.Bits32 && (expected == classfilebuilder.IntLocal || expected == classfilebuilder.FloatLocal) {
		if slot, ok := l.slotIfCompanion(l.index, register, expected, false); ok {
			return l.code.Load(expected, slot)
		}
	}
	kind := l.resolvedUseCategoryAt(l.index, register).Kind
	if kind != ir.Null {
		expected = localKind(kind)
	}
	return l.code.Load(expected, l.loadSlotAt(l.index, register, expected))
}

func zeroOpcodeForLocalKind(lk classfilebuilder.LocalKind) classfilebuilder.Opcode {
	switch lk {
	case classfilebuilder.IntLocal:
		return classfilebuilder.ICONST_0
	case classfilebuilder.FloatLocal:
		return classfilebuilder.FCONST_0
	default:
		return classfilebuilder.ACONST_NULL
	}
}

func (l *methodLowerer) load(register uint16) *classfilebuilder.Code {
	return l.loadAs(register, localKind(l.resolvedUseCategoryAt(l.index, register).Kind))
}

func (l *methodLowerer) store(register uint16) *classfilebuilder.Code {
	return l.storeAt(register, l.index)
}

// storeAt stores using the category defined by the given instruction, which is
// how an invoke or filled-new-array writes its move-result destination.
func (l *methodLowerer) storeAt(register uint16, index int) *classfilebuilder.Code {
	l.markThrowEnd()
	category := l.resolvedDefCategoryAt(index, register)
	lk := localKind(category.Kind)
	return l.code.Store(lk, l.storeSlotAt(index, register, lk))
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

// deferredStore is a field store that DEX allows on uninitialized this before
// super(). The JVM requires the store after the constructor call, so the value
// is captured in a fresh local at the original position and stored afterwards.
type deferredStore struct {
	index    int
	register uint16
	temp     uint16
	category ir.Category
	direct   bool
}

func isBranchOpcode(opcode byte) bool {
	return opcode >= 0x28 && opcode <= 0x2a || opcode >= 0x32 && opcode <= 0x3d
}

func (l *methodLowerer) isDelegationSite(index int) bool {
	if len(l.delegationSites) > 0 {
		return l.delegationSites[index]
	}
	return index >= 0 && index == l.delegationIndex
}

// canDirectDeferredStore reports whether the stored value register and its
// receiver remain unmodified between store.index and l.delegationIndex, so the
// pre-delegation temporary copy can be elided safely.
func (l *methodLowerer) canDirectDeferredStore(store *deferredStore) bool {
	if jdecenv.Get("JDEC_DEX_DIRECT_DEFERRED_OFF") == "1" || len(l.delegationSites) > 1 || l.delegationIndex < 0 || store.index < 0 || store.index >= l.delegationIndex {
		return false
	}
	startBlock := l.blockOfInstruction(store.index)
	if startBlock < 0 {
		return false
	}
	thisReg := l.method.Instructions[store.index].Regs[1]
	valWords := store.category.Kind.Words()
	conflicts := func(idx int) bool {
		_, defs := l.method.EffectsAt(idx)
		defWidth := 1
		if len(defs) > 0 {
			defWidth = l.instructionDefWidth(idx, defs[0], defs)
		}
		if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 {
			r := uint16(def.Register)
			if r == thisReg || r == store.register || (valWords == 2 && r == store.register+1) {
				return true
			}
			if def.Category.Kind.Words() == 2 && r+1 == store.register {
				return true
			}
		}
		for _, r := range defs {
			if r == thisReg || r == store.register || (valWords == 2 && r == store.register+1) {
				return true
			}
			if defWidth == 2 && r+1 == store.register {
				return true
			}
		}
		return false
	}

	seen := map[int]bool{}
	var queue []int
	started := false
	for _, idx := range l.method.Blocks[startBlock].Instructions {
		if idx == store.index {
			started = true
			continue
		}
		if !started {
			continue
		}
		if idx == l.delegationIndex {
			return true
		}
		if conflicts(idx) {
			return false
		}
	}
	seen[startBlock] = true
	for _, succ := range l.method.Blocks[startBlock].Successors {
		if l.method.Blocks[succ].Reachable {
			queue = append(queue, succ)
		}
	}
	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		if seen[b] {
			continue
		}
		seen[b] = true
		reachedDelegation := false
		for _, idx := range l.method.Blocks[b].Instructions {
			if idx == l.delegationIndex {
				reachedDelegation = true
				break
			}
			if conflicts(idx) {
				return false
			}
		}
		if reachedDelegation {
			continue
		}
		for _, succ := range l.method.Blocks[b].Successors {
			if l.method.Blocks[succ].Reachable && !seen[succ] {
				queue = append(queue, succ)
			}
		}
	}
	return true
}

// assignDeferredTemps gives every deferred store a fresh local wide enough for
// its value, after the DEX register homes, unless direct loading is safe.
func (l *methodLowerer) assignDeferredTemps() error {
	next := int(l.homeBase) + int(l.homeLimit)
	for index := range l.deferredStores {
		store := &l.deferredStores[index]
		category := l.method.CategoryAt(store.index, store.register)
		if category.Kind == ir.Null {
			field := l.file.Fields[l.method.Instructions[store.index].Index]
			if fk, err := fieldKind(field.Type); err == nil {
				category = ir.Category{Kind: fk, Descriptor: field.Type}
			}
		}
		if category.Kind == ir.Unknown {
			return methodDiagnostic(l.method.Context, &l.method.Instructions[store.index], "", fmt.Errorf("deferred field store value v%d has no category", store.register))
		}
		store.category = category
		if l.canDirectDeferredStore(store) {
			store.direct = true
			continue
		}
		if next > math.MaxUint16 {
			return methodDiagnostic(l.method.Context, &l.method.Instructions[store.index], "", fmt.Errorf("deferred field store values require too many locals"))
		}
		store.temp = uint16(next)
		next += category.Kind.Words()
	}
	l.tempLimit = uint16(next - (int(l.homeBase) + int(l.homeLimit)))
	return nil
}

// captureDeferredValue evaluates the stored value at its original DEX position
// and keeps it in a local that survives until the constructor call.
func (l *methodLowerer) captureDeferredValue(store *deferredStore) {
	if store.direct {
		return
	}
	previous := l.index
	l.index = store.index
	lk := localKind(store.category.Kind)
	l.loadAs(store.register, lk)
	l.code.Store(lk, store.temp)
	l.index = previous
}

// emitDeferredStore performs the field store after this is initialized.
func (l *methodLowerer) emitDeferredStore(store *deferredStore) error {
	instruction := &l.method.Instructions[store.index]
	field := l.file.Fields[instruction.Index]
	owner, err := objectInternalName(field.Class)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	if err := validateFieldOpcode(instruction.Opcode, field.Type, store.category.Kind); err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	constant, err := l.builder.Pool().Fieldref(owner, field.Name, field.Type)
	if err != nil {
		return methodDiagnostic(l.method.Context, instruction, "", err)
	}
	previous := l.index
	l.index = store.index
	l.code.Load(classfilebuilder.RefLocal, 0)
	if store.direct {
		l.loadAs(store.register, localKind(store.category.Kind))
	} else {
		l.code.Load(localKind(store.category.Kind), store.temp)
	}
	l.index = previous
	l.code.CP(classfilebuilder.PUTFIELD, constant)
	return nil
}
