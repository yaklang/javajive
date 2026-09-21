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

func t20Tools(t *testing.T) (javac, java string) {
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

func t20Read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "t20", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func t20CompileRun(t *testing.T, release, main string, sources map[string]string) (stdout string, classes map[string][]byte) {
	t.Helper()
	javac, java := t20Tools(t)
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
		rel, _ := filepath.Rel(outDir, p)
		classes[strings.TrimSuffix(rel, ".class")] = b
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

func t20Decompile(t *testing.T, raw []byte, target int) DecompileResult {
	t.Helper()
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: target})
	if err != nil && res.Source == "" {
		t.Fatalf("decompile: %v status=%s", err, res.Status)
	}
	return res
}

func t20RebuildRun(t *testing.T, release, main string, sources map[string]string, want string) {
	t.Helper()
	got, _ := t20CompileRun(t, release, main, sources)
	if got != want {
		t.Fatalf("rebuild stdout mismatch\n got %q\nwant %q", got, want)
	}
}

func TestTaskT20C01RecordProbe(t *testing.T) {
	t.Log("T20-C01")
	src := t20Read(t, "RecordProbe.java")
	orig, classes := t20CompileRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": src})
	raw := classes["RecordProbe"]
	if raw == nil {
		t.Fatal("missing RecordProbe.class")
	}
	t.Logf("T20-C01 input sha256=%x", sha256.Sum256(raw))
	res := t20Decompile(t, raw, 17)
	if !strings.Contains(res.Source, "record ") {
		t.Fatalf("T20-C01 expected record keyword, status=%s\n%s", res.Status, res.Source)
	}
	if strings.Contains(res.Source, "extends Record") {
		t.Fatalf("T20-C01 must not emit class extends Record as success:\n%s", res.Source)
	}
	if res.Status != "complete" {
		t.Fatalf("T20-C01 status=%s diag=%+v\n%s", res.Status, res.Diagnostics, res.Source)
	}
	t20RebuildRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": res.Source}, orig)
}

func TestTaskT20C02RecordCustom(t *testing.T) {
	t.Log("T20-C02")
	src := t20Read(t, "RecordCustom.java")
	orig, classes := t20CompileRun(t, "17", "RecordCustom", map[string]string{"RecordCustom.java": src})
	raw := classes["RecordCustom"]
	t.Logf("T20-C02 input sha256=%x", sha256.Sum256(raw))
	res := t20Decompile(t, raw, 17)
	if !strings.Contains(res.Source, "record ") {
		t.Fatalf("T20-C02 expected record:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "CUSTOM:") && !strings.Contains(res.Source, "\"CUSTOM:\"") {
		t.Fatalf("T20-C02 custom toString must be kept:\n%s", res.Source)
	}
	if strings.Contains(res.Source, `Rec[`) || strings.Contains(res.Source, "RecordCustom[x=") {
		t.Fatalf("T20-C02 must not replace custom toString with ObjectMethods:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "nil") {
		t.Fatalf("T20-C02 compact ctor null-normalize missing:\n%s", res.Source)
	}
	t20RebuildRun(t, "17", "RecordCustom", map[string]string{"RecordCustom.java": res.Source}, orig)
}

func TestTaskT20C03GenericComponents(t *testing.T) {
	t.Log("T20-C03")
	sources := map[string]string{
		"Ann.java":        t20Read(t, "Ann.java"),
		"GenericBox.java": t20Read(t, "GenericBox.java"),
	}
	orig, classes := t20CompileRun(t, "17", "GenericBox", sources)
	t.Logf("T20-C03 GenericBox sha256=%x Ann sha256=%x", sha256.Sum256(classes["GenericBox"]), sha256.Sum256(classes["Ann"]))
	rebuilt := map[string]string{}
	for name, raw := range classes {
		if strings.Contains(name, "/") || strings.Contains(name, "$") {
			continue
		}
		res := t20Decompile(t, raw, 17)
		if name == "GenericBox" {
			if !strings.Contains(res.Source, "record ") {
				t.Fatalf("T20-C03 GenericBox not a record:\n%s", res.Source)
			}
			if !strings.Contains(res.Source, "@Ann") && !strings.Contains(res.Source, "Ann(") {
				t.Fatalf("T20-C03 missing component annotation:\n%s", res.Source)
			}
			if !strings.Contains(res.Source, "<T>") && !strings.Contains(res.Source, "<T ") {
				t.Fatalf("T20-C03 missing generic type param:\n%s", res.Source)
			}
		}
		rebuilt[name+".java"] = res.Source
	}
	if len(rebuilt) < 2 {
		t.Fatalf("T20-C03 expected to rebuild all app classes, got %v", rebuilt)
	}
	t20RebuildRun(t, "17", "GenericBox", rebuilt, orig)
}

func TestTaskT20C04SpecialValues(t *testing.T) {
	t.Log("T20-C04")
	src := t20Read(t, "SpecialValues.java")
	orig, classes := t20CompileRun(t, "17", "SpecialValues", map[string]string{"SpecialValues.java": src})
	if !strings.Contains(orig, "nanEquals=") || !strings.Contains(orig, "signedZeroEquals=false") {
		t.Fatalf("T20-C04 original stdout: %q", orig)
	}
	if strings.Contains(orig, "arrContent=true") && strings.Contains(orig, "deep") {
		t.Fatalf("T20-C04 original used deepEquals: %q", orig)
	}
	raw := classes["SpecialValues"]
	t.Logf("T20-C04 input sha256=%x", sha256.Sum256(raw))
	res := t20Decompile(t, raw, 17)
	if strings.Contains(res.Source, "deepEquals") {
		t.Fatalf("T20-C04 must not use Arrays.deepEquals:\n%s", res.Source)
	}
	t20RebuildRun(t, "17", "SpecialValues", map[string]string{"SpecialValues.java": res.Source}, orig)
}

func TestTaskT20C05NeighborAndFakeRecord(t *testing.T) {
	t.Log("T20-C05")
	src := t20Read(t, "RecordProbe.java")
	_, classes := t20CompileRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": src})
	raw := classes["RecordProbe"]
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	filtered := make([]AttributeInfo, 0, len(obj.Attributes))
	for _, a := range obj.Attributes {
		if u, ok := a.(*UnparsedAttribute); ok && u != nil && u.Name == "Record" {
			continue
		}
		filtered = append(filtered, a)
	}
	obj.Attributes = filtered
	stripped := obj.Bytes()
	t.Logf("T20-C05 stripped-Record sha256=%x", sha256.Sum256(stripped))
	res := t20Decompile(t, stripped, 17)
	if strings.Contains(res.Source, "record RecordProbe") {
		t.Fatalf("T20-C05 class without Record attribute must not be emitted as record:\n%s", res.Source)
	}
}

func TestTaskT20C06LowTarget(t *testing.T) {
	t.Log("T20-C06")
	src := t20Read(t, "RecordProbe.java")
	_, classes := t20CompileRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": src})
	raw := classes["RecordProbe"]
	t.Logf("T20-C06 input sha256=%x", sha256.Sum256(raw))
	res := t20Decompile(t, raw, 8)
	if res.Status == "complete" {
		t.Fatalf("T20-C06 target 8 must not be complete:\n%s", res.Source)
	}
	if res.Status != "unsupported" {
		t.Fatalf("T20-C06 expected unsupported, got %s diag=%+v", res.Status, res.Diagnostics)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == core.DiagBootstrapVersion || strings.Contains(d.Message, "cannot be lossless") {
			found = true
		}
	}
	if !found {
		t.Fatalf("T20-C06 missing version diagnostic: %+v", res.Diagnostics)
	}
}
