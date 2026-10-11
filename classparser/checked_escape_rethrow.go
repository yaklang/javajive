package javaclassparser

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/rewriter"
)

// Java precise rethrow propagates the protected operations' checked domain.
// A broad catch parameter is not an independently produced Throwable. This
// source-only proof removes its redundant broad classification; the original
// invoke declarations and explicit payload ATHROWs are still checked normally.
func (c *ClassObjectDumper) preciseCatchRethrows(body []statements.Statement, code *CodeAttribute) map[int]bool {
	result := map[int]bool{}
	remaining := 8192
	active := map[statements.Statement]bool{}
	type binding struct {
		ref   *values.JavaRef
		entry int
	}
	withoutShadow := func(scope []binding, ref *values.JavaRef) []binding {
		if ref == nil || ref.Id == nil {
			return nil
		}
		out := make([]binding, 0, len(scope))
		for _, b := range scope {
			if b.ref.Id != ref.Id {
				out = append(out, b)
			}
		}
		return out
	}
	var walk func([]statements.Statement, []binding) bool
	walk = func(list []statements.Statement, scope []binding) bool {
		for _, st := range list {
			remaining--
			if remaining < 0 || sourceProofNil(st) || active[st] {
				return false
			}
			active[st] = true
			if thrown, ok := st.(*statements.CustomStatement); ok && thrown.ThrownValue != nil && thrown.HasOriginPC {
				precise := false
				for i := len(scope) - 1; i >= 0; i-- {
					if directCatchOperand(thrown.ThrownValue, scope[i].ref, scope[i].entry) {
						precise = true
						break
					}
				}
				if previous, seen := result[thrown.OriginPC]; !seen {
					result[thrown.OriginPC] = precise
				} else {
					result[thrown.OriginPC] = previous && precise
				}
			}
			if tr, ok := st.(*statements.TryCatchStatement); ok {
				if view, proved := rewriter.RecoverCatchAllFinally(tr); proved {
					if !walk(view.TryBody, scope) || !walk(view.Cleanup, scope) {
						return false
					}
					for i, arm := range view.CatchBodies {
						inner := scope
						if i < len(view.Exceptions) {
							inner = withoutShadow(scope, view.Exceptions[i])
						}
						// The finally view retains original typed handlers in order.
						if i < len(tr.Handlers) && i < len(view.Exceptions) && c.originalCatchParameter(tr.Handlers[i], view.Exceptions[i], code) && catchParameterUnwritten(arm, view.Exceptions[i], &remaining) {
							inner = append(inner, binding{view.Exceptions[i], tr.Handlers[i].EntryPC})
						}
						if !walk(arm, inner) {
							return false
						}
					}
				} else {
					if !walk(tr.TryBody, scope) {
						return false
					}
					for i, arm := range tr.CatchBodies {
						inner := scope
						if i < len(tr.Exception) {
							inner = withoutShadow(scope, tr.Exception[i])
						}
						if i < len(tr.Handlers) && i < len(tr.Exception) && c.originalCatchParameter(tr.Handlers[i], tr.Exception[i], code) && catchParameterUnwritten(arm, tr.Exception[i], &remaining) {
							inner = append(inner, binding{tr.Exception[i], tr.Handlers[i].EntryPC})
						}
						if !walk(arm, inner) {
							return false
						}
					}
				}
			} else {
				_, children, known := catchSourceChildren(st)
				if !known {
					return false
				}
				for _, child := range children {
					if !walk(child, scope) {
						return false
					}
				}
			}
			delete(active, st)
		}
		return true
	}
	if c == nil || code == nil || !walk(body, nil) {
		return map[int]bool{}
	}
	return result
}

func (c *ClassObjectDumper) originalCatchParameter(h statements.CatchHandler, ref *values.JavaRef, code *CodeAttribute) bool {
	if ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil || h.EntryPC < 0 {
		return false
	}
	name, known := types.RawClassFQN(ref.Type())
	if !known {
		return false
	}
	name = strings.ReplaceAll(name, ".", "/")
	found := false
	for _, entry := range code.ExceptionTable {
		if entry == nil || int(entry.HandlerPc) != h.EntryPC {
			continue
		}
		expected := "java/lang/Throwable"
		if entry.CatchType != 0 {
			var valid bool
			expected, valid = sourceBridgeClassName(c.obj, entry.CatchType)
			if !valid {
				return false
			}
		}
		if expected != name || h.CatchAll != (entry.CatchType == 0) {
			return false
		}
		found = true
	}
	return found
}

// Slot wrappers are transparent source uses; simulator Ref.Val histories are
// not. A different local alias loses Java's precise catch-parameter semantics.
func directCatchOperand(value values.JavaValue, ref *values.JavaRef, entry int) bool {
	for depth := 0; depth < 32 && !sourceProofNil(value); depth++ {
		if slot, ok := value.(*values.SlotValue); ok {
			value = slot.GetValue()
			continue
		}
		switch v := value.(type) {
		case *values.JavaRef:
			return v.Id != nil && v.Id == ref.Id && v.CustomValue == nil && v.StackVar == nil && !v.IsThis
		case *values.CustomValue:
			return v.Flag == "exception" && v.HasOriginPC && v.OriginPC == entry
		}
		return false
	}
	return false
}

func sourceProofNil(node any) bool {
	return node == nil || reflect.ValueOf(node).Kind() == reflect.Ptr && reflect.ValueOf(node).IsNil()
}

// Enumerate source children, including loop headers and switch discriminants.
// Opaque closures cannot establish that a catch variable is effectively final.
func catchSourceChildren(st statements.Statement) ([]values.JavaValue, [][]statements.Statement, bool) {
	if sourceProofNil(st) {
		return nil, nil, false
	}
	switch x := st.(type) {
	case *nativeAssertStatement:
		condition, call, _, known := x.SourceAssertionProtocol()
		if !known {
			return nil, nil, false
		}
		return []values.JavaValue{condition, call}, nil, true
	case *statements.AssignStatement:
		// An element store has a distinct lvalue packet. Its absent local
		// LeftValue is structural, not an unknown operand. Visit the actual
		// array/index and RHS so hidden writes still invalidate stability.
		if x.ArrayMember != nil {
			if x.IsDeclare || x.LeftValue != nil || sourceProofNil(x.ArrayMember.Object) || sourceProofNil(x.ArrayMember.Index) || sourceProofNil(x.JavaValue) {
				return nil, nil, false
			}
			return []values.JavaValue{x.ArrayMember, x.JavaValue}, nil, true
		}
		if sourceProofNil(x.LeftValue) {
			return nil, nil, false
		}
		roots := []values.JavaValue{x.LeftValue}
		if x.JavaValue != nil {
			roots = append(roots, x.JavaValue)
		}
		return roots, nil, true
	case *statements.ExpressionStatement:
		return []values.JavaValue{x.Expression}, nil, true
	case *values.JavaExpression:
		return []values.JavaValue{x}, nil, true
	case *statements.ConditionStatement:
		return []values.JavaValue{x.Condition}, nil, true
	case *statements.CustomStatement:
		if x.ThrownValue != nil {
			return []values.JavaValue{x.ThrownValue}, nil, true
		}
		return nil, nil, x.Name == "break" || x.Name == "continue" || x.LoopTransferKind == "break" || x.LoopTransferKind == "continue"
	case *statements.ReturnStatement:
		if x.JavaValue == nil {
			return nil, nil, true
		}
		return []values.JavaValue{x.JavaValue}, nil, true
	case *statements.MiddleStatement:
		return nil, nil, x.Data == nil && (x.Flag == "start" || x.Flag == "end")
	case *statements.IfStatement:
		return []values.JavaValue{x.Condition}, [][]statements.Statement{x.IfBody, x.ElseBody}, true
	case *statements.TryCatchStatement:
		return nil, append([][]statements.Statement{x.TryBody}, x.CatchBodies...), true
	case *statements.WhileStatement:
		return []values.JavaValue{x.ConditionValue}, [][]statements.Statement{x.Body}, true
	case *statements.DoWhileStatement:
		return []values.JavaValue{x.ConditionValue}, [][]statements.Statement{x.Body}, true
	case *statements.ForStatement:
		children := [][]statements.Statement{x.SubStatements}
		for _, header := range []statements.Statement{x.InitVar, x.Condition, x.EndExp} {
			if !sourceProofNil(header) {
				children = append(children, []statements.Statement{header})
			}
		}
		return nil, children, true
	case *statements.SynchronizedStatement:
		return []values.JavaValue{x.Argument}, [][]statements.Statement{x.Body}, true
	case *statements.SwitchStatement:
		var children [][]statements.Statement
		for _, arm := range x.Cases {
			if arm == nil {
				return nil, nil, false
			}
			children = append(children, arm.Body)
		}
		return []values.JavaValue{x.Value}, children, true
	}
	return nil, nil, false
}

func catchParameterUnwritten(body []statements.Statement, ref *values.JavaRef, remaining *int, declaration ...*statements.AssignStatement) bool {
	return sourceParameterUnwritten(body, ref, remaining, catchSourceChildren, declaration...)
}

// Stable capture binding asks whether a source declaration is overwritten, not
// whether an original instruction can move or enter a protected region. Sealed
// operand-free source leaves participate only in this name/identity traversal;
// precise rethrow and control proofs retain their stricter statement visitor.
func nativeCaptureParameterUnwritten(body []statements.Statement, ref *values.JavaRef, remaining *int, declaration ...*statements.AssignStatement) bool {
	return sourceParameterUnwritten(body, ref, remaining, nativeSourceNameChildren, declaration...)
}
func sourceParameterUnwritten(body []statements.Statement, ref *values.JavaRef, remaining *int, childSource func(statements.Statement) ([]values.JavaValue, [][]statements.Statement, bool), declaration ...*statements.AssignStatement) bool {
	if ref == nil || ref.Id == nil {
		return false
	}
	same := func(v values.JavaValue) bool {
		for i := 0; i < 32 && !sourceProofNil(v); i++ {
			if slot, ok := v.(*values.SlotValue); ok {
				v = slot.GetValue()
				continue
			}
			local, ok := v.(*values.JavaRef)
			return ok && (local.Id == nil || local.Id == ref.Id)
		}
		return true
	}
	activeValue := map[values.JavaValue]bool{}
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		*remaining--
		if *remaining < 0 || sourceProofNil(v) || activeValue[v] {
			return false
		}
		activeValue[v] = true
		defer delete(activeValue, v)
		switch x := v.(type) {
		case *values.AssignmentExpression:
			if same(x.Target) {
				return false
			}
		case *values.JavaExpression:
			if x.Op == "=" || strings.HasSuffix(string(x.Op), "=") && x.Op != "==" && x.Op != "!=" && x.Op != "<=" && x.Op != ">=" || x.Op == values.INC || x.Op == values.DEC {
				if len(x.Values) == 0 || same(x.Values[0]) {
					return false
				}
			}
		case *values.FunctionCallExpression:
			if !x.IsStatic && !value(x.Object) {
				return false
			}
			for _, arg := range x.Arguments {
				if !value(arg) {
					return false
				}
			}
			return true
		case *values.CustomValue:
			if x.Flag == "exception" && x.HasOriginPC {
				return true
			}
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child) {
				return false
			}
		}
		return true
	}
	active := map[statements.Statement]bool{}
	var walk func([]statements.Statement) bool
	walk = func(list []statements.Statement) bool {
		for _, st := range list {
			*remaining--
			if *remaining < 0 || sourceProofNil(st) || active[st] {
				return false
			}
			active[st] = true
			if assign, ok := st.(*statements.AssignStatement); ok && assign.ArrayMember == nil && same(assign.LeftValue) {
				allowed := len(declaration) == 1 && assign == declaration[0] && (assign.IsDeclare || assign.IsFirst)
				if !allowed {
					return false
				}
			}
			if tr, ok := st.(*statements.TryCatchStatement); ok {
				for _, caught := range tr.Exception {
					if caught == nil || caught.Id == nil || caught.Id == ref.Id {
						return false
					}
				}
			}
			roots, children, known := childSource(st)
			if !known {
				return false
			}
			for _, root := range roots {
				if !value(root) {
					return false
				}
			}
			for _, child := range children {
				if !walk(child) {
					return false
				}
			}
			delete(active, st)
		}
		return true
	}
	return walk(body)
}
