package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestEmbedAssignCallReadIntIsLoadBearing pins the embedded-assign-with-call int recovery
// (JDEC_NO_EMBED_ASSIGN_INT). The `while ((c = this.read()) != -1)` / `(c = in.read()) < n` drain-loop
// idiom hides its only definition of `c` inside a condition, so the missing-decl safety net must
// synthesize one. The RHS is a method call (`this.read()`), so the int-detection pattern has to span a
// single level of `()`; without that it defaulted to `Object c = null` and javac rejected the loop with
// "bad operand types for binary operator '!='/'<'". Seed is spring-core's real
// UpdateMessageDigestInputStream.class. With the fix ON the synthesized declaration is `int`; with the
// kill-switch OFF it regresses to the broken `Object ... = null`.
func TestEmbedAssignCallReadIntIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/EmbedAssignCallReadSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_NO_EMBED_ASSIGN_INT")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	// The embedded-assign target must be declared int, never a bogus Object=null that breaks the
	// numeric comparison in the loop guard.
	if !strings.Contains(on, "int var5 = 0;") {
		t.Errorf("fix ON: expected synthesized `int var5 = 0;`, got:\n%s", on)
	}
	if strings.Contains(on, "Object var5 = null;") {
		t.Errorf("fix ON: broken `Object var5 = null;` still present:\n%s", on)
	}

	t.Setenv("JDEC_NO_EMBED_ASSIGN_INT", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "Object var5 = null;") {
		t.Errorf("fix OFF: expected the broken `Object var5 = null;` to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
