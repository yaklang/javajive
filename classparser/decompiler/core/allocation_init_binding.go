package core

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type constructorAllocationEntry struct {
	rootStore   *OpCode
	ne          *values.NewExpression
	closure     []*OpCode
	closureSet  map[*OpCode]bool
	inits       []*OpCode
	multiInit   bool
	retired     bool
	origAssigns []*statements.AssignStatement
}

type constructorAllocationPlan struct {
	byInit map[*OpCode]*constructorAllocationEntry
}

func (p *constructorAllocationPlan) entryFor(initOp *OpCode) *constructorAllocationEntry {
	if p == nil || initOp == nil {
		return nil
	}
	return p.byInit[initOp]
}

// planConstructorAllocations binds each uninitialized local-slot NEW
// allocation to its constructor call(s) and collects the unavoidable
// pre-constructor alias chain so statement generation can forward the entire
// allocation + alias chain to the <init> site in argument-evaluated order.
// Kill-switch: JDEC_ALLOC_INIT_BIND_OFF=1.
func (d *Decompiler) planConstructorAllocations(protectedStores map[*OpCode]bool) *constructorAllocationPlan {
	if d == nil || d.getenv("JDEC_ALLOC_INIT_BIND_OFF") != "" || len(d.opCodes) > 8192 {
		return nil
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes))) != nil || d.Work.CheckAlloc(int64(len(d.opCodes))*128) != nil) {
		return nil
	}
	decoded := make(map[*OpCode]bool, len(d.opCodes))
	for _, op := range d.opCodes {
		if op != nil {
			decoded[op] = true
		}
	}

	type rawGroup struct {
		root      *OpCode
		ne        *values.NewExpression
		inits     []*OpCode
		recvLoads map[*OpCode]bool
	}
	groups := map[*OpCode]*rawGroup{}
	groupOrder := []*OpCode{}

	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL || len(op.stackProduced) != 0 {
			continue
		}
		targetClass, methodName, paramCount, ok := d.invokespecialConstructorTarget(op)
		if !ok || methodName != "<init>" {
			continue
		}
		var receiver values.JavaValue
		if len(op.stackConsumed) > paramCount {
			receiver = op.stackConsumed[paramCount]
		} else if call := d.invokeFuncCall[op]; call != nil {
			receiver = call.Object
		}
		if receiver == nil {
			continue
		}
		recvLoad := d.allocationReceiverLoad(receiver)
		if recvLoad == nil {
			continue
		}
		rootStore, ne, ok := d.resolveLocalAllocationRoot(recvLoad, targetClass)
		if !ok || rootStore == nil || ne == nil || !decoded[rootStore] || protectedStores[rootStore] {
			continue
		}
		g := groups[rootStore]
		if g == nil {
			g = &rawGroup{
				root:      rootStore,
				ne:        ne,
				recvLoads: map[*OpCode]bool{},
			}
			groups[rootStore] = g
			groupOrder = append(groupOrder, rootStore)
		}
		g.inits = append(g.inits, op)
		g.recvLoads[recvLoad] = true
	}

	if len(groupOrder) == 0 {
		return nil
	}

	plan := &constructorAllocationPlan{byInit: map[*OpCode]*constructorAllocationEntry{}}
	for _, rootStore := range groupOrder {
		g := groups[rootStore]
		if len(g.inits) == 0 {
			continue
		}
		closure, closureSet, ok := d.collectAndVerifyAllocationClosure(g.root, g.inits, g.recvLoads, decoded, protectedStores)
		if !ok || len(closure) == 0 {
			continue
		}
		multiInit := false
		if len(g.inits) >= 2 {
			if !d.initsMutuallyExclusive(g.root, g.inits) {
				continue
			}
			multiInit = true
		}
		entry := &constructorAllocationEntry{
			rootStore:  g.root,
			ne:         g.ne,
			closure:    closure,
			closureSet: closureSet,
			inits:      g.inits,
			multiInit:  multiInit,
		}
		for _, initOp := range g.inits {
			plan.byInit[initOp] = entry
		}
	}
	if len(plan.byInit) == 0 {
		return nil
	}
	return plan
}

func (d *Decompiler) invokespecialConstructorTarget(op *OpCode) (className, methodName string, paramCount int, ok bool) {
	if call := d.invokeFuncCall[op]; call != nil && call.FuncType != nil {
		return call.ClassName, call.FunctionName, len(call.FuncType.ParamTypes), true
	}
	if d.constantPoolGetter == nil || len(op.Data) < 2 {
		return "", "", 0, false
	}
	cm, okCP := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
	if !okCP || cm == nil || cm.JavaType == nil {
		return "", "", 0, false
	}
	ft := cm.JavaType.FunctionType()
	if ft == nil {
		return "", "", 0, false
	}
	return cm.Name, cm.Member, len(ft.ParamTypes), true
}

func (d *Decompiler) allocationReachingStores(load *OpCode, slot int) ([]*OpCode, bool) {
	if d == nil || load == nil || slot < 0 {
		return nil, false
	}
	if d.semanticCFG != nil && d.semanticCFG.Err == nil {
		return d.semanticCFG.ReachingDefinitions(load, slot)
	}
	return reachingStoresOf(load, slot)
}

func (d *Decompiler) allocationReceiverLoad(val values.JavaValue) *OpCode {
	if d == nil || val == nil {
		return nil
	}
	unwrapped := values.OriginalStackLifetimeUse(val)
	if unwrapped != nil {
		if op := d.stackValueProducers[unwrapped]; op != nil && op.Instr != nil && isReferenceLoadOpcode(op.Instr.OpCode) {
			return op
		}
	}
	if op := d.stackValueProducers[val]; op != nil && op.Instr != nil && isReferenceLoadOpcode(op.Instr.OpCode) {
		return op
	}
	if sv, ok := unwrapped.(*values.SlotValue); ok && sv != nil {
		if inner := sv.GetValue(); inner != nil {
			if op := d.stackValueProducers[inner]; op != nil && op.Instr != nil && isReferenceLoadOpcode(op.Instr.OpCode) {
				return op
			}
		}
	}
	return nil
}

func normalizeBinaryClassName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "/", ".")
}

func allocationMatchesClass(candType types.JavaType, targetClass string) bool {
	if candType == nil || targetClass == "" {
		return false
	}
	normTarget := normalizeBinaryClassName(targetClass)
	if cls, ok := candType.RawType().(*types.JavaClass); ok && cls != nil {
		return normalizeBinaryClassName(cls.Name) == normTarget
	}
	if fqn, ok := types.RawClassFQN(candType); ok {
		return normalizeBinaryClassName(fqn) == normTarget
	}
	return false
}

func (d *Decompiler) resolveLocalAllocationRoot(recvLoad *OpCode, targetClass string) (*OpCode, *values.NewExpression, bool) {
	const maxHops = 16
	curLoad := recvLoad
	visited := map[*OpCode]bool{}
	for hop := 0; hop < maxHops && curLoad != nil; hop++ {
		if curLoad.Instr == nil || !isReferenceLoadOpcode(curLoad.Instr.OpCode) {
			return nil, nil, false
		}
		slot := GetRetrieveIdx(curLoad)
		if slot < 0 {
			return nil, nil, false
		}
		defs, entry := d.allocationReachingStores(curLoad, slot)
		if entry || len(defs) != 1 {
			return nil, nil, false
		}
		def := defs[0]
		if def == nil || def.Instr == nil || visited[def] || !isReferenceStoreOpcode(def.Instr.OpCode) || len(def.stackConsumed) != 1 {
			return nil, nil, false
		}
		visited[def] = true
		rhs := def.stackConsumed[0]
		if rhs == nil {
			return nil, nil, false
		}
		if cand, isNew := UnpackSoltValue(rhs).(*values.NewExpression); isNew && cand != nil && !cand.IsArray() {
			if !allocationMatchesClass(cand.Type(), targetClass) {
				return nil, nil, false
			}
			return def, cand, true
		}
		prevLoad := d.allocationReceiverLoad(rhs)
		if prevLoad == nil {
			return nil, nil, false
		}
		curLoad = prevLoad
	}
	return nil, nil, false
}

func (d *Decompiler) opsBetween(rootStore *OpCode, inits []*OpCode) (map[*OpCode]bool, bool) {
	if d == nil || rootStore == nil || len(inits) == 0 {
		return nil, false
	}
	initSet := make(map[*OpCode]bool, len(inits))
	for _, initOp := range inits {
		if initOp == nil {
			return nil, false
		}
		initSet[initOp] = true
	}
	fwd := map[*OpCode]bool{rootStore: true}
	fQueue := []*OpCode{}
	if !initSet[rootStore] {
		fQueue = append(fQueue, rootStore.Target...)
	}
	for len(fQueue) > 0 {
		cur := fQueue[0]
		fQueue = fQueue[1:]
		if cur == nil || fwd[cur] {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return nil, false
		}
		fwd[cur] = true
		if initSet[cur] {
			continue
		}
		fQueue = append(fQueue, cur.Target...)
	}
	for initOp := range initSet {
		if !fwd[initOp] {
			return nil, false
		}
	}

	bwd := make(map[*OpCode]bool, len(inits))
	bQueue := make([]*OpCode, 0, len(inits)*2)
	for initOp := range initSet {
		bwd[initOp] = true
		if initOp != rootStore {
			bQueue = append(bQueue, initOp.Source...)
		}
	}
	for len(bQueue) > 0 {
		cur := bQueue[0]
		bQueue = bQueue[1:]
		if cur == nil || bwd[cur] {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return nil, false
		}
		bwd[cur] = true
		if cur == rootStore {
			continue
		}
		bQueue = append(bQueue, cur.Source...)
	}
	if !bwd[rootStore] {
		return nil, false
	}

	between := make(map[*OpCode]bool)
	for op := range fwd {
		if bwd[op] {
			between[op] = true
		}
	}
	return between, between[rootStore]
}

// mustExecuteBetween checks that every path from rootStore to any initOp in
// `between` passes through `target`.
func (d *Decompiler) mustExecuteBetween(rootStore, target *OpCode, inits []*OpCode, between map[*OpCode]bool) bool {
	if target == rootStore {
		return true
	}
	initSet := make(map[*OpCode]bool, len(inits))
	for _, initOp := range inits {
		initSet[initOp] = true
	}
	visited := map[*OpCode]bool{target: true, rootStore: true}
	queue := []*OpCode{}
	if !initSet[rootStore] {
		queue = append(queue, rootStore.Target...)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == nil || !between[cur] || visited[cur] {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		if initSet[cur] {
			return false
		}
		visited[cur] = true
		queue = append(queue, cur.Target...)
	}
	return true
}

func (d *Decompiler) collectAndVerifyAllocationClosure(rootStore *OpCode, inits []*OpCode, recvLoads map[*OpCode]bool, decoded, protectedStores map[*OpCode]bool) ([]*OpCode, map[*OpCode]bool, bool) {
	between, ok := d.opsBetween(rootStore, inits)
	if !ok {
		return nil, nil, false
	}
	initSet := make(map[*OpCode]bool, len(inits))
	for _, initOp := range inits {
		initSet[initOp] = true
	}
	betweenList := make([]*OpCode, 0, len(between))
	for op := range between {
		betweenList = append(betweenList, op)
	}
	sort.Slice(betweenList, func(i, j int) bool {
		if betweenList[i].CurrentOffset != betweenList[j].CurrentOffset {
			return betweenList[i].CurrentOffset < betweenList[j].CurrentOffset
		}
		return betweenList[i].Id < betweenList[j].Id
	})

	closure := []*OpCode{rootStore}
	closureSet := map[*OpCode]bool{rootStore: true}
	allowedLoads := make(map[*OpCode]bool, len(recvLoads))
	for rl := range recvLoads {
		allowedLoads[rl] = true
	}

	changed := true
	for changed {
		changed = false
		for _, op := range betweenList {
			if op == nil || op.Instr == nil || closureSet[op] || initSet[op] || !decoded[op] || protectedStores[op] {
				continue
			}
			if !isReferenceStoreOpcode(op.Instr.OpCode) || len(op.stackConsumed) != 1 {
				continue
			}
			prevLoad := d.allocationReceiverLoad(op.stackConsumed[0])
			if prevLoad == nil || !between[prevLoad] || prevLoad.Instr == nil || !isReferenceLoadOpcode(prevLoad.Instr.OpCode) {
				continue
			}
			slot := GetRetrieveIdx(prevLoad)
			defs, entry := d.allocationReachingStores(prevLoad, slot)
			if entry || len(defs) != 1 || !closureSet[defs[0]] {
				continue
			}
			if !d.mustExecuteBetween(rootStore, op, inits, between) {
				continue
			}
			closure = append(closure, op)
			closureSet[op] = true
			allowedLoads[prevLoad] = true
			changed = true
		}
	}

	closureRefIDs := make(map[*utils2.VariableId]bool, len(closure))
	closureSlots := make(map[int]bool, len(closure))
	for _, store := range closure {
		if !decoded[store] || protectedStores[store] || store.Instr == nil {
			return nil, nil, false
		}
		infos := d.opcodeIdToRef[store]
		if len(infos) != 1 {
			return nil, nil, false
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil {
			return nil, nil, false
		}
		closureRefIDs[ref.Id] = true
		if s := GetStoreIdx(store); s >= 0 {
			closureSlots[s] = true
		}
	}

	for _, op := range betweenList {
		if op == nil || op.Instr == nil {
			continue
		}
		if isLocalStoreOpcode(op.Instr.OpCode) && !closureSet[op] {
			infos := d.opcodeIdToRef[op]
			if len(infos) == 0 && closureSlots[GetStoreIdx(op)] {
				return nil, nil, false
			}
			for _, info := range infos {
				if ref, ok := info[0].(*values.JavaRef); ok && ref != nil && ref.Id != nil && closureRefIDs[ref.Id] {
					return nil, nil, false
				}
			}
		}
		if isLocalLoadOpcode(op.Instr.OpCode) && !allowedLoads[op] {
			slot := GetRetrieveIdx(op)
			if slot >= 0 && closureSlots[slot] {
				defs, _ := d.allocationReachingStores(op, slot)
				for _, def := range defs {
					if closureSet[def] {
						return nil, nil, false
					}
				}
			}
		}
	}

	return closure, closureSet, true
}

func (d *Decompiler) initsMutuallyExclusive(rootStore *OpCode, inits []*OpCode) bool {
	initSet := make(map[*OpCode]bool, len(inits))
	for _, initOp := range inits {
		initSet[initOp] = true
	}
	for _, start := range inits {
		visited := map[*OpCode]bool{start: true, rootStore: true}
		queue := append([]*OpCode{}, start.Target...)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == nil || visited[cur] {
				continue
			}
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
				return false
			}
			if initSet[cur] {
				return false
			}
			visited[cur] = true
			queue = append(queue, cur.Target...)
		}
	}
	return true
}
