package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

func TestProtectedDependenciesMatchIndependentReachability(t *testing.T) {
	for mask := 0; mask < 1<<12; mask++ {
		nodes := []*core.Node{jumpTestNode("a"), jumpTestNode("b"), jumpTestNode("c"), jumpTestNode("d")}
		needs := map[*core.Node]map[*core.Node]bool{}
		reach := [4][4]bool{}
		bit := 0
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				if i == j {
					continue
				}
				if mask&(1<<bit) != 0 {
					if needs[nodes[i]] == nil {
						needs[nodes[i]] = map[*core.Node]bool{}
					}
					needs[nodes[i]][nodes[j]] = true
					reach[i][j] = true
				}
				bit++
			}
		}
		for k := 0; k < 4; k++ {
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
				}
			}
		}
		cyclic := false
		for i := 0; i < 4; i++ {
			cyclic = cyclic || reach[i][i]
		}
		for _, base := range [][]*core.Node{{nodes[3], nodes[1], nodes[0], nodes[2]}, {nodes[2], nodes[0], nodes[1], nodes[3]}} {
			before := slices.Clone(base)
			budget := 4096
			out, ok := stableProtectedDependencies(base, needs, &budget)
			if ok == cyclic || !slices.Equal(base, before) {
				t.Fatalf("mask%x completion=%v cycle=%v", mask, ok, cyclic)
			}
			if !ok {
				if !slices.Equal(out, base) {
					t.Fatal("cycle changed order")
				}
				continue
			}
			pos := map[*core.Node]int{}
			for i, n := range out {
				if _, dup := pos[n]; dup {
					t.Fatal("duplicate node")
				}
				pos[n] = i
			}
			if len(pos) != 4 {
				t.Fatal("lost node")
			}
			for i := 0; i < 4; i++ {
				for j := 0; j < 4; j++ {
					if reach[i][j] && pos[nodes[j]] >= pos[nodes[i]] {
						t.Fatalf("mask%x unsatisfied dependency", mask)
					}
				}
			}
		}
	}
	a, b, c := jumpTestNode("a"), jumpTestNode("b"), jumpTestNode("c")
	base := []*core.Node{a, b, c}
	budget := 100
	out, ok := stableProtectedDependencies(base, map[*core.Node]map[*core.Node]bool{a: {b: true}}, &budget)
	if !ok || !slices.Equal(out, []*core.Node{b, a, c}) {
		t.Fatal("did not retain earliest available original choice")
	}
	for _, kind := range []string{"unknown node", "unknown dependency", "nil dependency", "cycle", "budget", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			input := slices.Clone(base)
			needs := map[*core.Node]map[*core.Node]bool{a: {b: true}}
			budget := 100
			switch kind {
			case "unknown node":
				needs[jumpTestNode("unknown")] = map[*core.Node]bool{a: true}
			case "unknown dependency":
				needs[a][jumpTestNode("unknown")] = true
			case "nil dependency":
				needs[a][nil] = true
			case "cycle":
				needs[b] = map[*core.Node]bool{a: true}
			case "budget":
				budget = 0
			case "duplicate":
				input = append(input, a)
			}
			out, ok := stableProtectedDependencies(input, needs, &budget)
			if ok || !slices.Equal(out, input) {
				t.Fatal("incomplete constraints changed order")
			}
		})
	}
}

func TestProtectedSiblingOrderRequiresOriginalRegionAndForwardEdge(t *testing.T) {
	for _, variant := range []string{"proved", "recorded interval", "typed cleanup packet", "missing cleanup witness", "missing cleanup edge", "already ready", "incomplete dominators", "unknown dominator", "unknown table", "no try", "missing interval", "wrong handler", "catchall", "wrong ranges", "shared ranges", "missing origin", "different domain", "not condition", "encoded edge", "catch edge", "cycle", "hidden", "duplicate dominator", "detached key"} {
		t.Run(variant, func(t *testing.T) {
			truth := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			cond := func(pc int) *core.Node {
				n := core.NewNode(&statements.ConditionStatement{Condition: truth})
				n.HasOriginPC, n.OriginPC = true, pc
				return n
			}
			root, branch, fallback := cond(1), cond(5), cond(10)
			owner := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			owner.HasProtectedRange = true
			owner.ProtectedStartPC, owner.ProtectedEndPC = 20, 30
			body, handler, exit := cond(25), jumpTestNode("catch"), jumpTestNode("exit")
			handler.HasOriginPC, handler.OriginPC = true, 40
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 40, ProtectedRanges: [][2]int{{20, 30}}}
			root.AddNext(branch)
			root.AddNext(fallback)
			branch.AddNext(fallback)
			branch.AddNext(exit)
			fallback.AddNext(owner)
			fallback.AddNext(exit)
			owner.AddNext(body)
			owner.AddNext(handler)
			body.AddNext(exit)
			m := NewRootStatementManager(root)
			m.TryNodes = []*core.Node{owner}
			m.SetOriginalExceptionTable([]*core.ExceptionTableEntry{{StartPc: 20, EndPc: 30, HandlerPc: 40, CatchType: 1}})
			m.DominatorMap = map[*core.Node][]*core.Node{root: {branch, fallback, exit}, fallback: {owner}, owner: {body, handler}}
			keys := []*core.Node{root, branch, fallback, owner, body}
			switch variant {
			case "typed cleanup packet", "missing cleanup witness", "missing cleanup edge":
				cleanup := jumpTestNode("cleanup")
				cleanup.IsCatchStart = true
				cleanup.HasOriginPC, cleanup.OriginPC = true, 50
				cleanup.CatchHandler = &statements.CatchHandler{EntryPC: 50, CatchAll: true, ProtectedRanges: [][2]int{{20, 30}}}
				if variant != "missing cleanup edge" {
					owner.AddNext(cleanup)
					m.DominatorMap[owner] = append(m.DominatorMap[owner], cleanup)
				}
				if variant != "missing cleanup witness" {
					m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 20, EndPc: 30, HandlerPc: 50, CatchType: 0})
				}
			case "already ready":
				m.DominatorMap[root] = []*core.Node{fallback, branch, exit}
			case "incomplete dominators":
				m.DominatorMap[owner] = []*core.Node{body}
			case "unknown dominator":
				m.DominatorMap[jumpTestNode("unknown")] = nil
			case "unknown table":
				m.originalHandlersKnown = false
			case "no try":
				m.TryNodes = nil
			case "missing interval":
				owner.HasProtectedRange = false
			case "wrong handler":
				handler.CatchHandler.EntryPC++
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "wrong ranges":
				handler.CatchHandler.ProtectedRanges[0][1]++
			case "recorded interval":
				owner.SharedProtectedRanges = []core.HandlerRange{{StartPc: 20, EndPc: 30, HandlerPc: 40, CatchType: 1}}
			case "shared ranges":
				owner.SharedProtectedRanges = []core.HandlerRange{{StartPc: 20, EndPc: 30}}
			case "missing origin":
				branch.HasOriginPC = false
			case "different domain":
				m.originalHandlers = append(m.originalHandlers, core.HandlerRange{StartPc: 5, EndPc: 6, HandlerPc: 80, CatchType: 1})
			case "not condition":
				root.Statement = &statements.MiddleStatement{}
			case "encoded edge":
				branch.EncodedJumps = map[*core.Node]bool{fallback: true}
			case "catch edge":
				fallback.IsCatchStart = true
			case "cycle":
				body.AddNext(branch)
			case "hidden":
				branch.HideNext = exit
			case "duplicate dominator":
				m.DominatorMap[branch] = []*core.Node{body}
			}
			base := m.TopologicalSortReverse(keys)
			if variant == "detached key" {
				base = append(base, jumpTestNode("detached"))
			}
			before := map[*core.Node][]*core.Node{}
			for _, n := range []*core.Node{root, branch, fallback, owner, body, handler, exit} {
				before[n] = slices.Clone(n.Next)
			}
			out := protectedSiblingRewriteOrder(m, base)
			if variant == "proved" || variant == "recorded interval" || variant == "typed cleanup packet" {
				if slices.Index(out, fallback) > slices.Index(out, branch) || slices.Index(out, owner) > slices.Index(out, fallback) || slices.Index(out, body) > slices.Index(out, owner) {
					t.Fatal("shared protected successor or its body not ready")
				}
				if slices.Equal(out, base) {
					t.Fatal("positive fixture did not change scheduling")
				}
			} else if !slices.Equal(out, base) {
				t.Fatalf("%s changed unproved order", variant)
			}
			for n, next := range before {
				if !slices.Equal(n.Next, next) {
					t.Fatal("scheduler mutated executable CFG")
				}
			}
		})
	}
}
