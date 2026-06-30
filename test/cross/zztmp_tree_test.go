package cross

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TEMP tree-errLines diagnostic. ZZJAR selects jar, ZZERROUT dumps error lines. Delete after use.
func TestZZTmpTree(t *testing.T) {
	javac := lookJavac(t)
	jarName := os.Getenv("ZZJAR")
	if jarName == "" {
		jarName = "fastjson2"
	}
	for _, kv := range strings.Split(os.Getenv("ZZENV"), ";") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
			defer os.Unsetenv(parts[0])
		}
	}
	jarPath := resolveJar(jarSpecs[jarName].relPath)
	if jarPath == "" {
		t.Skip("jar not found")
	}
	deps := resolveDeps(jarSpecs[jarName].depGlob)
	root := "/tmp/zz_tree_src_" + jarName
	os.RemoveAll(root)
	os.MkdirAll(root, 0o755)
	files, _, _ := decompileAll(t, jarPath, root, 0)
	cp := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))
	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000", "-cp", cp, "-d", outDir)
	args = append(args, files...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = outDir
	out, _ := cmd.CombinedOutput()
	text := string(out)
	errLines := strings.Count(text, "error:")
	defect := map[string]bool{}
	for _, ln := range strings.Split(text, "\n") {
		if i := strings.Index(ln, ".java:"); i >= 0 && strings.Contains(ln, "error:") {
			defect[ln[:i]] = true
		}
	}
	t.Logf("RESULT %s errLines=%d defectClasses=%d", jarName, errLines, len(defect))
	if eo := os.Getenv("ZZERROUT"); eo != "" {
		var b strings.Builder
		for _, ln := range strings.Split(text, "\n") {
			if strings.Contains(ln, "error:") {
				b.WriteString(ln)
				b.WriteByte('\n')
			}
		}
		os.WriteFile(eo, []byte(b.String()), 0o644)
	}
}
