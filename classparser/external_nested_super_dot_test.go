package javaclassparser

// TestExternalNestedSuperDotIsLoadBearing pins that an EXTERNAL nested SUPERCLASS is dotted
// (`extends Holder.Inner`) rather than left as the binary flat name (`extends Holder$Inner`).
//
// ShortTypeName already dots field/method refs after SiblingSuperTypes is wired, but the class
// header (`extends` / `implements`) used to render BEFORE that wiring. The result: header stays
// flat while GetAllImported (after wiring) imports the OUTER class -- javac "cannot find symbol:
// class InputAccessor$Std" / "class ObjectIdGenerators$PropertyGenerator" (jackson-databind
// DataFormatReaders$AccessorForReader / PropertyBasedObjectIdGenerator).
//
// Kill-switch JDEC_EXTERNAL_NESTED_DOT_OFF (same as field-ref TestExternalNestedDotIsLoadBearing).

import (
	"os"
	"strings"
	"testing"
)

func TestExternalNestedSuperDotIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/ExtendsExternalNestedSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	holder, err := os.ReadFile("testdata/regression/ExtHolder.class")
	if err != nil {
		t.Fatalf("read holder: %v", err)
	}

	externalResolver := func(internalName string) ([]byte, bool) { return nil, false }
	os.Unsetenv("JDEC_EXTERNAL_NESTED_DOT_OFF")
	ext, err := DecompileWithResolver(seed, externalResolver)
	if err != nil {
		t.Fatalf("decompile (external) failed: %v", err)
	}
	if !strings.Contains(ext, "extends Holder.Inner") {
		t.Errorf("external: expected `extends Holder.Inner`, got:\n%s", ext)
	}
	if strings.Contains(ext, "extends Holder$Inner") {
		t.Errorf("external: flat `extends Holder$Inner` must NOT appear, got:\n%s", ext)
	}

	inJarResolver := func(internalName string) ([]byte, bool) {
		if internalName == "q/Holder" {
			return holder, true
		}
		return nil, false
	}
	inJar, err := DecompileWithResolver(seed, inJarResolver)
	if err != nil {
		t.Fatalf("decompile (in-jar) failed: %v", err)
	}
	if !strings.Contains(inJar, "extends Holder$Inner") {
		t.Errorf("in-jar: expected flat `extends Holder$Inner`, got:\n%s", inJar)
	}
	if strings.Contains(inJar, "extends Holder.Inner") {
		t.Errorf("in-jar: dotted `extends Holder.Inner` must NOT appear for a same-jar flat unit, got:\n%s", inJar)
	}

	t.Setenv("JDEC_EXTERNAL_NESTED_DOT_OFF", "1")
	off, err := DecompileWithResolver(seed, externalResolver)
	if err != nil {
		t.Fatalf("decompile (kill-switch) failed: %v", err)
	}
	if !strings.Contains(off, "extends Holder$Inner") {
		t.Errorf("kill-switch: expected flat `extends Holder$Inner` fallback, got:\n%s", off)
	}
	if strings.Contains(off, "extends Holder.Inner") {
		t.Errorf("kill-switch: dotted `extends Holder.Inner` must NOT appear, got:\n%s", off)
	}
}
