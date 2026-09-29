package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
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
