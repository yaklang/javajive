package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestMultiCatchLocalDeclIsLoadBearing pins localDeclType collapsing a multi-catch union
// (`A | B`) when it is hoisted into an ordinary local declaration. That form is legal only
// inside `catch (A | B e)`; a bare `A | B varN;` is a syntax error. Real hit: jackson
// FromStringDeserializer.deserialize. Kill-switch: JDEC_MULTICATCH_LOCAL_DECL_OFF.
func TestMultiCatchLocalDeclIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/FromStringDeserializer.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_MULTICATCH_LOCAL_DECL_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if strings.Contains(on, "IllegalArgumentException | MalformedURLException var") &&
		!onlyInCatch(on, "IllegalArgumentException | MalformedURLException") {
		t.Errorf("fix ON: union type leaked into a local declaration:\n%s", snippetUnion(on))
	}
	if !strings.Contains(on, "catch(IllegalArgumentException | MalformedURLException") {
		t.Errorf("fix ON: expected catch union to stay, got:\n%s", snippetUnion(on))
	}

	t.Setenv("JDEC_MULTICATCH_LOCAL_DECL_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "IllegalArgumentException | MalformedURLException var") {
		t.Errorf("fix OFF: expected leaked union local decl, got:\n%s", snippetUnion(off))
	}
}

func onlyInCatch(src, union string) bool {
	for _, ln := range strings.Split(src, "\n") {
		if !strings.Contains(ln, union) {
			continue
		}
		trim := strings.TrimSpace(ln)
		if !strings.Contains(trim, "catch(") {
			return false
		}
	}
	return true
}

func snippetUnion(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, " | ") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	s := b.String()
	if len(s) > 800 {
		return s[:800]
	}
	return s
}
