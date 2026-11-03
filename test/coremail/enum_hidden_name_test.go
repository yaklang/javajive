package coremail_test

import (
	parser "github.com/yaklang/javajive/classparser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// J2B-020: an obfuscator can rename the ACC_ENUM field while leaving the
// Enum.<init> name string unchanged. JLS 8.9.1 uses one identifier for both,
// and JLS 8.1.4 forbids writing `class T extends Enum`. The denotable form is
// the original name as the enum constant plus a static alias for the field.
func TestHiddenEnumNameKeepsAliasAndName(t *testing.T) {
	javac, java := tool(t, "javac"), tool(t, "java")
	dir := t.TempDir()
	command(t, javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, "testdata/enum/HiddenName.java", "testdata/enum/HiddenNameDriver.java")
	obj, err := parser.Parse(readFile(t, filepath.Join(dir, "fixtures/HiddenName.class")))
	if err != nil {
		t.Fatal(err)
	}
	renameEnumField(t, obj, "OK", "a")
	renameEnumField(t, obj, "NET_ERR", "b")
	renameEnumField(t, obj, "$VALUES", "k")
	raw := obj.Bytes()
	put(t, filepath.Join(dir, "fixtures/HiddenName.class"), raw)
	original := command(t, java, "-cp", dir, "fixtures.HiddenNameDriver")
	const want = "OK:NET_ERR:0:1:7:8:true:true:true\n"
	if original != want {
		t.Fatalf("patched class did not preserve hidden names: %q", original)
	}
	generated := decompileClass(t, dir, "fixtures/HiddenName.class", raw)
	if strings.Contains(generated, "decompile dump failed") || strings.Contains(generated, parser.DecompileStubMarker) {
		t.Fatalf("denotable hidden name refused:\n%s", generated)
	}
	for _, piece := range []string{"OK(7)", "NET_ERR(8)", "public static final HiddenName a = OK;", "public static final HiddenName b = NET_ERR;"} {
		if !strings.Contains(generated, piece) {
			t.Fatalf("missing %q:\n%s", piece, generated)
		}
	}
	rebuilt := filepath.Join(dir, "rebuilt")
	source := filepath.Join(rebuilt, "fixtures/HiddenName.java")
	put(t, source, []byte(generated))
	command(t, javac, "-proc:none", "--release", "8", "-d", rebuilt, source)
	got := command(t, java, "-cp", rebuilt+string(os.PathListSeparator)+dir, "fixtures.HiddenNameDriver")
	if got != original {
		t.Fatalf("original %q regenerated %q\n%s", original, got, generated)
	}
}

func TestHiddenEnumNameRejectsKeywordAndDuplicate(t *testing.T) {
	javac := tool(t, "javac")
	dir := t.TempDir()
	command(t, javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, "testdata/enum/HiddenName.java")
	base := readFile(t, filepath.Join(dir, "fixtures/HiddenName.class"))
	t.Run("keyword", func(t *testing.T) {
		obj, err := parser.Parse(base)
		if err != nil {
			t.Fatal(err)
		}
		renameEnumField(t, obj, "OK", "a")
		renameEnumField(t, obj, "NET_ERR", "b")
		replaceUTF8(t, obj, "OK", "class")
		src := decompileClass(t, dir, "fixtures/HiddenName.class", obj.Bytes())
		if !strings.Contains(src, "hidden name or ordinal differs") {
			t.Fatalf("keyword name was emitted:\n%s", src)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		obj, err := parser.Parse(base)
		if err != nil {
			t.Fatal(err)
		}
		renameEnumField(t, obj, "OK", "a")
		renameEnumField(t, obj, "NET_ERR", "b")
		replaceUTF8(t, obj, "NET_ERR", "OK")
		src := decompileClass(t, dir, "fixtures/HiddenName.class", obj.Bytes())
		if !strings.Contains(src, "hidden name or ordinal differs") {
			t.Fatalf("duplicate hidden name was emitted:\n%s", src)
		}
	})
}

func renameEnumField(t *testing.T, obj *parser.ClassObject, from, to string) {
	t.Helper()
	obj.ConstantPool = append(obj.ConstantPool, &parser.ConstantUtf8Info{Value: to})
	index := uint16(len(obj.ConstantPool))
	desc := ""
	found := false
	for _, field := range obj.Fields {
		if utf8(t, obj, field.NameIndex) == from {
			desc = utf8(t, obj, field.DescriptorIndex)
			field.NameIndex = index
			found = true
		}
	}
	if !found || desc == "" {
		t.Fatalf("field %s missing", from)
	}
	retargeted := 0
	for _, info := range obj.ConstantPool {
		nt, ok := info.(*parser.ConstantNameAndTypeInfo)
		if !ok || utf8(t, obj, nt.DescriptorIndex) != desc {
			continue
		}
		if utf8(t, obj, nt.NameIndex) == from {
			nt.NameIndex = index
			retargeted++
		}
	}
	if retargeted == 0 {
		t.Fatalf("no name-and-type for %s", from)
	}
}

func replaceUTF8(t *testing.T, obj *parser.ClassObject, from, to string) {
	t.Helper()
	n := 0
	for _, info := range obj.ConstantPool {
		u, ok := info.(*parser.ConstantUtf8Info)
		if ok && u.Value == from {
			u.Value = to
			u.Units = nil
			u.Raw = nil
			n++
		}
	}
	if n != 1 {
		t.Fatalf("utf8 %s replacements=%d", from, n)
	}
}

func decompileClass(t *testing.T, dir, name string, raw []byte) string {
	t.Helper()
	path := archive(t, dir, map[string][]byte{name: raw})
	fs, err := parser.NewJarFSFromLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	src, err := fs.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}
