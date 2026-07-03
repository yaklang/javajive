package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestNewRecvJDKGenericDiamondIsLoadBearing pins the raw-`new`-receiver diamond fix
// (JDEC_NEW_RECV_DIAMOND_OFF). A RAW `new HashMap(typedMap)` used directly as the receiver of a
// lambda-taking call (forEach) erases the method's functional-interface parameter (raw receiver, JLS
// 4.8), so the lambda parameters degrade to Object and a body dereferencing them fails ("Object cannot
// be converted to String"; spring SimpleAliasRegistry). Restoring the diamond `new HashMap<>(typedMap)`
// lets javac re-infer the type arguments from the constructor argument, rebinding the lambda. With the
// fix ON the diamond is present; with the kill-switch OFF it degrades to the broken raw form, proving
// the fix is load-bearing.
func TestNewRecvJDKGenericDiamondIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NewRecvDiamondSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_NEW_RECV_DIAMOND_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "new HashMap<>(this.aliasMap).forEach(") {
		t.Errorf("fix ON: expected diamond `new HashMap<>(this.aliasMap).forEach(`, got:\n%s", on)
	}

	t.Setenv("JDEC_NEW_RECV_DIAMOND_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "new HashMap<>(this.aliasMap).forEach(") ||
		!strings.Contains(off, "new HashMap(this.aliasMap).forEach(") {
		t.Errorf("fix OFF: expected the broken raw `new HashMap(this.aliasMap).forEach(` (kill-switch not load-bearing), got:\n%s", off)
	}
}
