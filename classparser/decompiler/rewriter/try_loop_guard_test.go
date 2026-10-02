package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/internal/utils"
)

func TestLoopHeaderGuardsTryRequiresExitOnOtherArm(t *testing.T) {
	for _, scenario := range []string{"guarded retry", "other body arm", "not header body", "not condition"} {
		t.Run(scenario, func(t *testing.T) {
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			header := core.NewNode(&statements.ConditionStatement{})
			tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			success, handler, step := core.NewNode(&statements.MiddleStatement{}), core.NewNode(&statements.MiddleStatement{}), core.NewNode(&statements.MiddleStatement{})
			exit := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			tr.ProtectedEnd = exit
			loop.AddNext(header)
			header.AddNext(tr)
			header.AddNext(exit)
			tr.AddNext(success)
			tr.AddNext(handler)
			success.AddNext(exit)
			handler.AddNext(step)
			step.AddNext(loop)
			switch scenario {
			case "other body arm":
				header.ReplaceNext(exit, step)
			case "not header body":
				header.ReplaceNext(tr, success)
			case "not condition":
				header.Statement = &statements.MiddleStatement{}
			}
			manager := NewRootStatementManager(loop)
			manager.LoopRegionReducible = true
			manager.DominatorMap = GenerateDominatorTree(loop)
			if got := loopHeaderGuardsTry(manager, loop, tr); got != (scenario == "guarded retry") {
				t.Fatalf("guarded retry=%v", got)
			}
		})
	}
}

// A switch in the retry catch has terminal early-return arms. The successful
// return is nevertheless the normal loop exit: its expression may call a
// downstream consumer outside the predicate's exception-table interval.
func TestRetryProtectedSuccessIsNormalTerminalExit(t *testing.T) {
	loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
	store := core.NewNode(&statements.AssignStatement{})
	end := core.NewNode(&statements.ReturnStatement{})
	handler := core.NewNode(&statements.MiddleStatement{})
	decision := core.NewNode(&statements.ConditionStatement{})
	skip := core.NewNode(&statements.ReturnStatement{})
	tr.HasProtectedRange, tr.ProtectedStartPC, tr.ProtectedEndPC = true, 10, 20
	// Boolean reduction deleted this old boundary. It is deliberately not
	// connected; choosing its ID or pointer cannot establish the new exit.
	tr.ProtectedEnd = core.NewNode(&statements.ConditionStatement{})
	store.HasOriginPC, store.OriginPC = true, 15
	end.HasOriginPC, end.OriginPC = true, 25
	handler.IsCatchStart = true
	loop.AddNext(tr)
	tr.AddNext(store)
	tr.AddNext(handler)
	store.AddNext(end)
	handler.AddNext(decision)
	decision.AddNext(loop)
	decision.AddNext(skip)
	dominators := GenerateDominatorTree(loop)
	if got := searchCircleEndNode(loop, tr, dominators, true); got != end {
		t.Fatal("successful outside return was confused with a catch's early return")
	}
	body := circleElementSet(loop, tr, dominators, true)
	if !body.Has(store) || body.Has(end) || body.Has(skip) {
		t.Fatal("protected store and unprotected returns have incorrect loop ownership")
	}
}

func TestRetryProtectedPrefixRequiresOriginalPrivatePath(t *testing.T) {
	cases := []string{"private", "shared cleanup", "missing range", "empty range", "negative start",
		"unknown store", "unknown boundary", "store before start", "boundary inside range", "backward PC",
		"alternate store entry", "multiple body entries", "branching store", "catch on success", "synthetic jump",
		"opaque statement", "cycle", "budget", "missing retry", "shared retry", "alternate retry entry",
		"incomplete body", "boundary inside body"}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			store := core.NewNode(&statements.AssignStatement{})
			end := core.NewNode(&statements.ReturnStatement{})
			handler := core.NewNode(&statements.MiddleStatement{})
			tr.HasProtectedRange, tr.ProtectedStartPC, tr.ProtectedEndPC = true, 10, 20
			store.HasOriginPC, store.OriginPC = true, 15
			end.HasOriginPC, end.OriginPC = true, 25
			handler.IsCatchStart = true
			tr.AddNext(store)
			tr.AddNext(handler)
			store.AddNext(end)
			body := utils2.NewSet[*core.Node]()
			for _, node := range []*core.Node{tr, store, handler} {
				body.Add(node)
			}
			switch scenario {
			case "shared cleanup":
				cleanup := core.NewNode(&statements.ReturnStatement{})
				cleanup.IsCatchStart, cleanup.SharedProtectedHandler = true, true
				tr.SharedProtectedHandler = true
				tr.AddNext(cleanup)
			case "missing range":
				tr.HasProtectedRange = false
			case "empty range":
				tr.ProtectedEndPC = tr.ProtectedStartPC
			case "negative start":
				tr.ProtectedStartPC = -1
			case "unknown store":
				store.HasOriginPC = false
			case "unknown boundary":
				end.HasOriginPC = false
			case "store before start":
				store.OriginPC = 9
			case "boundary inside range":
				end.OriginPC = 19
			case "backward PC":
				end.OriginPC = store.OriginPC
			case "alternate store entry":
				core.NewNode(&statements.MiddleStatement{}).AddNext(store)
			case "multiple body entries":
				tr.AddNext(core.NewNode(&statements.AssignStatement{}))
			case "branching store":
				store.AddNext(core.NewNode(&statements.ReturnStatement{}))
			case "catch on success":
				store.IsCatchStart = true
			case "synthetic jump":
				store.IsJmp = true
			case "opaque statement":
				store.Statement = &statements.MiddleStatement{}
			case "cycle":
				store.ReplaceNext(end, store)
			case "budget":
				tr.ProtectedEndPC = 100
				last := store
				last.RemoveNext(end)
				for pc := 16; pc < 48; pc++ {
					next := core.NewNode(&statements.AssignStatement{})
					next.HasOriginPC, next.OriginPC = true, pc
					last.AddNext(next)
					body.Add(next)
					last = next
				}
				end.OriginPC = 101
				last.AddNext(end)
			case "missing retry":
				body.Remove(handler)
			case "shared retry":
				tr.SharedProtectedHandler, handler.SharedProtectedHandler = true, true
			case "alternate retry entry":
				core.NewNode(&statements.MiddleStatement{}).AddNext(handler)
			case "incomplete body":
				body.Remove(store)
			case "boundary inside body":
				body.Add(end)
			}
			got := protectedRetryContinuations(body, loop)
			want := scenario == "private" || scenario == "shared cleanup"
			if got[end] != want || (want && len(got) != 1) || (!want && len(got) != 0) {
				t.Fatalf("normal exits=%v, want private proof=%v", got, want)
			}
		})
	}
}

func TestRetryProtectedPathDistinguishesSharedCleanup(t *testing.T) {
	for _, sharedRetry := range []bool{false, true} {
		loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
		call := core.NewNode(&statements.MiddleStatement{})
		end := core.NewNode(&statements.ReturnStatement{})
		retry := core.NewNode(&statements.MiddleStatement{})
		cleanup := core.NewNode(&statements.ReturnStatement{})
		tr.ProtectedEnd, tr.SharedProtectedHandler = end, true
		retry.IsCatchStart, retry.SharedProtectedHandler = true, sharedRetry
		cleanup.IsCatchStart, cleanup.SharedProtectedHandler = true, true
		loop.AddNext(tr)
		tr.AddNext(call)
		tr.AddNext(retry)
		tr.AddNext(cleanup)
		call.AddNext(end)
		retry.AddNext(loop)
		set := circleElementSet(loop, tr, GenerateDominatorTree(loop), true)
		if set.Has(call) == sharedRetry || set.Has(end) {
			t.Fatalf("sharedRetry=%t: incorrect protected-path boundary", sharedRetry)
		}
	}
}

func TestProtectedRetryBranchProofRequiresCompletePrivateIntervalGraph(t *testing.T) {
	for _, kind := range []string{"private", "split", "foreign entry", "unknown PC", "unprotected effect", "wrong handler", "overlapping ranges", "back edge", "distinct exits", "opaque statement", "missing successor"} {
		t.Run(kind, func(t *testing.T) {
			tr := core.NewNode(&statements.MiddleStatement{})
			tr.HasProtectedRange = true
			tr.ProtectedStartPC = 10
			tr.ProtectedEndPC = 40
			condition := core.NewNode(&statements.ConditionStatement{})
			left := core.NewNode(&statements.ExpressionStatement{})
			right := core.NewNode(&statements.ExpressionStatement{})
			exit := core.NewNode(&statements.ReturnStatement{})
			for n, pc := range map[*core.Node]int{condition: 15, left: 20, right: 30, exit: 45} {
				n.HasOriginPC = true
				n.OriginPC = pc
			}
			tr.AddNext(condition)
			condition.AddNext(left)
			condition.AddNext(right)
			left.AddNext(exit)
			right.AddNext(exit)
			switch kind {
			case "split", "unprotected effect", "wrong handler", "overlapping ranges":
				tr.SharedProtectedHandler = true
				tr.ProtectedEndPC = 25
				tr.SharedProtectedRanges = []core.HandlerRange{{StartPc: 10, EndPc: 25, HandlerPc: 60, CatchType: 2}, {StartPc: 30, EndPc: 40, HandlerPc: 60, CatchType: 2}}
				if kind == "unprotected effect" {
					right.OriginPC = 26
				}
				if kind == "wrong handler" {
					tr.SharedProtectedRanges[1].HandlerPc++
				}
				if kind == "overlapping ranges" {
					tr.SharedProtectedRanges[1].StartPc = 24
				}
			case "foreign entry":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(left)
			case "unknown PC":
				right.HasOriginPC = false
			case "back edge":
				right.ReplaceNext(exit, condition)
			case "distinct exits":
				other := core.NewNode(&statements.ReturnStatement{})
				other.HasOriginPC = true
				other.OriginPC = 50
				right.ReplaceNext(exit, other)
			case "opaque statement":
				right.Statement = &statements.MiddleStatement{}
			case "missing successor":
				right.RemoveNext(exit)
			}
			prefix, boundary := protectedRetryBranchPrefix(tr, nil)
			want := kind == "private" || kind == "split"
			if (boundary == exit) != want {
				t.Fatalf("accepted=%v want%v", boundary != nil, want)
			}
			if want && len(prefix) != 3 {
				t.Fatal("branch/effect omitted")
			}
		})
	}
}

func TestRetryLoopHeaderExceptionalEdgeCannotBecomeNormalExit(t *testing.T) {
	loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	outer := core.NewNode(&statements.MiddleStatement{})
	tr := core.NewNode(&statements.MiddleStatement{})
	tr.HasProtectedRange = true
	tr.ProtectedStartPC = 10
	tr.ProtectedEndPC = 40
	decision := core.NewNode(&statements.ConditionStatement{})
	decision.HasOriginPC = true
	decision.OriginPC = 15
	left := core.NewNode(&statements.ExpressionStatement{})
	left.HasOriginPC = true
	left.OriginPC = 20
	right := core.NewNode(&statements.ExpressionStatement{})
	right.HasOriginPC = true
	right.OriginPC = 30
	exit := core.NewNode(&statements.ReturnStatement{})
	exit.HasOriginPC = true
	exit.OriginPC = 45
	retry := core.NewNode(&statements.AssignStatement{})
	retry.IsCatchStart = true
	outerCatch := core.NewNode(&statements.AssignStatement{})
	outerCatch.IsCatchStart = true
	catchReturn := core.NewNode(&statements.ReturnStatement{})
	loop.AddNext(outer)
	outer.AddNext(tr)
	outer.AddNext(outerCatch)
	outerCatch.AddNext(catchReturn)
	tr.ProtectedEnd = exit
	tr.AddNext(decision)
	tr.AddNext(retry)
	retry.AddNext(loop)
	decision.AddNext(left)
	decision.AddNext(right)
	left.AddNext(exit)
	right.AddNext(exit)
	got := searchCircleEndNode(loop, outer, GenerateDominatorTree(loop), true)
	if got != exit {
		t.Fatal("header exceptional edge became loop's normal continuation")
	}
}

func TestProtectedTryLoopHeaderKeepsNonRetryHandlersOutsideLoop(t *testing.T) {
	for _, scenario := range []string{"proved", "retry catch", "two normal arms", "missing range", "not try", "irreducible", "no backedge"} {
		t.Run(scenario, func(t *testing.T) {
			entry := core.NewNode(&statements.MiddleStatement{})
			tr := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
			header := core.NewNode(&statements.ConditionStatement{})
			step := core.NewNode(&statements.ExpressionStatement{})
			exit := core.NewNode(&statements.ReturnStatement{})
			handler := core.NewNode(&statements.ReturnStatement{})
			tr.HasProtectedRange = true
			tr.ProtectedStartPC = 10
			tr.ProtectedEndPC = 30
			handler.IsCatchStart = true
			entry.AddNext(tr)
			tr.AddNext(header)
			tr.AddNext(handler)
			header.AddNext(step)
			header.AddNext(exit)
			step.AddNext(tr)
			manager := NewRootStatementManager(entry)
			manager.LoopRegionReducible = true
			switch scenario {
			case "retry catch":
				handler.AddNext(tr)
			case "two normal arms":
				tr.AddNext(exit)
			case "missing range":
				tr.HasProtectedRange = false
			case "not try":
				tr.Statement = &statements.MiddleStatement{}
			case "irreducible":
				manager.LoopRegionReducible = false
			case "no backedge":
				step.RemoveNext(tr)
			}
			manager.DominatorMap = GenerateDominatorTree(entry)
			normal, backedges := protectedTryLoopHeader(manager, tr)
			if (normal == header) != (scenario == "proved") {
				t.Fatal("incorrect header containment", scenario)
			}
			if scenario == "proved" && (len(backedges) != 1 || backedges[0] != step) {
				t.Fatal("external entry must not become a loop backedge")
			}
		})
	}
}
