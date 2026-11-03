package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Dropping EnclosingMethod is not permission to invent <T> with an Object
// erasure. The source must recompile and keep the physical Bound descriptors
// and behavior, even when the missing method shadows a Number-bound class T.
func TestMissingEnclosingMethodPreservesPhysicalDescriptors(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, flattenedMethodBindingFixture, debug)
			const child = "ErasureCaptureOwner$1.class"
			obj, err := Parse(files[child])
			if err != nil {
				t.Fatal(err)
			}
			var kept []AttributeInfo
			removed := 0
			for _, a := range obj.Attributes {
				if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" {
					removed++
					continue
				}
				kept = append(kept, a)
			}
			if removed != 1 {
				t.Fatalf("EnclosingMethod count=%d", removed)
			}
			obj.Attributes = kept
			files[child] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "ErasureCaptureDriver")
			if want != "10:lexical:method:erasure\n" {
				t.Fatalf("oracle=%q", want)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			raw, err := archive.ReadFile(child)
			if err != nil {
				t.Fatal(err)
			}
			source := string(raw)
			if strings.Contains(source, "decompile dump failed") || strings.Contains(source, "decompile parse failed") || strings.Contains(source, "undecompilable method body") {
				t.Fatalf("descriptor projection failed:\n%s", source)
			}
			if strings.Contains(source, "<T>") || strings.Contains(source, "T extends Number") {
				t.Fatalf("invented bound:\n%s", source)
			}
			rebuilt := t.TempDir()
			path := filepath.Join(rebuilt, "ErasureCaptureOwner$1.java")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, path).CombinedOutput(); err != nil {
				t.Fatalf("recompile: %v\n%s\n%s", err, out, source)
			}
			got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "ErasureCaptureDriver")
			if got != want {
				t.Fatalf("original=%q rebuilt=%q\n%s", want, got, source)
			}
			// The preparation projection must not mutate the archived original.
			if !bytes.Equal(obj.Bytes(), files[child]) {
				t.Fatal("original class mutated")
			}
		})
	}
}

// method_index=0 does not prove an unbounded type parameter. A class whose
// constructor was removed also cannot be emitted with a default constructor
// assigning none of its blank finals. Keep refusal until there is a proof-led
// representation; syntactically parsable text alone is not sufficient.
func TestZeroMethodIndexWithoutConstructorDoesNotInventTypeNames(t *testing.T) {
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
	archive := nativeArchive(t, files)
	defer archive.Close()
	raw, err := archive.ReadFile("ErasureCaptureOwner$1.class")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "unproven original enclosing method type scope") {
		t.Fatalf("uncertified class emitted:\n%s", raw)
	}
}
