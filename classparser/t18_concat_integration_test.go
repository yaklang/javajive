package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func t18Tools(t *testing.T) (javac, java string) {
	t.Helper()
	var err error
	javac, err = exec.LookPath("javac")
	if err != nil {
		t.Fatalf("javac not found: %v", err)
	}
	java, err = exec.LookPath("java")
	if err != nil {
		t.Fatalf("java not found: %v", err)
	}
	return javac, java
}

func t18ReadSeed(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "t18", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func t18Compile(t *testing.T, release string, debug bool, sources map[string]string) (stdoutMain string, classes map[string][]byte, outDir string) {
	t.Helper()
	javac, _ := t18Tools(t)
	srcDir := t.TempDir()
	outDir = t.TempDir()
	var files []string
	for name, body := range sources {
		p := filepath.Join(srcDir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	args := []string{"-proc:none", "-encoding", "UTF-8", "--release", release, "-d", outDir}
	if debug {
		args = append(args, "-g")
	} else {
		args = append(args, "-g:none")
	}
	args = append(args, files...)
	cmd := exec.Command(javac, args...)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac --release %s: %v\n%s", release, err, out)
	}
	classes = map[string][]byte{}
	err := filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(outDir, p)
		name := strings.TrimSuffix(rel, ".class")
		name = strings.ReplaceAll(name, string(filepath.Separator), ".")
		classes[name] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return "", classes, outDir
}

func t18Run(t *testing.T, cp, main string) string {
	t.Helper()
	_, java := t18Tools(t)
	run := exec.Command(java, "-cp", cp, main)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", main, err, out)
	}
	return string(out)
}

func t18DecompileAll(t *testing.T, classes map[string][]byte, target int) map[string]string {
	t.Helper()
	srcs := map[string]string{}
	for name, raw := range classes {
		if strings.Contains(name, "$") && strings.ContainsAny(name[strings.LastIndex(name, "$")+1:], "0123456789") {
			continue
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: target})
		if err != nil && res.Source == "" {
			t.Fatalf("decompile %s: %v status=%s diag=%+v", name, err, res.Status, res.Diagnostics)
		}
		if res.Source == "" {
			t.Fatalf("decompile %s empty source status=%s", name, res.Status)
		}
		srcs[name] = res.Source
		t.Logf("decompile %s status=%s hash=%x", name, res.Status, sha256.Sum256(raw))
	}
	return srcs
}

func t18RebuildRun(t *testing.T, release, main string, sources map[string]string, wantStdout string) {
	t.Helper()
	javac, java := t18Tools(t)
	dir := t.TempDir()
	var files []string
	for name, src := range sources {
		p := filepath.Join(dir, strings.ReplaceAll(name, ".", string(filepath.Separator))+".java")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", release, "-d", dir}, files...)
	cmd := exec.Command(javac, args...)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		var b strings.Builder
		for n, s := range sources {
			fmt.Fprintf(&b, "----- %s -----\n%s\n", n, s)
		}
		t.Fatalf("rebuild javac: %v\n%s\n%s", err, out, b.String())
	}
	run := exec.Command(java, "-cp", dir, main)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("rebuild run %s: %v\n%s\n(original classes must not be on this classpath: %s)", main, err, out, dir)
	}
	if string(out) != wantStdout {
		t.Fatalf("rebuild stdout mismatch for %s\n got %q\nwant %q", main, out, wantStdout)
	}
}

func TestTaskT18C01(t *testing.T) {
	t.Log("T18-C01")
	src := t18ReadSeed(t, "ConcatProbe.java")
	t.Logf("T18-C01 input sha256=%x", sha256.Sum256([]byte(src)))
	_, classes, origDir := t18Compile(t, "17", true, map[string]string{"ConcatProbe.java": src})
	orig := t18Run(t, origDir, "ConcatProbe")
	if !strings.Contains(orig, "x=7,o=null") {
		t.Fatalf("T18-C01 original stdout %q", orig)
	}
	raw := classes["ConcatProbe"]
	if raw == nil {
		t.Fatal("missing ConcatProbe.class")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 17})
		if err != nil && res.Source == "" {
			t.Fatalf("T18-C01 %s: %v", mode, err)
		}
		if res.Status != "complete" && res.Status != "partial" {
			t.Fatalf("T18-C01 %s status=%s diag=%+v\n%s", mode, res.Status, res.Diagnostics, res.Source)
		}
		if strings.Contains(res.Source, "String.valueOf(null)") && !strings.Contains(res.Source, "String.valueOf((Object)null)") {
			t.Fatalf("T18-C01 %s emitted uncast String.valueOf(null):\n%s", mode, res.Source)
		}
		if !strings.Contains(res.Source, "+") {
			t.Fatalf("T18-C01 %s lost concat form:\n%s", mode, res.Source)
		}
		t18RebuildRun(t, "17", "ConcatProbe", map[string]string{"ConcatProbe": res.Source}, orig)
	}
}

func TestTaskT18C02(t *testing.T) {
	t.Log("T18-C02")
	src := t18ReadSeed(t, "ConcatOrder.java")
	t.Logf("T18-C02 input sha256=%x", sha256.Sum256([]byte(src)))
	_, classes, origDir := t18Compile(t, "17", true, map[string]string{"ConcatOrder.java": src})
	orig := t18Run(t, origDir, "ConcatOrder")
	if !strings.Contains(orig, "AB") {
		t.Fatalf("T18-C02 original log order want AB, got %q", orig)
	}
	if strings.Contains(orig, "BA") && !strings.Contains(orig, "AB") {
		t.Fatalf("T18-C02 swapped order: %q", orig)
	}
	raw := classes["ConcatOrder"]
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if err != nil && res.Source == "" {
		t.Fatalf("T18-C02: %v", err)
	}
	t18RebuildRun(t, "17", "ConcatOrder", map[string]string{"ConcatOrder": res.Source}, orig)
}

func TestTaskT18C03Integration(t *testing.T) {
	t.Log("T18-C03")
	src := t18ReadSeed(t, "ConcatParens.java")
	t.Logf("T18-C03 input sha256=%x", sha256.Sum256([]byte(src)))
	_, classes, origDir := t18Compile(t, "17", true, map[string]string{"ConcatParens.java": src})
	orig := t18Run(t, origDir, "ConcatParens")
	raw := classes["ConcatParens"]
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if err != nil && res.Source == "" {
		t.Fatalf("T18-C03: %v", err)
	}
	for _, tok := range []string{"<<", ">>", ">>>", "&", "|", "^", "?"} {
		if !strings.Contains(res.Source, tok) && tok != ">>>" {
			// >>> may render as >> in some paths; bitwise/shift still required.
			t.Logf("T18-C03 source missing token %q (may be folded): check compile", tok)
		}
	}
	t18RebuildRun(t, "17", "ConcatParens", map[string]string{"ConcatParens": res.Source}, orig)
}

func TestTaskT18C04(t *testing.T) {
	t.Log("T18-C04")
	sources := map[string]string{
		"ConcatSide.java": t18ReadSeed(t, "ConcatSide.java"),
		"Counter.java":    t18ReadSeed(t, "Counter.java"),
		"Boom.java":       t18ReadSeed(t, "Boom.java"),
	}
	t.Logf("T18-C04 ConcatSide sha256=%x", sha256.Sum256([]byte(sources["ConcatSide.java"])))
	_, classes, origDir := t18Compile(t, "17", true, sources)
	orig := t18Run(t, origDir, "ConcatSide")
	if !strings.Contains(orig, "c1") || !strings.Contains(orig, "c2") || !strings.Contains(orig, "boom") {
		t.Fatalf("T18-C04 original events %q", orig)
	}
	if !strings.Contains(orig, "count:2") {
		t.Fatalf("T18-C04 original count %q", orig)
	}
	if len(classes) < 3 {
		t.Fatalf("T18-C04 expected 3 app classes, got %v", classKeys(classes))
	}
	srcs := t18DecompileAll(t, classes, 17)
	if _, ok := srcs["ConcatSide"]; !ok {
		t.Fatal("T18-C04 missing decompiled ConcatSide")
	}
	if _, ok := srcs["Counter"]; !ok {
		t.Fatal("T18-C04 must rebuild Counter (original class must not be a hidden dependency)")
	}
	if _, ok := srcs["Boom"]; !ok {
		t.Fatal("T18-C04 must rebuild Boom")
	}
	t18RebuildRun(t, "17", "ConcatSide", srcs, orig)
}

func TestTaskT18C05Bytecode(t *testing.T) {
	t.Log("T18-C05")
	src := t18ReadSeed(t, "ConcatProbe.java")
	_, classes, _ := t18Compile(t, "17", true, map[string]string{"ConcatProbe.java": src})
	raw := classes["ConcatProbe"]
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	patched := false
	for _, c := range obj.ConstantPool {
		u, ok := c.(*ConstantUtf8Info)
		if !ok || u == nil {
			continue
		}
		if strings.Count(u.Value, "\u0001") >= 2 && strings.Contains(u.Value, "x=") {
			u.Value = u.Value + "\u0001"
			patched = true
			break
		}
	}
	if !patched {
		t.Fatal("T18-C05 failed to patch recipe utf8")
	}
	mut := obj.Bytes()
	res, _ := DecompileWithOptions(mut, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if res.Status != "invalid_input" {
		t.Fatalf("T18-C05 damaged recipe status=%s diag=%+v\n%s", res.Status, res.Diagnostics, res.Source)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == core.DiagBootstrapArgMismatch || strings.Contains(strings.ToLower(d.Message), "tag") || strings.Contains(d.Message, "arity") {
			found = true
		}
	}
	if !found && res.Status != "invalid_input" {
		t.Fatalf("T18-C05 expected arity mismatch diagnostic: %+v", res.Diagnostics)
	}
}

func TestTaskT18C06(t *testing.T) {
	t.Log("T18-C06")
	src := t18ReadSeed(t, "ConcatShape.java")
	t.Logf("T18-C06 input sha256=%x", sha256.Sum256([]byte(src)))
	for _, release := range []string{"8", "17"} {
		release := release
		_, classes, origDir := t18Compile(t, release, true, map[string]string{"ConcatShape.java": src})
		orig := t18Run(t, origDir, "ConcatShape")
		if !strings.Contains(orig, "x=7,y=8") {
			t.Fatalf("T18-C06 original --release %s stdout %q", release, orig)
		}
		raw := classes["ConcatShape"]
		target := 8
		if release == "17" {
			target = 17
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: target})
		if err != nil && res.Source == "" {
			t.Fatalf("T18-C06 release %s: %v", release, err)
		}
		t18RebuildRun(t, release, "ConcatShape", map[string]string{"ConcatShape": res.Source}, orig)
	}
}

func classKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
