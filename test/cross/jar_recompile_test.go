package cross

// 本文件是「反编译 -> javac 重编译 -> 错误计数」的权威度量 harness, 对应 CODEC_TODO 里
// 引用的 TestScratchPerFileIso / TestScratchProfile / TestScratchJarErrDelta。它从本机
// ~/.m2 解析真实 jar (guava / fastjson2 / commons-codec / spring-core) 及其传递依赖,
// 用生产 JarFS 路径反编译每个 .class, 再用 javac 重编译, 统计 decompiler 错误数。
//
// 两种度量口径:
//   - tree: 整目录一次性 javac (deps 上 classpath)。快, 但受 javac 错误遮蔽, 仅用于找最大杠杆。
//   - iso : 逐文件隔离 javac (deps + 原 jar 上 classpath, 并行)。免遮蔽, 是治本 delta 的准绳。
//
// 用法 (全部 opt-in, 缺 javac 或缺 jar 自动 t.Skip):
//   PROFILE_JAR=fastjson2 go test -run TestJarRecompileProfile ./test/cross/
//   PROFILE_JAR=all RECOMPILE_MODE=iso go test -run TestJarRecompileProfile ./test/cross/
//   PROFILE_JAR=fastjson2 KILL_SWITCH=JDEC_LIVEINTERVAL_OFF go test -run TestJarRecompileDelta ./test/cross/
//
// 其它可选环境变量: MAXFILES (限制单元数, 快速冒烟), RECOMPILE_WORKERS (并行 javac 数)。
//
// 基线快照 (javac 17.0.12, 本机 ~/.m2, 各 Phase 治本以此为 delta 参照系; 绝对值随 JDK 版本浮动):
//
//	jar         units   tree decErr   iso fail
//	codec       106     1             38
//	fastjson2   681     680           342
//	guava       1825    688           1154
//	spring      974     14            379
//
// tree 口径与 CODEC_TODO 权威整树度量吻合 (codec=1 / spring=14 精确, guava≈699, 受 javac 遮蔽);
// iso 口径与 TODO 逐文件率吻合 (fastjson2≈50.5% / spring≈61.6% / codec≈68.9%)。

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

// compileTimeout bounds a single javac invocation so a pathological unit cannot stall the profile.
const compileTimeout = 60 * time.Second

// javacLocaleArgs forces javac diagnostics to English. The local JDK may be localized (e.g. Chinese
// "错误:"), which would make the tree-mode `: error:` line count read zero; pinning the locale makes
// the error histogram reliable. iso mode keys off the exit code so it is unaffected, but the flag is
// harmless there too.
var javacLocaleArgs = []string{"-J-Duser.language=en", "-J-Duser.country=US"}

// jarSpec 描述一个待度量的真实 jar 及其依赖 jar 的 maven 相对路径 (相对 ~/.m2/repository)。
// 依赖用 glob 模式表达 (按 artifact 名匹配, 不写死版本), 解析不到的依赖被静默跳过 (classpath 降级)。
type jarSpec struct {
	relPath string   // 主 jar 相对 ~/.m2/repository 的路径
	depGlob []string // 依赖 jar 的 glob (相对 ~/.m2/repository), 找不到则跳过
}

var jarSpecs = map[string]jarSpec{
	"guava": {
		relPath: "com/google/guava/guava/28.2-android/guava-28.2-android.jar",
		depGlob: []string{
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
			"com/google/errorprone/error_prone_annotations/*/error_prone_annotations-*.jar",
			"com/google/j2objc/j2objc-annotations/*/j2objc-annotations-*.jar",
			"com/google/guava/failureaccess/*/failureaccess-*.jar",
			"org/checkerframework/checker-compat-qual/*/checker-compat-qual-*.jar",
			"org/checkerframework/checker-qual/*/checker-qual-*.jar",
		},
	},
	"fastjson2": {
		relPath: "com/alibaba/fastjson2/fastjson2/2.0.43/fastjson2-2.0.43.jar",
	},
	"codec": {
		relPath: "commons-codec/commons-codec/1.15/commons-codec-1.15.jar",
	},
	"spring": {
		relPath: "org/springframework/spring-core/5.3.27/spring-core-5.3.27.jar",
		depGlob: []string{
			"org/springframework/spring-jcl/5.3.27/spring-jcl-5.3.27.jar",
		},
	},
	// 以下四个与 benchmark_test.go 的 benchmarkJars 对齐, 使 tree/iso inventory 也能对它们分桶选靶。
	"gson": {
		relPath: "com/google/code/gson/gson/2.8.9/gson-2.8.9.jar",
	},
	"commons-lang3": {
		relPath: "org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar",
	},
	"jsoup": {
		relPath: "org/jsoup/jsoup/1.10.2/jsoup-1.10.2.jar",
	},
	"snakeyaml": {
		relPath: "org/yaml/snakeyaml/2.2/snakeyaml-2.2.jar",
	},
}

func m2Repo() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".m2", "repository")
}

// resolveJar returns the absolute path of relPath under ~/.m2/repository, or "" when absent.
func resolveJar(relPath string) string {
	p := filepath.Join(m2Repo(), filepath.FromSlash(relPath))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// resolveDeps globs each dep pattern under ~/.m2/repository and returns the first match for each.
func resolveDeps(globs []string) []string {
	var out []string
	for _, g := range globs {
		matches, _ := filepath.Glob(filepath.Join(m2Repo(), filepath.FromSlash(g)))
		if len(matches) > 0 {
			sort.Strings(matches)
			out = append(out, matches[len(matches)-1]) // 取版本号最大的一个
		}
	}
	return out
}

// classEntries lists every *.class entry name (slash form) inside jarPath.
func classEntries(t *testing.T, jarPath string) []string {
	t.Helper()
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open jar %s: %v", jarPath, err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".class") && !strings.HasSuffix(f.Name, "module-info.class") {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	return names
}

// decompileFailedMarker reports whether src is a decompile-failure sentinel comment (the
// decompileClassBytes fallback). Such a "source" is just a comment and would silently compile to
// nothing, so it must be counted as a failure rather than a success.
func decompileFailedMarker(src string) bool {
	s := strings.TrimSpace(src)
	return strings.HasPrefix(s, "// decompile parse failed") ||
		strings.HasPrefix(s, "// decompile dump failed")
}

// enumFoldSuppressed reports whether src is the synthetic enum-subclass suppression marker (folded
// into its enum, regenerated by javac). These are intentional no-ops and excluded from the unit set.
func enumFoldSuppressed(src string) bool {
	return strings.Contains(src, "synthetic enum constant-body subclass folded into enum")
}

type recompileResult struct {
	units      int // 参与编译的单元数 (排除 enum-fold 抑制单元)
	decErr     int // 编译失败的单元数 (iso) 或 javac 报告的 error 行数 (tree)
	decompFail int // 反编译本身失败 (sentinel comment) 的单元数, 计入 decErr
}

// decompileAll decompiles every class entry via the production JarFS path and writes each unit to
// <root>/<package path>/<SimpleName>.java. Returns the written file paths and the unit/fail counts.
func decompileAll(t *testing.T, jarPath, root string, maxFiles int) (files []string, units, decompFail int) {
	t.Helper()
	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal %s: %v", jarPath, err)
	}
	entries := classEntries(t, jarPath)
	if maxFiles > 0 && len(entries) > maxFiles {
		entries = entries[:maxFiles]
	}
	for _, entry := range entries {
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			// ReadFile maps a class entry to its decompiled source; a read error is itself a failure.
			decompFail++
			continue
		}
		src := string(raw)
		if enumFoldSuppressed(src) {
			continue // 折叠进 enum, 不算独立单元
		}
		units++
		if decompileFailedMarker(src) {
			decompFail++
			// 仍写出 (会编译失败), 让 iso 计数把它算进 decErr
		}
		// entry 形如 com/google/common/math/LongMath$1.class
		rel := strings.TrimSuffix(entry, ".class") + ".java" // 保留扁平 $ 名
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(dst, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		files = append(files, dst)
	}
	return files, units, decompFail
}

// recompileISO compiles each file in isolation (deps + original jar on classpath) in parallel and
// returns the number of units that fail to compile. This is the un-masked, authoritative metric.
func recompileISO(t *testing.T, files []string, classpath string, workers int) int {
	t.Helper()
	javac := lookJavac(t)
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed int
		ch     = make(chan string, len(files))
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
					"-encoding", "UTF-8", "--release", "8", "-nowarn",
					"-cp", classpath, "-d", outDir, f)
				cmd := exec.CommandContext(ctx, javac, args...)
				// Run javac from the throwaway out dir: with a long deps+jar classpath the JDK
				// launcher auto-spills a `javac.<ts>.args` argfile into the CWD, which would
				// otherwise litter test/cross. Mirrors recompileTree. f and classpath are absolute.
				cmd.Dir = outDir
				err := cmd.Run()
				cancel()
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return failed
}

// recompileTree compiles all files in one javac invocation (deps on classpath) and returns the
// number of error lines javac reports. This is fast but masked; use only to find the biggest levers.
func recompileTree(t *testing.T, files []string, classpath string) int {
	t.Helper()
	javac := lookJavac(t)
	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", classpath, "-d", outDir)
	args = append(args, files...)
	cmd := exec.Command(javac, args...)
	// Run javac from the throwaway out dir: the JDK launcher auto-spills a `javac.<ts>.args`
	// argfile into the CWD when the (huge, whole-jar) command line exceeds the OS arg limit, which
	// would otherwise litter the repo's test/cross directory. The temp dir is removed by t.Cleanup.
	cmd.Dir = outDir
	out, _ := cmd.CombinedOutput()
	return strings.Count(string(out), ": error:")
}

// runProfile decompiles+recompiles one jar under the current environment and returns the result.
func runProfile(t *testing.T, name string, maxFiles int) recompileResult {
	t.Helper()
	spec, ok := jarSpecs[name]
	if !ok {
		t.Fatalf("unknown jar %q (have: %v)", name, jarKeys())
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skipf("jar %s not found under %s; skipping", spec.relPath, m2Repo())
	}
	deps := resolveDeps(spec.depGlob)

	root := t.TempDir()
	files, units, decompFail := decompileAll(t, jarPath, root, maxFiles)

	mode := os.Getenv("RECOMPILE_MODE")
	if mode == "" {
		mode = "iso"
	}
	workers, _ := strconv.Atoi(os.Getenv("RECOMPILE_WORKERS"))

	var decErr int
	switch mode {
	case "tree":
		cp := withSunMisc(t, strings.Join(deps, string(os.PathListSeparator)))
		decErr = recompileTree(t, files, cp)
	default: // iso
		cpParts := append([]string{jarPath}, deps...)
		cp := withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator)))
		decErr = recompileISO(t, files, cp, workers)
	}
	return recompileResult{units: units, decErr: decErr, decompFail: decompFail}
}

func jarKeys() []string {
	var ks []string
	for k := range jarSpecs {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// TestJarRecompileProfile measures decompile->recompile error counts for one jar (or all). It is the
// baseline-capture tool: run it per jar to snapshot decErr, then compare across phases.
func TestJarRecompileProfile(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		t.Skip("set PROFILE_JAR=<guava|fastjson2|codec|spring|all> to run the recompile profile")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			res := runProfile(t, name, maxFiles)
			t.Logf("[%s] mode=%s units=%d decErr=%d decompileFail=%d",
				name, orDefault(os.Getenv("RECOMPILE_MODE"), "iso"), res.units, res.decErr, res.decompFail)
		})
	}
}

// TestJarRecompileDelta runs the A/B kill-switch comparison: pass A with the switch unset (fix ON =
// baseline), pass B with the switch set (fix OFF). A positive delta (B-A) is the fix's real benefit.
func TestJarRecompileDelta(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	ks := os.Getenv("KILL_SWITCH")
	if target == "" || ks == "" {
		t.Skip("set PROFILE_JAR=<jar> and KILL_SWITCH=<JDEC_X> to run the A/B delta")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			prev, had := os.LookupEnv(ks)
			defer func() {
				if had {
					os.Setenv(ks, prev)
				} else {
					os.Unsetenv(ks)
				}
			}()

			os.Unsetenv(ks) // 修复 ON (baseline)
			on := runProfile(t, name, maxFiles)
			os.Setenv(ks, "1") // 修复 OFF
			off := runProfile(t, name, maxFiles)

			delta := off.decErr - on.decErr
			t.Logf("[%s] %s ON(decErr)=%d OFF(decErr)=%d delta(OFF-ON)=%+d units=%d",
				name, ks, on.decErr, off.decErr, delta, on.units)
			if delta < 0 {
				t.Errorf("kill-switch %s shows fix INCREASED errors by %d (regression?): ON=%d OFF=%d",
					ks, -delta, on.decErr, off.decErr)
			}
		})
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
