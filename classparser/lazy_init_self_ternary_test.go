package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestLazyInitSelfTernaryNarrowIsLoadBearing pins the lazy-init self-guard narrowing fix
// (JDEC_LAZY_INIT_SELF_TERNARY_OFF). The idiom `x = (x != null) ? x : new Concrete()` compiles to a
// conditional store whose control-flow merge types the slot as the LUB of its null-init (Object) arm
// and the concrete `new` arm — i.e. Object. The reconstructed ternary then reads `x` back at Object and
// a later `x.add(..)` fails to recompile ("cannot find symbol"; spring-core StringDecoder). Because the
// only concrete value the slot ever holds is the `new` arm, narrowing the declaration to that arm's
// type is safe. With the fix ON the local is declared `ArrayList`; with the kill-switch it degrades to
// the broken `Object`, proving the fix is load-bearing.
func TestLazyInitSelfTernaryNarrowIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/LazyInitSelfTernarySeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_LAZY_INIT_SELF_TERNARY_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "ArrayList var2 = null;") {
		t.Errorf("fix ON: expected `ArrayList var2 = null;` declaration, got:\n%s", on)
	}

	t.Setenv("JDEC_LAZY_INIT_SELF_TERNARY_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "Object var2 = null;") {
		t.Errorf("fix OFF: expected the broken `Object var2 = null;` declaration (kill-switch not load-bearing), got:\n%s", off)
	}
}
