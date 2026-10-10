package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func lookJavacT(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found in PATH")
	}
	return p
}

func lookJavaT(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found in PATH")
	}
	return p
}

func writeJavaTree(t *testing.T, dir string, sources map[string]string) []string {
	t.Helper()
	var files []string
	for rel, src := range sources {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	return files
}

func compileJavaRelease(t *testing.T, dir string, sources map[string]string, release string) {
	t.Helper()
	javac := lookJavacT(t)
	files := writeJavaTree(t, dir, sources)
	args := []string{"-encoding", "UTF-8", "-d", dir}
	if release != "" {
		args = append(args, "--release", release)
	}
	args = append(args, files...)
	out, err := exec.Command(javac, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
}

func runJavaMain(t *testing.T, dir, mainClass string) string {
	t.Helper()
	java := lookJavaT(t)
	cmd := exec.Command(java, "-Xverify:all", "-cp", dir, mainClass)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s: %v\n%s", mainClass, err, out)
	}
	return string(out)
}

func readClassBytes(t *testing.T, dir, internalName string) []byte {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(internalName)+".class")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func classMapFromDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".class") {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		internal := strings.TrimSuffix(filepath.ToSlash(rel), ".class")
		out[internal] = raw
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func resolverFromClasses(classes map[string][]byte) func(string) ([]byte, bool) {
	return func(internal string) ([]byte, bool) {
		internal = strings.ReplaceAll(internal, ".", "/")
		data, ok := classes[internal]
		return data, ok
	}
}

func decompilePrecision(t *testing.T, data []byte, resolve func(string) ([]byte, bool)) DecompileResult {
	t.Helper()
	res, err := DecompileWithOptions(data, DecompileOptions{Mode: Precision, Resolve: resolve})
	if err != nil {
		t.Fatalf("decompile precision: %v", err)
	}
	return res
}

func decompileCompatibility(t *testing.T, data []byte, resolve func(string) ([]byte, bool)) DecompileResult {
	t.Helper()
	res, err := DecompileWithOptions(data, DecompileOptions{Mode: Compatibility, Resolve: resolve})
	if err != nil {
		t.Fatalf("decompile compatibility: %v", err)
	}
	return res
}

func javaFileForClass(internal string) string {
	return filepath.FromSlash(internal) + ".java"
}

func roundTripFamily(t *testing.T, sources map[string]string, mainClass, release string) (originalOut, rebuiltOut string, decompiled map[string]string) {
	t.Helper()
	srcDir := t.TempDir()
	compileJavaRelease(t, srcDir, sources, release)
	originalOut = runJavaMain(t, srcDir, mainClass)
	classes := classMapFromDir(t, srcDir)
	resolve := resolverFromClasses(classes)
	decompiled = map[string]string{}
	rebuildDir := t.TempDir()
	rebuildSources := map[string]string{}
	for internal, data := range classes {
		res := decompilePrecision(t, data, resolve)
		if res.Source == "" {
			t.Fatalf("empty decompile of %s status=%s", internal, res.Status)
		}
		decompiled[internal] = res.Source
		rebuildSources[javaFileForClass(internal)] = res.Source
	}
	compileJavaRelease(t, rebuildDir, rebuildSources, release)
	rebuiltOut = runJavaMain(t, rebuildDir, mainClass)
	return originalOut, rebuiltOut, decompiled
}
