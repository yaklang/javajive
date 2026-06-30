package cross

import (
	"os"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// TestEnumSwitchFoldIsLoadBearing pins the Bug V enum-switch ($SwitchMap) cross-class fold and its
// kill-switch JDEC_NO_ENUM_SWITCH_FOLD. Seed: commons-codec PhoneticEngine, whose `switch(nameType)`
// javac lowered to `switch(PhoneticEngine$1.$SwitchMap$...NameType[nameType.ordinal()]){case 1:...}`.
//
// With the fold ON (default) the decompiled outer must render the idiomatic enum switch
// (`switch (this.nameType)` + `case SEPHARDIC:` ...); with the fold OFF the original $SwitchMap form
// must reappear. The OFF branch is the load-bearing assertion -- it proves the fold (not some
// unrelated change) is what produces the idiomatic output, and that disabling it restores the exact
// prior behavior for regression bisection.
func TestEnumSwitchFoldIsLoadBearing(t *testing.T) {
	jar := resolveJar("commons-codec/commons-codec/1.15/commons-codec-1.15.jar")
	if jar == "" {
		t.Skip("commons-codec jar not found under ~/.m2")
	}
	entry := "org/apache/commons/codec/language/bm/PhoneticEngine.class"

	decompile := func(foldOff bool) string {
		t.Helper()
		if foldOff {
			t.Setenv("JDEC_NO_ENUM_SWITCH_FOLD", "1")
		} else {
			os.Unsetenv("JDEC_NO_ENUM_SWITCH_FOLD")
		}
		jfs, err := classparser.NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatalf("NewJarFSFromLocal: %v", err)
		}
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", entry, err)
		}
		return string(raw)
	}

	// Fold ON: idiomatic enum switch, no $SwitchMap selector.
	on := decompile(false)
	if !strings.Contains(on, "switch (this.nameType)") {
		t.Errorf("fold ON: expected `switch (this.nameType)`, not found")
	}
	for _, c := range []string{"case SEPHARDIC:", "case ASHKENAZI:", "case GENERIC:"} {
		if !strings.Contains(on, c) {
			t.Errorf("fold ON: expected %q, not found", c)
		}
	}
	if strings.Contains(on, "$SwitchMap$org$apache$commons$codec$language$bm$NameType[") {
		t.Errorf("fold ON: $SwitchMap selector should have been folded away")
	}

	// Fold OFF (kill-switch): original $SwitchMap form reappears, integer cases.
	off := decompile(true)
	if !strings.Contains(off, "$SwitchMap$org$apache$commons$codec$language$bm$NameType[this.nameType.ordinal()]") {
		t.Errorf("fold OFF: expected original $SwitchMap switch selector to reappear (kill-switch not load-bearing)")
	}
	if strings.Contains(off, "switch (this.nameType)") {
		t.Errorf("fold OFF: idiomatic `switch (this.nameType)` must NOT appear when fold disabled")
	}
}
