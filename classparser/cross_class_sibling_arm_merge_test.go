package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestCrossClassSiblingArmMergeIsLoadBearing pins reachingRefSlotCrossClassSiblingArmMerge
// (JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF). Two disjoint if/else arms store SIBLING jar-internal
// types (SiblingArmLeft / SiblingArmRight, both extending SiblingArmBase) into one slot, both flowing
// into the post-merge `sink(node)` read. With the fix ON the shared ref is widened to the arms' nearest
// jar-internal common ancestor (SiblingArmBase), so both arms assign a single `SiblingArmBase var2` that
// is safely read after the merge. With the kill-switch the later arm splits off a fresh variable and the
// post-merge read is left unassigned on that path — the definite-assignment bug this merge repairs
// (jsoup HtmlTreeBuilder.insert). The sibling relation is proven from SiblingArmBase's bytes supplied by
// the resolver.
func TestCrossClassSiblingArmMergeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CrossClassSiblingArmMergeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF")
	on, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "SiblingArmBase var2") ||
		!strings.Contains(on, "var2 = new SiblingArmLeft();") ||
		!strings.Contains(on, "var2 = new SiblingArmRight();") {
		t.Errorf("fix ON: expected a single SiblingArmBase var2 assigned in BOTH arms, got:\n%s", on)
	}

	t.Setenv("JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF", "1")
	off, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "var2 = new SiblingArmLeft();") &&
		strings.Contains(off, "var2 = new SiblingArmRight();") &&
		strings.Contains(off, "SiblingArmBase var2") {
		t.Errorf("fix OFF: expected the arms to split into distinct variables (kill-switch not load-bearing), got:\n%s", off)
	}
}
