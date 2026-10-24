package rewriter

import (
	"reflect"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Structuring can put the only normal continuation inside the other arm of
// an early return. It belongs after finally when every alternative exits the
// method, every normal prefix has its original cleanup, and every continuation
// effect/handler is outside the original protected domain. Keep the selector
// in the try, evaluated once; never turn a conditional tail into an extra path.
func factorFinallyConditionalContinuation(tr *statements.TryCatchStatement, body []statements.Statement, ranges [][2]int) []statements.Statement {
	branch, known := body[len(body)-1].(*statements.IfStatement)
	if !known || branch == nil {
		return nil
	}
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(ranges, pc) }
	for _, first := range []bool{true, false} {
		arm, other := finallyWithoutEnd(branch.IfBody), branch.ElseBody
		if !first {
			arm, other = finallyWithoutEnd(branch.ElseBody), branch.IfBody
		}
		if len(arm) < 2 || len(arm) > 128 || !finallyMethodExit(other, 0) {
			continue
		}
		for split := 1; split < len(arm); split++ {
			suffix := arm[split:]
			proof := outsideHandlerDomainProof(ranges)
			if !proof.block(suffix, outside, 0) {
				continue
			}
			copy := *branch
			if first {
				copy.IfBody = slices.Clone(arm[:split])
			} else {
				copy.ElseBody = slices.Clone(arm[:split])
			}
			prefix := *tr
			prefix.TryBody = append(slices.Clone(body[:len(body)-1]), &copy)
			view, complete := recoverFinallyDirectSource(&prefix)
			if !complete || view == nil || len(view.Cleanup) == 0 || view.Continuation != nil {
				continue
			}
			// A retained typed handler is another possible normal completion.
			// It must not acquire this arm's continuation on falling through.
			allExit := true
			for _, handler := range view.CatchBodies {
				allExit = allExit && finallyMethodExit(handler, 0)
			}
			if !allExit || !finallyContinuationKeepsLexicalBindings([]statements.Statement{&prefix}, suffix) {
				continue
			}
			tr.TryBody = prefix.TryBody
			return slices.Clone(suffix)
		}
	}
	return nil
}

// Only method exits establish exclusivity here. A loop transfer, an empty
// arm, unknown control or a conditionally falling-through arm does not.
func finallyMethodExit(input []statements.Statement, depth int) bool {
	remaining := 256
	return finallyMethodExitProof(input, depth, &remaining)
}

func finallyMethodExitProof(input []statements.Statement, depth int, remaining *int) bool {
	*remaining--
	if *remaining < 0 || depth > 16 {
		return false
	}
	body := finallyWithoutEnd(input)
	if len(body) == 0 || len(body) > 128 {
		return false
	}
	switch st := body[len(body)-1].(type) {
	case *statements.ReturnStatement:
		return st != nil && st.HasOriginPC && (st.JavaValue == nil || finallyPureLocal(st.JavaValue))
	case *statements.CustomStatement:
		return st != nil && st.HasOriginPC && st.ThrownValue != nil
	case *statements.IfStatement:
		return st != nil && finallyMethodExitProof(st.IfBody, depth+1, remaining) && finallyMethodExitProof(st.ElseBody, depth+1, remaining)
	}
	return false
}

// Moving a continuation outside the try must not escape a declaration made
// inside it. Inspect typed uses and binder identities, never rendered names or
// ref.Val histories. The original outside locals remain in their existing scope.
func finallyContinuationKeepsLexicalBindings(prefix, suffix []statements.Statement) bool {
	bindings := map[*utils.VariableId]int{}
	queue := slices.Clone(prefix)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 {
		if len(seen) >= 256 || len(queue) > 512 {
			return false
		}
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || reflect.ValueOf(st).Kind() == reflect.Ptr && reflect.ValueOf(st).IsNil() {
			return false
		}
		if seen[st] {
			continue
		}
		seen[st] = true
		switch x := st.(type) {
		case *statements.AssignStatement:
			if x != nil && (x.IsDeclare || x.IsFirst) {
				if ref, local := plainTryValue(x.LeftValue).(*values.JavaRef); local && ref != nil && ref.Id != nil {
					bindings[ref.Id] = 1
				}
			}
		case *statements.IfStatement:
			if x == nil {
				return false
			}
			queue = append(queue, x.IfBody...)
			queue = append(queue, x.ElseBody...)
		case *statements.DoWhileStatement:
			queue = append(queue, x.Body...)
		case *statements.SynchronizedStatement:
			queue = append(queue, x.Body...)
		case *statements.TryCatchStatement:
			queue = append(queue, x.TryBody...)
			for _, arm := range x.CatchBodies {
				queue = append(queue, arm...)
			}
			for _, ref := range x.Exception {
				if ref != nil && ref.Id != nil {
					bindings[ref.Id] = 1
				}
			}
		case *statements.SwitchStatement:
			for _, arm := range x.Cases {
				if arm == nil {
					return false
				}
				queue = append(queue, arm.Body...)
			}
		}
	}
	return finallyCleanupBindingsScoped(suffix, nil, bindings)
}
