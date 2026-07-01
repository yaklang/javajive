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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// selfScore is JavaJive's SELF-evaluation on one jar (no cross-tool comparison). It measures the two
// things a decompiler is actually for: how much of the jar recompiles, and whether the recompiled
// output round-trips (repackages into a jar whose classes all load+verify). The primary metric is the
// CLEAN-CLASS RATE (compilable outer classes / total outer classes), NOT the raw javac error-line
// count -- an error-line count is masked by javac phase-abort (a single parse error suppresses every
// downstream attribution error) and conflates "one class with many errors" with "many broken classes".
// A class is the unit that either recompiles or does not, so the class-level rate is the honest metric.
type selfScore struct {
	units       int // flat .java units JavaJive emitted (it flattens each nested class to its own file)
	outers      int // distinct OUTER (top-level) classes among the emitted units
	failedOuter int // OUTER classes with >=1 javac error when tree-compiled (the defective classes)
	errLines    int // total javac error lines (context only; see the doc note on why this is secondary)
	syntaxErr   int // javac PARSE/lexer error lines (syntax errors). MUST be 0: any syntax error aborts
	// javac before attribution and phase-masks every other file's type errors, making the class metric a
	// dishonest overcount. A nonzero value invalidates this jar's failedOuter count.
	verifyOK   int    // classes in the repackaged jar that load+link under -Xverify:all
	verifyFail int    // classes in the repackaged jar that fail the bytecode verifier
	decErr     string // non-empty if decompilation itself errored (jar unreadable, etc.)
}

// cleanClassPct is the fraction of outer classes that recompile with zero javac errors.
func (s selfScore) cleanClassPct() float64 {
	if s.outers == 0 {
		return 0
	}
	return float64(s.outers-s.failedOuter) / float64(s.outers) * 100
}

// fullRoundTrip reports whether the jar round-trips completely: every class recompiled (no tree
// errors) AND every class in the repackaged jar loads+verifies.
func (s selfScore) fullRoundTrip() bool {
	return s.decErr == "" && s.failedOuter == 0 && s.errLines == 0 && s.verifyFail == 0
}

// TestBenchmarkSelfRecompile is JavaJive's self-evaluation benchmark (opt-in BENCHMARK=1, needs ~/.m2).
// For each target jar it decompiles the whole jar, tree-compiles the produced sources together (deps on
// the classpath), collapses failures to distinct OUTER classes, then repackages the recompiled classes
// into a jar and load+verifies every class. It prints two markdown tables for BENCHMARK.md / the site:
//
//	Table A - clean-class rate: compilable outer classes / total (the PRIMARY metric, error-ratio form).
//	Table B - round-trip: recompile errors + repackaged-jar verify (ok/fail) + a full-round-trip flag.
//
// This is a JavaJive-ONLY report; it does not run or compare against any other decompiler.
func TestBenchmarkSelfRecompile(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("set BENCHMARK=1 to run the JavaJive self-recompile benchmark")
	}
	lookJavac(t)
	lookJava(t)
	verifierDir := buildVerifier(t)

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
		s       selfScore
	}
	var rows []row

	for _, j := range jars {
		jarPath := resolveJar(j.relPath)
		if jarPath == "" {
			t.Logf("[skip] %s not found under %s", j.key, m2Repo())
			continue
		}
		deps := resolveDeps(j.depGlob)
		// Complete sun.misc so faithfully-decompiled sun.misc.Unsafe users are not counted as defects
		// under --release 8 (the same shim the round-trip and inventory harnesses use).
		classpath := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))
		nClasses := len(classEntries(t, jarPath))

		root := t.TempDir()
		files, units, decompFail := decompileAll(t, jarPath, root, 0)

		clsRoot := t.TempDir()
		errLines, raw := treeCompileToDir(t, files, classpath, clsRoot)
		bad := map[string]struct{}{}
		for _, m := range javacErrorFileRe.FindAllStringSubmatch(raw, -1) {
			bad[m[1]] = struct{}{}
		}
		outers, failedOuter := outerStats(root, files, bad)
		// Count syntax (parse/lexer) errors: any of them phase-masks this jar's attribution errors, so a
		// nonzero count means failedOuter is an unreliable overcount (see selfScore.syntaxErr).
		syntaxErr := 0
		for _, tr := range parseTreeErrors(raw, root) {
			if tr.reason == "syntax error (malformed output)" {
				syntaxErr++
			}
		}

		// Repackage whatever recompiled and load+verify every class in the resulting jar.
		repackaged := filepath.Join(t.TempDir(), j.key+"-recompiled.jar")
		zipClassesToJar(t, clsRoot, repackaged)
		vok, vfail, _ := verifyJarLoads(t, verifierDir, repackaged)

		s := selfScore{
			units: len(files), outers: outers, failedOuter: failedOuter,
			errLines: errLines, syntaxErr: syntaxErr, verifyOK: vok, verifyFail: vfail,
		}
		if decompFail > 0 {
			s.decErr = fmt.Sprintf("%d units failed to decompile", decompFail)
		}
		_ = units
		rows = append(rows, row{jar: j.key, classes: nClasses, s: s})
		t.Logf("[%s] classes=%d units=%d | clean-class %d/%d (%.1f%% ok) | recompileErrLines=%d syntaxErr=%d | repackagedVerify ok=%d fail=%d | fullRoundTrip=%v",
			j.key, nClasses, len(files),
			outers-failedOuter, outers, s.cleanClassPct(),
			errLines, syntaxErr, vok, vfail, s.fullRoundTrip())
	}

	var b strings.Builder
	b.WriteString("\n#### Table A - clean-class rate (compilable outer classes / total; higher is better; PRIMARY metric)\n\n")
	b.WriteString("Cell = clean / total outer classes (clean-class rate). A class is \"clean\" iff every unit it flattens into recompiles with zero javac errors.\n\n")
	b.WriteString("| jar | classes | clean classes | clean-class rate | defective classes |\n")
	b.WriteString("|---|---:|---:|---:|---:|\n")
	var totOuter, totClean int
	for _, r := range rows {
		clean := r.s.outers - r.s.failedOuter
		totOuter += r.s.outers
		totClean += clean
		fmt.Fprintf(&b, "| %s | %d | %d/%d | %.1f%% | %d |\n",
			r.jar, r.classes, clean, r.s.outers, r.s.cleanClassPct(), r.s.failedOuter)
	}
	pct := 0.0
	if totOuter > 0 {
		pct = float64(totClean) / float64(totOuter) * 100
	}
	fmt.Fprintf(&b, "| **total** | | **%d/%d** | **%.1f%%** | **%d** |\n", totClean, totOuter, pct, totOuter-totClean)

	b.WriteString("\n#### Table B - round-trip (decompile -> recompile -> repackage -> load+verify)\n\n")
	b.WriteString("| jar | recompile error lines | syntax errors | repackaged-jar verify (ok/fail) | full round-trip |\n")
	b.WriteString("|---|---:|---:|---:|:--:|\n")
	totalSyntax := 0
	for _, r := range rows {
		totalSyntax += r.s.syntaxErr
		flag := "no"
		if r.s.fullRoundTrip() {
			flag = "YES"
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d/%d | %s |\n",
			r.jar, r.s.errLines, r.s.syntaxErr, r.s.verifyOK, r.s.verifyOK+r.s.verifyFail, flag)
	}
	// The whole class metric is only honest when there are ZERO syntax errors: a single parse/lexer
	// error aborts javac before attribution and masks every other file's type errors. Assert it so a
	// future malformed-output regression fails the benchmark loudly instead of silently deflating counts.
	if totalSyntax != 0 {
		t.Errorf("benchmark integrity: %d syntax (parse/lexer) errors across the set phase-mask attribution; "+
			"clean-class counts are an unreliable overcount until fixed", totalSyntax)
	}
	t.Logf("JavaJive self-recompile benchmark (class-compile + round-trip metric); total syntax errors=%d:\n%s",
		totalSyntax, b.String())
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
		// Complete the JDK-internal sun.misc package (see jdk_sunmisc_test.go) so faithfully-decompiled
		// sun.misc.Unsafe users (guava) are not counted as defects under --release 8. Applied to BOTH
		// JavaJive and the external tools' classpaths below, so the comparison stays fair.
		classpath := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))
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
// JavaJive flattens every nested class to its own top-level `Outer$Inner.java` unit, so those units
// only resolve each other as a TREE (an isolated unit's flat `$` reference to a sibling is unresolvable);
// the tree metric is therefore the correct one for JavaJive. Crucially it is NOT phase-masked here:
// JavaJive's output is syntactically clean (zero parse errors across all benchmark jars), so javac
// always reaches the ATTRIBUTION phase and reports every type/import/generic error -- unlike CFR/
// Vineflower, whose parse-error files would mask their own attribution failures in a tree compile (see
// scoreExternal, which therefore measures them per-file in isolation).
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
	// PHASE-MASKING-ROBUST scoring (see scoreJavaJive's comment): javac aborts after the PARSE phase
	// whenever ANY file in a single compilation has a syntax error, so it never runs ATTRIBUTION and
	// therefore never reports the type/import/generic errors of the OTHER files. CFR/Vineflower both
	// emit a handful of syntactically-broken classes ('finally' without 'try', '(' expected, ...); in a
	// single whole-jar tree compile those parse errors MASK all of the tool's remaining attribution
	// failures (sun.misc.Unsafe, generic-erasure casts, ...), making the tool look far cleaner than it
	// is (measured: CFR guava tree=8 vs CFR guava iso=157 defective outer classes). To measure each
	// tool fairly we must defeat this masking. Because CFR/Vineflower INLINE nested classes (one emitted
	// file == one outer class, with no flattened `Outer$Inner` cross-references), each file can be
	// compiled in ISOLATION against the ORIGINAL jar + deps with no false positives -- this is both
	// phase-masking-immune (one file's parse error cannot hide another's attribution error) and at least
	// as lenient as the tree (the pristine jar resolves every cross-class reference). JavaJive cannot use
	// iso (it flattens, so its `Outer$Inner` units only resolve as a tree); it stays on the tree metric,
	// which is already unmasked because its output is parse-clean (asserted in scoreJavaJive).
	isoCP := classpath
	if isoCP == "" {
		isoCP = jarPath
	} else {
		isoCP = jarPath + string(os.PathListSeparator) + classpath
	}
	failed, errLines, bad := isoCompileFiles(t, files, isoCP)
	outers, failedOuter := outerStats(outDir, files, bad)
	return threeWayScore{files: len(files), failed: failed, errLines: errLines, outers: outers, failedOuter: failedOuter}
}

// isoCompileFiles compiles each file in ISOLATION (in parallel) against classpath and returns the
// number of files that fail, the total javac error-line count, and the set of failing file paths. It
// is the phase-masking-robust counterpart of treeCompileFiles for tools whose every emitted file is a
// self-contained outer class (CFR/Vineflower inline nested classes): isolating each file guarantees one
// file's PARSE error cannot suppress another file's ATTRIBUTION error (see scoreExternal). Workers
// default to NumCPU; override with RECOMPILE_WORKERS.
func isoCompileFiles(t *testing.T, files []string, classpath string) (failedFiles, errLines int, badPaths map[string]struct{}) {
	t.Helper()
	badPaths = map[string]struct{}{}
	if len(files) == 0 {
		return 0, 0, badPaths
	}
	javac := lookJavac(t)
	workers, _ := strconv.Atoi(os.Getenv("RECOMPILE_WORKERS"))
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ch = make(chan string, len(files))
	)
	for _, f := range files {
		ch <- f
	}
	close(ch)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outDir := t.TempDir()
			for f := range ch {
				ctx, cancel := context.WithTimeout(context.Background(), compileTimeout)
				args := append(append([]string{}, javacLocaleArgs...),
					"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "1000000",
					"-proc:none", "-cp", classpath, "-d", outDir, f)
				cmd := exec.CommandContext(ctx, javac, args...)
				cmd.Dir = outDir // keep the auto-spilled javac.<ts>.args out of test/cross
				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					n := strings.Count(string(out), ": error:")
					if n == 0 {
						n = 1 // non-zero exit with no parsed error line (e.g. timeout) still counts as a failure
					}
					mu.Lock()
					badPaths[f] = struct{}{}
					errLines += n
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return len(badPaths), errLines, badPaths
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
