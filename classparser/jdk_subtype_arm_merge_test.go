package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestJDKSubtypeArmMergeIsLoadBearing pins reachingRefSlotJDKSubtypeArmMerge
// (JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF). One control-flow arm stores a JDK supertype (Map, via a
// cast) onto a slot and the disjoint arm stores a JDK subtype allocation (new HashMap()); both flow into
// the post-merge `m.put(..)` read. With the fix ON both arms assign a single `Map var4` variable that is
// safely read after the merge. With the kill-switch the subtype arm's allocation is folded into the
// `put(..)` call and its assignment to var4 is dropped, leaving var4 assigned on only one branch — the
// definite-assignment bug this merge repairs (jsoup Whitelist.addProtocols). Mirroring the real class,
// the ON output must assign var4 in BOTH arms; the OFF output must show the folded, single-arm form.
func TestJDKSubtypeArmMergeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/JDKSubtypeArmMergeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "var4 = new HashMap();") ||
		!strings.Contains(on, "var4 = ((Map)(this.outer.get(var1)));") {
		t.Errorf("fix ON: expected var4 assigned in BOTH arms, got:\n%s", on)
	}

	t.Setenv("JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "var4 = new HashMap();") {
		t.Errorf("fix OFF: expected the subtype arm folded away (kill-switch not load-bearing), got:\n%s", off)
	}
}
