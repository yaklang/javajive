package javaclassparser

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
)

func declarationTestArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, raw := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExpandedLooseClassDecompiles(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "Loose.java")
	if err := os.WriteFile(file, []byte(`package sample; public class Loose { public int run() { return 7; } }`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	fsys := NewExpandedLocalFileSystem()
	name := filepath.Join(dir, "sample", "Loose.class")
	beforeScratch := classDumpScratchCount()
	raw, err := fsys.ReadFile(name)
	if got := classDumpScratchCount(); got != beforeScratch {
		t.Fatalf("DumpClass retained reconstruction scratch: before=%d after=%d", beforeScratch, got)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("class Loose")) || !bytes.Contains(raw, []byte("return 7")) {
		t.Fatalf("loose class was not converted to Java: %q", raw[:min(120, len(raw))])
	}
	classBytes, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	base := filesys.NewVirtualFs()
	base.AddFile("sample/Loose.class", string(classBytes))
	wrapped := MaybeWrapExpandedArchiveFS(base, true)
	prepared, err := wrapped.ReadFile("sample/Loose.class")
	if err != nil || !bytes.Equal(prepared, raw) {
		t.Fatalf("class-only project was not wrapped: %v\n%q", err, prepared)
	}
	opened, err := fsys.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	fromOpen, err := io.ReadAll(opened)
	if err != nil || !bytes.Equal(fromOpen, raw) {
		t.Fatalf("Open differs from ReadFile: %v", err)
	}
}

// A directory source has no container archive, so ExpandedZipFS must take the
// directory branch. 9012988 restored decompilation there after an earlier
// change returned raw bytes; this witness locks both ReadFile and Open without
// requiring a JDK.
func TestExpandedDirectoryClassReadDecompiles(t *testing.T) {
	seed, err := os.ReadFile(filepath.Join("testdata", "regression", "SignatureAttribute.class"))
	if err != nil {
		t.Fatal(err)
	}
	base := filesys.NewVirtualFs()
	base.AddFile("sample/Subject.class", string(seed))
	wrapped := MaybeWrapExpandedArchiveFS(base, false)
	fs, ok := wrapped.(*ExpandedZipFS)
	if !ok {
		t.Fatalf("class-only directory was not wrapped: %T", wrapped)
	}
	src, err := fs.ReadFile("sample/Subject.class")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(src, []byte("class SignatureAttribute")) || bytes.Contains(src, []byte("decompile dump failed")) {
		t.Fatalf("directory class was not decompiled: %q", src[:min(120, len(src))])
	}
	opened, err := fs.Open("sample/Subject.class")
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	fromOpen, err := io.ReadAll(opened)
	if err != nil || !bytes.Equal(fromOpen, src) {
		t.Fatalf("Open differs from ReadFile: %v", err)
	}
}

// A dependency in WEB-INF/lib belongs to this WAR's declaration namespace,
// not to every unrelated application next to it. The target itself always wins.
func TestExpandedWarSiblingDeclarations(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	source := `class ScopeParent { public static RuntimeException jdec$rethrow$0(Throwable t){return new IllegalStateException(t);} }
class ScopeActions { static Object action() throws java.io.IOException { throw new java.io.IOException("original"); } }
public class ScopeOwner extends ScopeParent { Object invoke() throws java.io.IOException { return ScopeActions.action(); } public static void main(String[] args) { try { new ScopeOwner().invoke(); } catch (Throwable caught) { System.out.println(caught.getClass().getName()+":"+caught.getMessage()); } } }`
	file := filepath.Join(dir, "ScopeOwner.java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	classes := classMapFromDir(t, dir)
	owner, err := Parse(classes["ScopeOwner"])
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range owner.Methods {
		name, _ := owner.getUtf8(m.NameIndex)
		if name != "invoke" {
			continue
		}
		var attrs []AttributeInfo
		for _, a := range m.Attributes {
			if _, ok := a.(*ExceptionsAttribute); !ok {
				attrs = append(attrs, a)
			}
		}
		m.Attributes = attrs
	}
	target := declarationTestArchive(t, map[string][]byte{"ScopeOwner.class": owner.Bytes(), "ScopeActions.class": classes["ScopeActions"]})
	deps := declarationTestArchive(t, map[string][]byte{"ScopeParent.class": classes["ScopeParent"]})
	war := declarationTestArchive(t, map[string][]byte{"WEB-INF/lib/target.jar": target, "WEB-INF/lib/deps.jar": deps})
	warPath := filepath.Join(dir, "app.war")
	if err := os.WriteFile(warPath, war, 0600); err != nil {
		t.Fatal(err)
	}
	fsys := NewExpandedLocalFileSystem()
	got, err := fsys.ReadFile(warPath + "/WEB-INF/lib/target.jar/ScopeOwner.java")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "undecompilable method body") {
		t.Fatalf("sibling declaration was not supplied:\n%s", got)
	}
	if !strings.Contains(string(got), "ScopeActions.action()") {
		t.Fatalf("original call disappeared:\n%s", got)
	}
	// Recompilation and exception identity/message are stronger than accepting
	// a syntactically legal stub or a helper that selects an inherited decoy.
	rebuilt := t.TempDir()
	generated := filepath.Join(rebuilt, "ScopeOwner.java")
	if err := os.WriteFile(generated, got, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, generated).CombinedOutput(); err != nil {
		t.Fatalf("rebuild: %v\n%s\n%s", err, out, got)
	}
	if want, observed := t04RunJava(t, java, dir, "ScopeOwner"), t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "ScopeOwner"); observed != want || want != "java.io.IOException:original\n" {
		t.Fatalf("original=%q rebuilt=%q", want, observed)
	}
	expanded := fsys.(*ExpandedZipFS)
	if expanded.jarCache.Count() == 0 || len(expanded.declarations) == 0 {
		t.Fatal("fixture did not populate caches")
	}
	expanded.ReleasePreparationCaches()
	if expanded.jarCache.Count() != 0 || expanded.zipCache.Count() != 0 || len(expanded.declarations) != 0 {
		t.Fatal("prepare graphs survived release")
	}
	again, err := fsys.ReadFile(warPath + "/WEB-INF/lib/target.jar/ScopeOwner.java")
	if err != nil || !bytes.Equal(again, got) {
		t.Fatalf("cache release changed source: %v\n%s", err, again)
	}
	// Identical duplicates are admissible; incompatible class declarations are
	// unknown, independent of ZIP order. Never pick a parent by first hit.
	for _, same := range []bool{true, false} {
		parent := append([]byte(nil), classes["ScopeParent"]...)
		if !same {
			parent = bytes.ReplaceAll(parent, []byte("jdec$rethrow$0"), []byte("jdec$rethrow$1"))
		}
		duplicateWar := declarationTestArchive(t, map[string][]byte{
			"WEB-INF/lib/target.jar": target, "WEB-INF/lib/deps.jar": deps,
			"WEB-INF/lib/duplicate.jar": declarationTestArchive(t, map[string][]byte{"ScopeParent.class": parent}),
		})
		name := filepath.Join(dir, "conflicting.war")
		if same {
			name = filepath.Join(dir, "identical.war")
		}
		if err := os.WriteFile(name, duplicateWar, 0600); err != nil {
			t.Fatal(err)
		}
		duplicate, err := fsys.ReadFile(name + "/WEB-INF/lib/target.jar/ScopeOwner.java")
		if err != nil {
			t.Fatal(err)
		}
		stub := strings.Contains(string(duplicate), "complete inherited member names")
		if stub == same {
			t.Fatalf("same=%v stub=%v:\n%s", same, stub, duplicate)
		}
	}
	missingWar := declarationTestArchive(t, map[string][]byte{"WEB-INF/lib/target.jar": target, "WEB-INF/lib/actions.jar": declarationTestArchive(t, map[string][]byte{"ScopeActions.class": classes["ScopeActions"]})})
	missingPath := filepath.Join(dir, "missing.war")
	if err := os.WriteFile(missingPath, missingWar, 0600); err != nil {
		t.Fatal(err)
	}
	missing, err := fsys.ReadFile(missingPath + "/WEB-INF/lib/target.jar/ScopeOwner.java")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(missing), "complete inherited member names") {
		t.Fatalf("another WAR supplied missing declarations:\n%s", missing)
	}
}
