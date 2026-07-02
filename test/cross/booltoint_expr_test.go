package cross

// 承重测试:「结构性布尔值(短路 `||`/`&&` 的三元物化、比较、返回 Z 的方法调用)赋给 int 目标时缺 `? 1 : 0`
// 造型」治本(CODEC_TODO §2「内在布尔值赋 int 缺 `? 1 : 0` 造型」的结构性扩展支)。
//
// 真实 fastjson2:`FieldWriterObject.getObjectWriter` 里 slot 4 是源码 `int typeMatch = (a==b) || ... ? 1 : 0;`
// (旧 javac 把已是 0/1 的布尔留栈直接 `istore`, 省去分支), 反编译复原成:
//   line 52: `int var4 = ((var3)==(var2)) || (...)`         —— 短路 `||`(由 `(cond)?1:0` 三元树折叠而来)
//   line 54: `var4 = typeMatch(var3,var2)`                  —— 返回 Z 的方法调用
// 二者赋给 int `var4` → javac `boolean cannot be converted to int`(2 处)。
//
// 既有 `IntrinsicBooleanValue` 只认「非短路布尔连接 `&|^` 或双臂皆 boolean 的三元」, 够不到:
//   (a) 短路 `||` 的三元物化(外层三元一臂是字面量、一臂是嵌套三元, 直接双臂判定为假);
//   (b) 返回 Z 的方法调用。
// 治法: 新增 `structurallyBooleanForIntCoerce`(JDEC_BOOL_TO_INT_COERCE_EXPR_OFF)—— 对 `*JavaCompare`、
// boolean 型 `*JavaExpression`/`*FunctionCallExpression`、以及经 `boolReduce` 折成 boolean 的三元(短路
// `||`/`&&` 物化)判定为「按构造为布尔」, 补 `(rhs) ? (1) : (0)`。严格排除裸 ref(避免误编译被错并成
// boolean 实则持 int 的槽), 与既有 `IntrinsicBooleanValue` 路径互补。
// JDEC_BOOL_TO_INT_COERCE_EXPR_OFF=1 关掉扩展必复现 2 处 `boolean cannot be converted to int`。
//
// 该缺陷是逐文件(iso)可复现的真错(与扁平 $ import 假阳性无关): 单编 FieldWriterObject.java 对原始 jar
// 即可暴露, 故本测试走整 jar 反编译 + 单文件 iso 重编译口径, 用唯一错误签名
// "boolean cannot be converted to int" 精确隔离本缺陷。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// boolExprToIntCount decompiles the WHOLE fastjson2 jar under the kill-switch, then compiles
// FieldWriterObject.java ALONE against the original jar (iso), and counts the
// "boolean cannot be converted to int" error. That substring is unique to the getObjectWriter
// int-slot boolean assignments (line 52 short-circuit `||`, line 54 typeMatch()Z), so it isolates
// precisely this defect (unaffected by flat-$ import false positives).
func boolExprToIntCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_BOOL_TO_INT_COERCE_EXPR_OFF"
	prev, had := os.LookupEnv(sw)
	if killOff {
		os.Setenv(sw, "1")
	} else {
		os.Unsetenv(sw)
	}
	defer func() {
		if had {
			os.Setenv(sw, prev)
		} else {
			os.Unsetenv(sw)
		}
	}()

	spec := jarSpecs["fastjson2"]
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	javac := lookJavac(t)
	deps := resolveDeps(spec.depGlob)

	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)

	// Find the decompiled FieldWriterObject.java among the flattened units.
	var target string
	for _, f := range files {
		if strings.HasSuffix(f, filepath.FromSlash("com/alibaba/fastjson2/writer/FieldWriterObject.java")) {
			target = f
			break
		}
	}
	if target == "" {
		t.Fatal("decompiled FieldWriterObject.java not found")
	}

	// iso classpath: ORIGINAL jar + deps (so FieldWriterObject's own references resolve; the defect is a
	// genuine per-file type error, independent of sibling flattened units).
	cpParts := append([]string{jarPath}, deps...)
	cp := withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator)))

	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", cp, "-d", outDir, target)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ": error:") && strings.Contains(line, "boolean cannot be converted to int") {
			n++
		}
	}
	return n
}

// TestBoolExprToIntCoerceIsLoadBearing pins fastjson2's FieldWriterObject.getObjectWriter: a
// structurally-boolean value (short-circuit `||` folded from a `(cond)?1:0` ternary, and a Z-returning
// typeMatch() call) assigned to an int slot must be re-wrapped `? 1 : 0`. Disabling the structural
// extension via the kill-switch must reintroduce the "boolean cannot be converted to int" errors.
func TestBoolExprToIntCoerceIsLoadBearing(t *testing.T) {
	lookJavac(t)
	if resolveJar(jarSpecs["fastjson2"].relPath) == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}

	on := boolExprToIntCount(t, false) // fix ON
	off := boolExprToIntCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 FieldWriterObject boolean->int errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the structural boolean->int errors: ON=%d (want 0)", on)
	}
}
