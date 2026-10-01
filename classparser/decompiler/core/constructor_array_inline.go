package core

import (
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// inlineDelegatingConstructorArrayTemp removes proven constructor-entry spills.
// Initialized arrays require their full DUP/array-store span; ordinary CHECKCAST
// spills (including a null-check result consumed by arraylength) use the eager
// single-use effect-motion proof. Java requires this()/super() to be the first constructor
// statement, so an otherwise-correct `T[] temp = ...; this(temp);` is invalid.
//
// The rewrite is limited to an entry prefix and a straight-line spill sequence:
// every entry path must reach the same temp, the next statement must be the
// delegation call, the temp must have no other uses, and exception-handler
// coverage must match. Substituting the array expression at its original
// argument position preserves Java's left-to-right evaluation order with the
// other constructor arguments. Conditional scaffolding is allowed only when
// its bytecode identity belongs to an earlier, already reconstructed argument.
func (d *Decompiler) inlineDelegatingConstructorArrayTemp(origins map[int]*OpCode) bool {
	skip := func(reason string, args ...any) bool {
		d.tracef("ctor-array-inline", "skip: %s", fmt.Sprintf(reason, args...))
		return false
	}
	if d == nil || d.RootNode == nil || d.FunctionContext == nil ||
		d.FunctionContext.FunctionName != "<init>" || d.getenv("JDEC_CTOR_ARRAY_ARG_INLINE_OFF") != "" {
		return false
	}

	entry, prefix, conditions, ok := constructorArrayEntry(d.RootNode)
	if !ok {
		return skip("constructor entry does not converge through value-only scaffolding")
	}

	type spill struct {
		node   *Node
		temp   *values.JavaRef
		value  values.JavaValue
		opcode *OpCode
		arg    int
	}
	spills := make([]spill, 0, 2)
	seenSpills := map[*Node]bool{}
	current := entry
	var callNode *Node
	for {
		if seenSpills[current] || len(seenSpills) >= 256 {
			return skip("array spill sequence is cyclic or exceeds the proof bound")
		}
		seenSpills[current] = true
		assign, ok := current.Statement.(*statements.AssignStatement)
		if !ok || assign.ArrayMember != nil || len(current.Next) != 1 {
			return skip("entry sequence is not a single-successor local assignment: node=%d stmt=%T next=%d", current.Id, current.Statement, len(current.Next))
		}
		temp, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
		if !ok || temp == nil {
			return skip("assignment target is not a local ref: node=%d left=%T", current.Id, assign.LeftValue)
		}
		if assign.JavaValue == nil {
			return skip("entry spill has no value: node=%d", current.Id)
		}
		spills = append(spills, spill{node: current, temp: temp, value: assign.JavaValue, opcode: origins[current.Id], arg: -1})
		next := current.Next[0]
		if next == nil {
			return skip("array assignment has a nil successor")
		}
		if _, moreAssignments := next.Statement.(*statements.AssignStatement); moreAssignments {
			// Once the first array spill is found, consecutive assignments are part
			// of the same entry prefix. If one is not a literal array spill, fail
			// closed instead of silently skipping an observable statement.
			current = next
			continue
		}
		callNode = next
		break
	}
	reachable := map[*Node]bool{}
	_ = WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		reachable[node] = true
		return node.Next, nil
	})
	spillNodes := make(map[*Node]bool, len(spills))
	spillTemps := make(map[*values.JavaRef]bool, len(spills))
	for i, candidate := range spills {
		spillNodes[candidate.node] = true
		spillTemps[candidate.temp] = true
		if i == 0 && entry == d.RootNode {
			if candidate.node != d.RootNode || len(candidate.node.Source) != 0 {
				return skip("first array spill has an unexpected predecessor")
			}
		} else {
			foundSource := false
			for _, source := range candidate.node.Source {
				if reachable[source] {
					expected := i == 0 && prefix[source]
					if i > 0 {
						expected = source == spills[i-1].node
					}
					if !expected {
						return skip("array spill has an alternate reachable predecessor")
					}
					foundSource = true
				}
			}
			if !foundSource {
				return skip("array spill is not reached from the preceding entry node")
			}
		}
		if candidate.opcode == nil || candidate.opcode.Instr == nil {
			return skip("array spill has no bytecode origin: node=%d", candidate.node.Id)
		}
		// Literal arrays need a complete fill-span proof. Other spills retain
		// the same unique-use, effect-order and producer-identity requirements.
		proven := d.canInlineDelegationValue(candidate.value, candidate.temp, candidate.opcode)
		if isInitializedArrayLiteral(candidate.value) {
			proven = d.canInlineDelegatingArrayValue(candidate.value, candidate.temp, candidate.opcode)
		}
		if !proven {
			return skip("entry spill lacks producer or use-order proof: node=%d", candidate.node.Id)
		}
		if !sameIntSlice(d.handlersAt(candidate.opcode), d.handlersAt(origins[callNode.Id])) {
			return skip("array spill and delegation have different or unknown handler domains")
		}
	}
	lastSpillNode := spills[len(spills)-1].node
	foundLastSpillSource := false
	for _, source := range callNode.Source {
		if !reachable[source] {
			continue
		}
		if source != lastSpillNode {
			return skip("delegation call has an alternate reachable predecessor")
		}
		foundLastSpillSource = true
	}
	if !foundLastSpillSource {
		return skip("delegation call is not reached from the final array spill")
	}
	for _, candidate := range spills {
		for temp := range spillTemps {
			if valueMentionsLocal(candidate.value, temp) {
				return skip("array initializer depends on a constructor spill")
			}
		}
	}
	expr, ok := callNode.Statement.(*statements.ExpressionStatement)
	if !ok {
		return skip("array consumer is not an expression statement: node=%d stmt=%T", callNode.Id, callNode.Statement)
	}
	call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.IsSpecialInvoke || call.FunctionName != "<init>" {
		return skip("array consumer is not a special constructor call")
	}
	receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !ok || receiver == nil || !receiver.IsThis {
		return skip("constructor call receiver is not this")
	}
	callOp := origins[callNode.Id]
	if callOp == nil {
		return skip("delegation call origin is unavailable")
	}
	spillTempsInOrder := make([]*values.JavaRef, len(spills))
	for i := range spills {
		spillTempsInOrder[i] = spills[i].temp
	}
	argIndexes, ordered := orderedDelegatingConstructorTempArgIndexes(call.Arguments, spillTempsInOrder)
	if !ordered {
		return skip("constructor arguments must consume each array spill once in source order")
	}
	for i := range spills {
		spills[i].arg = argIndexes[i]
	}
	if !constructorConditionsBelongToPrefixArguments(conditions, origins, call.Arguments[:argIndexes[0]]) {
		return skip("entry condition is not owned by an earlier constructor argument")
	}

	// Refuse the rewrite if any other statement reads or writes the temporary.
	// This catches aliases and later uses that a local adjacency check alone
	// would miss, while allowing the parameter references inside the initializer.
	usedElsewhere := false
	_ = WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if !spillNodes[node] && node != callNode {
			if nodeValues, known := constructorInlineNodeValues(node.Statement); !known {
				usedElsewhere = true
				return nil, nil
			} else {
				for _, value := range nodeValues {
					for temp := range spillTemps {
						if valueMentionsLocal(value, temp) {
							usedElsewhere = true
							return nil, nil
						}
					}
				}
			}
		}
		return node.Next, nil
	})
	if usedElsewhere {
		return skip("temporary has another use or crosses an unsupported statement")
	}
	for _, candidate := range spills {
		call.Arguments[candidate.arg] = replaceDelegatingConstructorTemp(call.Arguments[candidate.arg], candidate.temp, candidate.value)
	}

	// Reconnect each entry edge in place. The surviving conditions still supply
	// their argument callbacks; only the array declarations disappear.
	// MiscRewriter can leave stale Source links from removed array-fill nodes; they
	// are unreachable from the method root, so discard those backlinks before the
	// constructor call participates in later predecessor-sensitive rewrites.
	for _, source := range append([]*Node(nil), callNode.Source...) {
		if !reachable[source] {
			source.RemoveNext(callNode)
		}
	}
	for _, source := range slices.Clone(entry.Source) {
		if prefix[source] {
			source.ReplaceNextSliceKeepOrder(entry, []*Node{callNode})
		}
	}
	for _, candidate := range spills {
		for _, next := range append([]*Node(nil), candidate.node.Next...) {
			candidate.node.RemoveNext(next)
		}
		for _, source := range append([]*Node(nil), candidate.node.Source...) {
			source.RemoveNext(candidate.node)
		}
		candidate.node.Source = nil
		candidate.node.Next = nil
		if candidate.temp.Id != nil {
			candidate.temp.Id.Delete()
		}
	}
	if entry == d.RootNode {
		d.RootNode = callNode
	}
	for _, source := range append([]*Node(nil), callNode.Source...) {
		if spillNodes[source] {
			callNode.RemoveSource(source)
		}
	}
	return true
}

// orderedDelegatingConstructorTempArgIndexes proves that each spill has one
// eager use in the constructor call, in the same order that the bytecode
// evaluated the spill initializers. Several temps may occur inside one nested
// argument (for example new Format(temp1, temp2)); their tree order must still
// match bytecode order because replacing them removes the preceding stores.
func orderedDelegatingConstructorTempArgIndexes(args []values.JavaValue, temps []*values.JavaRef) ([]int, bool) {
	indexes := make([]int, len(temps))
	for i := range indexes {
		indexes[i] = -1
	}
	var sequence []int
	for argIndex, arg := range args {
		argSequence, supported := delegatingConstructorTempOrder(arg, temps)
		if !supported {
			return nil, false
		}
		for _, tempIndex := range argSequence {
			if tempIndex < 0 || tempIndex >= len(indexes) || indexes[tempIndex] >= 0 {
				return nil, false
			}
			indexes[tempIndex] = argIndex
			sequence = append(sequence, tempIndex)
		}
	}
	if len(sequence) != len(temps) {
		return nil, false
	}
	for i, tempIndex := range sequence {
		if tempIndex != i {
			return nil, false
		}
	}
	for _, argIndex := range indexes {
		if argIndex < 0 {
			return nil, false
		}
	}
	return indexes, true
}

// delegatingConstructorTempOrder walks only eager Java evaluation positions
// and returns the spills in the order their values are read. Conditional and
// opaque containers fail closed if they mention any spill.
func delegatingConstructorTempOrder(value values.JavaValue, temps []*values.JavaRef) ([]int, bool) {
	return delegatingConstructorTempOrderPath(value, temps, make(map[values.JavaValue]bool))
}

func delegatingConstructorTempOrderPath(value values.JavaValue, temps []*values.JavaRef, path map[values.JavaValue]bool) ([]int, bool) {
	value = values.UnpackSoltValue(value)
	if value == nil {
		return nil, true
	}
	if path[value] {
		return nil, false
	}
	path[value] = true
	defer delete(path, value)
	switch v := value.(type) {
	case *values.JavaRef:
		for i, temp := range temps {
			if values.SameLocal(v, temp) {
				return []int{i}, true
			}
		}
		return nil, true
	case *values.ArrayLengthExpression:
		return delegatingConstructorTempOrderPath(v.Array, temps, path)
	case *values.CastExpression:
		return delegatingConstructorTempOrderPath(v.Value, temps, path)
	case *values.JavaExpression:
		if v.Op == "&&" || v.Op == "||" {
			return nil, !constructorValueMentionsAnyTemp(value, temps)
		}
		return orderedConstructorTempChildrenPath(v.Values, temps, path)
	case *values.FunctionCallExpression:
		children := append([]values.JavaValue{v.Object}, v.Arguments...)
		return orderedConstructorTempChildrenPath(children, temps, path)
	case *values.NewExpression:
		children := append([]values.JavaValue{}, v.Length...)
		children = append(children, v.Initializer...)
		if v.ConstructorCall != nil {
			// ConstructorCall.Object points back to this allocation node; only
			// its arguments are evaluated after the allocation.
			children = append(children, v.ConstructorCall.Arguments...)
		} else if v.ArgumentsGetter != nil {
			return nil, !constructorValueMentionsAnyTemp(value, temps)
		}
		return orderedConstructorTempChildrenPath(children, temps, path)
	case *values.JavaArrayMember:
		return orderedConstructorTempChildrenPath([]values.JavaValue{v.Object, v.Index}, temps, path)
	case *values.RefMember:
		return delegatingConstructorTempOrderPath(v.Object, temps, path)
	case *values.JavaCompare:
		return orderedConstructorTempChildrenPath([]values.JavaValue{v.JavaValue1, v.JavaValue2}, temps, path)
	case *values.AssignmentExpression:
		if ref, ok := values.UnpackSoltValue(v.Target).(*values.JavaRef); ok {
			for _, temp := range temps {
				if values.SameLocal(ref, temp) {
					return nil, false
				}
			}
		}
		return orderedConstructorTempChildrenPath([]values.JavaValue{v.Target, v.Value}, temps, path)
	case *values.TernaryExpression:
		return nil, !constructorValueMentionsAnyTemp(value, temps)
	default:
		return nil, !constructorValueMentionsAnyTemp(value, temps)
	}
}

func orderedConstructorTempChildren(children []values.JavaValue, temps []*values.JavaRef) ([]int, bool) {
	return orderedConstructorTempChildrenPath(children, temps, make(map[values.JavaValue]bool))
}

func orderedConstructorTempChildrenPath(children []values.JavaValue, temps []*values.JavaRef, path map[values.JavaValue]bool) ([]int, bool) {
	var sequence []int
	for _, child := range children {
		childSequence, supported := delegatingConstructorTempOrderPath(child, temps, path)
		if !supported {
			return nil, false
		}
		sequence = append(sequence, childSequence...)
	}
	return sequence, true
}

func constructorValueMentionsAnyTemp(value values.JavaValue, temps []*values.JavaRef) bool {
	for _, temp := range temps {
		if temp == nil {
			return true
		}
		if valueMentionsLocal(value, temp) {
			return true
		}
	}
	return false
}

// delegatingConstructorTempUses counts references in eager source positions
// only. A short-circuit arm, ternary, or lambda capture can defer evaluation,
// so substituting the initializer there would change when allocation or an
// initializer expression runs. Unknown containers fail closed.
func delegatingConstructorTempUses(value values.JavaValue, temp *values.JavaRef) (int, bool) {
	value = values.UnpackSoltValue(value)
	if value == nil {
		return 0, true
	}
	switch v := value.(type) {
	case *values.JavaRef:
		if values.SameLocal(v, temp) {
			return 1, true
		}
		return 0, true
	case *values.ArrayLengthExpression:
		return delegatingConstructorTempUses(v.Array, temp)
	case *values.CastExpression:
		return delegatingConstructorTempUses(v.Value, temp)
	case *values.JavaExpression:
		if v.Op == "&&" || v.Op == "||" {
			if valueMentionsLocal(value, temp) {
				return 0, false
			}
			return 0, true
		}
		return countConstructorTempChildren(v.Values, temp)
	case *values.FunctionCallExpression:
		children := append([]values.JavaValue{v.Object}, v.Arguments...)
		return countConstructorTempChildren(children, temp)
	case *values.NewExpression:
		children := append([]values.JavaValue{}, v.Length...)
		children = append(children, v.Initializer...)
		if v.ConstructorCall != nil {
			children = append(children, v.ConstructorCall.Arguments...)
		} else if v.ArgumentsGetter != nil {
			return 0, !valueMentionsLocal(value, temp)
		}
		return countConstructorTempChildren(children, temp)
	case *values.JavaArrayMember:
		return countConstructorTempChildren([]values.JavaValue{v.Object, v.Index}, temp)
	case *values.RefMember:
		return delegatingConstructorTempUses(v.Object, temp)
	case *values.JavaCompare:
		return countConstructorTempChildren([]values.JavaValue{v.JavaValue1, v.JavaValue2}, temp)
	case *values.AssignmentExpression:
		if ref, ok := values.UnpackSoltValue(v.Target).(*values.JavaRef); ok && values.SameLocal(ref, temp) {
			// Replacing an assignment target with an array expression would turn
			// a legal local write into an invalid lvalue (and could change later
			// reads), so this shape is outside the single-read proof.
			return 0, false
		}
		return countConstructorTempChildren([]values.JavaValue{v.Target, v.Value}, temp)
	case *values.TernaryExpression:
		if valueMentionsLocal(value, temp) {
			return 0, false
		}
		return 0, true
	default:
		if valueMentionsLocal(value, temp) {
			return 0, false
		}
		return 0, true
	}
}

func countConstructorTempChildren(children []values.JavaValue, temp *values.JavaRef) (int, bool) {
	count := 0
	for _, child := range children {
		n, supported := delegatingConstructorTempUses(child, temp)
		if !supported {
			return 0, false
		}
		count += n
	}
	return count, true
}

// replaceDelegatingConstructorTemp mutates the already-proven single eager
// use. NewExpression's ArgumentsGetter renders ConstructorCall.Arguments, so
// updating that typed tree also updates the final source text.
func replaceDelegatingConstructorTemp(value values.JavaValue, temp *values.JavaRef, replacement values.JavaValue) values.JavaValue {
	value = values.UnpackSoltValue(value)
	switch v := value.(type) {
	case *values.JavaRef:
		if values.SameLocal(v, temp) {
			return replacement
		}
	case *values.ArrayLengthExpression:
		v.Array = replaceDelegatingConstructorTemp(v.Array, temp, replacement)
	case *values.CastExpression:
		v.Value = replaceDelegatingConstructorTemp(v.Value, temp, replacement)
	case *values.JavaExpression:
		for i := range v.Values {
			v.Values[i] = replaceDelegatingConstructorTemp(v.Values[i], temp, replacement)
		}
	case *values.FunctionCallExpression:
		v.Object = replaceDelegatingConstructorTemp(v.Object, temp, replacement)
		for i := range v.Arguments {
			v.Arguments[i] = replaceDelegatingConstructorTemp(v.Arguments[i], temp, replacement)
		}
	case *values.NewExpression:
		for i := range v.Length {
			v.Length[i] = replaceDelegatingConstructorTemp(v.Length[i], temp, replacement)
		}
		for i := range v.Initializer {
			v.Initializer[i] = replaceDelegatingConstructorTemp(v.Initializer[i], temp, replacement)
		}
		if v.ConstructorCall != nil {
			for i := range v.ConstructorCall.Arguments {
				v.ConstructorCall.Arguments[i] = replaceDelegatingConstructorTemp(v.ConstructorCall.Arguments[i], temp, replacement)
			}
		}
	case *values.JavaArrayMember:
		v.Object = replaceDelegatingConstructorTemp(v.Object, temp, replacement)
		v.Index = replaceDelegatingConstructorTemp(v.Index, temp, replacement)
	case *values.RefMember:
		v.Object = replaceDelegatingConstructorTemp(v.Object, temp, replacement)
	case *values.JavaCompare:
		v.JavaValue1 = replaceDelegatingConstructorTemp(v.JavaValue1, temp, replacement)
		v.JavaValue2 = replaceDelegatingConstructorTemp(v.JavaValue2, temp, replacement)
	case *values.AssignmentExpression:
		v.Target = replaceDelegatingConstructorTemp(v.Target, temp, replacement)
		v.Value = replaceDelegatingConstructorTemp(v.Value, temp, replacement)
	}
	return value
}

func constructorInlineNodeValues(statement statements.Statement) ([]values.JavaValue, bool) {
	switch s := statement.(type) {
	case *statements.AssignStatement:
		out := []values.JavaValue{s.LeftValue, s.JavaValue}
		if s.ArrayMember != nil {
			out = append(out, s.ArrayMember)
		}
		return out, true
	case *statements.ExpressionStatement:
		return []values.JavaValue{s.Expression}, true
	case *statements.ConditionStatement:
		return []values.JavaValue{s.Condition}, true
	case *statements.ReturnStatement:
		return []values.JavaValue{s.JavaValue}, true
	case *statements.StackAssignStatement:
		return []values.JavaValue{s.JavaValue}, true
	case *statements.GOTOStatement:
		return nil, true
	case *statements.MiddleStatement:
		if value, ok := s.Data.(values.JavaValue); ok {
			return []values.JavaValue{value}, true
		}
		return nil, s.Data == nil
	default:
		return nil, false
	}
}

func valueMentionsLocal(value values.JavaValue, target *values.JavaRef) bool {
	seen := map[values.JavaValue]bool{}
	var visit func(values.JavaValue) bool
	visit = func(current values.JavaValue) bool {
		if current == nil || seen[current] {
			return false
		}
		seen[current] = true
		if ref, ok := current.(*values.JavaRef); ok && values.SameLocal(ref, target) {
			return true
		}
		children, known := values.Children(current)
		if !known {
			return true
		}
		for _, child := range children {
			if visit(child) {
				return true
			}
		}
		return false
	}
	return visit(value)
}

func containsNode(nodes []*Node, target *Node) bool {
	for _, node := range nodes {
		if node == target {
			return true
		}
	}
	return false
}
