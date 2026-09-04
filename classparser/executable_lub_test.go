package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestExecutableArmMergeIsLoadBearing pins reachingRefSlotExecutableArmMerge. A Method-returning
// helper types the slot; a later Constructor<?> store into the same slot is rejected as
// "Constructor<CAP#1> cannot be converted to Method" unless the shared ref is widened to
// Executable. Kill-switch: JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF. Real hit: spring-core
// ObjectToObjectConverter.getValidatedExecutable.
func TestExecutableArmMergeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ExecutableLubSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !executableSlotThenConstructorStore(on) {
		t.Errorf("fix ON: expected Executable local assigned from both Method and Constructor helpers, got:\n%s", on)
	}
	if methodSlotThenConstructorStore(on) {
		t.Errorf("fix ON: Method local must not receive the Constructor store, got:\n%s", on)
	}

	t.Setenv("JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !methodSlotThenConstructorStore(off) {
		t.Errorf("fix OFF: expected Method local assigned the Constructor store, got:\n%s", off)
	}
}

// TestExecutableArmMergeSpringObjectToObjectConverter drives the real spring-core class.
func TestExecutableArmMergeSpringObjectToObjectConverter(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringObjectToObjectConverter.class")
	if err != nil {
		t.Fatalf("read spring class: %v", err)
	}

	os.Unsetenv("JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if methodSlotThenConstructorStore(on) {
		t.Errorf("fix ON: spring getValidatedExecutable must not assign Constructor to Method, got:\n%s", snippetValidatedExecutable(on))
	}
	if !strings.Contains(snippetValidatedExecutable(on), "Executable ") {
		t.Errorf("fix ON: expected an Executable local in getValidatedExecutable, got:\n%s", snippetValidatedExecutable(on))
	}

	t.Setenv("JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !methodSlotThenConstructorStore(off) {
		t.Errorf("fix OFF: expected Method local assigned Constructor (kill-switch load-bearing), got:\n%s", snippetValidatedExecutable(off))
	}
}

func methodSlotThenConstructorStore(src string) bool {
	body := snippetValidatedExecutable(src)
	return strings.Contains(body, "Method var") &&
		strings.Contains(body, "determineToMethod(") &&
		strings.Contains(body, " = determineFactoryConstructor(")
}

func executableSlotThenConstructorStore(src string) bool {
	body := snippetValidatedExecutable(src)
	return strings.Contains(body, "Executable var") &&
		strings.Contains(body, " = determineFactoryConstructor(") &&
		!strings.Contains(body, "Method var")
}

func snippetValidatedExecutable(src string) string {
	idx := strings.Index(src, "static Executable getValidatedExecutable")
	if idx < 0 {
		idx = strings.Index(src, "getValidatedExecutable")
	}
	if idx < 0 {
		return src
	}
	rest := src[idx:]
	if i := strings.Index(rest, "boolean isApplicable"); i > 0 {
		rest = rest[:i]
	}
	return rest
}
