package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestCommonLoopExitUsesEveryPath(t *testing.T) {
	for _, scenario := range []string{"shared", "third terminal", "bypass", "hidden break", "method end"} {
		t.Run(scenario, func(t *testing.T) {
			newNode := func() *core.Node { return core.NewNode(&statements.MiddleStatement{}) }
			a, b, c, merge, tail := newNode(), newNode(), newNode(), newNode(), newNode()
			a.AddNext(merge)
			b.AddNext(merge)
			c.AddNext(merge)
			merge.AddNext(tail)
			want := merge
			switch scenario {
			case "third terminal":
				c.RemoveNext(merge)
				want = nil
			case "bypass":
				// Reachability alone includes merge, but this path skips it.
				c.AddNext(tail)
				want = tail
			case "hidden break":
				c.RemoveNext(merge)
				c.HideNext = merge
			case "method end":
				merge.Statement = &statements.MiddleStatement{Flag: "end"}
				want = nil
			}
			for _, exits := range [][]*core.Node{{a, b, c}, {c, a, b}, {b, c, a}} {
				if got := commonLoopExit(exits); got != want {
					t.Fatalf("common exit=%p want=%p", got, want)
				}
			}
		})
	}
}

func TestCommonLoopExitAllowsOnlyTerminalBypasses(t *testing.T) {
	for _, bypass := range []string{"return", "unknown sink", "cycle"} {
		t.Run(bypass, func(t *testing.T) {
			entry := core.NewNode(&statements.ConditionStatement{})
			other := core.NewNode(&statements.ConditionStatement{})
			merge := core.NewNode(&statements.MiddleStatement{})
			terminal := core.NewNode(&statements.ReturnStatement{})
			alternative := core.NewNode(&statements.ReturnStatement{})
			entry.AddNext(merge)
			other.AddNext(merge)
			other.AddNext(alternative)
			merge.AddNext(terminal)
			if bypass != "return" {
				alternative.Statement = &statements.MiddleStatement{}
			}
			if bypass == "cycle" {
				alternative.AddNext(alternative)
			}
			want := merge
			if bypass != "return" {
				want = nil
			}
			if got := commonLoopExit([]*core.Node{entry, other}); got != want {
				t.Fatalf("common exit=%p want=%p", got, want)
			}
		})
	}
}

func TestTerminalLoopArmRetainsEffectsAndAlternativeReturns(t *testing.T) {
	for _, scenario := range []string{"return", "diamond", "shared entry", "shared tail", "cycle", "unknown sink"} {
		t.Run(scenario, func(t *testing.T) {
			newNode := func() *core.Node { return core.NewNode(&statements.MiddleStatement{}) }
			entry, effect, other := newNode(), newNode(), newNode()
			terminal := core.NewNode(&statements.ReturnStatement{})
			entry.AddNext(effect)
			effect.AddNext(terminal)
			want := true
			switch scenario {
			case "diamond":
				entry.AddNext(other)
				other.AddNext(terminal)
			case "shared entry":
				newNode().AddNext(entry)
				newNode().AddNext(entry)
				want = false
			case "shared tail":
				other.AddNext(effect)
				want = false
			case "cycle":
				effect.AddNext(entry)
				want = false
			case "unknown sink":
				effect.AddNext(other)
				want = false
			}
			if got := exclusiveTerminalBranch(entry); got != want {
				t.Fatalf("exclusive terminal arm=%v want=%v", got, want)
			}
		})
	}
}

func TestStructuredTryBreakIsNotALoopContinuation(t *testing.T) {
	for _, abrupt := range []bool{true, false} {
		loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		header := core.NewNode(&statements.MiddleStatement{})
		condition := core.NewNode(&statements.ConditionStatement{})
		branch := core.NewNode(&statements.ConditionStatement{})
		terminal := core.NewNode(&statements.ReturnStatement{})
		body := []statements.Statement{statements.NewCustomStatement(func(*class_context.ClassContext) string { return "break" }, nil)}
		if !abrupt {
			body = nil
		}
		tr := core.NewNode(statements.NewTryCatchStatement(body, [][]statements.Statement{{&statements.ReturnStatement{}}}))
		loop.AddNext(header)
		header.AddNext(condition)
		condition.AddNext(terminal)
		condition.AddNext(branch)
		branch.AddNext(tr)
		branch.AddNext(loop)
		want := tr
		if abrupt {
			want = nil
		}
		if got := searchCircleEndNode(loop, header, GenerateDominatorTree(loop), true); got != want {
			t.Fatalf("abrupt=%v continuation=%p want=%p", abrupt, got, want)
		}
	}
}

func TestTerminalHeaderNeedsAnEnclosingContinuationWitness(t *testing.T) {
	for _, enclosing := range []bool{false, true} {
		outer := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		header := core.NewNode(&statements.ConditionStatement{})
		compare := core.NewNode(&statements.ConditionStatement{})
		found := core.NewNode(&statements.ReturnStatement{})
		step := core.NewNode(&statements.MiddleStatement{})
		// Preserve bytecode order used to distinguish a forward pre-header
		// entry from the inner loop's back edge.
		outer.Id, header.Id, compare.Id, step.Id = 1, 10, 11, 12
		root := loop
		if enclosing {
			outer.AddNext(loop)
			step.AddNext(outer)
			root = outer
		} else {
			step.AddNext(core.NewNode(&statements.ReturnStatement{}))
		}
		loop.AddNext(header)
		header.AddNext(found)
		header.AddNext(compare)
		compare.AddNext(loop)
		compare.AddNext(step)
		want := found
		if enclosing {
			want = step
		}
		if got := searchCircleEndNode(loop, header, GenerateDominatorTree(root), true); got != want {
			t.Fatalf("enclosing=%v continuation=%p want=%p", enclosing, got, want)
		}
	}
}

func TestLoopTerminalRegionUsesLiveEdgesAndRespectsAncestorTargets(t *testing.T) {
	for _, scenario := range []string{"closed nested loop", "detached backlink", "resume ancestor", "resume owner", "encoded ancestor", "foreign live entry", "unknown cycle", "unknown sink"} {
		t.Run(scenario, func(t *testing.T) {
			outer := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			owner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			decision := core.NewNode(&statements.ConditionStatement{})
			entry := core.NewNode(&statements.AssignStatement{})
			nested := core.NewNode(statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil))
			condition := core.NewNode(&statements.ConditionStatement{})
			step := core.NewNode(&statements.ExpressionStatement{})
			ret := core.NewNode(&statements.ReturnStatement{})
			outer.AddNext(owner)
			owner.AddNext(decision)
			decision.AddNext(owner)
			decision.AddNext(entry)
			entry.AddNext(nested)
			nested.AddNext(condition)
			condition.AddNext(step)
			condition.AddNext(ret)
			step.AddNext(nested)
			switch scenario {
			case "detached backlink":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(nested)
			case "resume ancestor":
				step.AddNext(outer)
			case "resume owner":
				step.AddNext(owner)
			case "encoded ancestor":
				step.Statement = &statements.CustomStatement{StringFunc: func(*class_context.ClassContext) string { return "continue outer" }}
				step.AddNext(outer)
				step.EncodedJumps = map[*core.Node]bool{outer: true}
			case "foreign live entry":
				outer.AddNext(step)
			case "unknown cycle":
				nested.Statement = &statements.MiddleStatement{}
			case "unknown sink":
				step.AddNext(core.NewNode(&statements.ExpressionStatement{}))
			}
			got := exclusiveTerminalLoopBranch(entry, owner, GenerateDominatorTree(outer))
			want := scenario == "closed nested loop" || scenario == "detached backlink"
			if got != want {
				t.Fatalf("closed=%v want=%v", got, want)
			}
		})
	}
}

func TestTerminalHeaderGuardKeepsSharedNormalExit(t *testing.T) {
	root := core.NewNode(&statements.ConditionStatement{})
	loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	header := core.NewNode(&statements.ConditionStatement{})
	check := core.NewNode(&statements.ConditionStatement{})
	guard := core.NewNode(&statements.ExpressionStatement{})
	thrown := core.NewNode(statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, nil))
	shared := core.NewNode(&statements.ExpressionStatement{})
	ret := core.NewNode(&statements.ReturnStatement{})
	root.Id, header.Id, check.Id, guard.Id, thrown.Id, shared.Id = 1, 10, 11, 12, 13, 14
	root.AddNext(loop)
	root.AddNext(shared)
	loop.AddNext(header)
	header.AddNext(guard)
	header.AddNext(check)
	guard.AddNext(thrown)
	check.AddNext(loop)
	check.AddNext(shared)
	shared.AddNext(ret)
	if got := searchCircleEndNode(loop, header, GenerateDominatorTree(root), true); got != shared {
		t.Fatalf("normal exit=%p want=%p", got, shared)
	}
}

func TestLoopNormalExitRejectsSameOwnerReentry(t *testing.T) {
	for _, kind := range []string{"normal", "direct", "encoded", "wrapped", "unknown sink"} {
		t.Run(kind, func(t *testing.T) {
			owner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			entry := core.NewNode(&statements.ExpressionStatement{})
			tail := core.NewNode(&statements.ReturnStatement{})
			entry.AddNext(tail)
			switch kind {
			case "direct":
				entry.AddNext(owner)
			case "encoded":
				entry.EncodedJumps = map[*core.Node]bool{owner: true}
			case "wrapped":
				tail.Statement = &statements.IfStatement{}
				tail.EncodedJumps = map[*core.Node]bool{owner: true}
			case "unknown sink":
				tail.Statement = &statements.ExpressionStatement{}
			}
			if got := loopExitCannotResumeOwner(entry, owner); got != (kind == "normal") {
				t.Fatalf("proved normal=%v", got)
			}
		})
	}
}

func TestThrowingLoopGuardProof(t *testing.T) {
	for _, kind := range []string{"throw", "shared throws", "return", "mixed", "cycle", "unknown sink"} {
		t.Run(kind, func(t *testing.T) {
			entry := core.NewNode(&statements.ConditionStatement{})
			thrown := core.NewNode(statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, nil))
			left := core.NewNode(&statements.ExpressionStatement{})
			right := core.NewNode(&statements.ExpressionStatement{})
			entry.AddNext(left)
			entry.AddNext(right)
			left.AddNext(thrown)
			right.AddNext(thrown)
			switch kind {
			case "return":
				thrown.Statement = &statements.ReturnStatement{}
			case "mixed":
				right.RemoveNext(thrown)
				right.AddNext(core.NewNode(&statements.ReturnStatement{}))
			case "cycle":
				right.AddNext(entry)
			case "unknown sink":
				right.RemoveNext(thrown)
			case "throw":
				entry = thrown
			}
			if got := terminalRegionOnlyThrows(entry); got != (kind == "throw" || kind == "shared throws") {
				t.Fatalf("throw-only=%v", got)
			}
		})
	}
}
