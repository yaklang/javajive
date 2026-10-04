package core

import "testing"

func TestEncodedJumpTargetRewiring(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		n, old, target := NewNode(nil), NewNode(nil), NewNode(nil)
		n.AddNext(old)
		n.EncodedJumps = map[*Node]bool{old: true}
		if mixed {
			n.AddNext(target)
		}
		n.ReplaceNext(old, target)
		if n.EncodedJumps[old] || n.EncodedJumps[target] == mixed {
			t.Fatalf("mixed=%t: abrupt marker did not follow edge identity", mixed)
		}
		n.RemoveNext(target)
		if n.EncodedJumps[target] {
			t.Fatal("removed edge retained stale abrupt marker")
		}
	}
}

func TestEncodedJumpSplitDoesNotInventTargetIdentity(t *testing.T) {
	n, old, left, right := NewNode(nil), NewNode(nil), NewNode(nil), NewNode(nil)
	n.AddNext(old)
	n.AddNext(left)
	n.EncodedJumps = map[*Node]bool{old: true, left: true}
	n.ReplaceNextSliceKeepOrder(old, []*Node{left, right})
	if len(n.EncodedJumps) != 0 {
		t.Fatal("split edge retained or invented an abrupt target")
	}
}

func TestNormalEdgeCoalescingClearsAbruptTarget(t *testing.T) {
	n, normal, abrupt := NewNode(nil), NewNode(nil), NewNode(nil)
	n.AddNext(normal)
	n.AddNext(abrupt)
	n.EncodedJumps = map[*Node]bool{abrupt: true}
	n.ReplaceNext(normal, abrupt)
	if len(n.EncodedJumps) != 0 {
		t.Fatal("normal entrance coalesced into an exclusively abrupt target")
	}
}
