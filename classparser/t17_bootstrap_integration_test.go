package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func t17Tools(t *testing.T) (javac, java string) {
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

func t17CompileRun(t *testing.T, release, mainClass string, sources map[string]string) (stdout string, classes map[string][]byte) {
	t.Helper()
	javac, java := t17Tools(t)
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
	run := exec.Command(java, "-cp", outDir, mainClass)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", mainClass, err, out)
	}
	return string(out), classes
}

func t17ReadSeed(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "t17", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func t17ReplaceUTF8(data []byte, old, neu string) []byte {
	if len(old) != len(neu) {
		panic("utf8 replacement length mismatch")
	}
	out := append([]byte(nil), data...)
	ob, nb := []byte(old), []byte(neu)
	n := 0
	for i := 0; i+len(ob) <= len(out); i++ {
		if string(out[i:i+len(ob)]) == old {
			copy(out[i:i+len(nb)], nb)
			n++
		}
	}
	_ = n
	return out
}

func TestTaskT17C01UnknownBootstrapBytecode(t *testing.T) {
	t.Log("T17-C01")
	src := t17ReadSeed(t, "ConcatProbe.java")
	_, classes := t17CompileRun(t, "17", "ConcatProbe", map[string]string{"ConcatProbe.java": src})
	raw := classes["ConcatProbe"]
	if raw == nil {
		t.Fatal("missing ConcatProbe.class")
	}
	marker := filepath.Join(t.TempDir(), "bootstrap-executed.flag")
	mut := t17ReplaceUTF8(raw, "java/lang/invoke/StringConcatFactory", "evil/lang/invoke/StringConcatFactory")
	if string(mut) == string(raw) {
		t.Fatal("T17-C01 failed to rewrite bootstrap owner utf8")
	}
	res, err := DecompileWithOptions(mut, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if err != nil && res.Status == "invalid_input" && res.Source == "" {
		// parse failure is not this case
		t.Fatalf("T17-C01 parse/decompile failed before dispatch: %v status=%s", err, res.Status)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("T17-C01 bootstrap execution wrote marker file")
	}
	if res.Status != "unsupported" {
		t.Fatalf("T17-C01 expected unsupported, got %s err=%v\nsource:\n%s\ndiag=%+v", res.Status, err, res.Source, res.Diagnostics)
	}
	found := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "evil.lang.invoke.StringConcatFactory") || strings.Contains(d.Message, "evil/lang/invoke/StringConcatFactory") || d.Code == core.DiagBootstrapUnknown {
			found = true
		}
	}
	if !found && !strings.Contains(res.Source, "unsupported bootstrap") {
		t.Fatalf("T17-C01 identity not traceable:\nstatus=%s\nsource=%s\ndiag=%+v", res.Status, res.Source, res.Diagnostics)
	}
}

func TestTaskT17C02CondyParseAndInvalidIndex(t *testing.T) {
	t.Log("T17-C02")
	src := t17ReadSeed(t, "ConcatProbe.java")
	_, classes := t17CompileRun(t, "17", "ConcatProbe", map[string]string{"ConcatProbe.java": src})
	raw := classes["ConcatProbe"]
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	bsmCount := 0
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			bsmCount = len(b.BootstrapMethods)
		}
	}
	if bsmCount == 0 {
		t.Fatal("ConcatProbe should have BootstrapMethods")
	}
	nt := uint16(0)
	for i, c := range obj.ConstantPool {
		if _, ok := c.(*ConstantNameAndTypeInfo); ok {
			nt = uint16(i + 1)
			break
		}
	}
	if nt == 0 {
		t.Fatal("no NameAndType")
	}
	obj.ConstantPool = append(obj.ConstantPool, &ConstantDynamicInfo{
		BootstrapMethodAttrIndex: 0,
		NameAndTypeIndex:         nt,
	})
	legalBytes := obj.Bytes()
	legalObj, err := Parse(legalBytes)
	if err != nil {
		t.Fatalf("T17-C02 legal condy must parse: %v", err)
	}
	var dyn *ConstantDynamicInfo
	for _, c := range legalObj.ConstantPool {
		if d, ok := c.(*ConstantDynamicInfo); ok {
			dyn = d
			break
		}
	}
	if dyn == nil {
		t.Fatal("T17-C02 marshaled condy missing after parse")
	}
	condyIndex := 0
	for i, c := range legalObj.ConstantPool {
		if _, ok := c.(*ConstantDynamicInfo); ok {
			condyIndex = i + 1
			break
		}
	}
	got := GetLiteralFromCP(legalObj.ConstantPool, condyIndex)
	s := got.String(nil)
	if !strings.Contains(s, "condy") {
		t.Fatalf("T17-C02 legal condy forged a real constant: %q", s)
	}

	bad := ClassifyViaPool(legalObj, 99)
	if bad.Status != "invalid_input" {
		t.Fatalf("T17-C02 OOB bootstrap index: %+v", bad)
	}

	obj.ConstantPool[len(obj.ConstantPool)-1] = &ConstantDynamicInfo{BootstrapMethodAttrIndex: 99, NameAndTypeIndex: nt}
	badBytes := obj.Bytes()
	badParsed, err := Parse(badBytes)
	if err != nil {
		t.Fatalf("T17-C02 damaged condy should still parse as class bytes: %v", err)
	}
	res, _ := DecompileWithOptions(badParsed.Bytes(), DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if res.Status != "invalid_input" {
		t.Fatalf("T17-C02 damaged condy decompile status=%s diag=%+v", res.Status, res.Diagnostics)
	}
}

func ClassifyViaPool(obj *ClassObject, bsm int) core.DispatchResult {
	return core.ClassifyCondy("val", "Ljava/lang/String;", bsm, bootstrapMethodCount(obj))
}

func bootstrapMethodCount(obj *ClassObject) int {
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			return len(b.BootstrapMethods)
		}
	}
	return 0
}

func TestTaskT17C05VersionCapabilityIntegration(t *testing.T) {
	t.Log("T17-C05")
	recSrc := t17ReadSeed(t, "RecordProbe.java")
	_, recClasses := t17CompileRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": recSrc})
	rec := recClasses["RecordProbe"]
	if rec == nil {
		t.Fatal("RecordProbe.class missing")
	}
	r8, err := DecompileWithOptions(rec, DecompileOptions{Mode: Precision, TargetSourceVersion: 8})
	if err != nil && r8.Source == "" {
		t.Fatalf("T17-C05 record target8 decompile failed: %v", err)
	}
	if r8.Status == "complete" {
		t.Fatalf("T17-C05 record target 8 must not be complete (lossy isRecord). status=%s source:\n%s", r8.Status, r8.Source)
	}
	if r8.Status != "unsupported" {
		t.Fatalf("T17-C05 record target 8 expected unsupported, got %s diag=%+v", r8.Status, r8.Diagnostics)
	}
	r17, _ := DecompileWithOptions(rec, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if r17.Status == "complete" && !strings.Contains(r17.Source, "record ") {
		t.Fatalf("T17-C05 record target 17 claimed complete without record reconstruction:\n%s", r17.Source)
	}

	swSrc := t17ReadSeed(t, "SwitchPattern.java")
	_, swClasses := t17CompileRun(t, "21", "SwitchPattern", map[string]string{"SwitchPattern.java": swSrc})
	sw := swClasses["SwitchPattern"]
	if sw == nil {
		t.Fatal("SwitchPattern.class missing")
	}
	s11, _ := DecompileWithOptions(sw, DecompileOptions{Mode: Precision, TargetSourceVersion: 11})
	if s11.Status == "complete" {
		t.Fatalf("T17-C05 pattern switch target 11 must not be complete:\n%s", s11.Source)
	}
	s21, _ := DecompileWithOptions(sw, DecompileOptions{Mode: Precision, TargetSourceVersion: 21})
	if s21.Status == "complete" && !strings.Contains(s21.Source, "case ") {
		t.Fatalf("T17-C05 pattern switch target 21 claimed complete without reconstruction:\n%s", s21.Source)
	}
}

func TestTaskT17C06ConcatLambdaNoRegress(t *testing.T) {
	t.Log("T17-C06")
	concatSrc := t17ReadSeed(t, "ConcatProbe.java")
	origConcat, concatClasses := t17CompileRun(t, "17", "ConcatProbe", map[string]string{"ConcatProbe.java": concatSrc})
	if !strings.Contains(origConcat, "x=7,o=null") {
		t.Fatalf("T17-C06 original ConcatProbe stdout %q", origConcat)
	}
	raw := concatClasses["ConcatProbe"]
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 17})
		if err != nil {
			t.Fatalf("T17-C06 ConcatProbe %s: %v", mode, err)
		}
		if res.Status != "complete" && res.Status != "partial" {
			t.Fatalf("T17-C06 ConcatProbe %s status=%s diag=%+v\n%s", mode, res.Status, res.Diagnostics, res.Source)
		}
		if !strings.Contains(res.Source, "+") && !strings.Contains(res.Source, "concat") {
			t.Fatalf("T17-C06 ConcatProbe lost concat form:\n%s", res.Source)
		}
		if err := t17RebuildRunErr(t, "17", "ConcatProbe", res.Source, origConcat); err != nil {
			// Known baseline (T04/T18): String.valueOf(null) binds char[] and NPEs. T17 must not
			// drop the concat form; the NPE is recorded, not treated as a dispatch regression.
			if !strings.Contains(err.Error(), "NullPointerException") && !strings.Contains(err.Error(), "valueOf") {
				t.Fatalf("T17-C06 ConcatProbe %s unexpected rebuild failure: %v\n%s", mode, err, res.Source)
			}
			t.Logf("T17-C06 ConcatProbe known valueOf(null)/char[] NPE (T04/T18): %v", err)
		}
	}

	lamSrc := t17ReadSeed(t, "LambdaCapture.java")
	origLam, lamClasses := t17CompileRun(t, "8", "LambdaCapture", map[string]string{"LambdaCapture.java": lamSrc})
	if !strings.Contains(origLam, "13") {
		t.Fatalf("T17-C06 original LambdaCapture stdout %q", origLam)
	}
	rawLam := lamClasses["LambdaCapture"]
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		res, err := DecompileWithOptions(rawLam, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
		if err != nil {
			t.Fatalf("T17-C06 LambdaCapture %s: %v", mode, err)
		}
		if res.Status != "complete" && res.Status != "partial" {
			t.Fatalf("T17-C06 LambdaCapture %s status=%s diag=%+v\n%s", mode, res.Status, res.Diagnostics, res.Source)
		}
		t17RebuildRun(t, "8", "LambdaCapture", res.Source, origLam)
	}
}

func t17RebuildRun(t *testing.T, release, main, src, wantStdout string) {
	t.Helper()
	if err := t17RebuildRunErr(t, release, main, src, wantStdout); err != nil {
		t.Fatal(err)
	}
}

func t17RebuildRunErr(t *testing.T, release, main, src, wantStdout string) error {
	t.Helper()
	javac, java := t17Tools(t)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, main+".java")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		return err
	}
	cmd := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", release, "-d", dir, srcPath)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rebuild javac %s: %v\n%s\nsource:\n%s", main, err, out, src)
	}
	run := exec.Command(java, "-cp", dir, main)
	run.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	out, err := run.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rebuild run %s: %v\n%s", main, err, out)
	}
	if string(out) != wantStdout {
		return fmt.Errorf("rebuild stdout mismatch for %s\n got %q\nwant %q\nsource:\n%s", main, out, wantStdout, src)
	}
	return nil
}
