package lower

import (
	"fmt"

	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/model"
)

func (l *methodLowerer) validateInvokes() error {
	isConstructor := l.encoded.Method.Name == "<init>"
	for index := range l.method.Instructions {
		instruction := &l.method.Instructions[index]
		if instruction.Opcode == 0xfa || instruction.Opcode == 0xfb {
			callee := l.file.Methods[instruction.Index]
			if callee.Name == "<clinit>" || callee.Name == "<init>" {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-polymorphic target %q is invalid", callee.Name))
			}
			continue
		}
		if !isInvoke(instruction.Opcode) {
			continue
		}
		callee := l.file.Methods[instruction.Index]
		if callee.Name == "<clinit>" {
			return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("class initializer uses an invalid invoke form"))
		}
		if callee.Name == "<init>" {
			if !isDirectInvoke(instruction.Opcode) {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor uses an invalid invoke form"))
			}
			if allocation, ok := l.allocationByCtor[index]; ok {
				if allocation.Fused {
					if err := l.validateFusedConstructor(instruction, allocation); err != nil {
						return err
					}
				}
				continue
			}
			if !isConstructor {
				// A conditional constructor call may initialize an allocation on
				// a different path; the receiver still aliases that allocation
				// and its static type names the class being initialized.
				if l.isAllocationAlias(instruction.Regs[0], callee.Class) {
					continue
				}
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is not a fused new-instance allocation", instruction.Regs[0]))
			}
			continue
		}
		switch instruction.Opcode {
		case 0x6f, 0x75:
			if _, _, ok := resolveInvokeSuperTarget(l.class, callee.Class, l.classIndex()); !ok {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-super target %q is not a superclass or superinterface of %q", callee.Class, l.class.Descriptor))
			}
		case 0x70, 0x76:
			if callee.Class != l.class.Descriptor {
				return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("invoke-direct non-constructor target must belong to the current class"))
			}
			if err := l.validateDefinedDirectTarget(instruction, callee); err != nil {
				return err
			}
		}
	}
	if !isConstructor {
		return nil
	}
	return l.validateConstructorDelegations()
}

type thisInitState uint8

const (
	thisUninit thisInitState = iota
	thisInit
	thisMixed
)

func joinThisInit(a, b thisInitState) thisInitState {
	if a == b {
		return a
	}
	return thisMixed
}

func cloneRegSet(src map[uint16]bool) map[uint16]bool {
	out := make(map[uint16]bool, len(src))
	for k, v := range src {
		if v {
			out[k] = true
		}
	}
	return out
}

func intersectRegSets(a, b map[uint16]bool) map[uint16]bool {
	out := make(map[uint16]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

func unionRegSets(a, b map[uint16]bool) map[uint16]bool {
	out := cloneRegSet(a)
	for k := range b {
		out[k] = true
	}
	return out
}

func regSetsEqual(a, b map[uint16]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func (l *methodLowerer) isConstructorDelegationInsn(index int) (model.MethodID, bool) {
	instruction := &l.method.Instructions[index]
	if !isInvoke(instruction.Opcode) || !isDirectInvoke(instruction.Opcode) {
		return model.MethodID{}, false
	}
	if _, ok := l.allocationByCtor[index]; ok {
		return model.MethodID{}, false
	}
	callee := l.file.Methods[instruction.Index]
	if callee.Name != "<init>" {
		return model.MethodID{}, false
	}
	return callee, true
}

func (l *methodLowerer) validateConstructorDelegations() error {
	if len(l.method.Blocks) == 0 {
		return methodDiagnostic(l.method.Context, nil, "constructor", fmt.Errorf("constructor does not initialize this or delegate"))
	}
	thisRegister := l.method.Code.RegistersSize - l.method.Code.InsSize
	numBlocks := len(l.method.Blocks)

	blockOf := make([]int, len(l.method.Instructions))
	for i := range blockOf {
		blockOf[i] = -1
	}
	blockByOffset := make(map[uint32]int, numBlocks)
	for b := range l.method.Blocks {
		blockByOffset[l.method.Blocks[b].Start] = b
		for _, idx := range l.method.Blocks[b].Instructions {
			blockOf[idx] = b
		}
	}

	handlersForInsn := make([][]int, len(l.method.Instructions))
	for _, try := range l.method.Code.Tries {
		startAddr := try.StartAddress
		endAddr := try.StartAddress + uint32(try.InstructionCount)
		var hbs []int
		seenHB := make(map[int]bool)
		for _, h := range try.Handler.Handlers {
			if hb, ok := blockByOffset[h.Address]; ok && !seenHB[hb] {
				seenHB[hb] = true
				hbs = append(hbs, hb)
			}
		}
		if try.Handler.CatchAllAddress != nil {
			if hb, ok := blockByOffset[*try.Handler.CatchAllAddress]; ok && !seenHB[hb] {
				seenHB[hb] = true
				hbs = append(hbs, hb)
			}
		}
		if len(hbs) == 0 {
			continue
		}
		for idx := range l.method.Instructions {
			insn := &l.method.Instructions[idx]
			if insn.Offset >= startAddr && insn.Offset < endAddr && ir.MayThrow(insn.Opcode) {
				handlersForInsn[idx] = append(handlersForInsn[idx], hbs...)
			}
		}
	}

	inState := make([]thisInitState, numBlocks)
	inMust := make([]map[uint16]bool, numBlocks)
	inMay := make([]map[uint16]bool, numBlocks)
	visited := make([]bool, numBlocks)

	visited[0] = true
	inState[0] = thisUninit
	inMust[0] = map[uint16]bool{thisRegister: true}
	inMay[0] = map[uint16]bool{thisRegister: true}

	worklist := []int{0}
	inQueue := make([]bool, numBlocks)
	inQueue[0] = true

	propagate := func(succ int, st thisInitState, must, may map[uint16]bool) {
		if !visited[succ] {
			visited[succ] = true
			inState[succ] = st
			inMust[succ] = cloneRegSet(must)
			inMay[succ] = cloneRegSet(may)
			inQueue[succ] = true
			worklist = append(worklist, succ)
			return
		}
		js := joinThisInit(inState[succ], st)
		jMust := intersectRegSets(inMust[succ], must)
		jMay := unionRegSets(inMay[succ], may)
		if js != inState[succ] || !regSetsEqual(jMust, inMust[succ]) || !regSetsEqual(jMay, inMay[succ]) {
			inState[succ] = js
			inMust[succ] = jMust
			inMay[succ] = jMay
			if !inQueue[succ] {
				inQueue[succ] = true
				worklist = append(worklist, succ)
			}
		}
	}

	transferAliases := func(index int, must, may map[uint16]bool) {
		instruction := &l.method.Instructions[index]
		if instruction.Opcode >= 0x01 && instruction.Opcode <= 0x09 && len(instruction.Regs) >= 2 {
			dst, src := instruction.Regs[0], instruction.Regs[1]
			if must[src] {
				must[dst] = true
			} else {
				delete(must, dst)
			}
			if may[src] {
				may[dst] = true
			} else {
				delete(may, dst)
			}
			return
		}
		_, defs := l.method.EffectsAt(index)
		for _, r := range defs {
			delete(must, r)
			delete(may, r)
		}
	}

	maxSteps := numBlocks*64 + 1024
	for steps := 0; len(worklist) > 0 && steps < maxSteps; steps++ {
		b := worklist[0]
		worklist = worklist[1:]
		inQueue[b] = false

		st := inState[b]
		must := cloneRegSet(inMust[b])
		may := cloneRegSet(inMay[b])

		for _, idx := range l.method.Blocks[b].Instructions {
			insn := &l.method.Instructions[idx]
			if _, isCtorCall := l.isConstructorDelegationInsn(idx); isCtorCall && len(insn.Regs) > 0 && must[insn.Regs[0]] {
				if len(handlersForInsn[idx]) > 0 {
					for _, hb := range handlersForInsn[idx] {
						propagate(hb, thisUninit, map[uint16]bool{}, map[uint16]bool{})
					}
				}
				st = thisInit
				continue
			}
			if ir.MayThrow(insn.Opcode) && len(handlersForInsn[idx]) > 0 {
				for _, hb := range handlersForInsn[idx] {
					propagate(hb, st, must, may)
				}
			}
			transferAliases(idx, must, may)
		}

		for _, succ := range l.method.Blocks[b].Successors {
			if l.blockHasNormalEdgeTo(b, succ) {
				propagate(succ, st, must, may)
			}
		}
	}

	findRepeatBranch := func(fromIdx, targetIdx int) (*ir.Instruction, bool) {
		fromBlock := blockOf[fromIdx]
		targetBlock := blockOf[targetIdx]
		if fromBlock < 0 || targetBlock < 0 {
			return nil, false
		}
		past := false
		for _, idx := range l.method.Blocks[fromBlock].Instructions {
			if idx == fromIdx {
				past = true
				continue
			}
			if past && idx == targetIdx {
				return &l.method.Instructions[idx], true
			}
		}
		seen := make([]bool, numBlocks)
		var q []int
		lastInsn := func(b int) *ir.Instruction {
			ins := l.method.Blocks[b].Instructions
			if len(ins) == 0 {
				return nil
			}
			return &l.method.Instructions[ins[len(ins)-1]]
		}
		for _, succ := range l.method.Blocks[fromBlock].Successors {
			if !l.blockHasNormalEdgeTo(fromBlock, succ) {
				otherThrows := false
				if len(handlersForInsn) > 0 {
					pastFrom := false
					for _, idx := range l.method.Blocks[fromBlock].Instructions {
						if idx == fromIdx {
							pastFrom = true
							continue
						}
						if pastFrom {
							for _, hb := range handlersForInsn[idx] {
								if hb == succ {
									otherThrows = true
									break
								}
							}
						}
						if otherThrows {
							break
						}
					}
				}
				if !otherThrows {
					continue
				}
			}
			if succ == targetBlock {
				return lastInsn(fromBlock), true
			}
			if !seen[succ] {
				seen[succ] = true
				q = append(q, succ)
			}
		}
		for len(q) > 0 {
			cur := q[0]
			q = q[1:]
			for _, succ := range l.method.Blocks[cur].Successors {
				if succ == targetBlock {
					return lastInsn(cur), true
				}
				if !seen[succ] {
					seen[succ] = true
					q = append(q, succ)
				}
			}
		}
		return nil, false
	}

	delegationSites := make(map[int]bool)
	deferredSeen := make(map[int]bool)
	l.deferredStores = nil

	for b := range l.method.Blocks {
		if !visited[b] || !l.method.Blocks[b].Reachable {
			continue
		}
		st := inState[b]
		must := cloneRegSet(inMust[b])
		may := cloneRegSet(inMay[b])

		for _, idx := range l.method.Blocks[b].Instructions {
			insn := &l.method.Instructions[idx]
			if callee, isCtorCall := l.isConstructorDelegationInsn(idx); isCtorCall {
				recv := insn.Regs[0]
				if !must[recv] {
					return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("constructor receiver v%d is initialized; delegation requires uninitialized this v%d", recv, thisRegister))
				}
				if callee.Class != l.class.Descriptor && callee.Class != l.class.Superclass {
					return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("uninitialized this may only delegate to %s.<init> or %s.<init>", l.class.Descriptor, l.class.Superclass))
				}
				if st != thisInit {
					for _, arg := range insn.Regs[1:] {
						if may[arg] {
							return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("uninitialized this v%d is also a constructor argument", arg))
						}
					}
				}
				if st == thisInit {
					return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("constructor initializes this more than once"))
				}
				if st == thisMixed {
					if edgeInsn, repeated := findRepeatBranch(idx, idx); repeated {
						if edgeInsn == nil {
							edgeInsn = insn
						}
						return methodDiagnostic(l.method.Context, edgeInsn, "", fmt.Errorf("control flow may repeat constructor delegation"))
					}
					return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("constructor initializes this more than once"))
				}
				delegationSites[idx] = true
				st = thisInit
				continue
			}

			if st != thisInit {
				uses, _ := l.method.EffectsAt(idx)
				aliasMove := insn.Opcode >= 0x01 && insn.Opcode <= 0x09 && len(insn.Regs) >= 2 && may[insn.Regs[1]]
				if isBranchOpcode(insn.Opcode) {
					for _, r := range uses {
						if may[r] {
							return l.uninitializedThisUse(insn, r)
						}
					}
					continue
				}
				usesThis := false
				aliased := uint16(0)
				for _, r := range uses {
					if may[r] {
						usesThis = true
						aliased = r
						break
					}
				}
				if usesThis {
					switch {
					case aliasMove:
					case insn.Opcode >= 0x59 && insn.Opcode <= 0x5f && st == thisUninit && must[insn.Regs[1]] && !may[insn.Regs[0]]:
						if !deferredSeen[idx] {
							deferredSeen[idx] = true
							l.deferredStores = append(l.deferredStores, deferredStore{index: idx, register: insn.Regs[0]})
						}
					default:
						return l.uninitializedThisUse(insn, aliased)
					}
				}
			}

			if insn.Opcode >= 0x0e && insn.Opcode <= 0x11 && st != thisInit {
				return methodDiagnostic(l.method.Context, insn, "", fmt.Errorf("constructor does not initialize this or delegate"))
			}
			transferAliases(idx, must, may)
		}
	}

	l.delegationSites = delegationSites
	l.delegationIndex = -1
	if len(delegationSites) == 1 {
		for idx := range delegationSites {
			l.delegationIndex = idx
		}
	}
	l.deferredByDelegation = make(map[int][]int, len(delegationSites))

	for storePos := range l.deferredStores {
		store := &l.deferredStores[storePos]
		storeBlock := blockOf[store.index]
		if storeBlock < 0 {
			return l.uninitializedThisUse(&l.method.Instructions[store.index], l.method.Instructions[store.index].Regs[1])
		}
		var targets []int
		seenTarget := make(map[int]bool)
		stoppedInStart := false
		pastStore := false
		for _, idx := range l.method.Blocks[storeBlock].Instructions {
			if idx == store.index {
				pastStore = true
				continue
			}
			if pastStore && delegationSites[idx] {
				targets = append(targets, idx)
				seenTarget[idx] = true
				stoppedInStart = true
				break
			}
		}
		if !stoppedInStart {
			seenBlk := map[int]bool{storeBlock: true}
			var q []int
			for _, succ := range l.method.Blocks[storeBlock].Successors {
				if l.method.Blocks[succ].Reachable && !seenBlk[succ] {
					seenBlk[succ] = true
					q = append(q, succ)
				}
			}
			for len(q) > 0 {
				cur := q[0]
				q = q[1:]
				hitDelegation := false
				for _, idx := range l.method.Blocks[cur].Instructions {
					if delegationSites[idx] {
						if !seenTarget[idx] {
							seenTarget[idx] = true
							targets = append(targets, idx)
						}
						hitDelegation = true
						break
					}
				}
				if hitDelegation {
					continue
				}
				for _, succ := range l.method.Blocks[cur].Successors {
					if l.method.Blocks[succ].Reachable && !seenBlk[succ] {
						seenBlk[succ] = true
						q = append(q, succ)
					}
				}
			}
		}
		if len(targets) == 0 {
			return l.uninitializedThisUse(&l.method.Instructions[store.index], l.method.Instructions[store.index].Regs[1])
		}
		for _, d := range targets {
			dBlock := blockOf[d]
			if dBlock < 0 || (storeBlock == dBlock && store.index > d) || (storeBlock != dBlock && !l.dominates(storeBlock, dBlock)) {
				return methodDiagnostic(l.method.Context, &l.method.Instructions[store.index], "", fmt.Errorf("field store before delegation is not on every path to the constructor call"))
			}
			l.deferredByDelegation[d] = append(l.deferredByDelegation[d], storePos)
		}
	}
	return nil
}

func (l *methodLowerer) validateFusedConstructor(instruction *ir.Instruction, allocation ir.Allocation) error {
	if allocation.Descriptor != l.file.Methods[instruction.Index].Class {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("fused allocation type %q does not match constructor owner %q", allocation.Descriptor, l.file.Methods[instruction.Index].Class))
	}
	for _, alias := range allocation.AliasRegisters {
		if instruction.Regs[0] == alias {
			return nil
		}
	}
	return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("constructor receiver v%d is not an alias of its fused new-instance allocation", instruction.Regs[0]))
}

// implementsInterfaceTransitively reports whether startDesc equals targetDesc
// or inherits targetDesc through declared superinterfaces or superclasses in
// index. targetDesc may be an external platform interface absent from index.
func implementsInterfaceTransitively(startDesc, targetDesc string, index map[string]*model.Class) bool {
	if startDesc == "" || targetDesc == "" || targetDesc == "Ljava/lang/Object;" {
		return false
	}
	queue := []string{startDesc}
	visited := map[string]bool{startDesc: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == targetDesc {
			return true
		}
		cls, ok := index[cur]
		if !ok || cls == nil {
			continue
		}
		for _, iface := range cls.Interfaces {
			if iface != "" && !visited[iface] {
				visited[iface] = true
				queue = append(queue, iface)
			}
		}
		if cls.Superclass != "" && cls.Superclass != "Ljava/lang/Object;" && !visited[cls.Superclass] {
			visited[cls.Superclass] = true
			queue = append(queue, cls.Superclass)
		}
	}
	return false
}

// resolveInvokeSuperTarget resolves a DEX invoke-super target descriptor to the
// legal JVM INVOKESPECIAL owner descriptor and constant-pool kind. Under
// JVMS 4.9.2 / 6.5, invoke-super targeting an indirect superinterface must be
// retargeted to either the direct superclass (Classref) or a direct
// superinterface (InterfaceMethodref, Java 8+), while Kotlin -Xjvm-default=all
// static bridges on an interface may target the interface itself.
func resolveInvokeSuperTarget(class *model.Class, calleeClass string, index map[string]*model.Class) (targetDesc string, isInterface bool, ok bool) {
	if class == nil || calleeClass == "" {
		return "", false, false
	}
	// Rule 0: Kotlin -Xjvm-default=all static bridges ($jd) invoke methods on the current interface itself.
	if class.AccessFlags&dexInterface != 0 && calleeClass == class.Descriptor {
		return calleeClass, true, true
	}
	// Rule 1: Direct or transitive superclass chain in index.
	if class.AccessFlags&dexInterface == 0 {
		if calleeClass == class.Superclass {
			return calleeClass, false, true
		}
		visited := map[string]bool{}
		for current := class.Superclass; current != "" && !visited[current]; {
			visited[current] = true
			if current == calleeClass {
				return calleeClass, false, true
			}
			parent, inIndex := index[current]
			if !inIndex || parent == nil {
				break
			}
			current = parent.Superclass
		}
	}
	// Rule 2: Direct superinterface.
	for _, iface := range class.Interfaces {
		if iface == calleeClass {
			return calleeClass, true, true
		}
	}
	// Rule 3: Indirect superinterface via direct superclass or direct superinterface.
	if class.AccessFlags&dexInterface == 0 && class.Superclass != "" && class.Superclass != "Ljava/lang/Object;" {
		if implementsInterfaceTransitively(class.Superclass, calleeClass, index) {
			return class.Superclass, false, true
		}
	}
	for _, iface := range class.Interfaces {
		if implementsInterfaceTransitively(iface, calleeClass, index) {
			return iface, true, true
		}
	}
	// Rule 4: Type declared outside the application hierarchy (platform classpath):
	// its hierarchy is not in the DEX, and verified DEX inputs guarantee it is an
	// ancestor when not already resolved through an application type above.
	if _, inIndex := index[calleeClass]; !inIndex {
		return calleeClass, false, true
	}
	return "", false, false
}

// blockOfInstruction returns the index of the block containing an instruction.
func (l *methodLowerer) blockOfInstruction(instructionIndex int) int {
	for index := range l.method.Blocks {
		for _, candidate := range l.method.Blocks[index].Instructions {
			if candidate == instructionIndex {
				return index
			}
		}
	}
	return -1
}

// dominates reports whether every path from the method entry to afterBlock
// passes through beforeBlock. Instructions in the same block dominate in
// program order, which the caller checks separately.
func (l *methodLowerer) dominates(beforeBlock, afterBlock int) bool {
	if beforeBlock == afterBlock || beforeBlock == 0 {
		return true
	}
	seen := make([]bool, len(l.method.Blocks))
	seen[0] = true
	queue := []int{0}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == afterBlock {
			return false
		}
		for _, successor := range l.method.Blocks[current].Successors {
			if successor == beforeBlock || seen[successor] {
				continue
			}
			seen[successor] = true
			queue = append(queue, successor)
		}
	}
	return true
}

// isAllocationAlias reports whether the register still aliases an allocation
// of the given class. A direct constructor call on such a register is a second
// initialization site on another path.
func (l *methodLowerer) isAllocationAlias(register uint16, descriptor string) bool {
	for _, allocation := range l.method.Allocations {
		if allocation.Descriptor != descriptor {
			continue
		}
		for _, alias := range allocation.AliasRegisters {
			if alias == register {
				return true
			}
		}
	}
	return false
}

func (l *methodLowerer) uninitializedThisUse(instruction *ir.Instruction, register uint16) error {
	return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("uninitialized this v%d is used before constructor delegation", register))
}

func (l *methodLowerer) validateDefinedDirectTarget(instruction *ir.Instruction, callee model.MethodID) error {
	definition, direct, ok := l.definedMethod(callee)
	if !ok {
		return nil
	}
	if !direct || definition.AccessFlags&dexPrivate == 0 || definition.AccessFlags&dexStatic != 0 {
		return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("defined invoke-direct non-constructor target must be a private, non-static direct method"))
	}
	return nil
}

func (l *methodLowerer) definedMethod(callee model.MethodID) (*model.EncodedMethod, bool, bool) {
	for index := range l.class.DirectMethods {
		method := &l.class.DirectMethods[index]
		if sameMethod(method.Method, callee) {
			return method, true, true
		}
	}
	for index := range l.class.VirtualMethods {
		method := &l.class.VirtualMethods[index]
		if sameMethod(method.Method, callee) {
			return method, false, true
		}
	}
	return nil, false, false
}

func sameMethod(left, right model.MethodID) bool {
	return left.Class == right.Class && left.Name == right.Name && left.ProtoIndex == right.ProtoIndex
}

func (l *methodLowerer) validateDefiniteAssignments() error {
	count := int(l.method.Code.RegistersSize)
	entry := make([]bool, count)
	parameterBase := int(l.method.Code.RegistersSize - l.method.Code.InsSize)
	for register := parameterBase; register < count; register++ {
		entry[register] = true
	}
	reachable := make([]bool, len(l.method.Blocks))
	queue := []int{0}
	reachable[0] = true
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		for _, successor := range l.method.Blocks[block].Successors {
			if !reachable[successor] {
				reachable[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	in := make([][]bool, len(l.method.Blocks))
	out := make([][]bool, len(l.method.Blocks))
	for index := range l.method.Blocks {
		in[index] = make([]bool, count)
		out[index] = make([]bool, count)
		if index == 0 {
			copy(in[index], entry)
		} else {
			for register := range in[index] {
				in[index][register] = true
			}
		}
		for register := range out[index] {
			out[index][register] = true
		}
	}
	for pass := 0; pass <= len(l.method.Blocks); pass++ {
		changed := false
		for blockIndex, block := range l.method.Blocks {
			if blockIndex != 0 {
				merged := make([]bool, count)
				for register := range merged {
					merged[register] = true
				}
				for _, predecessor := range block.Predecessors {
					for register := range merged {
						merged[register] = merged[register] && out[predecessor][register]
					}
				}
				if !boolsEqual(in[blockIndex], merged) {
					in[blockIndex] = merged
					changed = true
				}
			}
			state := append([]bool(nil), in[blockIndex]...)
			for _, instructionIndex := range block.Instructions {
				_, definitions := l.registerEffects(instructionIndex)
				for _, register := range definitions {
					state[register] = true
				}
			}
			if !boolsEqual(out[blockIndex], state) {
				out[blockIndex] = state
				changed = true
			}
		}
		if !changed {
			break
		}
		if pass == len(l.method.Blocks) {
			return methodDiagnostic(l.method.Context, nil, "cfg", fmt.Errorf("definite-assignment analysis did not converge"))
		}
	}
	for blockIndex, block := range l.method.Blocks {
		state := append([]bool(nil), in[blockIndex]...)
		for _, instructionIndex := range block.Instructions {
			instruction := &l.method.Instructions[instructionIndex]
			uses, definitions := l.registerEffects(instructionIndex)
			for _, register := range uses {
				if !state[register] {
					return methodDiagnostic(l.method.Context, instruction, "", fmt.Errorf("v%d may be read before it is assigned", register))
				}
			}
			for _, register := range definitions {
				state[register] = true
			}
		}
	}
	return nil
}

// registerEffects comes from the IR's single use/define table so lowering,
// liveness and category analysis cannot drift apart.
func (l *methodLowerer) registerEffects(index int) (uses, definitions []uint16) {
	return l.method.EffectsAt(index)
}
func boolsEqual(left, right []bool) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
