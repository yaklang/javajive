package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"testing"
)

// The header's false arm performs an exit-specific write; another exit does
// a different write. Their shared return is the continuation, so neither
// write can be lifted after the whole loop and imposed on the other arm.
func TestLoopOwnedSharedContinuationKeepsExitSpecificEffects(t *testing.T) {
	for _, variant := range []string{"shared return", "long prefix", "shared tail effect", "catch prefix", "encoded escape", "unknown sink", "unshared return", "cyclic alternative", "ancestor continuation", "protected prefix", "foreign prefix entry"} {
		t.Run(variant, func(t *testing.T) {
			entry := core.NewNode(&statements.MiddleStatement{})
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			header := core.NewNode(&statements.ConditionStatement{})
			inner := core.NewNode(&statements.ConditionStatement{})
			advance := core.NewNode(&statements.MiddleStatement{})
			empty := core.NewNode(&statements.MiddleStatement{})
			found := core.NewNode(&statements.MiddleStatement{})
			ret := core.NewNode(&statements.ReturnStatement{})
			entry.AddNext(loop)
			loop.AddNext(header)
			header.AddNext(inner)
			header.AddNext(empty)
			inner.AddNext(advance)
			inner.AddNext(found)
			advance.AddNext(loop)
			empty.AddNext(ret)
			found.AddNext(ret)
			want := ret
			switch variant {
			case "long prefix":
				extra := core.NewNode(&statements.MiddleStatement{})
				empty.RemoveNext(ret)
				empty.AddNext(extra)
				extra.AddNext(ret)
			case "shared tail effect":
				ret.Statement = &statements.MiddleStatement{}
				ret.AddNext(core.NewNode(&statements.ReturnStatement{}))
			case "catch prefix":
				found.IsCatchStart = true
				want = empty
			case "encoded escape":
				found.EncodedJumps = map[*core.Node]bool{loop: true}
				want = empty
			case "unknown sink":
				found.RemoveNext(ret)
				want = empty
			case "unshared return":
				found.RemoveNext(ret)
				found.Statement = &statements.ReturnStatement{}
				want = empty
			case "cyclic alternative":
				found.AddNext(found)
				want = empty
			case "ancestor continuation":
				found.RemoveNext(ret)
				found.AddNext(entry)
				want = empty
			case "protected prefix":
				empty.HasProtectedRange = true
				empty.ProtectedStartPC = 0
				empty.ProtectedEndPC = 20
				want = empty
			case "foreign prefix entry":
				entry.AddNext(found)
				want = empty
			}
			dom := GenerateDominatorTree(entry)
			if got := searchCircleEndNode(loop, header, dom, true); got != want {
				t.Fatalf("exit=%p want=%p", got, want)
			}
		})
	}
}

// Region ownership is a graph property. The same enclosing continuation must
// stay a back edge before and after the outer loop is structurally rewritten.
func TestLoopSharedExitRejectsDominatingContinuationBeforeStructure(t *testing.T) {
	for _, shape := range []string{"ordinary", "condition", "structured-loop"} {
		for _, forward := range []bool{false, true} {
			name := shape + "/ancestor"
			if forward {
				name = shape + "/forward"
			}
			t.Run(name, func(t *testing.T) {
				entry := core.NewNode(&statements.MiddleStatement{})
				join := core.NewNode(&statements.MiddleStatement{})
				owner := core.NewNode(statements.NewDoWhileStatement(nil, nil))
				header := core.NewNode(&statements.ConditionStatement{})
				left := core.NewNode(&statements.MiddleStatement{})
				right := core.NewNode(&statements.MiddleStatement{})
				if shape == "condition" {
					join.Statement = &statements.ConditionStatement{}
				}
				if shape == "structured-loop" {
					join.Statement = statements.NewDoWhileStatement(nil, nil)
				}
				owner.AddNext(header)
				header.AddNext(left)
				header.AddNext(right)
				left.AddNext(join)
				right.AddNext(join)
				want := join
				if forward {
					entry.AddNext(owner)
					join.AddNext(core.NewNode(&statements.ReturnStatement{}))
				} else {
					entry.AddNext(join)
					join.AddNext(owner)
					join.AddNext(core.NewNode(&statements.ReturnStatement{}))
					want = nil
				}
				if got := ownedSharedLoopExit([]*core.Node{left, right}, owner, GenerateDominatorTree(entry)); got != want {
					t.Fatalf("continuation=%p want=%p", got, want)
				}
			})
		}
	}
}
