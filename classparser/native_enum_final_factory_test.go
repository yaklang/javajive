package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Old javac emitted enum values() with ACC_FINAL (javap: flags 0x0019), e.g.
// CXF MessageFormatType from this corpus. ACC_FINAL is legal on a static JVM
// method; the source protocol only requires public+static. Refusing it turns a
// valid enum into a failure comment.
func TestNativeEnumAcceptsFinalValuesFactory(t *testing.T) {
	const source = `enum FinalFactoryChoice{BINARY("binary"),BYTE("byte"),TEXT("text");private final String value;FinalFactoryChoice(String value){this.value=value;}public String value(){return value;}}`
	_, java := t04Tools(t)
	files := nativeCompileDebugClasses(t, source, "none")
	obj, err := Parse(files["FinalFactoryChoice.class"])
	if err != nil {
		t.Fatal(err)
	}
	var values, valueOf *MemberInfo
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		switch n {
		case "values":
			values = m
		case "valueOf":
			valueOf = m
		}
	}
	if values == nil || valueOf == nil {
		t.Fatal("compiler factories missing")
	}
	// The fixture must carry the historical flag shape, not only today's javac.
	values.AccessFlags = 0x0019
	if err := nativeEnumBackingProtocolProbe(t, obj); err != nil {
		t.Fatalf("final values() was refused: %v", err)
	}
	if nativeEnumValueOfFactoryProof(obj, valueOf, nil) != true {
		t.Fatal("final values() must not change valueOf proof")
	}
	archive := nativeArchive(t, map[string][]byte{"FinalFactoryChoice.class": obj.Bytes()})
	defer archive.Close()
	raw, err := archive.ReadFile("FinalFactoryChoice.class")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "decompile dump failed") || strings.Contains(string(raw), "enum regeneration") {
		t.Fatalf("final values() refused through archive: %s", raw)
	}
	for _, want := range []string{"enum FinalFactoryChoice", "BINARY", "TEXT", "String value"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %q:\n%s", want, raw)
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "FinalFactoryChoice.java")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Compile the regenerated enum; javac then creates the real factories and
	// the source no longer needs the original final flag.
	javac, _ := t04Tools(t)
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("regenerated enum does not compile: %v\n%s\n%s", err, out, raw)
	}
	_ = java
}

// The protocol probe uses the same original-bytecode producer checks as the
// archive path without requiring a ZIP fixture.
func nativeEnumBackingProtocolProbe(t *testing.T, obj *ClassObject) error {
	t.Helper()
	d := NewClassObjectDumper(obj)
	init, err := d.nativeEnumConstantInitializations()
	if err != nil {
		return err
	}
	return d.nativeEnumBackingArrayProtocol(init)
}
