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
	for i, arg := range call.Arguments {
		if ref, isRef := values.UnpackSoltValue(arg).(*values.JavaRef); isRef && values.SameLocal(ref, temp) {
			if argIndex >= 0 {
				return skip("temporary occurs in multiple constructor arguments")
			}
			argIndex = i
			continue
		}
		if valueMentionsLocal(arg, temp) {
			return skip("temporary is nested in a non-direct constructor argument")
		}
	}
	if argIndex < 0 {
		return skip("constructor call does not directly consume the array temporary")
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

	call.Arguments[argIndex] = array

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
