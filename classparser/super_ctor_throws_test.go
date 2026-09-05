package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestObjectInputStreamAnonCtorThrowsIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PureJavaReflectionProvider$1.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_SUPER_CTOR_THROWS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "throws IOException") {
		t.Errorf("ON expected throws IOException, got:\n%s", on)
	}
	t.Setenv("JDEC_SUPER_CTOR_THROWS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if strings.Contains(off, "throws IOException") {
		t.Errorf("OFF expected no throws, got:\n%s", off)
	}
}

func TestMaxCoreAnonymousSuiteCtorThrowsIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/MaxCore$1$1.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	suite, err := os.ReadFile("testdata/regression/Suite.class")
	if err != nil {
		t.Fatalf("read Suite: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		switch internalName {
		case "org/junit/runners/Suite":
			return suite, true
		case "org/junit/experimental/max/MaxCore$1$1":
			return seed, true
		default:
			return nil, false
		}
	}

	os.Unsetenv("JDEC_SUPER_CTOR_THROWS_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "throws InitializationError") {
		t.Errorf("ON expected throws InitializationError, got:\n%s", on)
	}

	t.Setenv("JDEC_SUPER_CTOR_THROWS_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if strings.Contains(off, "throws InitializationError") {
		t.Errorf("OFF expected no throws, got:\n%s", off)
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}
