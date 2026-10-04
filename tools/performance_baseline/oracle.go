package performance_baseline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	javaclassparser "github.com/yaklang/javajive/classparser"
)

type runnableEntry struct {
	kind     string // static_run_i | instance_main_z | instance_main_i
	javaName string
	pkg      string
	simple   string
	expr     string
}

func observeStubOracle(classBytes []byte, stub WorkCounts) OracleObservation {
	entry, err := findRunnableEntry(classBytes)
	if err != nil {
		return OracleObservation{Attempted: true, Note: err.Error()}
	}
	if entry == nil {
		return OracleObservation{
			Attempted: true,
			Ran:       false,
			Equal:     false,
			Note:      "no static run()I or instance main() entry; rebuilt-run stdout unavailable. Stub classified by decompile status/skipped_analysis, not by Dump() source hash.",
		}
	}
	if stub.SkippedAnalysis {
		return OracleObservation{
			Attempted: true,
			Ran:       false,
			Equal:     false,
			Note:      "candidate skipped analysis; rebuilt-run oracle cannot execute FastStub as " + entry.javaName + ". Classified by skip/status, not source hash.",
		}
	}
	return tryRebuiltRunOracle(classBytes, entry.javaName, "")
}

func findRunnableEntry(classBytes []byte) (*runnableEntry, error) {
	obj, err := javaclassparser.Parse(classBytes)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	internal := strings.ReplaceAll(obj.GetClassName(), ".", "/")
	javaName := strings.ReplaceAll(internal, "/", ".")
	pkg, simple := splitJavaName(javaName)
	entry := &runnableEntry{javaName: javaName, pkg: pkg, simple: simple}
	for _, m := range methodsWithCode(obj) {
		static := m.access&javaclassparser.StaticFlag != 0
		if m.name == "run" && m.desc == "()I" && static {
			entry.kind = "static_run_i"
			entry.expr = javaName + ".run()"
			return entry, nil
		}
	}
	for _, m := range methodsWithCode(obj) {
		static := m.access&javaclassparser.StaticFlag != 0
		if m.name == "main" && !static && (m.desc == "()Z" || m.desc == "()I") {
			entry.kind = "instance_main_z"
			if m.desc == "()I" {
				entry.kind = "instance_main_i"
			}
			entry.expr = "new " + simple + "().main()"
			return entry, nil
		}
	}
	return nil, nil
}

func splitJavaName(javaName string) (pkg, simple string) {
	i := strings.LastIndex(javaName, ".")
	if i < 0 {
		return "", javaName
	}
	return javaName[:i], javaName[i+1:]
}

func driverSource(entry *runnableEntry) string {
	var b strings.Builder
	if entry.pkg != "" {
		fmt.Fprintf(&b, "package %s;\n", entry.pkg)
	}
	fmt.Fprintf(&b, "public class T32OracleDriver {\n  public static void main(String[] args) {\n    System.out.print(%s);\n  }\n}\n", entry.expr)
	return b.String()
}

func tryRebuiltRunOracle(classBytes []byte, internalName, decompiledSrc string) OracleObservation {
	obs := OracleObservation{Attempted: true}
	if _, err := exec.LookPath("javac"); err != nil {
		obs.Note = "javac not on PATH; rebuilt-run oracle unavailable"
		return obs
	}
	if _, err := exec.LookPath("java"); err != nil {
		obs.Note = "java not on PATH; rebuilt-run oracle unavailable"
		return obs
	}
	entry, err := findRunnableEntry(classBytes)
	if err != nil {
		obs.Note = err.Error()
		return obs
	}
	if entry == nil {
		obs.Note = "no runnable entry; rebuilt-run oracle not applicable"
		return obs
	}
	if internalName == "" {
		internalName = strings.ReplaceAll(entry.javaName, ".", "/")
	}
	tmp, err := os.MkdirTemp("", "t32-oracle-*")
	if err != nil {
		obs.Note = "tmpdir: " + err.Error()
		return obs
	}
	defer os.RemoveAll(tmp)

	orig := filepath.Join(tmp, "orig")
	rebuilt := filepath.Join(tmp, "rebuilt")
	classRel := filepath.FromSlash(internalName) + ".class"
	if err := os.MkdirAll(filepath.Join(orig, filepath.Dir(classRel)), 0o755); err != nil {
		obs.Note = err.Error()
		return obs
	}
	if err := os.MkdirAll(filepath.Join(rebuilt, filepath.Dir(classRel)), 0o755); err != nil {
		obs.Note = err.Error()
		return obs
	}
	if err := os.WriteFile(filepath.Join(orig, classRel), classBytes, 0o644); err != nil {
		obs.Note = err.Error()
		return obs
	}

	driverRel := "T32OracleDriver.java"
	if entry.pkg != "" {
		driverRel = filepath.Join(filepath.FromSlash(strings.ReplaceAll(entry.pkg, ".", "/")), "T32OracleDriver.java")
		if err := os.MkdirAll(filepath.Join(tmp, filepath.Dir(driverRel)), 0o755); err != nil {
			obs.Note = err.Error()
			return obs
		}
	}
	driverPath := filepath.Join(tmp, driverRel)
	if err := os.WriteFile(driverPath, []byte(driverSource(entry)), 0o644); err != nil {
		obs.Note = err.Error()
		return obs
	}
	if err := runCmdTimeout(15*time.Second, tmp, "javac", "--release", "8", "-cp", orig, "-d", orig, driverPath); err != nil {
		obs.Note = "javac driver vs original: " + err.Error()
		return obs
	}
	baseOut, verifyNote, err := runJavaStdout(tmp, orig, entry)
	if err != nil {
		obs.Note = "java original: " + err.Error()
		return obs
	}
	obs.BaselineOut = baseOut
	if strings.TrimSpace(decompiledSrc) == "" {
		obs.Ran = true
		obs.Note = "original class stdout captured; no candidate source supplied. " + verifyNote
		return obs
	}
	srcPath := filepath.Join(rebuilt, filepath.FromSlash(internalName)+".java")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		obs.Note = err.Error()
		return obs
	}
	if err := os.WriteFile(srcPath, []byte(decompiledSrc), 0o644); err != nil {
		obs.Note = err.Error()
		return obs
	}
	if err := runCmdTimeout(15*time.Second, tmp, "javac", "--release", "8", "-d", rebuilt, srcPath); err != nil {
		obs.Note = "javac rebuilt source: " + err.Error() + "; original stdout=" + baseOut
		return obs
	}
	if err := runCmdTimeout(15*time.Second, tmp, "javac", "--release", "8", "-cp", rebuilt, "-d", rebuilt, driverPath); err != nil {
		obs.Note = "javac driver vs rebuilt: " + err.Error()
		return obs
	}
	candOut, candNote, err := runJavaStdout(tmp, rebuilt, entry)
	if err != nil {
		obs.Note = "java rebuilt: " + err.Error()
		return obs
	}
	obs.CandidateOut = candOut
	obs.Ran = true
	obs.Equal = baseOut == candOut
	notes := []string{verifyNote, candNote}
	if obs.Equal {
		notes = append(notes, "original stdout equals rebuilt decompiled stdout")
	} else {
		notes = append(notes, "rebuilt run stdout differs from original")
	}
	obs.Note = strings.TrimSpace(strings.Join(notes, "; "))
	return obs
}

func runJavaStdout(dir, classpath string, entry *runnableEntry) (stdout, note string, err error) {
	driverClass := "T32OracleDriver"
	if entry.pkg != "" {
		driverClass = entry.pkg + ".T32OracleDriver"
	}
	out, err := runCmdOutput(15*time.Second, dir, "java", "-Xverify:all", "-cp", classpath, driverClass)
	if err != nil {
		return "", "", fmt.Errorf("java -Xverify:all rejected class (not accepted): %w", err)
	}
	return out, "java -Xverify:all accepted the class", nil
}

func runCmdTimeout(d time.Duration, dir, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, buf.String())
	}
	return nil
}

func runCmdOutput(d time.Duration, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		return "", fmt.Errorf("%v: %s", err, msg)
	}
	return stdout.String(), nil
}
