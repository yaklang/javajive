package javaclassparser

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSamePackageStaticCallSurvivesPackageNameShadow(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	files := map[string]string{
		"a/a.java":     "package a;\npublic class a {}\n",
		"a/k.java":     "package a;\npublic class k { public static int a() { return 1; } }\n",
		"other/k.java": "package other;\npublic class k { public static int a() { return 2; } }\n",
		"a/l.java":     "package a;\npublic class l { public int a() { return k.a() + other.k.a(); } }\n",
	}
	var sources []string
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, path)
	}
	if out, err := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-d", dir}, sources...)...).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	jarPath := filepath.Join(dir, "sample.jar")
	jar, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(jar)
	for _, name := range []string{"a/a.class", "a/k.class", "a/l.class", "other/k.class"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := jar.Close(); err != nil {
		t.Fatal(err)
	}
	fsys, err := NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fsys.ReadFile("a/l.class")
	if err != nil {
		t.Fatal(err)
	}
	source := string(got)
	if strings.Contains(source, "decompile dump failed") || strings.Contains(source, "decompile parse failed") {
		t.Fatalf("static call was refused:\n%s", source)
	}
	for _, want := range []string{"k.a()", "other.k.a()"} {
		if !strings.Contains(source, want) {
			t.Fatalf("missing %q\n%s", want, source)
		}
	}
	if strings.Contains(source, "import other.k") || strings.Contains(source, "((a.k)null)") {
		t.Fatalf("package-shadowed type was imported or cast through an illegal name\n%s", source)
	}
	rebuilt := filepath.Join(dir, "rebuilt")
	if err := os.MkdirAll(rebuilt, 0700); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(rebuilt, "l.java")
	if err := os.WriteFile(outFile, got, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, outFile).CombinedOutput(); err != nil {
		t.Fatalf("recompile: %v\n%s\n%s", err, out, source)
	}
}
