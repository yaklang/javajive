package cross

// 本文件在权威 iso 度量 (jar_recompile_test.go) 之上加一层「失败枚举 + reason 分桶」，
// 把"有多少单元编译失败"细化成"具体哪些单元、因为什么 javac 错误失败"，作为根目录
// TODO.md 缺陷清单与单点修复选靶的唯一数据源。它只读、不改任何反编译逻辑，复用同一套
// decompileAll / resolveJar / resolveDeps / jarSpecs 助手，确保口径与 iso delta 完全一致。
//
// 用法 (opt-in, 缺 javac 或缺 jar 自动 t.Skip):
//   PROFILE_JAR=codec      go test -run TestJarIsoInventory -v ./test/cross/
//   PROFILE_JAR=all        go test -run TestJarIsoInventory -v ./test/cross/
//   PROFILE_JAR=all ISO_REPORT_DIR=/tmp/jdec-inventory go test -run TestJarIsoInventory -v ./test/cross/
//
// 产物 (默认写到 ISO_REPORT_DIR, 缺省 /tmp/jdec-inventory):
//   <jar>.fails.txt    每个失败单元一行: <unit relpath>\t<reason 分类>\t<首条 javac error>
//   <jar>.reasons.txt  reason 分类直方图 (按计数降序), 用于定位"最大杠杆"的失败模式
// 控制台同时打印每个 jar 的 units / fail / top-5 reasons 摘要。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// isoFailure 记录单个失败单元: 反编译产物相对路径 + javac 首条错误 + 归一化 reason 分类。
type isoFailure struct {
	unit   string // 相对 root 的 .java 路径 (扁平 $ 名), 即 jar 内 class 的全限定
	reason string // 归一化错误分类 (见 categorizeJavacError)
	errMsg string // javac 首条 ": error:" 行原文 (去掉临时目录前缀)
}

// categorizeJavacError 把一条 javac 诊断归一化成稳定的 reason 分类, 让形态相同、标识符不同的
// 失败落进同一个桶。返回的分类用于直方图与 TODO 分桶；未识别的归入 "other: <首段>"。
func categorizeJavacError(line string) string {
	i := strings.Index(line, "error:")
	msg := line
	if i >= 0 {
		msg = strings.TrimSpace(line[i+len("error:"):])
	}
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "cannot find symbol"):
		return "cannot find symbol"
	case strings.Contains(low, "bad type in conditional expression"):
		return "incompatible types: bad conditional (ternary LUB)"
	case strings.Contains(low, "inconvertible types"):
		return "inconvertible types (bad cast)"
	case strings.Contains(low, "incompatible types"):
		return "incompatible types (assignment/return)"
	case strings.Contains(low, "cannot be applied to given types"),
		strings.Contains(low, "no suitable method found"):
		return "method invocation: cannot be applied / no suitable method"
	case strings.Contains(low, "cannot infer type arguments"),
		strings.Contains(low, "inference variable"):
		return "generic type inference"
	case strings.Contains(low, "unreported exception"):
		return "unreported checked exception"
	case strings.Contains(low, "might not have been initialized"),
		strings.Contains(low, "variable") && strings.Contains(low, "already been assigned"):
		return "definite-assignment (init/final flow)"
	case strings.Contains(low, "is not abstract and does not override"),
		strings.Contains(low, "does not override abstract method"):
		return "abstract method not overridden"
	case strings.Contains(low, "incomparable types"):
		return "incomparable types (==/!=)"
	case strings.Contains(low, "is ambiguous"):
		return "ambiguous reference"
	case strings.Contains(low, "does not exist"):
		return "package/import does not exist"
	case strings.Contains(low, "cannot be dereferenced"):
		return "cannot be dereferenced"
	case strings.Contains(low, "unexpected type"):
		return "unexpected type (lvalue/rvalue)"
	case strings.Contains(low, "array required"):
		return "array required but found"
	case strings.Contains(low, "bad operand type"):
		return "bad operand type for operator"
	case strings.Contains(low, "already defined"),
		strings.Contains(low, "is already defined"):
		return "duplicate declaration"
	case strings.Contains(low, "non-static") && strings.Contains(low, "cannot be referenced"):
		return "non-static reference from static context"
	case strings.Contains(low, "illegal start"),
		strings.Contains(low, "not a statement"),
		strings.Contains(low, "';' expected"),
		strings.Contains(low, "expected"):
		return "syntax error (malformed output)"
	case strings.Contains(low, "unreachable statement"):
		return "unreachable statement"
	default:
		head := msg
		if len(head) > 60 {
			head = head[:60]
		}
		return "other: " + head
	}
}

// firstJavacError 从 javac 合并输出里取第一条 ": error:" 行, 并把临时目录前缀压成 basename,
// 便于稳定记录与比对 (临时目录每次随机)。无 error 行时返回空串。
func firstJavacError(out string) string {
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, ": error:") {
			ln = strings.TrimSpace(ln)
			// 把绝对临时路径压成 basename: "/tmp/xxx/com/foo/Bar.java:12: error: ..."
			if idx := strings.Index(ln, ".java:"); idx >= 0 {
				if slash := strings.LastIndex(ln[:idx], "/"); slash >= 0 {
					ln = ln[slash+1:]
				}
			}
			return ln
		}
	}
	return ""
}

// recompileISOInventory 与 recompileISO 同口径逐文件隔离编译, 但对失败单元额外捕获首条
// javac error 并归类。返回的失败列表按 unit 字典序排序, 保证报告确定性。
func recompileISOInventory(t *testing.T, files []string, root, classpath string, workers int) []isoFailure {
	t.Helper()
	javac := lookJavac(t)
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		fails []isoFailure
		ch    = make(chan string, len(files))
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
				// Multi-Release versioned units compile under their own --release N (see mrFileRelease).
				args := append(append([]string{}, javacLocaleArgs...),
					"-encoding", "UTF-8", "--release", strconv.Itoa(mrFileRelease(f, 8)), "-nowarn",
					"-cp", classpath, "-d", outDir, f)
				cmd := exec.CommandContext(ctx, javac, args...)
				cmd.Dir = outDir // 同 recompileISO: 防 javac.<ts>.args 落进 test/cross
				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					rel, _ := filepath.Rel(root, f)
					errLine := firstJavacError(string(out))
					reason := "other: (no error line / timeout)"
					if errLine != "" {
						reason = categorizeJavacError(errLine)
					} else if ctx.Err() == context.DeadlineExceeded {
						reason = "javac timeout (pathological output)"
					}
					mu.Lock()
					fails = append(fails, isoFailure{unit: filepath.ToSlash(rel), reason: reason, errMsg: errLine})
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	sort.Slice(fails, func(i, j int) bool { return fails[i].unit < fails[j].unit })
	return fails
}

// treeReason 记录一条整树 javac 诊断: 归属文件(相对 root)、reason 分类、原文。
type treeReason struct {
	unit   string
	reason string
	errMsg string
}

// parseTreeErrors 从整树 javac 合并输出里抽取每条 ": error:" 行, 归属到文件并归类。
// 行形如 `/tmp/xxx/com/foo/Bar.java:12: error: msg`; 用 root 还原成 jar 内全限定单元名。
func parseTreeErrors(out, root string) []treeReason {
	var rs []treeReason
	for _, ln := range strings.Split(out, "\n") {
		if !strings.Contains(ln, ": error:") {
			continue
		}
		unit := ""
		if idx := strings.Index(ln, ".java:"); idx >= 0 {
			path := ln[:idx] + ".java"
			if rel, err := filepath.Rel(root, path); err == nil {
				unit = filepath.ToSlash(rel)
			} else {
				unit = filepath.Base(path)
			}
		}
		rs = append(rs, treeReason{unit: unit, reason: categorizeJavacError(ln), errMsg: strings.TrimSpace(ln)})
	}
	return rs
}

// TestJarTreeInventory 反编译目标 jar(或 all), 整树一次性 javac 重编译, 把每条 javac error 归属
// 到文件+reason 落盘。整树是重打包的真实口径: 兄弟扁平单元一起编译, 不会有 iso 的扁平 `$` 假阳性,
// 所以这里的失败才是真正阻碍"反编译→重编译→重打包"的缺陷。这是 TODO.md 与单点修复的权威选靶源。
func TestJarTreeInventory(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		t.Skip("set PROFILE_JAR=<guava|fastjson2|codec|spring|all> to run the tree (repackage) inventory")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))
	reportDir := os.Getenv("ISO_REPORT_DIR")
	if reportDir == "" {
		reportDir = filepath.Join(os.TempDir(), "jdec-inventory")
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			spec, ok := jarSpecs[name]
			if !ok {
				t.Fatalf("unknown jar %q (have: %v)", name, jarKeys())
			}
			jarPath := resolveJar(spec.relPath)
			if jarPath == "" {
				t.Skipf("jar %s not found under %s; skipping", spec.relPath, m2Repo())
			}
			deps := resolveDeps(spec.depGlob)
			cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))
			root := t.TempDir()
			files, units, _ := decompileAll(t, jarPath, root, maxFiles)

			outDir := t.TempDir()
			// treeCompileToDir gives Multi-Release `META-INF/versions/N/` units their own
			// `--release N` pass (see splitMRFiles), so they are not false-failed under --release 8.
			_, out := treeCompileToDir(t, files, cp, outDir)
			errs := parseTreeErrors(out, root)

			// reason 直方图 + 失败单元集合 (整树口径下, 一个单元可能贡献多条 error)。
			hist := map[string]int{}
			blockerUnits := map[string]bool{}
			for _, e := range errs {
				hist[e.reason]++
				if e.unit != "" {
					blockerUnits[e.unit] = true
				}
			}
			type kv struct {
				reason string
				n      int
			}
			var ranked []kv
			for r, n := range hist {
				ranked = append(ranked, kv{r, n})
			}
			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].n != ranked[j].n {
					return ranked[i].n > ranked[j].n
				}
				return ranked[i].reason < ranked[j].reason
			})

			var failBuf strings.Builder
			fmt.Fprintf(&failBuf, "# %s tree errors: %d error lines across %d blocker units / %d total units\n",
				name, len(errs), len(blockerUnits), units)
			sort.Slice(errs, func(i, j int) bool {
				if errs[i].unit != errs[j].unit {
					return errs[i].unit < errs[j].unit
				}
				return errs[i].errMsg < errs[j].errMsg
			})
			for _, e := range errs {
				fmt.Fprintf(&failBuf, "%s\t%s\t%s\n", e.unit, e.reason, e.errMsg)
			}
			failPath := filepath.Join(reportDir, name+".tree.fails.txt")
			if err := os.WriteFile(failPath, []byte(failBuf.String()), 0o644); err != nil {
				t.Fatalf("write tree fails: %v", err)
			}

			var reasonBuf strings.Builder
			fmt.Fprintf(&reasonBuf, "# %s tree reason histogram: %d error lines / %d blocker units / %d total units\n",
				name, len(errs), len(blockerUnits), units)
			for _, k := range ranked {
				fmt.Fprintf(&reasonBuf, "%6d  %s\n", k.n, k.reason)
			}
			reasonPath := filepath.Join(reportDir, name+".tree.reasons.txt")
			if err := os.WriteFile(reasonPath, []byte(reasonBuf.String()), 0o644); err != nil {
				t.Fatalf("write tree reasons: %v", err)
			}

			top := ranked
			if len(top) > 6 {
				top = top[:6]
			}
			var topStr []string
			for _, k := range top {
				topStr = append(topStr, fmt.Sprintf("%s=%d", k.reason, k.n))
			}
			t.Logf("[%s] units=%d treeErrLines=%d blockerUnits=%d report=%s | top: %s",
				name, units, len(errs), len(blockerUnits), failPath, strings.Join(topStr, " ; "))
		})
	}
}

// TestJarIsoInventory 反编译目标 jar(或 all), 逐文件 iso 重编译, 把失败单元 + reason 分桶
// 落盘并打印摘要。这是 TODO.md 缺陷清单的数据源, 也是单点修复的选靶入口。
func TestJarIsoInventory(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		t.Skip("set PROFILE_JAR=<guava|fastjson2|codec|spring|all> to run the iso failure inventory")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))
	workers, _ := strconv.Atoi(os.Getenv("RECOMPILE_WORKERS"))
	reportDir := os.Getenv("ISO_REPORT_DIR")
	if reportDir == "" {
		reportDir = filepath.Join(os.TempDir(), "jdec-inventory")
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
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
			files, units, _ := decompileAll(t, jarPath, root, maxFiles)

			cpParts := append([]string{jarPath}, deps...)
			cp := withJfr(t, withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator))))
			fails := recompileISOInventory(t, files, root, cp, workers)

			// reason 直方图 (按计数降序, 同计数按字母序, 确定性)。
			hist := map[string]int{}
			for _, f := range fails {
				hist[f.reason]++
			}
			type kv struct {
				reason string
				n      int
			}
			var ranked []kv
			for r, n := range hist {
				ranked = append(ranked, kv{r, n})
			}
			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].n != ranked[j].n {
					return ranked[i].n > ranked[j].n
				}
				return ranked[i].reason < ranked[j].reason
			})

			// 落盘失败明细。
			var failBuf strings.Builder
			fmt.Fprintf(&failBuf, "# %s iso failures: %d of %d units\n", name, len(fails), units)
			for _, f := range fails {
				fmt.Fprintf(&failBuf, "%s\t%s\t%s\n", f.unit, f.reason, f.errMsg)
			}
			failPath := filepath.Join(reportDir, name+".fails.txt")
			if err := os.WriteFile(failPath, []byte(failBuf.String()), 0o644); err != nil {
				t.Fatalf("write fails: %v", err)
			}

			// 落盘 reason 直方图。
			var reasonBuf strings.Builder
			fmt.Fprintf(&reasonBuf, "# %s reason histogram: %d failures / %d units\n", name, len(fails), units)
			for _, k := range ranked {
				fmt.Fprintf(&reasonBuf, "%6d  %s\n", k.n, k.reason)
			}
			reasonPath := filepath.Join(reportDir, name+".reasons.txt")
			if err := os.WriteFile(reasonPath, []byte(reasonBuf.String()), 0o644); err != nil {
				t.Fatalf("write reasons: %v", err)
			}

			top := ranked
			if len(top) > 5 {
				top = top[:5]
			}
			var topStr []string
			for _, k := range top {
				topStr = append(topStr, fmt.Sprintf("%s=%d", k.reason, k.n))
			}
			t.Logf("[%s] units=%d isoFail=%d report=%s | top: %s",
				name, units, len(fails), failPath, strings.Join(topStr, " ; "))
		})
	}
}
