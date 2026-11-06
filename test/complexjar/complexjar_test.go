// Package complexjar round-trips deliberately modern, complex JARs through the
// production archive decompiler, javac, JVM verification and behavioral
// comparison. It is opt-in:
//
//	JAVAJIVE_COMPLEX_JAR_EVAL=1 go test ./test/complexjar -count=1 -v
//
// The suite compiles only repository-authored Java sources and the reviewed
// checked-in irreducible fixtures. It never executes third-party bytecode.
package complexjar

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

func enabled() bool { return os.Getenv("JAVAJIVE_COMPLEX_JAR_EVAL") != "" }

func needTools(t *testing.T) (javac, java string) {
	t.Helper()
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	java, err = exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	return javac, java
}

func writeSources(t *testing.T, root string, sources map[string]string) []string {
	t.Helper()
	files := make([]string, 0, len(sources))
	for name, source := range sources {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func compileFiles(t *testing.T, javac, srcRoot, outDir string, release int, classpath string, files []string) {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-encoding", "UTF-8", "-proc:none", "-Xlint:-options", "--release", strconv.Itoa(release)}
	if classpath != "" {
		args = append(args, "-cp", classpath)
	}
	args = append(args, "-d", outDir)
	args = append(args, files...)
	cmd := exec.Command(javac, args...)
	cmd.Dir = srcRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac --release %d failed: %v\n%s", release, err, out)
	}
}

func runMain(t *testing.T, java, classpath, mainClass string) string {
	t.Helper()
	cmd := exec.Command(java, "-Xverify:all", "-cp", classpath, mainClass)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s failed: %v\n%s", mainClass, err, out)
	}
	return string(out)
}

func buildJar(t *testing.T, jarPath string, manifest string, roots map[string]string) {
	t.Helper()
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()
	if manifest != "" {
		hdr := &zip.FileHeader{Name: "META-INF/MANIFEST.MF", Method: zip.Deflate}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	prefixes := make([]string, 0, len(roots))
	for prefix := range roots {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		root := roots[prefix]
		var files []string
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(files)
		for _, rel := range files {
			data, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.ToSlash(filepath.Join(prefix, rel))
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(data); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// decompileJar writes every produced Java unit under outDir and returns the
// files. A read error is a hard failure: it would otherwise hide a class the
// archive path silently dropped.
func decompileJar(t *testing.T, jarPath, outDir string, target int) []string {
	t.Helper()
	jfs, err := classparser.NewJarFSFromLocalWithSourceVersion(jarPath, target, nil)
	if err != nil {
		t.Fatalf("open jar %s: %v", jarPath, err)
	}
	defer jfs.Close()
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open zip %s: %v", jarPath, err)
	}
	defer zr.Close()
	var entries []string
	for _, entry := range zr.File {
		if strings.HasSuffix(entry.Name, ".class") {
			entries = append(entries, entry.Name)
		}
	}
	sort.Strings(entries)
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		source, err := jfs.ReadFile(entry)
		if err != nil {
			t.Fatalf("decompile %s: %v", entry, err)
		}
		rel := strings.TrimSuffix(entry, ".class") + ".java"
		path := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	return files
}

func assertNoDecompileStubs(t *testing.T, files []string) {
	t.Helper()
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, marker := range []string{"yak-decompiler: undecompilable", "decompile dump failed"} {
			if strings.Contains(source, marker) {
				t.Errorf("%s contains a decompiler stub marker %q:\n%s", filepath.Base(file), marker, firstLines(source, 8))
			}
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func compileTree(t *testing.T, javac string, files []string, outDir string, release int) {
	t.Helper()
	if len(files) == 0 {
		t.Fatal("no decompiled sources")
	}
	compileFiles(t, javac, outDir, outDir, release, "", files)
	assertClassFiles(t, outDir)
}

func assertClassFiles(t *testing.T, dir string) {
	t.Helper()
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".class") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("javac produced no class files under %s", dir)
	}
}

func TestComplexModernJarRoundTrip(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	srcRoot := t.TempDir()
	sources := map[string]string{
		"ModernFeatures.java":    mustRead(t, "testdata/modern/ModernFeatures.java"),
		"SwitchExpressions.java": mustRead(t, "testdata/modern/SwitchExpressions.java"),
	}
	files := writeSources(t, srcRoot, sources)
	origClasses := filepath.Join(t.TempDir(), "classes")
	compileFiles(t, javac, srcRoot, origClasses, 21, "", files)
	origJar := filepath.Join(t.TempDir(), "modern.jar")
	buildJar(t, origJar, "", map[string]string{"": origClasses})

	origModern := runMain(t, java, origClasses, "ModernFeatures")
	origSwitch := runMain(t, java, origClasses, "SwitchExpressions")

	decRoot := t.TempDir()
	decFiles := decompileJar(t, origJar, decRoot, 21)
	assertNoDecompileStubs(t, decFiles)
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 21)

	if got := runMain(t, java, rebuiltClasses, "ModernFeatures"); got != origModern {
		t.Fatalf("ModernFeatures behavior changed:\n--- original ---\n%s\n--- rebuilt ---\n%s", origModern, got)
	}
	if got := runMain(t, java, rebuiltClasses, "SwitchExpressions"); got != origSwitch {
		t.Fatalf("SwitchExpressions behavior changed:\n--- original ---\n%s\n--- rebuilt ---\n%s", origSwitch, got)
	}
	t.Logf("modern Java 21 JAR: %d units, behavior identical", len(decFiles))
}

func TestComplexControlFlowJarRoundTrip(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	srcRoot := t.TempDir()
	files := writeSources(t, srcRoot, map[string]string{"ControlFlow.java": mustRead(t, "testdata/controlflow/ControlFlow.java")})
	origClasses := filepath.Join(t.TempDir(), "classes")
	compileFiles(t, javac, srcRoot, origClasses, 21, "", files)
	origJar := filepath.Join(t.TempDir(), "controlflow.jar")
	buildJar(t, origJar, "", map[string]string{"": origClasses})
	want := runMain(t, java, origClasses, "ControlFlow")

	decRoot := t.TempDir()
	decFiles := decompileJar(t, origJar, decRoot, 21)
	assertNoDecompileStubs(t, decFiles)
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 21)
	got := runMain(t, java, rebuiltClasses, "ControlFlow")
	if got != want {
		t.Fatalf("ControlFlow behavior changed:\n--- original ---\n%s\n--- rebuilt ---\n%s", want, got)
	}
	t.Logf("control-flow JAR: %d units, behavior identical", len(decFiles))
}

func TestComplexLegacyJarRoundTrip(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	srcRoot := t.TempDir()
	files := writeSources(t, srcRoot, map[string]string{"Legacy.java": mustRead(t, "testdata/legacy/Legacy.java")})
	origClasses := filepath.Join(t.TempDir(), "classes")
	compileFiles(t, javac, srcRoot, origClasses, 8, "", files)
	origJar := filepath.Join(t.TempDir(), "legacy.jar")
	buildJar(t, origJar, "", map[string]string{"": origClasses})
	want := runMain(t, java, origClasses, "Legacy")

	decRoot := t.TempDir()
	decFiles := decompileJar(t, origJar, decRoot, 8)
	assertNoDecompileStubs(t, decFiles)
	for _, file := range decFiles {
		if strings.HasSuffix(file, "Legacy$Op.java") {
			data, _ := os.ReadFile(file)
			if !strings.Contains(string(data), "javac regenerates") {
				t.Fatalf("enum constant body was not folded into the owner source: %s", file)
			}
		}
	}
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 8)
	got := runMain(t, java, rebuiltClasses, "Legacy")
	if got != want {
		t.Fatalf("Legacy behavior changed:\n--- original ---\n%s\n--- rebuilt ---\n%s", want, got)
	}
	t.Logf("legacy Java 8 JAR: %d units, behavior identical", len(decFiles))
}

func TestComplexScaleJarRoundTrip(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	const classCount = 100
	const chunk = 20
	srcRoot := t.TempDir()
	sources := map[string]string{}
	for i := 0; i < classCount; i++ {
		name := fmt.Sprintf("C%03d", i)
		sources["scale/"+name+".java"] = fmt.Sprintf(`package scale;
public class %s {
    private int seed = %d;
    static int f%03d(int x) { int s = 0; for (int j = 0; j < x; j++) { s += (j * 31 + x) ^ (j >>> 1); if ((j & 7) == 3) s -= %d; if (s > 100000) break; } return s; }
    int g%03d(int x) { switch ((x + seed) & 7) { case 0: return x + 1; case 1: return x * 2; case 2: return x - 3; case 3: return -x; default: return seed + x; } }
    int h%03d(int[] xs) { int acc = seed; for (int x : xs) acc = acc * 3 + x; return acc; }
    String s%03d(String v) { return v + ":" + (%d %% 5) + ":" + seed; }
}
`, name, i*37-11, i, i%13+2, i, i, i, i)
	}
	var main strings.Builder
	main.WriteString("package scale;\npublic class ScaleMain {\n")
	for start := 0; start < classCount; start += chunk {
		end := start + chunk
		if end > classCount {
			end = classCount
		}
		fmt.Fprintf(&main, "    static long part%d() { long total = 0;\n", start/chunk)
		for i := start; i < end; i++ {
			name := fmt.Sprintf("C%03d", i)
			fmt.Fprintf(&main, "        %s c%d = new %s();\n", name, i, name)
			fmt.Fprintf(&main, "        for (int x = -3; x <= 9; x++) total += %s.f%03d(x) + c%d.g%03d(x) + c%d.h%03d(new int[]{x, x + 1, %d}) + c%d.s%03d(\"v\").length();\n", name, i, i, i, i, i, i, i, i)
		}
		main.WriteString("        return total;\n    }\n")
	}
	main.WriteString("    public static void main(String[] args) { long total = 0;\n")
	for start := 0; start < classCount; start += chunk {
		fmt.Fprintf(&main, "        total += part%d();\n", start/chunk)
	}
	main.WriteString("        System.out.println(total);\n    }\n}\n")
	sources["scale/ScaleMain.java"] = main.String()

	files := writeSources(t, srcRoot, sources)
	origClasses := filepath.Join(t.TempDir(), "classes")
	compileFiles(t, javac, srcRoot, origClasses, 21, "", files)
	origJar := filepath.Join(t.TempDir(), "scale.jar")
	buildJar(t, origJar, "", map[string]string{"": origClasses})
	want := runMain(t, java, origClasses, "scale.ScaleMain")

	decRoot := t.TempDir()
	decFiles := decompileJar(t, origJar, decRoot, 21)
	assertNoDecompileStubs(t, decFiles)
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 21)
	got := runMain(t, java, rebuiltClasses, "scale.ScaleMain")
	if got != want {
		t.Fatalf("scale behavior changed:\n--- original ---\n%s\n--- rebuilt ---\n%s", want, got)
	}
	t.Logf("scale JAR: %d classes, %d units, behavior identical", classCount, len(decFiles))
}

func TestComplexMultiReleaseJar(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	makeVersion := func(release int, value string) string {
		root := t.TempDir()
		files := writeSources(t, root, map[string]string{"mr/Version.java": fmt.Sprintf(`package mr;
public class Version {
    public static String value() { return %q; }
    public static void main(String[] args) { System.out.println(value()); }
}
`, value)})
		classes := filepath.Join(t.TempDir(), "classes")
		compileFiles(t, javac, root, classes, release, "", files)
		return classes
	}
	base := makeVersion(8, "base")
	v17 := makeVersion(17, "v17")
	v21 := makeVersion(21, "v21")
	jarPath := filepath.Join(t.TempDir(), "mr.jar")
	buildJar(t, jarPath, "Manifest-Version: 1.0\nMulti-Release: true\n\n", map[string]string{
		"":                      base,
		"META-INF/versions/17/": v17,
		"META-INF/versions/21/": v21,
	})
	wants := map[string]string{
		"base": runMain(t, java, base, "mr.Version"),
		"v17":  runMain(t, java, v17, "mr.Version"),
		"v21":  runMain(t, java, v21, "mr.Version"),
	}

	decRoot := t.TempDir()
	decFiles := decompileJar(t, jarPath, decRoot, 21)
	assertNoDecompileStubs(t, decFiles)
	var baseFiles, v17Files, v21Files []string
	for _, file := range decFiles {
		rel, _ := filepath.Rel(decRoot, file)
		slash := filepath.ToSlash(rel)
		switch {
		case strings.HasPrefix(slash, "META-INF/versions/17/"):
			v17Files = append(v17Files, file)
		case strings.HasPrefix(slash, "META-INF/versions/21/"):
			v21Files = append(v21Files, file)
		case strings.HasPrefix(slash, "META-INF/versions/"):
			t.Fatalf("unexpected multi-release unit %s", slash)
		default:
			baseFiles = append(baseFiles, file)
		}
	}
	if len(baseFiles) != 1 || len(v17Files) != 1 || len(v21Files) != 1 {
		t.Fatalf("multi-release units: base=%d v17=%d v21=%d", len(baseFiles), len(v17Files), len(v21Files))
	}
	baseOut := filepath.Join(t.TempDir(), "base")
	compileTree(t, javac, baseFiles, baseOut, 8)
	for _, version := range []struct {
		name  string
		files []string
		rel   int
		out   string
		want  string
	}{
		{"17", v17Files, 17, filepath.Join(t.TempDir(), "v17"), wants["v17"]},
		{"21", v21Files, 21, filepath.Join(t.TempDir(), "v21"), wants["v21"]},
	} {
		compileFiles(t, javac, filepath.Dir(version.files[0]), version.out, version.rel, baseOut, version.files)
		got := runMain(t, java, version.out+string(os.PathListSeparator)+baseOut, "mr.Version")
		if got != version.want {
			t.Fatalf("multi-release %s behavior changed: got %q want %q", version.name, got, version.want)
		}
	}
	if got := runMain(t, java, baseOut, "mr.Version"); got != wants["base"] {
		t.Fatalf("multi-release base behavior changed: got %q want %q", got, wants["base"])
	}
	t.Logf("multi-release JAR: %d units, every release behavior identical", len(decFiles))
}

func TestComplexModuleJar(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, java := needTools(t)
	srcRoot := t.TempDir()
	files := writeSources(t, srcRoot, map[string]string{
		"module-info.java": mustRead(t, "testdata/module/module-info.java"),
		"pkg/Api.java":     mustRead(t, "testdata/module/pkg/Api.java"),
	})
	origClasses := filepath.Join(t.TempDir(), "classes")
	compileFiles(t, javac, srcRoot, origClasses, 21, "", files)
	origJar := filepath.Join(t.TempDir(), "module.jar")
	buildJar(t, origJar, "", map[string]string{"": origClasses})
	want := runMain(t, java, origClasses, "pkg.Api")

	decRoot := t.TempDir()
	decFiles := decompileJar(t, origJar, decRoot, 21)
	assertNoDecompileStubs(t, decFiles)
	moduleSource := ""
	for _, file := range decFiles {
		if filepath.Base(file) == "module-info.java" {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			moduleSource = string(raw)
		}
	}
	if moduleSource == "" {
		t.Fatal("module-info.class produced no source unit")
	}
	if strings.Contains(moduleSource, "synthetic module-info descriptor") {
		t.Fatalf("module descriptor was not reconstructed:\n%s", moduleSource)
	}
	if !strings.Contains(moduleSource, "module m.example") || !strings.Contains(moduleSource, "exports pkg;") {
		t.Fatalf("unexpected module source:\n%s", moduleSource)
	}
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 21)
	got := runMain(t, java, rebuiltClasses, "pkg.Api")
	if got != want {
		t.Fatalf("module JAR behavior changed: got %q want %q", got, want)
	}
	moduleRun := exec.Command(java, "-Xverify:all", "--module-path", rebuiltClasses, "--module", "m.example/pkg.Api")
	moduleOut, err := moduleRun.CombinedOutput()
	if err != nil {
		t.Fatalf("rebuilt module failed on the module path: %v\n%s", err, moduleOut)
	}
	if string(moduleOut) != want {
		t.Fatalf("module-path behavior changed: got %q want %q", moduleOut, want)
	}
	t.Logf("module JAR: %d units; module descriptor reconstructed; classpath and module-path behavior identical", len(decFiles))
}

func TestComplexIrreducibleJarArchive(t *testing.T) {
	if !enabled() {
		t.Skip("set JAVAJIVE_COMPLEX_JAR_EVAL=1 to run complex JAR evaluation")
	}
	javac, _ := needTools(t)
	srcRoot := filepath.Join("..", "cross", "testdata", "irreducible", "original")
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		t.Fatalf("read irreducible fixtures: %v", err)
	}
	jarPath := filepath.Join(t.TempDir(), "irreducible.jar")
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	decRoot := t.TempDir()
	decFiles := decompileJar(t, jarPath, decRoot, 8)
	assertNoDecompileStubs(t, decFiles)
	if len(decFiles) != len(entries) {
		t.Fatalf("irreducible archive units=%d want=%d", len(decFiles), len(entries))
	}
	rebuiltClasses := filepath.Join(t.TempDir(), "rebuilt")
	compileTree(t, javac, decFiles, rebuiltClasses, 8)
	t.Logf("irreducible archive: %d classes, tree recompiled", len(decFiles))
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
