package coremail_test

import (
	parser "github.com/yaklang/javajive/classparser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func utf8(t *testing.T, obj *parser.ClassObject, i uint16) string {
	t.Helper()
	u, ok := obj.ConstantPool[i-1].(*parser.ConstantUtf8Info)
	if !ok {
		t.Fatalf("not UTF8 index %d", i)
	}
	return u.Value
}

// J2B-011: mutate only the historical ACC_FINAL bit; keep executable bytecode
// and constants unchanged. Compare original and regenerated executable behavior.
func TestLegacyFinalEnumFactoryRoundTrip(t *testing.T) {
	javac, java := tool(t, "javac"), tool(t, "java")
	for _, method := range []string{"values", "valueOf"} {
		t.Run(method, func(t *testing.T) {
			dir := t.TempDir()
			command(t, javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, "testdata/enum/FinalFactoryChoice.java", "testdata/enum/FactoryDriver.java")
			raw := readFile(t, filepath.Join(dir, "fixtures/FinalFactoryChoice.class"))
			obj, err := parser.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, m := range obj.Methods {
				if utf8(t, obj, m.NameIndex) == method {
					m.AccessFlags = 0x0019
					found = true
				}
			}
			if !found {
				t.Fatalf("no factory %s", method)
			}
			raw = obj.Bytes()
			put(t, filepath.Join(dir, "fixtures/FinalFactoryChoice.class"), raw)
			original := command(t, java, "-cp", dir, "fixtures.FactoryDriver")
			if original != "identity:order:ordinal:payload:clone:failure\n" {
				t.Fatalf("original: %q", original)
			}
			path := archive(t, dir, map[string][]byte{"fixtures/FinalFactoryChoice.class": raw})
			fs, err := parser.NewJarFSFromLocal(path)
			if err != nil {
				t.Fatal(err)
			}
			defer fs.Close()
			generated, err := fs.ReadFile("fixtures/FinalFactoryChoice.class")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(generated), "decompile dump failed") || strings.Contains(string(generated), parser.DecompileStubMarker) {
				t.Fatalf("refused or stubbed:\n%s", generated)
			}
			rebuilt := filepath.Join(dir, "rebuilt")
			source := filepath.Join(rebuilt, "fixtures/FinalFactoryChoice.java")
			put(t, source, generated)
			command(t, javac, "-proc:none", "--release", "8", "-d", rebuilt, source)
			got := command(t, java, "-cp", rebuilt+string(os.PathListSeparator)+dir, "fixtures.FactoryDriver")
			if got != original {
				t.Fatalf("original %q regenerated %q", original, got)
			}
		})
	}
}

// Accept exactly the old final variation, not arbitrary extra flags. In
// particular synchronized changes locking semantics and must not be erased.
func TestEnumFactoryRejectsSynchronizedProtocol(t *testing.T) {
	dir := t.TempDir()
	javac := tool(t, "javac")
	command(t, javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, "testdata/enum/FinalFactoryChoice.java")
	obj, err := parser.Parse(readFile(t, filepath.Join(dir, "fixtures/FinalFactoryChoice.class")))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range obj.Methods {
		if utf8(t, obj, m.NameIndex) == "values" {
			m.AccessFlags |= 0x0020
		}
	}
	p := archive(t, dir, map[string][]byte{"fixtures/FinalFactoryChoice.class": obj.Bytes()})
	fs, err := parser.NewJarFSFromLocal(p)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	src, err := fs.ReadFile("fixtures/FinalFactoryChoice.class")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "decompile dump failed") {
		t.Fatalf("synchronized factory silently replaced:\n%s", src)
	}
}
