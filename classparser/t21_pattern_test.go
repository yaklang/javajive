package javaclassparser

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func t21Tools(t *testing.T) (javac, java string) {
	t.Helper()
	var err error
	javac, err = exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	java, err = exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	return javac, java
}

func t21Read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "t21", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func t21CompileRun(t *testing.T, release, main string, sources map[string]string) (stdout string, classes map[string][]byte) {
	t.Helper()
	javac, java := t21Tools(t)
	srcDir := t.TempDir()
	outDir := t.TempDir()
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
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", release, "-d", outDir}, files...)
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
		classes[strings.TrimSuffix(filepath.Base(p), ".class")] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	run := exec.Command(java, "-cp", outDir, main)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", main, err, out)
	}
	return string(out), classes
}

func t21Decompile(t *testing.T, raw []byte, target int) DecompileResult {
	t.Helper()
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: target})
	if err != nil && res.Source == "" {
		t.Fatalf("decompile: %v status=%s", err, res.Status)
	}
	return res
}

func t21RebuildRun(t *testing.T, release, main string, sources map[string]string, want string) {
	t.Helper()
	got, _ := t21CompileRun(t, release, main, sources)
	if got != want {
		t.Fatalf("rebuild stdout mismatch\n got %q\nwant %q", got, want)
	}
}

func TestTaskT21C01SwitchPattern(t *testing.T) {
	t.Log("T21-C01")
	src := t21Read(t, "SwitchPattern.java")
	orig, classes := t21CompileRun(t, "21", "SwitchPattern", map[string]string{"SwitchPattern.java": src})
	raw := classes["SwitchPattern"]
	t.Logf("T21-C01 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	if orig != "nil\nlong:ab\nshort:a\nint:2\nother\n" {
		t.Fatalf("T21-C01 original stdout %q", orig)
	}
	res := t21Decompile(t, raw, 21)
	if !strings.Contains(res.Source, "case ") {
		t.Fatalf("T21-C01 missing case reconstruction:\n%s", res.Source)
	}
	if res.Status != "complete" {
		t.Fatalf("T21-C01 status=%s diag=%+v\n%s", res.Status, res.Diagnostics, res.Source)
	}
	t21RebuildRun(t, "21", "SwitchPattern", map[string]string{"SwitchPattern.java": res.Source}, orig)
}

func TestTaskT21C02PatternGuardTrace(t *testing.T) {
	t.Log("T21-C02")
	src := t21Read(t, "PatternGuardTrace.java")
	orig, classes := t21CompileRun(t, "21", "PatternGuardTrace", map[string]string{"PatternGuardTrace.java": src})
	raw := classes["PatternGuardTrace"]
	t.Logf("T21-C02 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	if !strings.Contains(orig, "GG") || !strings.HasSuffix(strings.TrimSpace(orig), "5") {
		t.Fatalf("T21-C02 original stdout %q", orig)
	}
	res := t21Decompile(t, raw, 21)
	if strings.Count(res.Source, "select(") > 2 {
		t.Fatalf("T21-C02 selector appears over-evaluated in source:\n%s", res.Source)
	}
	t21RebuildRun(t, "21", "PatternGuardTrace", map[string]string{"PatternGuardTrace.java": res.Source}, orig)
}

func TestTaskT21C03MultiGuard(t *testing.T) {
	t.Log("T21-C03")
	src := t21Read(t, "MultiGuard.java")
	orig, classes := t21CompileRun(t, "21", "MultiGuard", map[string]string{"MultiGuard.java": src})
	raw := classes["MultiGuard"]
	t.Logf("T21-C03 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	res := t21Decompile(t, raw, 21)
	t21RebuildRun(t, "21", "MultiGuard", map[string]string{"MultiGuard.java": res.Source}, orig)
}

func TestTaskT21C04GuardThrows(t *testing.T) {
	t.Log("T21-C04")
	src := t21Read(t, "GuardThrows.java")
	orig, classes := t21CompileRun(t, "21", "GuardThrows", map[string]string{"GuardThrows.java": src})
	raw := classes["GuardThrows"]
	t.Logf("T21-C04 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	res := t21Decompile(t, raw, 21)
	t21RebuildRun(t, "21", "GuardThrows", map[string]string{"GuardThrows.java": res.Source}, orig)
}

func TestTaskT21C05NoNullCase(t *testing.T) {
	t.Log("T21-C05")
	src := t21Read(t, "NoNullCase.java")
	orig, classes := t21CompileRun(t, "21", "NoNullCase", map[string]string{"NoNullCase.java": src})
	raw := classes["NoNullCase"]
	t.Logf("T21-C05 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	if !strings.Contains(orig, "null=NPE") {
		t.Fatalf("T21-C05 original null semantics %q", orig)
	}
	res := t21Decompile(t, raw, 21)
	if strings.Contains(res.Source, "case null") {
		t.Fatalf("T21-C05 must not invent case null:\n%s", res.Source)
	}
	t21RebuildRun(t, "21", "NoNullCase", map[string]string{"NoNullCase.java": res.Source}, orig)
}

func TestTaskT21C06RecordDeconstruction(t *testing.T) {
	t.Log("T21-C06")
	src := t21Read(t, "RecDecon.java")
	orig, classes := t21CompileRun(t, "21", "RecDecon", map[string]string{"RecDecon.java": src})
	raw := classes["RecDecon"]
	t.Logf("T21-C06 input sha256=%x orig=%q", sha256.Sum256(raw), orig)
	res := t21Decompile(t, raw, 21)
	if res.Status == "complete" {
		t.Fatalf("T21-C06 record deconstruction must not be complete:\n%s", res.Source)
	}
	if res.Status != "unsupported" {
		t.Fatalf("T21-C06 expected unsupported, got %s diag=%+v\n%s", res.Status, res.Diagnostics, res.Source)
	}
	if strings.Contains(res.Source, "case Pair(") {
		t.Fatalf("T21-C06 must not claim record deconstruction syntax as reconstructed:\n%s", res.Source)
	}
	keptFallback := strings.Contains(res.Source, "instanceof") || strings.Contains(res.Source, "continue") ||
		strings.Contains(res.Source, "LOOP") || strings.Contains(res.Source, "while") ||
		strings.Contains(res.Source, "unsupported") || strings.Contains(res.Source, "undecompilable")
	if !keptFallback {
		t.Fatalf("T21-C06 fallback must remain explicit, not a silent toy switch:\n%s", res.Source)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == core.DiagBootstrapPartial || d.Code == core.DiagBootstrapUnknown || strings.Contains(d.Message, "pattern") {
			found = true
		}
	}
	if !found {
		t.Fatalf("T21-C06 missing unsupported diagnostic: %+v", res.Diagnostics)
	}
}
