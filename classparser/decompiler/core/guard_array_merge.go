package core

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// guardArrayCall proves that a branch-local literal String[] assignment can be
// evaluated at its only use, the immediately following conditional invocation.
// In particular, no other live statement may still read the array local.
func (d *Decompiler) guardArrayCall(definition, guard *Node, nodes []*Node, live map[*Node]bool) (*values.NewExpression, *values.FunctionCallExpression, int, bool) {
	if definition == nil || guard == nil || len(definition.Next) != 1 || definition.Next[0] != guard {
		return nil, nil, 0, false
	}
	assign, ok := definition.Statement.(*statements.AssignStatement)
	if !ok || assign.ArrayMember != nil {
		return nil, nil, 0, false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	if !ok || ref == nil {
		return nil, nil, 0, false
	}
	array, ok := GetRealValue(assign.JavaValue).(*values.NewExpression)
	if !ok || array == nil || array.Type() == nil || !array.IsArray() || !array.HasOriginPC ||
		!array.HasEvaluationEndPC || len(array.Initializer) == 0 ||
		array.Type().String(d.FunctionContext) != "String[]" {
		return nil, nil, 0, false
	}
	if len(array.Length) != 1 {
		return nil, nil, 0, false
	}
	length, ok := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	if !ok {
		return nil, nil, 0, false
	}
	if count, ok := length.Data.(int); !ok || count != len(array.Initializer) {
		return nil, nil, 0, false
	}
	for _, element := range array.Initializer {
		lit, ok := values.UnpackSoltValue(element).(*values.JavaLiteral)
		if !ok {
			return nil, nil, 0, false
		}
		if _, ok := lit.Data.(string); !ok {
			return nil, nil, 0, false
		}
	}
	condition, ok := guard.Statement.(*statements.ConditionStatement)
	if !ok || condition.Callback != nil || condition.TernaryChainArm ||
		guard.TrueNode == nil || guard.FalseNode == nil {
		return nil, nil, 0, false
	}
	guardValue := values.UnpackSoltValue(condition.Condition)
	if negated, ok := guardValue.(*values.JavaExpression); ok && negated.Op == "!" && len(negated.Values) == 1 {
		guardValue = values.UnpackSoltValue(negated.Values[0])
	}
	call, ok := guardValue.(*values.FunctionCallExpression)
	if !ok || call == nil || call.FuncType == nil {
		return nil, nil, 0, false
	}
	argIndex := -1
	for i, arg := range call.Arguments {
		if values.UnpackSoltValue(arg) == ref {
			if argIndex != -1 {
				return nil, nil, 0, false
			}
			argIndex = i
		}
	}
	if argIndex < 0 || argIndex >= len(call.FuncType.ParamTypes) ||
		call.FuncType.ParamTypes[argIndex] == nil ||
		call.FuncType.ParamTypes[argIndex].String(d.FunctionContext) != "String[]" ||
		array.EvaluationEndPC >= call.OriginPC {
		return nil, nil, 0, false
	}
	if call.Object != nil {
		if effect, _ := values.InspectValue(call.Object); effect != 0 {
			return nil, nil, 0, false
		}
	}
	for _, earlier := range call.Arguments[:argIndex] {
		if effect, _ := values.InspectValue(earlier); effect != 0 {
			return nil, nil, 0, false
		}
	}
	allocation, invocation := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
	if allocation == nil || invocation == nil || allocation.Instr == nil || invocation.Instr == nil ||
		allocation.Instr.OpCode != OP_ANEWARRAY || !branchArraySinglePath(d, allocation, invocation) {
		return nil, nil, 0, false
	}
	for _, n := range nodes {
		if live[n] && n != definition && n != guard && statementReferencesLocal(n.Statement, ref) {
			return nil, nil, 0, false
		}
	}
	// The selected argument must be the sole read even within this condition.
	originalArg := call.Arguments[argIndex]
	call.Arguments[argIndex] = array
	otherRead := statementReferencesLocal(guard.Statement, ref)
	call.Arguments[argIndex] = originalArg
	if otherRead {
		return nil, nil, 0, false
	}
	return array, call, argIndex, true
}

func guardGraph(root *Node) ([]*Node, map[*Node]bool) {
	var nodes []*Node
	live := map[*Node]bool{}
	WalkGraph[*Node](root, func(n *Node) ([]*Node, error) {
		live[n] = true
		nodes = append(nodes, n)
		return n.Next, nil
	})
	return nodes, live
}

func soleLiveSources(node *Node, live map[*Node]bool, expected ...*Node) bool {
	seen := map[*Node]bool{}
	for _, source := range node.Source {
		if live[source] {
			seen[source] = true
		}
	}
	if len(seen) != len(expected) {
		return false
	}
	for _, source := range expected {
		if !seen[source] {
			return false
		}
	}
	return true
}

// InlineLiteralArrayGuardChains exposes short-circuit guard CFG edges hidden
// behind synthetic varargs-array locals. It does not change unrelated locals
// or move an allocation across a side-effecting argument evaluation.
func (d *Decompiler) InlineLiteralArrayGuardChains() int {
	if d == nil || len(d.ExceptionTable) != 0 || d.getenv("JDEC_GUARD_ARRAY_MERGE_OFF") != "" {
		return 0
	}
	nodes, live := guardGraph(d.RootNode)
	changed := 0
	for _, parent := range nodes {
		parentCondition, ok := parent.Statement.(*statements.ConditionStatement)
		if !ok || parentCondition.Callback != nil || parentCondition.TernaryChainArm ||
			parent.TrueNode == nil || parent.FalseNode == nil {
			continue
		}
		definition, shared := parent.TrueNode(), parent.FalseNode()
		if definition == nil || shared == nil || !soleLiveSources(definition, live, parent) ||
			!slices.Contains(shared.Source, parent) {
			continue
		}
		if len(definition.Next) != 1 {
			continue
		}
		child := definition.Next[0]
		if child == nil || child.TrueNode == nil || child.FalseNode == nil ||
			(child.TrueNode() != shared && child.FalseNode() != shared) ||
			!soleLiveSources(child, live, definition) ||
			!slices.Contains(shared.Source, child) {
			continue
		}
		array, call, arg, ok := d.guardArrayCall(definition, child, nodes, live)
		if !ok {
			continue
		}
		call.Arguments[arg] = array
		parent.ReplaceNextSliceKeepOrder(definition, []*Node{child})
		definition.RemoveNext(child)
		for _, source := range slices.Clone(child.Source) {
			if !live[source] {
				source.RemoveNext(child)
			}
		}
		changed++
	}
	return changed + d.foldJoinedLiteralArrayFallback()
}

// foldJoinedLiteralArrayFallback recognizes:
//
//	P --true--> Q --one arm--> success
//	 \         \--other----> array -- C --one arm--> success
//	  \---------------------> array     \---------> fallback
//
// The array is created at the join of P and Q, then used only by C. With a
// literal-only array and side-effect-free preceding arguments, the equivalent
// Java condition is (P && Q_success) || C_success. Making that relation
// explicit prevents the if structurer from placing C under only one of its
// two incoming paths (or turning C's successful arm into an empty if).
func (d *Decompiler) foldJoinedLiteralArrayFallback() int {
	nodes, live := guardGraph(d.RootNode)
	changed := 0
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	for _, parent := range nodes {
		p, ok := parent.Statement.(*statements.ConditionStatement)
		if !ok || p.Condition == nil || p.Callback != nil || p.TernaryChainArm || parent.TrueNode == nil || parent.FalseNode == nil {
			continue
		}
		middle, definition := parent.TrueNode(), parent.FalseNode()
		if middle == nil || definition == nil || middle == definition || !soleLiveSources(middle, live, parent) ||
			!soleLiveSources(definition, live, parent, middle) || len(definition.Next) != 1 {
			continue
		}
		q, ok := middle.Statement.(*statements.ConditionStatement)
		if !ok || q.Condition == nil || q.Callback != nil || q.TernaryChainArm || middle.TrueNode == nil || middle.FalseNode == nil {
			continue
		}
		var success *Node
		switch {
		case middle.TrueNode() == definition:
			success = middle.FalseNode()
		case middle.FalseNode() == definition:
			success = middle.TrueNode()
		default:
			continue
		}
		guard := definition.Next[0]
		if success == nil || guard == nil || success == guard || !soleLiveSources(guard, live, definition) ||
			guard.TrueNode == nil || guard.FalseNode == nil {
			continue
		}
		c, ok := guard.Statement.(*statements.ConditionStatement)
		if !ok || c.Condition == nil || c.Callback != nil || c.TernaryChainArm {
			continue
		}
		var fallback *Node
		switch {
		case guard.TrueNode() == success:
			fallback = guard.FalseNode()
		case guard.FalseNode() == success:
			fallback = guard.TrueNode()
		default:
			continue
		}
		if fallback == nil || fallback == success || fallback == parent ||
			!slices.Contains(success.Source, middle) || !slices.Contains(success.Source, guard) {
			continue
		}
		array, call, arg, ok := d.guardArrayCall(definition, guard, nodes, live)
		if !ok {
			continue
		}
		qSuccess := q.Condition
		if middle.TrueNode() == definition {
			qSuccess = values.NewUnaryExpression(qSuccess, "!", boolType)
		}
		cSuccess := c.Condition
		if guard.FalseNode() == success {
			cSuccess = values.NewUnaryExpression(cSuccess, "!", boolType)
		}
		call.Arguments[arg] = array
		p.Condition = values.NewBinaryExpression(
			values.NewBinaryExpression(p.Condition, qSuccess, "&&", boolType),
			cSuccess, "||", boolType,
		)
		parent.RemoveAllNext()
		middle.RemoveAllNext()
		definition.RemoveAllNext()
		guard.RemoveAllNext()
		parent.AddNext(fallback)
		parent.AddNext(success)
		pn := parent
		pn.JmpNode = success
		pn.TrueNode = func() *Node { return pn.Next[1] }
		pn.FalseNode = func() *Node { return pn.Next[0] }
		changed++
	}
	return changed
}
