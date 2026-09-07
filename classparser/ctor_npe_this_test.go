package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCtorNPECheckBeforeThisIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/ThisFirstAdv.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_CTOR_NPE_THIS_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "requireNonNull") || strings.Contains(on, ".getClass();") {
		t.Fatalf("ON still has NPE-check before this():\n%s", on)
	}
	if !strings.Contains(on, "this(var1::append)") {
		t.Fatalf("ON missing this(method-ref):\n%s", on)
	}
	t.Setenv("JDEC_CTOR_NPE_THIS_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "Objects.requireNonNull(var1)") {
		t.Fatalf("OFF missing requireNonNull:\n%s", off)
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestEnumClinitIllegalNewIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/TypeDefinition$Sort.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_ENUM_CLINIT_NEW_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, `new TypeDefinition$Sort("NON_GENERIC"`) {
		t.Fatalf("ON still instantiates enum:\n%s", clipForTest(on, "NON_GENERIC"))
	}
	t.Setenv("JDEC_ENUM_CLINIT_NEW_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, `new TypeDefinition$Sort("NON_GENERIC"`) {
		t.Fatalf("OFF missing illegal new Enum:\n%s", clipForTest(off, "NON_GENERIC"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestBareNestedImportsIsLoadBearing(t *testing.T) {
	in := "package p;\nimport Advice.OnMethodEnter;\nimport net.bytebuddy.asm.Advice;\nclass C {}\n"
	os.Unsetenv("JDEC_BARE_NESTED_IMPORT_OFF")
	on := fixBareNestedImports(in)
	if strings.Contains(on, "import Advice.OnMethodEnter") {
		t.Fatalf("ON still has bare nested import:\n%s", on)
	}
	if !strings.Contains(on, "import net.bytebuddy.asm.Advice") {
		t.Fatalf("ON dropped the packaged import:\n%s", on)
	}
	t.Setenv("JDEC_BARE_NESTED_IMPORT_OFF", "1")
	if fixBareNestedImports(in) != in {
		t.Fatal("OFF expected identity")
	}
}

func TestBareNestedImportsJarFSMockito(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/mockito/internal/creation/bytebuddy/MockMethodAdvice$ForEquals.class"
	os.Unsetenv("JDEC_BARE_NESTED_IMPORT_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Contains(on, "import Advice.") {
		t.Fatalf("ON still has bare Advice import:\n%s", on[:min(len(on), 400)])
	}
	t.Setenv("JDEC_BARE_NESTED_IMPORT_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "import Advice.") {
		t.Fatalf("OFF expected bare Advice import:\n%s", off[:min(len(off), 400)])
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestMockMethodDispatcherRawClassDecompiles(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/MockMethodDispatcher.class")
	if err != nil {
		t.Fatal(err)
	}
	src, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "class MockMethodDispatcher") {
		head := src
		if len(head) > 400 {
			head = head[:400]
		}
		t.Fatalf("expected MockMethodDispatcher class, got:\n%s", head)
	}
}
