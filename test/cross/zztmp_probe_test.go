package cross

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// zztmp: decompile the whole jar twice (two independent JarFS) and assert every entry is byte-identical.
//   DET_JAR=fastjson2
func TestZZDeterminism(t *testing.T) {
	jarKey := os.Getenv("DET_JAR")
	if jarKey == "" {
		t.Skip("set DET_JAR")
	}
	jarPath := resolveJar(jarSpecs[jarKey].relPath)
	if jarPath == "" {
		t.Fatalf("jar %s not found", jarKey)
	}
	entries := classEntries(t, jarPath)
	hashAll := func() map[string]string {
		jfs, err := classparser.NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatalf("open jar: %v", err)
		}
		m := map[string]string{}
		for _, e := range entries {
			src, err := jfs.ReadFile(e)
			if err != nil {
				continue
			}
			sum := sha256.Sum256(src)
			m[e] = hex.EncodeToString(sum[:])
		}
		return m
	}
	a, b := hashAll(), hashAll()
	if len(a) != len(b) {
		t.Fatalf("entry count differs: %d vs %d", len(a), len(b))
	}
	diff := 0
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			diff++
			if diff <= 10 {
				t.Errorf("nondeterministic: %s (%s vs %s)", k, va, b[k])
			}
		}
	}
	t.Logf("determinism: %d entries, %d differ", len(a), diff)
}

// zztmp: decompile the whole jar to a tmp dir, javac the tree, and print raw javac output (with carets)
// for lines whose path contains DUMP_FILTER.
//   DUMP_JAR=fastjson2 DUMP_FILTER=schema/JSONSchema.java
func TestZZDumpTreeErrors(t *testing.T) {
	jarKey := os.Getenv("DUMP_JAR")
	if jarKey == "" {
		t.Skip("set DUMP_JAR")
	}
	filter := os.Getenv("DUMP_FILTER")
	javac := lookJavac(t)
	spec := jarSpecs[jarKey]
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Fatalf("jar %s not found", jarKey)
	}
	deps := resolveDeps(spec.depGlob)
	cp := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))
	root := t.TempDir()
	files, units, _ := decompileAll(t, jarPath, root, 0)
	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000")
	if cp != "" {
		args = append(args, "-cp", cp)
	}
	args = append(args, "-d", outDir)
	args = append(args, files...)
	out, _ := exec.Command(javac, args...).CombinedOutput()
	t.Logf("units=%d", units)
	var keep strings.Builder
	lines := strings.Split(string(out), "\n")
	for i := 0; i < len(lines); i++ {
		if filter == "" || strings.Contains(lines[i], filter) {
			keep.WriteString(lines[i])
			keep.WriteString("\n")
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				if strings.Contains(lines[j], "error:") || strings.Contains(lines[j], ".java:") {
					break
				}
				keep.WriteString(lines[j])
				keep.WriteString("\n")
			}
		}
	}
	t.Logf("\n%s", keep.String())
}

// zztmp: decompile the whole jar, compile every file in ISO mode (against the original jar), and print
// the sorted list of files that FAIL. Used to diff the failing-file set across kill-switch settings.
//   ISOFAIL_JAR=fastjson2
func TestZZDumpIsoFails(t *testing.T) {
	jarKey := os.Getenv("ISOFAIL_JAR")
	if jarKey == "" {
		t.Skip("set ISOFAIL_JAR")
	}
	javac := lookJavac(t)
	spec := jarSpecs[jarKey]
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Fatalf("jar %s not found", jarKey)
	}
	deps := resolveDeps(spec.depGlob)
	cpParts := append([]string{jarPath}, deps...)
	cp := withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator)))
	root := t.TempDir()
	files, units, _ := decompileAll(t, jarPath, root, 0)
	var filters []string
	if ff := os.Getenv("ISOFAIL_FILTER"); ff != "" {
		filters = strings.Split(ff, ",")
	}
	var fails []string
	for _, f := range files {
		if len(filters) > 0 {
			match := false
			for _, sub := range filters {
				if strings.Contains(f, sub) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		outDir := t.TempDir()
		args := append(append([]string{}, javacLocaleArgs...),
			"-encoding", "UTF-8", "--release", "8", "-nowarn",
			"-cp", cp, "-d", outDir, f)
		cmd := exec.Command(javac, args...)
		cmd.Dir = outDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			rel := strings.TrimPrefix(f, root)
			fails = append(fails, strings.TrimPrefix(rel, "/"))
			if len(filters) > 0 {
				t.Logf("FAIL %s\n%s", strings.TrimPrefix(rel, "/"), string(out))
			}
		}
	}
	sortStrings(fails)
	t.Logf("units=%d fails=%d\n%s", units, len(fails), strings.Join(fails, "\n"))
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// zztmp: decompile a single class entry and print a line window. Driven by env:
//   PROBE_JAR=fastjson2 PROBE_ENTRY=com/alibaba/fastjson2/writer/FieldWriter.class
//   PROBE_FROM=350 PROBE_TO=440
func TestZZProbeDecompile(t *testing.T) {
	jarKey := os.Getenv("PROBE_JAR")
	entry := os.Getenv("PROBE_ENTRY")
	if jarKey == "" || entry == "" {
		t.Skip("set PROBE_JAR + PROBE_ENTRY")
	}
	jarPath := resolveJar(jarSpecs[jarKey].relPath)
	if jarPath == "" {
		t.Fatalf("jar %s not found", jarKey)
	}
	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	src, err := jfs.ReadFile(entry)
	if err != nil {
		t.Fatalf("read %s: %v", entry, err)
	}
	from, _ := strconv.Atoi(os.Getenv("PROBE_FROM"))
	to, _ := strconv.Atoi(os.Getenv("PROBE_TO"))
	lines := strings.Split(string(src), "\n")
	if to == 0 || to > len(lines) {
		to = len(lines)
	}
	if from < 1 {
		from = 1
	}
	var b strings.Builder
	for i := from; i <= to && i <= len(lines); i++ {
		b.WriteString(strconv.Itoa(i))
		b.WriteString("| ")
		b.WriteString(lines[i-1])
		b.WriteString("\n")
	}
	t.Logf("\n%s", b.String())
}
