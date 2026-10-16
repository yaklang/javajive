package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A normal completion can continue with an original throw after finally. Its
// allocation/call/operand effects must stay after cleanup: evaluating it in the
// protected catch would change cleanup precedence and initialization effects.
// Match the complete conditional cleanup, retain the sealed original throw,
// and refuse references without a method-wide declaration witness.
// The caller separately requires every fall-through path to share this exact
// continuation; returns and protected throws keep their own exit semantics.
func finallyConditionalThrowContinuation(candidate, cleanup statements.Statement, rows []core.HandlerRange, excluded []*values.JavaRef) (statements.Statement, *statements.CustomStatement) {
	x, ok := candidate.(*statements.IfStatement)
	if !ok || x == nil {
		return candidate, nil
	}
	y, ok := cleanup.(*statements.IfStatement)
	if !ok || y == nil {
		return candidate, nil
	}
	for _, first := range []bool{true, false} {
		arm, witness := finallyWithoutEnd(x.IfBody), finallyWithoutEnd(y.IfBody)
		if !first {
			arm, witness = finallyWithoutEnd(x.ElseBody), finallyWithoutEnd(y.ElseBody)
		}
		if len(arm) != len(witness)+1 {
			continue
		}
		tail, ok := arm[len(arm)-1].(*statements.CustomStatement)
		if !ok || tail == nil || !tail.HasOriginPC || tail.OriginPC < 0 {
			continue
		}
		operand, sealed := tail.SourceThrowOperand()
		if !sealed || operand != tail.ThrownValue || !sameFinallyCleanup(rows, tail, tail, excluded, 0) || !finallyContinuationBindingsVisible(operand) {
			continue
		}
		clone := *x
		if first {
			clone.IfBody = arm[:len(arm)-1]
		} else {
			clone.ElseBody = arm[:len(arm)-1]
		}
		if sameFinallyCleanup(rows, &clone, y, excluded, 0) {
			return &clone, tail
		}
	}
	return candidate, nil
}

// Parameters and THIS remain in scope after the reconstructed region. An
// ordinary local needs a separate enclosing-declaration certificate, even when
// its spelling or its absence from a shallow declaration list looks plausible.
// Inspect dependencies without following a reference into its defining value.
func finallyContinuationBindingsVisible(root values.JavaValue) bool {
	remaining := 256
	active := map[values.JavaValue]bool{}
	var visit func(values.JavaValue, int) bool
	visit = func(v values.JavaValue, depth int) bool {
		remaining--
		if remaining < 0 || depth > 24 || active[v] {
			return false
		}
		if v == nil {
			return true // An absent static qualifier has no source binding.
		}
		if ref, ok := v.(*values.JavaRef); ok {
			return ref != nil && ref.Id != nil && ref.CustomValue == nil && ref.StackVar == nil && (ref.IsParam || ref.IsThis)
		}
		active[v] = true
		defer delete(active, v)
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child, depth+1) {
				return false
			}
		}
		return true
	}
	return visit(root, 0)
}
