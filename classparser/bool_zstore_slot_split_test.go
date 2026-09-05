package javaclassparser

// Boolean flag assigned from a Z-returning invoke reuses a disjoint int foreach-index slot.
// Kill-switch: JDEC_BOOL_ZSTORE_SLOT_SPLIT_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestTemporaryFolderZStoreSlotSplitIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TemporaryFolder.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BOOL_ZSTORE_SLOT_SPLIT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if strings.Contains(on, "bad operand") {
		t.Errorf("fix ON: unexpected operand text:\n%s", on)
	}
	// The foreach index must stay int: `(varN) < (varM)` with a boolean declaration is the defect.
	if strings.Contains(on, "boolean var5 = false") && strings.Contains(on, "(var5) < (") {
		t.Errorf("fix ON: foreach index still declared boolean:\n%s", on)
	}
	if !strings.Contains(on, "mkdirs") {
		t.Errorf("fix ON: expected newFolder/mkdirs body, got:\n%s", on)
	}

	t.Setenv("JDEC_BOOL_ZSTORE_SLOT_SPLIT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "boolean var5 = false") && strings.Contains(off, "(var5) < (") {
		if on == off {
			t.Fatal("ON/OFF identical")
		}
	}
}
