package javaclassparser

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two different-package types share one simple name, and one of them cannot be
// named fully qualified because its package root is a type in scope (JLS
// 6.5.5.1). Only a single-type import can name that owner, so it must win the
// simple name even when the other owner was rendered first.
func TestCrossPackageSimpleNameClashImportsUnnameableOwner(t *testing.T) {
	javac, java := t04Tools(t)
	dir, jarPath := crossPackageSimpleNameClashJar(t)
	original := t04RunJava(t, java, dir, "a.Driver")
	if !strings.Contains(original, "false:7") {
		t.Fatalf("original oracle=%q", original)
	}
	fsys, err := NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fsys.ReadFile("a/dv.class")
	if err != nil {
		t.Fatal(err)
	}
	source := string(got)
	if strings.Contains(source, "decompile dump failed") || strings.Contains(source, "decompile parse failed") {
		t.Fatalf("static call was refused:\n%s", source)
	}
	if !strings.Contains(source, "import cn.mailtech.utils.g;") {
		t.Fatalf("unnameable owner did not take the single-type import:\n%s", source)
	}
	if strings.Contains(source, "cn.mailtech.utils.g.a(") {
		t.Fatalf("unnameable owner was written fully qualified:\n%s", source)
	}
	if !strings.Contains(source, "g.a(") {
		t.Fatalf("static call did not use the imported simple name:\n%s", source)
	}
	if !strings.Contains(source, "com.tebie.conf.g") {
		t.Fatalf("nameable owner lost its qualified spelling:\n%s", source)
	}
	rebuilt := filepath.Join(dir, "rebuilt")
	if err := os.MkdirAll(rebuilt, 0700); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(rebuilt, "dv.java")
	if err := os.WriteFile(outFile, got, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, outFile).CombinedOutput(); err != nil {
		t.Fatalf("recompile: %v\n%s\n%s", err, out, source)
	}
	gotRun := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "a.Driver")
	if gotRun != original {
		t.Fatalf("original %q regenerated %q\n%s", original, gotRun, source)
	}
}

// crossPackageSimpleNameClashJar compiles the clash fixture and packs it into
// a jar. Two packages export `g`; package root `a` is also a type, so
// `cn.mailtech.utils.g` cannot be spelled fully qualified inside `a.dv`.
func crossPackageSimpleNameClashJar(t *testing.T) (dir, jarPath string) {
	t.Helper()
	javac, _ := t04Tools(t)
	dir = t.TempDir()
	files := map[string]string{
		"a/a.java":                 "package a;\npublic class a {}\n",
		"a/cn.java":                "package a;\nclass cn {}\n",
		"cn/mailtech/utils/g.java": "package cn.mailtech.utils;\npublic class g { public static String a(String x, String y) { return x + y; } }\n",
		"com/tebie/conf/g.java":    "package com.tebie.conf;\npublic class g { public int marker() { return 7; } }\n",
		"a/dv.java":                "package a;\nimport cn.mailtech.utils.g;\npublic class dv {\n  private int use(com.tebie.conf.g value) { return value.marker(); }\n  public boolean a(String x, String y) { return g.a(x, y).isEmpty(); }\n  public int viaOther(com.tebie.conf.g value) { return use(value); }\n}\n",
		"a/Driver.java":            "package a;\npublic class Driver { public static void main(String[] args) { dv v = new dv(); System.out.println(v.a(\"x\", \"y\") + \":\" + v.viaOther(new com.tebie.conf.g())); } }\n",
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
	entries := []string{"a/a.class", "a/cn.class", "a/dv.class", "cn/mailtech/utils/g.class", "com/tebie/conf/g.class", "a/Driver.class"}
	jarPath = filepath.Join(dir, "sample.jar")
	jar, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(jar)
	for _, name := range entries {
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
	return dir, jarPath
}

// The constant-pool owner index is what lets the dumper import the unnameable
// owner. Without it the static call cannot be bound and the whole class is
// refused, so JDEC_CROSSPKG_IMPORT_OWNER_OFF must reproduce that refusal.
func TestCrossPackageImportOwnerIsLoadBearing(t *testing.T) {
	_, jarPath := crossPackageSimpleNameClashJar(t)
	read := func(t *testing.T, off bool) string {
		t.Helper()
		if off {
			t.Setenv("JDEC_CROSSPKG_IMPORT_OWNER_OFF", "1")
		} else {
			t.Setenv("JDEC_CROSSPKG_IMPORT_OWNER_OFF", "")
		}
		fsys, err := NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatal(err)
		}
		defer fsys.Close()
		raw, err := fsys.ReadFile("a/dv.class")
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	on := read(t, false)
	if strings.Contains(on, "decompile dump failed") || !strings.Contains(on, "import cn.mailtech.utils.g;") {
		t.Fatalf("switch ON lost the single-type import:\n%s", on)
	}
	off := read(t, true)
	if !strings.Contains(off, "decompile dump failed") {
		t.Fatalf("switch OFF did not restore the refusal:\n%s", off)
	}
	if off == on {
		t.Fatal("kill-switch changed nothing")
	}
}

// The owner has a unique simple name, so no clash forces an import. Its
// package root is still a type in this compilation unit, so the qualified
// spelling is illegal (JLS 6.5.5.1). The caller's own method has the same
// simple name, so a static import is also illegal. Only `import` of the owner
// type plus `g.a(...)` compiles.
func TestObscuredPackageRootImportsUniqueOwner(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	files := map[string]string{
		"a/cn.java":                "package a;\nclass cn {}\n",
		"cn/mailtech/utils/g.java": "package cn.mailtech.utils;\npublic class g { public static String a(String x, String y) { return x + y; } }\n",
		"a/ct.java":                "package a;\nimport cn.mailtech.utils.g;\npublic class ct {\n  public static String a(Object value) { return \"own\"; }\n  public String b() { return g.a(\"x\", \"y\"); }\n}\n",
		"a/Driver.java":            "package a;\npublic class Driver { public static void main(String[] args) { System.out.print(new ct().b() + \":\" + ct.a(1)); } }\n",
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
	original := t04RunJava(t, java, dir, "a.Driver")
	if original != "xy:own" {
		t.Fatalf("oracle=%q", original)
	}
	entries := []string{"a/cn.class", "a/ct.class", "cn/mailtech/utils/g.class", "a/Driver.class"}
	jarPath := filepath.Join(dir, "sample.jar")
	jar, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(jar)
	for _, name := range entries {
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
	got, err := fsys.ReadFile("a/ct.class")
	if err != nil {
		t.Fatal(err)
	}
	source := string(got)
	if strings.Contains(source, "decompile dump failed") || strings.Contains(source, "cannot be bound") {
		t.Fatalf("unique obscured owner refused:\n%s", source)
	}
	if !strings.Contains(source, "import cn.mailtech.utils.g;") || strings.Contains(source, "cn.mailtech.utils.g.a(") || !strings.Contains(source, "g.a(") {
		t.Fatalf("owner was not imported by simple name:\n%s", source)
	}
	rebuilt := filepath.Join(dir, "rebuilt")
	if err := os.MkdirAll(rebuilt, 0700); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(rebuilt, "ct.java")
	if err := os.WriteFile(outFile, got, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, outFile).CombinedOutput(); err != nil {
		t.Fatalf("recompile: %v\n%s\n%s", err, out, source)
	}
	if gotRun := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "a.Driver"); gotRun != original {
		t.Fatalf("original %q regenerated %q\n%s", original, gotRun, source)
	}
}
