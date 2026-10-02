package core

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
	entry, prefix, conditions, ok := constructorArrayEntry(d.RootNode)
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
	component, ok := types.ClassFQNOf(array.Type().ElementType())
	if !ok || d.FunctionContext.IsTypeParam(component) {
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
		if !delegationArrayElementAssignable(statement.JavaValue, "L"+strings.ReplaceAll(component, ".", "/")+";", d.FunctionContext.InvocationMetadata) {
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

	// The last store must lead immediately to the one original delegation.
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
	last := len(call.Arguments) - 1
	if !delegationArraySameRef(call.Arguments[last], ref) || !sameExactArrayType(array.Type(), call.FuncType.ParamTypes[last]) {
		return false
	}
	invoke := origins[next.Id]
	allocation := d.opcodeAtOffset(array.OriginPC)
	if invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESPECIAL || int(invoke.CurrentOffset) != call.OriginPC || allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY {
		return false
	}

	method, err := types.ParseMethodDescriptor(call.Descriptor)
	if err != nil || method.FunctionType() == nil || len(method.FunctionType().ParamTypes) != len(call.Arguments) || !sameExactArrayType(array.Type(), method.FunctionType().ParamTypes[last]) || len(invoke.stackConsumed) != len(call.Arguments)+1 {
		return false
	}
	ret, ok := method.FunctionType().ReturnType.RawType().(*types.JavaPrimer)
	if !ok || ret.Name != types.JavaVoid {
		return false
	}
	decoded := d.invokeFuncCall[invoke]
	if decoded == nil || decoded.Descriptor != call.Descriptor || decoded.ClassName != call.ClassName || decoded.FunctionName != "<init>" || !delegationArraySameRef(decoded.Object, receiver) {
		return false
	}
	for i, argument := range call.Arguments {
		if values.UnpackSoltValue(argument) != values.UnpackSoltValue(invoke.stackConsumed[last-i]) {
			return false
		}
	}
	if !delegationArraySameRef(invoke.stackConsumed[last+1], receiver) {
		return false
	}
	if !d.privateDelegationArrayDAG(allocation, invoke, ref, array, stores, items) {
		return false
	}

	for _, arg := range call.Arguments[:last] {
		if !d.branchOperandPrecedesArray(arg, allocation) {
			return false
		}
	}
	// Use a private prospective tree. Failed ownership proofs publish nothing.
	copy := *array
	copy.Initializer = items
	args := slices.Clone(call.Arguments)
	args[last] = &copy

	if !constructorConditionsBelongToPrefixArguments(conditions, origins, args) {
		return false
	}

	nodes, _ := guardGraph(d.RootNode)
	reachable := map[*Node]bool{}
	for _, node := range nodes {
		reachable[node] = true
	}
	for node := range removed {
		if node.IsCatchStart || node.IsTryCatch {
			return false
		}
		for _, source := range node.Source {
			if reachable[source] && !removed[source] {
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
		}
	}
	// One allocation, every original store and every conditional value now belong
	// to the unchanged invoke operand. The source array type and RHS trees remain
	// identical; no cast is added and its AASTORE runtime checks remain in order.
	array.Initializer = items
	array.EvaluationEndPC = int(stores[len(stores)-1].CurrentOffset)
	array.HasEvaluationEndPC = true
	call.Arguments[last] = array
	for node := range removed {
		node.RemoveAllNext()
		node.RemoveAllSource()
	}
	d.RootNode = next
	if d.varUserMap != nil {
		d.varUserMap.Delete(ref)
	}
	if ref.Id != nil {
		ref.Id.Delete()
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
		name, ok := types.ClassFQNOf(value.Type())
		return ok && callbinding.Assignable("L"+strings.ReplaceAll(name, ".", "/")+";", target, metadata)
	}
	return visit(value)
}

// Every forward path must fill the same positions once and reach the original
// invokespecial. The array lives solely on the operand stack: no local/field
// publication, alias, partial-fill observation or changed exception domain.
func (d *Decompiler) privateDelegationArrayDAG(allocation, invoke *OpCode, ref *values.JavaRef, array *values.NewExpression, stores []*OpCode, items []values.JavaValue) bool {
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
		if op == nil || op.Instr == nil || op.CurrentOffset < allocation.CurrentOffset || op.CurrentOffset > invoke.CurrentOffset || active[op] || len(state) >= 1024 || !sameHandlerCoverage(domain, d.handlersAt(op)) {
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
			if next == nil || next.CurrentOffset <= op.CurrentOffset || !visit(next, filled) {
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
	if !delegationArrayEffectSites(items, stores, state, allocation, invoke, d.invokeFuncCall) {
		return false
	}
	uses := 0
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		for index, value := range op.stackConsumed {
			if !delegationArraySameRef(value, ref) {
				continue
			}
			if op == invoke && index == 0 {
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
	if len(stores) != len(items) {
		return false
	}
	for index, item := range items {
		upper = int(stores[index].CurrentOffset)
		if !visit(item) {
			return false
		}
		lower = upper
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
