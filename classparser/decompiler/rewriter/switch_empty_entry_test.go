package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/omap"
)

func TestSwitchEmptyEntryRequiresEveryLabelWitness(t *testing.T) {
	for _, scenario := range []string{"grouped jumps", "real fallthrough", "shared default", "missing witness", "other target"} {
		t.Run(scenario, func(t *testing.T) {
			candidate, other := &core.Node{}, &core.Node{}
			node := &core.Node{SwitchJumpOnlyCases: map[int]bool{1: true, 2: true}}
			cases := omap.NewEmptyOrderedMap[switchLabel, *core.Node]()
			cases.Set(switchLabel{Value: 1}, candidate)
			cases.Set(switchLabel{Value: 2}, candidate)
			cases.Set(switchLabel{Default: true}, other)
			switch scenario {
			case "real fallthrough":
				delete(node.SwitchJumpOnlyCases, 2)
			case "shared default":
				cases.Set(switchLabel{Default: true}, candidate)
			case "missing witness":
				node.SwitchJumpOnlyCases = nil
			case "other target":
				candidate = &core.Node{}
			}
			if got := switchCaseHasOnlyJumpEntries(node, candidate, cases); got != (scenario == "grouped jumps") {
				t.Fatalf("empty entry=%v", got)
			}
		})
	}
}

func TestSwitchExitCountsBodiesNotGroupedLabels(t *testing.T) {
	sw, first, second, exit := &core.Node{}, &core.Node{}, &core.Node{}, &core.Node{}
	first.AddNext(exit)
	second.AddNext(exit)
	manager := &RewriteManager{DominatorMap: map[*core.Node][]*core.Node{
		sw: {first, second, exit},
	}}
	if got := countOtherCasesExitingTo(manager, sw, exit, []*core.Node{first, first, exit}); got != 1 {
		t.Fatalf("grouped labels counted as %d bodies, want 1", got)
	}
	if got := countOtherCasesExitingTo(manager, sw, exit, []*core.Node{first, first, second, exit}); got != 2 {
		t.Fatalf("distinct bodies counted as %d, want 2", got)
	}
}
