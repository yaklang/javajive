package core

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// inlineDelegatingConstructorArrayTemp removes the synthetic local that can be
// emitted when javac lowers `this(new T[]{...})` through dup/array-store
// instructions. Java requires this()/super() to be the first constructor
// statement, so an otherwise-correct `T[] temp = ...; this(temp);` is invalid.
//
// The rewrite is intentionally limited to a straight-line entry sequence:
// the temp must be the first real statement, the next statement must be the
// delegation call, the temp must have no other uses, and exception-handler
// coverage must match. Substituting the array expression at its original
// argument position preserves Java's left-to-right evaluation order with the
// other constructor arguments.
func (d *Decompiler) inlineDelegatingConstructorArrayTemp(origins map[int]*OpCode) bool {
	skip := func(reason string, args ...any) bool {
		d.tracef("ctor-array-inline", "skip: %s", fmt.Sprintf(reason, args...))
		return false
	}
	if d == nil || d.RootNode == nil || d.FunctionContext == nil ||
		d.FunctionContext.FunctionName != "<init>" || d.getenv("JDEC_CTOR_ARRAY_ARG_INLINE_OFF") != "" {
		return false
	}

	entry := d.RootNode
	var entryPred *Node
	for {
		if _, ok := entry.Statement.(*statements.AssignStatement); ok {
			break
		}
		middle, ok := entry.Statement.(*statements.MiddleStatement)
		if !ok || middle.Flag != "start" || len(entry.Next) != 1 {
			return skip("constructor entry is not a single start-marker path")
		}
		entryPred, entry = entry, entry.Next[0]
		if entry == nil {
			return skip("start marker has no successor")
		}
	}
	if entryPred == nil {
		if entry != d.RootNode || len(entry.Source) != 0 {
			return skip("root assignment has an unexpected predecessor")
		}
	} else if len(entry.Source) != 1 || entry.Source[0] != entryPred {
		return skip("candidate is not uniquely reached from the constructor entry")
	}

	assign, ok := entry.Statement.(*statements.AssignStatement)
	if !ok || assign.ArrayMember != nil || len(entry.Next) != 1 {
		return skip("entry is not a single-successor local assignment: node=%d stmt=%T next=%d", entry.Id, entry.Statement, len(entry.Next))
	}
	temp, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	if !ok || temp == nil {
		return skip("assignment target is not a local ref: node=%d left=%T", entry.Id, assign.LeftValue)
	}
	array, ok := values.UnpackSoltValue(assign.JavaValue).(*values.NewExpression)
	if !ok || array == nil || array.JavaType == nil || array.JavaType.ArrayDim() == 0 || len(array.Initializer) == 0 {
		return skip("entry value is not an initialized array literal")
	}
	if valueMentionsLocal(assign.JavaValue, temp) {
		return skip("array initializer reads its own temporary")
	}

	callNode := entry.Next[0]
	if callNode == nil {
		return skip("array assignment has a nil successor")
	}
	reachable := map[*Node]bool{}
	_ = WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		reachable[node] = true
		return node.Next, nil
	})
	otherReachablePred := false
	for _, source := range callNode.Source {
		if reachable[source] && source != entry {
			otherReachablePred = true
		}
	}
	if otherReachablePred || !containsNode(callNode.Source, entry) {
		nextType := "<nil>"
		if len(entry.Next) > 0 && entry.Next[0] != nil {
			nextType = fmt.Sprintf("%T", entry.Next[0].Statement)
		}
		sources := []int{}
		if callNode != nil {
			for _, source := range callNode.Source {
				if source != nil {
					sources = append(sources, source.Id)
				}
			}
		}
		return skip("array assignment is not immediately followed by its only consumer: node=%d next=%d/%s target-sources=%v", entry.Id, len(entry.Next), nextType, sources)
	}
	expr, ok := callNode.Statement.(*statements.ExpressionStatement)
	if !ok {
		return skip("array consumer is not an expression statement")
	}
	call, ok := values.UnpackSoltValue(expr.Expression).(*values.FunctionCallExpression)
	if !ok || call == nil || !call.IsSpecialInvoke || call.FunctionName != "<init>" {
		return skip("array consumer is not a special constructor call")
	}
	receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
	if !ok || receiver == nil || !receiver.IsThis {
		return skip("constructor call receiver is not this")
	}
	entryOp, callOp := origins[entry.Id], origins[callNode.Id]
	if entryOp == nil || callOp == nil || !sameIntSlice(d.handlersAt(entryOp), d.handlersAt(callOp)) {
		return skip("origin or exception-handler proof is unavailable")
	}

	argIndex := -1
	tempUses := 0
	for i, arg := range call.Arguments {
		count, supported := delegatingConstructorTempUses(arg, temp)
		if !supported {
			return skip("temporary occurs through a conditional or unsupported expression")
		}
		if count > 0 {
			argIndex = i
			tempUses += count
		}
	}
	if argIndex < 0 || tempUses != 1 {
		return skip("constructor call must consume the array temporary exactly once (args=%d uses=%d)", len(call.Arguments), tempUses)
	}

	// Refuse the rewrite if any other statement reads or writes the temporary.
	// This catches aliases and later uses that a local adjacency check alone
	// would miss, while allowing the parameter references inside the initializer.
	usedElsewhere := false
	_ = WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if node != entry && node != callNode {
			if nodeValues, known := constructorInlineNodeValues(node.Statement); !known {
				usedElsewhere = true
				return nil, nil
			} else {
				for _, value := range nodeValues {
					if valueMentionsLocal(value, temp) {
						usedElsewhere = true
						return nil, nil
					}
				}
			}
		}
		return node.Next, nil
	})
	if usedElsewhere {
		return skip("temporary has another use or crosses an unsupported statement")
	}

	call.Arguments[argIndex] = replaceDelegatingConstructorTemp(call.Arguments[argIndex], temp, array)

	// The temp node is a straight-line entry node. Reconnect its sole predecessor
	// directly to the constructor call and keep the call's remaining graph intact.
	// MiscRewriter can leave stale Source links from removed array-fill nodes; they
	// are unreachable from the method root, so discard those backlinks before the
	// constructor call participates in later predecessor-sensitive rewrites.
	for _, source := range append([]*Node(nil), callNode.Source...) {
		if !reachable[source] {
			source.RemoveNext(callNode)
		}
	}
	predecessors := append([]*Node(nil), entry.Source...)
	for _, predecessor := range predecessors {
		predecessor.ReplaceNext(entry, callNode)
	}
	for i, source := range callNode.Source {
		if source == entry {
			callNode.Source = append(callNode.Source[:i], callNode.Source[i+1:]...)
			break
		}
	}
	for _, predecessor := range predecessors {
		if !containsNode(callNode.Source, predecessor) {
			callNode.Source = append(callNode.Source, predecessor)
		}
	}
	entry.Source = nil
	entry.Next = nil
	if entryPred == nil {
		d.RootNode = callNode
	}
	if temp.Id != nil {
		temp.Id.Delete()
	}
	return true
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
