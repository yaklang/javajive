package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestTypeParamBoundImportIsLoadBearing pins the type-variable-bound import fix
// (JDEC_TYPEPARAM_BOUND_IMPORT_OFF). A class header `<A extends Annotation>` renders the bound type
// java.lang.annotation.Annotation with the SHORT name `Annotation`, but the bound was previously
// rendered against a throwaway ClassContext, so its import was never registered and the class
// recompiled as "cannot find symbol: class Annotation" (spring MergedAnnotationSelector /
// MergedAnnotationPredicates$FirstRunOfPredicate class headers). With the fix ON the bound is rendered
// against the real context, which registers `import java.lang.annotation.Annotation;`; with the
// kill-switch OFF the import disappears again, proving the fix is load-bearing.
func TestTypeParamBoundImportIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TypeParamBoundImportSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_TYPEPARAM_BOUND_IMPORT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "<A extends Annotation>") {
		t.Errorf("fix ON: expected the short bound `<A extends Annotation>`, got:\n%s", on)
	}
	if !strings.Contains(on, "import java.lang.annotation.Annotation;") {
		t.Errorf("fix ON: expected `import java.lang.annotation.Annotation;` to be registered, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEPARAM_BOUND_IMPORT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "import java.lang.annotation.Annotation;") {
		t.Errorf("fix OFF: the bound import must be absent (kill-switch not load-bearing), got:\n%s", off)
	}
}
