package rewriter

import (
	"math/rand"
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

// The one-pass jump-only map must agree with the per-target query for every
// target, including grouped labels, a default target, and duplicate targets.
func TestSwitchJumpOnlyTargetsMatchesPerTargetQuery(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5eed))
	for trial := 0; trial < 500; trial++ {
		targetPool := make([]*core.Node, 1+rng.Intn(4))
		for i := range targetPool {
			targetPool[i] = &core.Node{}
		}
		node := &core.Node{SwitchJumpOnlyCases: map[int]bool{}}
		cases := omap.NewEmptyOrderedMap[switchLabel, *core.Node]()
		for value := 0; value < 1+rng.Intn(6); value++ {
			if rng.Intn(2) == 0 {
				node.SwitchJumpOnlyCases[value] = true
			}
			cases.Set(switchLabel{Value: int32(value)}, targetPool[rng.Intn(len(targetPool))])
		}
		hasDefault := rng.Intn(2) == 0
		if hasDefault {
			node.SwitchDefault = targetPool[rng.Intn(len(targetPool))]
			cases.Set(switchLabel{Default: true}, node.SwitchDefault)
		}

		only := switchJumpOnlyTargets(node, cases)
		for _, target := range targetPool {
			want := switchCaseHasOnlyJumpEntries(node, target, cases)
			if got := only[target]; got != want {
				t.Fatalf("trial %d target %p: jumpOnly=%t want %t", trial, target, got, want)
			}
		}

		// Same table through the prepared-switch path used by the loop scan.
		node.SwitchCases = omap.NewEmptyOrderedMap[int, *core.Node]()
		cases.ForEach(func(label switchLabel, target *core.Node) bool {
			if !label.Default {
				node.SwitchCases.Set(int(label.Value), target)
			}
			return true
		})
		direct, ok := switchNodeJumpOnlyTargets(node, nil)
		if !ok {
			t.Fatalf("trial %d: prepared switch tables were rejected", trial)
		}
		for _, target := range targetPool {
			want := switchCaseHasOnlyJumpEntries(node, target, cases)
			if got := direct[target]; got != want {
				t.Fatalf("trial %d prepared target %p: jumpOnly=%t want %t", trial, target, got, want)
			}
		}
	}
}
