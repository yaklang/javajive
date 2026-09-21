package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func t19Tools(t *testing.T) (javac, java string) {
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

func t19ReadSeed(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "t19", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func t19Compile(t *testing.T, release string, debug bool, sources map[string]string) (classes map[string][]byte, outDir string) {
	t.Helper()
	javac, _ := t19Tools(t)
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
	return classes, outDir
}

func t19Run(t *testing.T, cp, main string) string {
	t.Helper()
	_, java := t19Tools(t)
	run := exec.Command(java, "-cp", cp, main)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", main, err, out)
	}
	return string(out)
}

func t19DecompileAll(t *testing.T, classes map[string][]byte, target int) map[string]string {
	t.Helper()
	srcs := map[string]string{}
	for name, raw := range classes {
		if strings.Contains(name, "$") {
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
		t.Logf("decompile %s status=%s", name, res.Status)
	}
	return srcs
}

func t19RebuildRun(t *testing.T, release, main string, sources map[string]string, wantStdout string) {
	t.Helper()
	javac, java := t19Tools(t)
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
		t.Fatalf("rebuild run %s: %v\n%s", main, err, out)
	}
	if string(out) != wantStdout {
		t.Fatalf("rebuild stdout mismatch for %s\n got %q\nwant %q", main, out, wantStdout)
	}
}

func t19ReplaceUTF8(data []byte, old, neu string) []byte {
	if len(old) != len(neu) {
		panic("utf8 replacement length mismatch")
	}
	out := append([]byte(nil), data...)
	ob, nb := []byte(old), []byte(neu)
	for i := 0; i+len(ob) <= len(out); i++ {
		if string(out[i:i+len(ob)]) == old {
			copy(out[i:i+len(nb)], nb)
		}
	}
	return out
}

func TestTaskT19C01(t *testing.T) {
	t.Log("T19-C01")
	src := t19ReadSeed(t, "LambdaCapture.java")
	t.Logf("T19-C01 input sha256=%x", sha256.Sum256([]byte(src)))
	classes, origDir := t19Compile(t, "8", true, map[string]string{"LambdaCapture.java": src})
	orig := t19Run(t, origDir, "LambdaCapture")
	if !strings.Contains(orig, "13") {
		t.Fatalf("T19-C01 original stdout %q (want 13)", orig)
	}
	if strings.Contains(orig, "\n23\n") {
		t.Fatalf("T19-C01 captures swapped? %q", orig)
	}
	raw := classes["LambdaCapture"]
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil && res.Source == "" {
			t.Fatalf("T19-C01 %s: %v", mode, err)
		}
		if res.Status != "complete" && res.Status != "partial" {
			t.Fatalf("T19-C01 %s status=%s diag=%+v\n%s", mode, res.Status, res.Diagnostics, res.Source)
		}
		t19RebuildRun(t, "8", "LambdaCapture", map[string]string{"LambdaCapture": res.Source}, orig)
	}
}

func TestTaskT19C02(t *testing.T) {
	t.Log("T19-C02")
	src := t19ReadSeed(t, "MethodRefs.java")
	t.Logf("T19-C02 input sha256=%x", sha256.Sum256([]byte(src)))
	classes, origDir := t19Compile(t, "8", true, map[string]string{"MethodRefs.java": src})
	orig := t19Run(t, origDir, "MethodRefs")
	raw := classes["MethodRefs"]
	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if err != nil && res.Source == "" {
		t.Fatalf("T19-C02: %v", err)
	}
	srcOut := res.Source
	if !strings.Contains(srcOut, "::st") && !strings.Contains(srcOut, "MethodRefs::st") {
		t.Fatalf("T19-C02 missing static method ref:\n%s", srcOut)
	}
	if !strings.Contains(srcOut, "::new") {
		t.Fatalf("T19-C02 missing constructor/array ::new:\n%s", srcOut)
	}
	if !strings.Contains(srcOut, "::inst") {
		t.Fatalf("T19-C02 missing bound instance method ref:\n%s", srcOut)
	}
	if !strings.Contains(srcOut, "::length") {
		t.Fatalf("T19-C02 missing unbound instance method ref:\n%s", srcOut)
	}
	t19RebuildRun(t, "8", "MethodRefs", map[string]string{"MethodRefs": srcOut}, orig)
}

func TestTaskT19C03(t *testing.T) {
	t.Log("T19-C03")
	src := t19ReadSeed(t, "SlotReuse.java")
	t.Logf("T19-C03 input sha256=%x", sha256.Sum256([]byte(src)))
	for _, debug := range []bool{true, false} {
		label := "debug"
		if !debug {
			label = "no-debug"
		}
		classes, origDir := t19Compile(t, "8", debug, map[string]string{"SlotReuse.java": src})
		orig := t19Run(t, origDir, "SlotReuse")
		if !strings.Contains(orig, "6") || !strings.Contains(orig, "reused-slot") {
			t.Fatalf("T19-C03 %s original %q", label, orig)
		}
		raw := classes["SlotReuse"]
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
		if err != nil && res.Source == "" {
			t.Fatalf("T19-C03 %s: %v", label, err)
		}
		if strings.Contains(res.Source, "reused-slot") && strings.Contains(res.Source, "* reused") {
			t.Fatalf("T19-C03 %s capture re-read later slot:\n%s", label, res.Source)
		}
		t19RebuildRun(t, "8", "SlotReuse", map[string]string{"SlotReuse": res.Source}, orig)
	}
}

func TestTaskT19C04Integration(t *testing.T) {
	t.Log("T19-C04")
	serSrc := t19ReadSeed(t, "SerializableLambda.java")
	serClasses, _ := t19Compile(t, "8", true, map[string]string{"SerializableLambda.java": serSrc})
	serRes, _ := DecompileWithOptions(serClasses["SerializableLambda"], DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if serRes.Status != "unsupported" {
		t.Fatalf("T19-C04 SERIALIZABLE status=%s diag=%+v\n%s", serRes.Status, serRes.Diagnostics, serRes.Source)
	}
	found := false
	for _, d := range serRes.Diagnostics {
		if strings.Contains(strings.ToLower(d.Message), "serializable") || d.Code == core.DiagBootstrapUnknown {
			found = true
		}
	}
	if !found {
		t.Fatalf("T19-C04 SERIALIZABLE silently dropped: diag=%+v source=%s", serRes.Diagnostics, serRes.Source)
	}

	markerSources := map[string]string{
		"Marker.java":       t19ReadSeed(t, "Marker.java"),
		"MarkerLambda.java": t19ReadSeed(t, "MarkerLambda.java"),
	}
	classes, origDir := t19Compile(t, "8", true, markerSources)
	orig := t19Run(t, origDir, "MarkerLambda")
	srcs := t19DecompileAll(t, classes, 8)
	if srcs["MarkerLambda"] == "" {
		t.Fatal("T19-C04 MarkerLambda decompile empty")
	}
	if srcs["Marker"] == "" {
		t.Fatal("T19-C04 must rebuild Marker")
	}
	t19RebuildRun(t, "8", "MarkerLambda", srcs, orig)
}

func TestTaskT19C05(t *testing.T) {
	t.Log("T19-C05")
	t.Parallel()
	alphaSrc := t19ReadSeed(t, "NestedAlpha.java")
	betaSrc := t19ReadSeed(t, "NestedBeta.java")
	alphaClasses, alphaDir := t19Compile(t, "8", true, map[string]string{"NestedAlpha.java": alphaSrc})
	betaClasses, betaDir := t19Compile(t, "8", true, map[string]string{"NestedBeta.java": betaSrc})
	alphaOrig := t19Run(t, alphaDir, "NestedAlpha")
	betaOrig := t19Run(t, betaDir, "NestedBeta")
	alphaRaw := alphaClasses["NestedAlpha"]
	betaRaw := betaClasses["NestedBeta"]

	var wg sync.WaitGroup
	errCh := make(chan string, 16)
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			res, err := DecompileWithOptions(alphaRaw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
			if err != nil && res.Source == "" {
				errCh <- fmt.Sprintf("alpha decompile: %v", err)
				return
			}
			if strings.Contains(res.Source, "betaMethod") || strings.Contains(res.Source, "betaVar") {
				errCh <- "alpha source crossed with NestedBeta names:\n" + res.Source
				return
			}
			if !strings.Contains(res.Source, "alphaMethod") && !strings.Contains(res.Source, "alphaVar") {
				errCh <- "alpha source lost NestedAlpha names:\n" + res.Source
			}
		}()
		go func() {
			defer wg.Done()
			res, err := DecompileWithOptions(betaRaw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
			if err != nil && res.Source == "" {
				errCh <- fmt.Sprintf("beta decompile: %v", err)
				return
			}
			if strings.Contains(res.Source, "alphaMethod") || strings.Contains(res.Source, "alphaVar") {
				errCh <- "beta source crossed with NestedAlpha names:\n" + res.Source
				return
			}
			if !strings.Contains(res.Source, "betaMethod") && !strings.Contains(res.Source, "betaVar") {
				errCh <- "beta source lost NestedBeta names:\n" + res.Source
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Error(e)
	}

	aRes, _ := DecompileWithOptions(alphaRaw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	bRes, _ := DecompileWithOptions(betaRaw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	t19RebuildRun(t, "8", "NestedAlpha", map[string]string{"NestedAlpha": aRes.Source}, alphaOrig)
	t19RebuildRun(t, "8", "NestedBeta", map[string]string{"NestedBeta": bRes.Source}, betaOrig)
}

func TestTaskT19C06(t *testing.T) {
	t.Log("T19-C06")
	src := t19ReadSeed(t, "LambdaCapture.java")
	classes, origDir := t19Compile(t, "8", true, map[string]string{"LambdaCapture.java": src})
	orig := t19Run(t, origDir, "LambdaCapture")
	raw := classes["LambdaCapture"]
	mut := t19ReplaceUTF8(raw, "plus", "plux")
	if string(mut) == string(raw) {
		t.Fatal("T19-C06 failed to rename plus utf8")
	}
	res, err := DecompileWithOptions(mut, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if err != nil && res.Source == "" {
		t.Fatalf("T19-C06: %v", err)
	}
	if strings.Contains(res.Source, "::plus") && !strings.Contains(res.Source, "::plux") {
		t.Fatalf("T19-C06 still bound to old name plus (overfit on lambda$/plus):\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "plux") {
		t.Fatalf("T19-C06 handle identity did not keep renamed impl:\n%s", res.Source)
	}
	t19RebuildRun(t, "8", "LambdaCapture", map[string]string{"LambdaCapture": res.Source}, orig)
}
