package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageInfoKeepsRuntimeAnnotations(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	annoDir := filepath.Join(dir, "marks")
	if err := os.MkdirAll(annoDir, 0700); err != nil {
		t.Fatal(err)
	}
	anno := filepath.Join(annoDir, "PkgMark.java")
	if err := os.WriteFile(anno, []byte(`
package marks;
import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;
@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.PACKAGE)
public @interface PkgMark {
    String namespace();
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	infoDir := filepath.Join(dir, "sample")
	if err := os.MkdirAll(infoDir, 0700); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(infoDir, "package-info.java")
	if err := os.WriteFile(info, []byte(`
@Deprecated
@marks.PkgMark(namespace="http://example.test/rm")
package sample;
`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, anno, info).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sample", "package-info.class"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package sample;",
		"@Deprecated",
		`@marks.PkgMark(namespace="http://example.test/rm")`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("package-info source missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "class package-info") {
		t.Fatalf("package-info source is not legal Java\n%s", got)
	}
	rebuilt := filepath.Join(dir, "rebuilt")
	if err := os.MkdirAll(rebuilt, 0700); err != nil {
		t.Fatal(err)
	}
	rebuiltInfo := filepath.Join(rebuilt, "package-info.java")
	if err := os.WriteFile(rebuiltInfo, []byte(got), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, rebuiltInfo).CombinedOutput(); err != nil {
		t.Fatalf("recompile decompiled package-info: %v\n%s\n%s", err, out, got)
	}
}
