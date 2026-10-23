package javaclassparser

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// A handwritten enum valueOf is not javac's factory. The strict path still
// refuses it. Analysis output keeps the original loop instead of dropping the class.
func TestRetainedLegacyValueOfKeepsOriginalLoop(t *testing.T) {
	files := nativeCompileClasses(t, enumBackingReadFixture+`
class BackingDriver {
 public static void main(String[] args) { BackingRead.byName("ALPHA"); }
}`)
	raw := patchEnumBackingMethod(t, files["BackingRead.class"], "byName")
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var donor *CodeAttribute
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "byName" {
			continue
		}
		for _, a := range m.Attributes {
			if body, ok := a.(*CodeAttribute); ok {
				donor = body
			}
		}
	}
	if donor == nil {
		t.Fatal("missing donor")
	}
	moved := false
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "valueOf" {
			continue
		}
		for i, a := range m.Attributes {
			if _, ok := a.(*CodeAttribute); ok {
				m.Attributes[i] = donor
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("missing factory")
	}
	files["BackingRead.class"] = obj.Bytes()
	strict := readRetained(t, files, "BackingRead.class", false)
	if !strings.Contains(strict, "does not implement the source compiler protocol") {
		t.Fatalf("strict path emitted a factory:\n%s", strict)
	}
	source := readRetained(t, files, "BackingRead.class", true)
	for _, want := range []string{"enum BackingRead", "valueOf", "equals"} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing %q\n%s", want, source)
		}
	}
	for _, banned := range []string{"decompile dump failed", "undecompilable method body", "Enum.valueOf"} {
		if strings.Contains(source, banned) {
			t.Fatalf("retained source lost the original lookup (%s):\n%s", banned, source)
		}
	}
}

// method_index 0 does not name an enclosing formal. The strict path still
// refuses to invent <T>. Analysis output keeps the physical methods and fields.
func TestRetainedZeroMethodIndexKeepsPhysicalMethod(t *testing.T) {
	files := nativeCompileClasses(t, flattenedMethodBindingFixture)
	obj, err := Parse(files["ErasureCaptureOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	var methods []*MemberInfo
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "<init>" {
			methods = append(methods, m)
		}
	}
	obj.Methods = methods
	for _, a := range obj.Attributes {
		if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" && len(raw.Info) == 4 {
			raw.Info[2], raw.Info[3] = 0, 0
		}
	}
	files["ErasureCaptureOwner$1.class"] = obj.Bytes()
	strict := readRetained(t, files, "ErasureCaptureOwner$1.class", false)
	if !strings.Contains(strict, "unproven original enclosing method type scope") {
		t.Fatalf("strict path invented a scope:\n%s", strict)
	}
	source := readRetained(t, files, "ErasureCaptureOwner$1.class", true)
	for _, want := range []string{"forward", "number("} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing %q\n%s", want, source)
		}
	}
	for _, banned := range []string{"decompile dump failed", "<T>", "T extends", "undecompilable method body"} {
		if strings.Contains(source, banned) {
			t.Fatalf("physical method was not retained (%s):\n%s", banned, source)
		}
	}
}

// Constant headers that are not javac's new+invokespecial protocol still have
// fields and ordinary methods. Analysis output renders those as a class.
func TestRetainedNonJavacEnumKeepsMembers(t *testing.T) {
	files := nativeCompileClasses(t, `enum Proto {A, B; public int payload(){return ordinal()+1;}}`)
	obj, err := Parse(files["Proto.class"])
	if err != nil {
		t.Fatal(err)
	}
	cleared := false
	for _, method := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if n != "<clinit>" {
			continue
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok || code == nil {
				continue
			}
			code.Code = []byte{core.OP_RETURN}
			code.Attributes, code.ExceptionTable = nil, nil
			code.MaxStack, code.MaxLocals, code.AttrLen = 0, 0, uint32(12+len(code.Code))
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("missing clinit")
	}
	files["Proto.class"] = obj.Bytes()
	strict := readRetained(t, files, "Proto.class", false)
	if !strings.Contains(strict, "constant constructor origin") && !strings.Contains(strict, "original constant initialization is unproved") {
		t.Fatalf("strict path accepted a gutted enum:\n%s", strict)
	}
	source := readRetained(t, files, "Proto.class", true)
	for _, want := range []string{"class Proto", "extends", "A", "B", "payload"} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing %q\n%s", want, source)
		}
	}
	if strings.Contains(source, "enum Proto") || strings.Contains(source, "decompile dump failed") || strings.Contains(source, "undecompilable method body") {
		t.Fatalf("enum members were not retained:\n%s", source)
	}
}

// The call target is unique, but the package root, the simple type name and the
// method name are all already in scope. Strict output refuses the unit. Analysis
// output keeps the original owner instead of picking another method.
func TestRetainedObscuredStaticCallKeepsOriginalOwner(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	// The callers are legal only before the shadowing types exist. Compile
	// those types afterwards and put every class in one jar: the bytecode
	// target stays unique, while one compilation unit can no longer spell it.
	rounds := []map[string]string{
		{
			"cn/mailtech/utils/g.java":      "package cn.mailtech.utils;\npublic class g { public static int a(){return 2;} }\n",
			"cn/mailtech/cm/preview/g.java": "package cn.mailtech.cm.preview;\npublic class g {}\n",
			"a/n.java":                      "package a;\npublic class n { public static String a(String s, String t){return s+t;} }\n",
			"a/ct.java":                     "package a;\npublic class ct { public int a(){return 0;} public cn.mailtech.cm.preview.g use(){return null;} public int b(){return cn.mailtech.utils.g.a();} }\n",
			"module/n.java":                 "package module;\npublic class n { public String a(String s, String t){return a.n.a(s, t);} }\n",
		},
		{
			"a/cn.java":     "package a;\npublic class cn {}\n",
			"module/a.java": "package module;\npublic class a {}\n",
		},
	}
	for _, sources := range rounds {
		var paths []string
		for name, src := range sources {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(src), 0600); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, path)
		}
		if out, err := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", dir, "-d", dir}, paths...)...).CombinedOutput(); err != nil {
			t.Fatalf("compile: %v\n%s", err, out)
		}
	}
	jarPath := filepath.Join(dir, "sample.jar")
	jar, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(jar)
	for _, name := range []string{"a/cn.class", "a/ct.class", "a/n.class", "cn/mailtech/utils/g.class", "cn/mailtech/cm/preview/g.class", "module/a.class", "module/n.class"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil || jar.Close() != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ name, want string }{
		{"a/ct.class", "cn.mailtech.utils.g.a("},
		{"module/n.class", "a.n.a("},
	} {
		strict, err := NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatal(err)
		}
		got, err := strict.ReadFile(entry.name)
		strict.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "decompile dump failed") {
			t.Fatalf("strict %s was emitted:\n%s", entry.name, got)
		}
		kept, err := NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatal(err)
		}
		kept.SetRetainUnprovedSource(true)
		got, err = kept.ReadFile(entry.name)
		kept.Close()
		if err != nil {
			t.Fatal(err)
		}
		source := string(got)
		if !strings.Contains(source, entry.want) || strings.Contains(source, "decompile dump failed") || strings.Contains(source, "undecompilable method body") {
			t.Fatalf("retained %s missing %q\n%s", entry.name, entry.want, source)
		}
	}
}

func readRetained(t *testing.T, files map[string][]byte, name string, retain bool) string {
	t.Helper()
	archive := nativeArchive(t, files)
	defer archive.Close()
	if retain {
		archive.SetRetainUnprovedSource(true)
	}
	raw, err := archive.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
