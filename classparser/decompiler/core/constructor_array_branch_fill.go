package core

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A delegation array can contain already recovered value diamonds. Their
// condition/goto scaffolding is not an observable statement between stores.
// Recover this one constructor-entry operand transactionally from the original
// private stack DAG, then remove only the scaffolding owned by its value tree.
// The general named-array folding and ordinary motion rules stay conservative.
func (d *Decompiler) inlinePrivateDelegationBranchArray(origins map[int]*OpCode) bool {
	if d == nil || d.RootNode == nil || d.FunctionContext == nil || d.FunctionContext.FunctionName != "<init>" || d.getenv("JDEC_CTOR_ARRAY_ARG_INLINE_OFF") != "" {
		return false
	}
	nodes, reachable, known := d.privateDelegationArraySourceGraph()
	if !known {
		return false
	}
	for _, root := range nodes {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if root != d.RootNode {
			assign, known := root.Statement.(*statements.AssignStatement)
			if !known || assign.ArrayMember != nil {
				continue
			}
			array, known := values.UnpackSoltValue(assign.JavaValue).(*values.NewExpression)
			if !known || array == nil || !array.IsArray() || !array.HasOriginPC || array.HasEvaluationEndPC {
				continue
			}
		}
		if d.inlinePrivateDelegationBranchArrayAt(root, origins, nodes, reachable) {
			return true
		}
	}
	return false
}

// Bound and charge discovery before adding another node or edge. The same
// immutable source view is used by every candidate and by its outside-use proof.
func (d *Decompiler) privateDelegationArraySourceGraph() ([]*Node, map[*Node]bool, bool) {
	nodes := []*Node{d.RootNode}
	reachable := map[*Node]bool{d.RootNode: true}
	for index := 0; index < len(nodes); index++ {
		node := nodes[index]
		if node == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil, nil, false
		}
		for _, next := range node.Next {
			if next == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
				return nil, nil, false
			}
			if reachable[next] {
				continue
			}
			if len(nodes) >= 4096 || d.Work != nil && d.Work.CheckAlloc(int64(len(nodes)+1)*128) != nil {
				return nil, nil, false
			}
			reachable[next] = true
			nodes = append(nodes, next)
		}
	}
	return nodes, reachable, true
}

func (d *Decompiler) inlinePrivateDelegationBranchArrayAt(root *Node, origins map[int]*OpCode, nodes []*Node, reachable map[*Node]bool) bool {
	entry, prefix, conditions, ok := constructorArrayEntry(root)
	if !ok {
		return false
	}
	assign, ok := entry.Statement.(*statements.AssignStatement)
	if !ok || assign.ArrayMember != nil || len(entry.Next) != 1 {
		return false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	array, arrayOK := values.UnpackSoltValue(assign.JavaValue).(*values.NewExpression)
	if !ok || ref == nil || ref.IsParam || ref.IsThis || !arrayOK || !array.IsArray() || array.HasEvaluationEndPC || !array.HasOriginPC || len(array.Length) != 1 {
		return false
	}
	length, ok := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	if !ok {
		return false
	}
	n, ok := length.Data.(int)
	if !ok || n < 1 || n > 128 {
		return false
	}
	componentType := array.Type().ElementType()
	if name, named := types.ClassFQNOf(componentType); named && d.FunctionContext.IsTypeParam(name) {
		return false
	}
	// ANEWARRAY consumes an immediate reference component, not necessarily
	// a class: Object[][] has Object[] components. Retain that complete
	// descriptor so AASTORE assignability does not lose array rank or admit
	// a scalar primitive under the leaf class of a different array.
	component := values.ReferenceTypeDescriptor(componentType, d.FunctionContext)
	if component == "" {
		return false
	}

	removed := map[*Node]bool{entry: true}
	for node := range prefix {
		removed[node] = true
	}
	items := make([]values.JavaValue, 0, n)
	stores := make([]*OpCode, 0, n)
	next := entry.Next[0]
	for index := 0; index < n; index++ {
		store, scaffolding, owned, ok := constructorArrayEntry(next)
		if !ok {
			return false
		}
		for node := range scaffolding {
			if removed[node] {
				return false
			}
			removed[node] = true
		}
		conditions = append(conditions, owned...)
		statement, ok := store.Statement.(*statements.AssignStatement)
		if !ok || statement.ArrayMember == nil || len(store.Next) != 1 || removed[store] || !statement.HasOriginPC ||
			!delegationArraySameRef(statement.ArrayMember.Object, ref) {
			return false
		}
		slot, ok := values.UnpackSoltValue(statement.ArrayMember.Index).(*values.JavaLiteral)
		if !ok || slot.Data != index {
			return false
		}
		op := origins[store.Id]
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_AASTORE || int(op.CurrentOffset) != statement.OriginPC || len(op.stackConsumed) != 3 ||
			!delegationArraySameRef(op.stackConsumed[2], ref) || values.UnpackSoltValue(op.stackConsumed[0]) != values.UnpackSoltValue(statement.JavaValue) || values.UnpackSoltValue(op.stackConsumed[1]) != values.UnpackSoltValue(statement.ArrayMember.Index) {
			return false
		}
		if !delegationArrayElementAssignable(statement.JavaValue, component, d.FunctionContext.InvocationMetadata) {
			return false
		}
		effect, uses := values.InspectValue(statement.JavaValue)
		if effect&values.EffectOpaque != 0 {
			return false
		}
		for used := range uses {
			if values.SameLocal(used, ref) {
				return false
			}
		}
		removed[store] = true
		items = append(items, statement.JavaValue)
		stores = append(stores, op)
		next = store.Next[0]
	}

	// Later operands may have their own completed DUP array materializations.
	// Retain their original argument positions in the same source packet; a
	// general local definition or observable statement is not admitted here.
	var laterArrays []delegationArrayMaterialization
	for len(laterArrays) < 128 {
		assignment, known := next.Statement.(*statements.AssignStatement)
		if !known || assignment.ArrayMember != nil {
			break
		}
		later, known := values.UnpackSoltValue(assignment.JavaValue).(*values.NewExpression)
		local, localKnown := values.UnpackSoltValue(assignment.LeftValue).(*values.JavaRef)
		if !known || !localKnown || !later.IsArray() || !later.HasEvaluationEndPC || local.IsParam || local.IsThis || len(next.Next) != 1 || removed[next] || next.IsTryCatch || next.IsCatchStart {
			return false
		}
		laterArrays = append(laterArrays, delegationArrayMaterialization{node: next, array: later, ref: local})
		removed[next] = true
		next = next.Next[0]
	}
	// The completed packet must end at the one original delegation.
	expr, ok := next.Statement.(*statements.ExpressionStatement)
	if !ok {
		return false
	}
	call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
	if !ok || !call.IsSpecialInvoke || call.Kind != values.InvokeSpecial || call.FunctionName != "<init>" || !call.HasOriginPC || call.FuncType == nil {
		return false
	}
	// An uninitialized-this delegation can target only this class or its
	// direct superclass. The original call witness must retain that owner.
	owner := strings.ReplaceAll(call.ClassName, "/", ".")
	self := strings.ReplaceAll(d.FunctionContext.ClassName, "/", ".")
	parent := strings.ReplaceAll(d.FunctionContext.SupperClassName, "/", ".")
	if owner == "" || self == "" || (owner != self && owner != parent) {
		return false
	}
	receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !ok || !receiver.IsThis || len(call.Arguments) == 0 || len(call.Arguments) != len(call.FuncType.ParamTypes) {
		return false
	}
	consumer, invoke, argument, ok := d.privateDelegationArrayConsumer(call, origins[next.Id], ref, array)
	if !ok {
		return false
	}
	args := slices.Clone(consumer.Arguments)
	for _, later := range laterArrays {
		index := delegationArrayArgument(consumer.Arguments, later.ref)
		if index <= argument || !d.privateDelegationCompletedArray(later, consumer, invoke, index, origins) {
			return false
		}
		args[index] = later.array
	}
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY ||
		!d.privateArrayValueDAG(allocation, invoke, ref, array, stores, items, len(consumer.Arguments)-1-argument, nil, args[argument+1:]...) {
		return false
	}

	for _, arg := range consumer.Arguments[:argument] {
		if !d.branchOperandPrecedesArray(arg, allocation) {
			return false
		}
	}
	// Use a private prospective tree. Failed ownership proofs publish nothing.
	copy := *array
	copy.Initializer = items
	args[argument] = &copy

	if !constructorConditionsBelongToPrefixArguments(conditions, origins, args) {
		return false
	}

	for node := range removed {
		if node.IsCatchStart || node.IsTryCatch {
			return false
		}
		for _, source := range node.Source {
			if reachable[source] && !removed[source] && node != root {
				return false
			}
		}
	}
	for _, source := range next.Source {
		if reachable[source] && !removed[source] {
			return false
		}
	}
	for _, node := range nodes {
		if removed[node] || node == next {
			continue
		}
		nodeValues, known := constructorInlineNodeValues(node.Statement)
		if !known {
			return false
		}
		for _, v := range nodeValues {
			if valueMentionsLocal(v, ref) {
				return false
			}
			for _, later := range laterArrays {
				if valueMentionsLocal(v, later.ref) {
					return false
				}
			}
		}
	}
	// One allocation, every original store and every conditional value now belong
	// to the unchanged invoke operand. The source array type and RHS trees remain
	// identical; no cast is added and its AASTORE runtime checks remain in order.
	array.Initializer = items
	array.EvaluationEndPC = int(stores[len(stores)-1].CurrentOffset)
	array.HasEvaluationEndPC = true
	args[argument] = array
	consumer.Arguments = args
	// Retain any earlier source statement, such as a named member's physical
	// enclosing capture. Only the private array region is replaced. Every
	// outside source edge must enter its first node, just as the original
	// operand DAG permits outside bytecode edges only at its allocation.
	for _, source := range slices.Clone(root.Source) {
		if removed[source] {
			continue
		}
		source.ReplaceNextSliceKeepOrder(root, []*Node{next})
		source.ReplaceSwitchTarget(root, next)
		if source.JmpNode == root {
			source.JmpNode = next
		}
	}
	for node := range removed {
		node.RemoveAllNext()
		node.RemoveAllSource()
	}
	if d.RootNode == root {
		d.RootNode = next
	}
	if d.varUserMap != nil {
		d.varUserMap.Delete(ref)
	}
	if ref.Id != nil {
		ref.Id.Delete()
	}
	for _, later := range laterArrays {
		if d.varUserMap != nil {
			d.varUserMap.Delete(later.ref)
		}
		if later.ref.Id != nil {
			later.ref.Id.Delete()
		}
	}
	return true
}

func delegationArrayElementAssignable(value values.JavaValue, target string, metadata callbinding.Provider) bool {
	path := map[values.JavaValue]bool{}
	steps := 0
	var visit func(values.JavaValue) bool
	visit = func(value values.JavaValue) bool {
		value = values.UnpackSoltValue(value)
		if value == nil || path[value] || steps >= 256 {
			return false
		}
		steps++
		path[value] = true
		defer delete(path, value)
		if values.IsNullLiteral(value) {
			return true
		}
		if ternary, ok := value.(*values.TernaryExpression); ok {
			return visit(ternary.TrueValue) && visit(ternary.FalseValue)
		}
		if value.Type() == nil {
			return false
		}
		// An int[] remains a reference; an int remains a scalar. Use the same
		// bounded descriptor identity as cast binding, including literal String
		// tags and array rank, instead of guessing from an element/class name.
		descriptor := values.ReferenceTypeDescriptor(value.Type(), &class_context.ClassContext{InvocationMetadata: metadata})
		return descriptor != "" && callbinding.Assignable(descriptor, target, metadata)
	}
	return visit(value)
}

// Every forward path must fill the same positions once and reach the original
// invokespecial. The array lives solely on the operand stack: no local/field
// publication, alias, partial-fill observation or changed exception domain.
func (d *Decompiler) privateDelegationArrayDAG(allocation, invoke *OpCode, ref *values.JavaRef, array *values.NewExpression, stores []*OpCode, items []values.JavaValue) bool {
	return d.privateArrayValueDAG(allocation, invoke, ref, array, stores, items, 0, nil)
}

// An initialized stack-only array can end at an invocation operand or at its
// original indexed read. The latter retains the index as the final effect
// interval, after all element stores and before the unchanged load/checks.
func (d *Decompiler) privateArrayValueDAG(allocation, invoke *OpCode, ref *values.JavaRef, array *values.NewExpression, stores []*OpCode, items []values.JavaValue, useIndex int, indexValue values.JavaValue, trailing ...values.JavaValue) bool {
	if allocation == nil || invoke == nil || allocation.CurrentOffset >= invoke.CurrentOffset || len(allocation.stackProduced) != 1 || values.UnpackSoltValue(allocation.stackProduced[0]) != array {
		return false
	}
	if len(allocation.Target) != 1 {
		return false
	}
	dup := allocation.Target[0]
	if dup == nil || dup.Instr == nil || dup.Instr.OpCode != OP_DUP || len(dup.stackConsumed) != 1 || values.UnpackSoltValue(dup.stackConsumed[0]) != array || len(dup.stackProduced) != 2 || !delegationArraySameRef(dup.stackProduced[0], ref) || !delegationArraySameRef(dup.stackProduced[1], ref) {
		return false
	}
	storeIndex := map[*OpCode]int{}
	for index, op := range stores {
		if _, exists := storeIndex[op]; exists {
			return false
		}
		storeIndex[op] = index
	}
	state := map[*OpCode]int{}
	active := map[*OpCode]bool{}
	domain := d.handlersAt(allocation)
	var visit func(*OpCode, int) bool
	visit = func(op *OpCode, filled int) bool {
		if op == nil || op.Instr == nil || op.CurrentOffset < allocation.CurrentOffset || op.CurrentOffset > invoke.CurrentOffset || active[op] || len(state) >= 1024 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || !sameHandlerCoverage(domain, d.handlersAt(op)) {
			return false
		}
		if previous, seen := state[op]; seen {
			return previous == filled
		}
		state[op] = filled
		active[op] = true
		defer delete(active, op)
		if op == invoke {
			return filled == len(stores)
		}
		if index, store := storeIndex[op]; store {
			if index != filled {
				return false
			}
			filled++
		}
		if LocalAccessOf(op.Instr.OpCode).Write || op.Instr.OpCode == OP_PUTFIELD || op.Instr.OpCode == OP_PUTSTATIC {
			return false
		}
		if len(op.Target) == 0 || len(op.Target) > 2 {
			return false
		}
		for _, next := range op.Target {
			if next == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil || next.CurrentOffset <= op.CurrentOffset || !visit(next, filled) {
				return false
			}
		}
		return true
	}
	if !visit(allocation, 0) {
		return false
	}
	for op := range state {
		if op == allocation {
			continue
		}
		for _, source := range op.Source {
			if _, inside := state[source]; !inside {
				return false
			}
		}
	}
	effectValues := slices.Clone(items)
	effectEnds := make([]int, 0, len(stores)+len(trailing)+1)
	for _, store := range stores {
		effectEnds = append(effectEnds, int(store.CurrentOffset))
	}
	if indexValue != nil {
		if len(trailing) != 0 {
			return false
		}
		effectValues = append(effectValues, indexValue)
		effectEnds = append(effectEnds, int(invoke.CurrentOffset))
	}
	inclusiveFrom := len(effectValues)
	if len(trailing) != 0 {
		if len(effectEnds) == 0 {
			return false
		}
		lower := effectEnds[len(effectEnds)-1]
		for _, value := range trailing {
			end, known := d.delegationArrayTrailingEnd(value, lower, invoke)
			if !known {
				return false
			}
			effectValues = append(effectValues, value)
			effectEnds = append(effectEnds, end)
			lower = end
		}
	}
	if !delegationArrayEffectWindows(effectValues, effectEnds, inclusiveFrom, state, allocation, invoke, d.invokeFuncCall) {
		return false
	}
	uses := 0
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		for index, value := range op.stackConsumed {
			if !delegationArraySameRef(value, ref) {
				continue
			}
			if op == invoke && index == useIndex {
				uses++
				continue
			}
			if _, store := storeIndex[op]; store && index == 2 {
				continue
			}
			return false
		}
	}
	return uses == 1
}

func delegationArraySameRef(value values.JavaValue, ref *values.JavaRef) bool {
	candidate, ok := values.UnpackSoltValue(value).(*values.JavaRef)
	return ok && values.SameLocal(candidate, ref)
}

// Calls and fresh allocations represented in an initializer must correspond
// one-for-one with the original private DAG. In particular, a discarded call
// or a reused producer cannot silently disappear into an apparently equal RHS.
func delegationArrayEffectSites(items []values.JavaValue, stores []*OpCode, sites map[*OpCode]int, allocation, invoke *OpCode, decoded map[*OpCode]*values.FunctionCallExpression) bool {
	ends := make([]int, 0, len(stores))
	for _, store := range stores {
		if store == nil {
			return false
		}
		ends = append(ends, int(store.CurrentOffset))
	}
	return delegationArrayEffectWindows(items, ends, len(items), sites, allocation, invoke, decoded)
}

func delegationArrayEffectWindows(items []values.JavaValue, ends []int, inclusiveFrom int, sites map[*OpCode]int, allocation, invoke *OpCode, decoded map[*OpCode]*values.FunctionCallExpression) bool {
	calls := map[int]*values.FunctionCallExpression{}
	news := map[int]*values.NewExpression{}
	path := map[values.JavaValue]bool{}
	steps := 0
	lower, upper := int(allocation.CurrentOffset), 0
	addCall := func(call *values.FunctionCallExpression) bool {
		if call == nil || !call.HasOriginPC || calls[call.OriginPC] != nil || call.OriginPC <= lower || call.OriginPC >= upper {
			return false
		}
		calls[call.OriginPC] = call
		return true
	}
	var visit func(values.JavaValue) bool
	visit = func(value values.JavaValue) bool {
		if value == nil {
			return true
		}
		if path[value] || steps >= 4096 {
			return false
		}
		steps++
		path[value] = true
		defer delete(path, value)
		switch v := value.(type) {
		case *values.FunctionCallExpression:
			if !addCall(v) {
				return false
			}
		case *values.NewExpression:
			if !v.HasOriginPC || news[v.OriginPC] != nil || v.OriginPC <= lower || v.OriginPC >= upper {
				return false
			}
			news[v.OriginPC] = v
			if v.ConstructorCall != nil && !addCall(v.ConstructorCall) {
				return false
			}
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	if len(ends) != len(items) {
		return false
	}
	for index, item := range items {
		upper = ends[index]
		if index >= inclusiveFrom {
			upper++
		}
		if !visit(item) {
			return false
		}
		lower = ends[index]
	}
	for op := range sites {
		if op == allocation || op == invoke {
			continue
		}
		if isInvokeOpcode(op.Instr.OpCode) {
			original, kept := decoded[op], calls[int(op.CurrentOffset)]
			if original == nil || kept == nil || original.Witness() != kept.Witness() {
				return false
			}
			delete(calls, int(op.CurrentOffset))
		}
		switch op.Instr.OpCode {
		case OP_NEW, OP_ANEWARRAY, OP_NEWARRAY, OP_MULTIANEWARRAY:
			if news[int(op.CurrentOffset)] == nil {
				return false
			}
			delete(news, int(op.CurrentOffset))
		}
	}
	return len(calls) == 0 && len(news) == 0
}
