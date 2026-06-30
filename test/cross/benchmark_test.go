package cross

// JavaJive 反编译器评测 harness。两类实验:
//
//  1. TestBenchmarkRoundTripAlgorithms (CI 常驻, 仅需 javac/java): 自托管算法往返正确性。对
//     testdata/algorithms/ 下每个自实现算法 (MD5 / SHA-256 / CRC32 / 快排 / Base64), 走
//     源码 -> javac 编译 -> 运行(基准输出) -> JavaJive 反编译 -> javac 重编译 -> 运行(往返输出),
//     断言两次运行输出逐字节一致。这是「反编译后还能编译回去, 而且还能跑出正确结果」的语义级铁证。
//
//  2. TestBenchmarkThreeWayRecompile (opt-in BENCHMARK=1, 需 ~/.m2 + /tmp/decompilers): 三方
//     (JavaJive / CFR / Vineflower) 在真实 jar 上的可重编译率对照。每方各自反编译整包, 再把各自产出
//     整体 javac 重编译(依赖在 classpath), 统计「能干净编过的产出文件占比」。结果以 markdown 表打印,
//     供 BENCHMARK.md / 官网引用。
//
// 用法:
//
//	go test -run TestBenchmarkRoundTripAlgorithms -v ./test/cross/
//	BENCHMARK=1 go test -run TestBenchmarkThreeWayRecompile -v -timeout 60m ./test/cross/
//	BENCHMARK=1 BENCHMARK_JARS=codec,gson go test -run TestBenchmarkThreeWayRecompile -v ./test/cross/

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

// algorithmsDir holds the self-hosted algorithm sources used by the round-trip correctness benchmark.
const algorithmsDir = "testdata/algorithms"

// packageDeclRe extracts the `package x.y;` declaration from a Java source.
var packageDeclRe = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;`)

// javacErrorFileRe matches a javac error line's leading file path, e.g. `/tmp/x/Foo.java:12: error:`.
var javacErrorFileRe = regexp.MustCompile(`(?m)^(.*\.java):\d+: error:`)

// TestBenchmarkRoundTripAlgorithms proves end-to-end semantic fidelity: every self-hosted algorithm,
// after decompile + recompile, must run and print BYTE-IDENTICAL output to the original compile. A
// mismatch or a recompile failure fails the test (this is a correctness gate, not a triage tool).
func TestBenchmarkRoundTripAlgorithms(t *testing.T) {
	javac := lookJavac(t)
	java := lookJava(t)

	srcs, err := filepath.Glob(filepath.Join(algorithmsDir, "*.java"))
	if err != nil {
		t.Fatalf("glob algorithms: %v", err)
	}
	if len(srcs) == 0 {
		t.Skipf("no algorithm sources under %s", algorithmsDir)
	}
	sort.Strings(srcs)

	for _, src := range srcs {
		src, err := filepath.Abs(src)
		if err != nil {
			t.Fatalf("abs %s: %v", src, err)
		}
		simple := strings.TrimSuffix(filepath.Base(src), ".java")
		t.Run(simple, func(t *testing.T) {
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("read %s: %v", src, err)
			}
			pkg := ""
			if m := packageDeclRe.FindStringSubmatch(string(data)); m != nil {
				pkg = m[1]
			}
			fqcn := simple
			if pkg != "" {
				fqcn = pkg + "." + simple
			}

			// 1) Compile the original source and capture its reference stdout.
			origOut := t.TempDir()
			compileFile(t, javac, src, origOut, "")
			ref := runMainClass(t, java, origOut, fqcn)

			// 2) Decompile every .class the original produced (single self-contained classes here).
			decDir := t.TempDir()
			classes := collectFiles(origOut, ".class")
			if len(classes) == 0 {
				t.Fatalf("no .class produced for %s", simple)
			}
			for _, cls := range classes {
				raw, err := os.ReadFile(cls)
				if err != nil {
					t.Fatalf("read class %s: %v", cls, err)
				}
				srcOut, err := classparser.Decompile(raw)
				if err != nil {
					t.Fatalf("decompile %s failed: %v", cls, err)
				}
				// A public class must live in <SimpleName>.java; derive it from the class file name.
				name := strings.TrimSuffix(filepath.Base(cls), ".class")
				if err := os.WriteFile(filepath.Join(decDir, name+".java"), []byte(srcOut), 0o644); err != nil {
					t.Fatalf("write decompiled %s: %v", name, err)
				}
			}

			// 3) Recompile the decompiled sources and run; assert byte-identical output.
			recompOut := t.TempDir()
			decFiles := collectFiles(decDir, ".java")
			compileFiles(t, javac, decFiles, recompOut, "")
			got := runMainClass(t, java, recompOut, fqcn)

			if got != ref {
				t.Errorf("%s: round-trip output mismatch.\n--- original ---\n%s\n--- round-trip ---\n%s",
					simple, ref, got)
			}
		})
	}
}

// benchJar is a benchmark target jar plus its dependency globs (relative to ~/.m2/repository).
type benchJar struct {
	key     string
	relPath string
	depGlob []string
}

// benchmarkJars is the large-scale 3-way benchmark set: the four authoritative round-trip jars plus
// four additional widely-used libraries, spanning small utilities to large generic-heavy codebases.
var benchmarkJars = []benchJar{
	{key: "codec", relPath: "commons-codec/commons-codec/1.15/commons-codec-1.15.jar"},
	{key: "gson", relPath: "com/google/code/gson/gson/2.8.9/gson-2.8.9.jar"},
	{key: "commons-lang3", relPath: "org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar"},
	{key: "jsoup", relPath: "org/jsoup/jsoup/1.10.2/jsoup-1.10.2.jar"},
	{key: "snakeyaml", relPath: "org/yaml/snakeyaml/2.2/snakeyaml-2.2.jar"},
	{key: "spring", relPath: "org/springframework/spring-core/5.3.27/spring-core-5.3.27.jar",
		depGlob: []string{"org/springframework/spring-jcl/5.3.27/spring-jcl-5.3.27.jar"}},
	{key: "fastjson2", relPath: "com/alibaba/fastjson2/fastjson2/2.0.43/fastjson2-2.0.43.jar"},
	{key: "guava", relPath: "com/google/guava/guava/28.2-android/guava-28.2-android.jar",
		depGlob: []string{
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
			"com/google/errorprone/error_prone_annotations/*/error_prone_annotations-*.jar",
			"com/google/j2objc/j2objc-annotations/*/j2objc-annotations-*.jar",
			"com/google/guava/failureaccess/*/failureaccess-*.jar",
			"org/checkerframework/checker-compat-qual/*/checker-compat-qual-*.jar",
			"org/checkerframework/checker-qual/*/checker-qual-*.jar",
		}},
}

// threeWayScore is one decompiler's result on one jar under the tree-compile metric.
type threeWayScore struct {
	files       int // .java units the decompiler emitted
	failed      int // units that contain at least one javac error when compiled together
	errLines    int // total javac error lines (context)
	outers      int // distinct OUTER (top-level) classes among emitted files (packaging-independent)
	failedOuter int // distinct OUTER classes with at least one failing unit (primary by-class metric)
	decErr      string
}

func (s threeWayScore) passPct() float64 {
	if s.files == 0 {
		return 0
	}
	return float64(s.files-s.failed) / float64(s.files) * 100
}

// classPassPct is the clean-compile rate by OUTER class. Because JavaJive flattens nested classes to
// their own files while CFR/Vineflower inline them, the per-file rate (passPct) is NOT comparable
// across tools; collapsing to the outer class makes this rate packaging-independent and comparable.
func (s threeWayScore) classPassPct() float64 {
	if s.outers == 0 {
		return 0
	}
	return float64(s.outers-s.failedOuter) / float64(s.outers) * 100
}

// TestBenchmarkThreeWayRecompile compares JavaJive vs CFR vs Vineflower on real jars under the same
// tree-compile metric (decompile whole jar -> compile all produced .java together with deps on the
// classpath -> fraction of files that compile cleanly). It prints a markdown table for the docs/site.
func TestBenchmarkThreeWayRecompile(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("set BENCHMARK=1 to run the large-scale 3-way recompile benchmark")
	}
	javac := lookJavac(t)
	java := lookJava(t)
	cfr := findDecompiler("cfr-*.jar")
	vine := findDecompiler("vineflower-*.jar")

	jars := benchmarkJars
	if sel := os.Getenv("BENCHMARK_JARS"); sel != "" {
		want := map[string]bool{}
		for _, k := range strings.Split(sel, ",") {
			want[strings.TrimSpace(k)] = true
		}
		var filtered []benchJar
		for _, j := range benchmarkJars {
			if want[j.key] {
				filtered = append(filtered, j)
			}
		}
		jars = filtered
	}

	type row struct {
		jar     string
		classes int
		jj      threeWayScore
		cfr     threeWayScore
		vine    threeWayScore
	}
	var rows []row

	for _, j := range jars {
		jarPath := resolveJar(j.relPath)
		if jarPath == "" {
			t.Logf("[skip] %s not found under %s", j.key, m2Repo())
			continue
		}
		deps := resolveDeps(j.depGlob)
		classpath := strings.Join(deps, string(os.PathListSeparator))
		nClasses := len(classEntries(t, jarPath))

		r := row{jar: j.key, classes: nClasses}
		r.jj = scoreJavaJive(t, jarPath, classpath)
		if cfr != "" {
			r.cfr = scoreExternal(t, java, cfr, jarPath, classpath, "cfr")
		}
		if vine != "" {
			r.vine = scoreExternal(t, java, vine, jarPath, classpath, "vineflower")
		}
		rows = append(rows, r)
		t.Logf("[%s] classes=%d | JavaJive %d/%d bad-class (%.1f%% ok) %dunit-fail %derr | CFR %d/%d bad-class (%.1f%% ok) %derr | Vineflower %d/%d bad-class (%.1f%% ok) %derr",
			r.jar, r.classes,
			r.jj.failedOuter, r.jj.outers, r.jj.classPassPct(), r.jj.failed, r.jj.errLines,
			r.cfr.failedOuter, r.cfr.outers, r.cfr.classPassPct(), r.cfr.errLines,
			r.vine.failedOuter, r.vine.outers, r.vine.classPassPct(), r.vine.errLines)
	}

	// Emit markdown tables for the evaluation doc / website. The three decompilers emit DIFFERENT
	// numbers of files (JavaJive flattens each nested class to its own top-level unit, so its file
	// count ~= class count; CFR/Vineflower inline nested classes, emitting one file per OUTER class).
	// Therefore the PRIMARY, objective metric is Table A: the number of distinct OUTER (top-level)
	// classes that fail to recompile, which is packaging-independent and directly comparable. Table 1
	// (per-emitted-file rate) and Table 2 (raw error lines) are kept as finer-grained context.
	var b strings.Builder
	b.WriteString("\n#### Table A - defective classes (top-level/outer classes failing to recompile; lower is better; PRIMARY metric)\n\n")
	b.WriteString("Cell = defective / total outer classes (clean-class rate).\n\n")
	b.WriteString("| jar | classes | JavaJive | CFR | Vineflower |\n")
	b.WriteString("|---|---:|---:|---:|---:|\n")
	var jjBad, jjTot, cfrBad, cfrTot, vineBad, vineTot int
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n",
			r.jar, r.classes, classCell(r.jj), classCell(r.cfr), classCell(r.vine))
		jjBad += r.jj.failedOuter
		jjTot += r.jj.outers
		cfrBad += r.cfr.failedOuter
		cfrTot += r.cfr.outers
		vineBad += r.vine.failedOuter
		vineTot += r.vine.outers
	}
	fmt.Fprintf(&b, "| **total** | | **%d/%d** | **%d/%d** | **%d/%d** |\n",
		jjBad, jjTot, cfrBad, cfrTot, vineBad, vineTot)

	b.WriteString("\n#### Table 1 - recompilable-unit pass rate (cleanly-compiling files / emitted files; NOT cross-tool comparable)\n\n")
	b.WriteString("| jar | classes | JavaJive | CFR | Vineflower |\n")
	b.WriteString("|---|---:|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n",
			r.jar, r.classes, scoreCell(r.jj), scoreCell(r.cfr), scoreCell(r.vine))
	}
	b.WriteString("\n#### Table 2 - total javac error lines (lower is better; context only)\n\n")
	b.WriteString("| jar | classes | JavaJive | CFR | Vineflower |\n")
	b.WriteString("|---|---:|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n",
			r.jar, r.classes, errCell(r.jj), errCell(r.cfr), errCell(r.vine))
	}
	_ = javac
	t.Logf("3-way recompile benchmark (tree-compile metric):\n%s", b.String())
}

func errCell(s threeWayScore) string {
	if s.files == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", s.errLines)
}

func scoreCell(s threeWayScore) string {
	if s.files == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", s.files-s.failed, s.files, s.passPct())
}

// classCell renders the primary by-class metric: defective outer classes / total outer classes, plus
// the clean-class rate. Lower defective count is better.
func classCell(s threeWayScore) string {
	if s.files == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", s.failedOuter, s.outers, s.classPassPct())
}

// scoreJavaJive decompiles the jar via the production JarFS path and tree-compiles all units together.
func scoreJavaJive(t *testing.T, jarPath, classpath string) threeWayScore {
	t.Helper()
	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)
	failed, errLines, bad := treeCompileFiles(t, files, classpath)
	outers, failedOuter := outerStats(root, files, bad)
	return threeWayScore{files: len(files), failed: failed, errLines: errLines, outers: outers, failedOuter: failedOuter}
}

// scoreExternal runs an external decompiler (CFR/Vineflower) on the whole jar, then tree-compiles its
// produced .java files together.
func scoreExternal(t *testing.T, java, tool, jarPath, classpath, name string) threeWayScore {
	t.Helper()
	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	var args []string
	switch name {
	case "cfr":
		args = []string{"-jar", tool, jarPath, "--outputdir", outDir}
	default: // vineflower / fernflower
		args = []string{"-jar", tool, jarPath, outDir}
	}
	if out, err := runTool(ctx, java, args...); err != nil {
		return threeWayScore{decErr: name + " decompile error: " + strings.TrimSpace(firstNLines(out, 1))}
	}
	files := collectFiles(outDir, ".java")
	if len(files) == 0 {
		return threeWayScore{decErr: name + " produced no .java"}
	}
	failed, errLines, bad := treeCompileFiles(t, files, classpath)
	outers, failedOuter := outerStats(outDir, files, bad)
	return threeWayScore{files: len(files), failed: failed, errLines: errLines, outers: outers, failedOuter: failedOuter}
}

// treeCompileFiles compiles all files in one javac invocation (deps on classpath) and returns the
// number of DISTINCT files that contain at least one error, the total error-line count, and the set
// of failing file paths (so callers can collapse them to outer classes).
func treeCompileFiles(t *testing.T, files []string, classpath string) (failedFiles, errLines int, badPaths map[string]struct{}) {
	t.Helper()
	badPaths = map[string]struct{}{}
	if len(files) == 0 {
		return 0, 0, badPaths
	}
	javac := lookJavac(t)
	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "1000000",
		"-proc:none", "-cp", classpath, "-d", outDir)
	args = append(args, files...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, javac, args...)
	cmd.Dir = outDir
	out, _ := cmd.CombinedOutput()
	text := string(out)
	errLines = strings.Count(text, ": error:")
	for _, m := range javacErrorFileRe.FindAllStringSubmatch(text, -1) {
		badPaths[m[1]] = struct{}{}
	}
	return len(badPaths), errLines, badPaths
}

// outerKey maps an emitted .java file to its OUTER (top-level) class key relative to root: the
// package-qualified path with the `.java` suffix and any `$Inner` flattening removed. JavaJive emits
// one file per nested class (Outer$Inner.java); CFR/Vineflower inline nested classes into Outer.java.
// Collapsing to this key makes the defective-class count packaging-independent and cross-tool comparable.
func outerKey(root, file string) string {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		rel = file
	}
	rel = strings.TrimSuffix(rel, ".java")
	dir, base := filepath.Split(rel)
	if i := strings.IndexByte(base, '$'); i >= 0 {
		base = base[:i]
	}
	return dir + base
}

// outerStats collapses emitted files (and the failing subset) to distinct outer classes, returning
// the total outer-class count and how many of them have at least one failing unit.
func outerStats(root string, files []string, badPaths map[string]struct{}) (outers, failedOuter int) {
	all := map[string]struct{}{}
	bad := map[string]struct{}{}
	for _, f := range files {
		k := outerKey(root, f)
		all[k] = struct{}{}
		if _, ok := badPaths[f]; ok {
			bad[k] = struct{}{}
		}
	}
	return len(all), len(bad)
}

// compileFile compiles a single .java into outDir (classpath optional).
func compileFile(t *testing.T, javac, file, outDir, classpath string) {
	t.Helper()
	compileFiles(t, javac, []string{file}, outDir, classpath)
}

// compileFiles compiles the given .java files together into outDir; a failure fails the test.
func compileFiles(t *testing.T, javac string, files []string, outDir, classpath string) {
	t.Helper()
	args := append(append([]string{}, javacLocaleArgs...), "-encoding", "UTF-8", "--release", "8", "-nowarn")
	if classpath != "" {
		args = append(args, "-cp", classpath)
	}
	args = append(args, "-d", outDir)
	args = append(args, files...)
	ctx, cancel := context.WithTimeout(context.Background(), compileTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, javac, args...)
	cmd.Dir = outDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac failed for %v: %v\n%s", files, err, out)
	}
}

// runMainClass runs `java -cp dir fqcn` and returns its stdout+stderr; a non-zero exit fails the test.
func runMainClass(t *testing.T, java, dir, fqcn string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, java, "-cp", dir, fqcn)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("java run %s failed: %v\n%s", fqcn, err, out)
	}
	return string(out)
}

// collectFiles returns all files under dir with the given suffix, sorted.
func collectFiles(dir, suffix string) []string {
	var out []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, suffix) {
			return nil
		}
		out = append(out, p)
		return nil
	})
	sort.Strings(out)
	return out
}
