package cross

// 承重测试: 当一个 dup;astore 产生的嵌入式赋值 (`varN = expr` 在 invoke 参数中) 的目标变量
// 与一个 catch 参数 (`catch(T varN)`) 同名但类型不同时, 嵌入赋值的 varN 未声明导致
// javac "cannot find symbol"。治本: collectEmbeddedDeclInfos 也收集 TryCatchStatement.Exception
// (catch 参数), 使 SynthesizeUndeclaredEmbeddedAssignDecls 能识别到 catch 参数是不兼容 collider,
// 从而在方法顶部合成 `Type varN;` 声明。Kill-switch: JDEC_EMBED_ASSIGN_DECL_OFF=1。
//
// 真实 fastjson2 JDKUtils <clinit>:
// `var17.findStatic(var31 = Class.class, ...)` 在 try 块中 (slot 23),
// `catch(Throwable var31)` 在另一个 try-catch 中 (slot 24)。
// 治本前: var31 (Class) 未声明 → "cannot find symbol: variable var31"。
// 治本后: 在方法顶部合成 `Class var31;`。

import (
	"os"
	"strings"
	"testing"
)

// embedCatchColliderErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then compiles
// ALL units in tree mode (deps-only classpath) and counts "cannot find symbol.*var31" errors in
// JDKUtils.java (the #JDKUtils-304 signature).
func embedCatchColliderErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_EMBED_ASSIGN_DECL_OFF"
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

	spec, ok := jarSpecs["fastjson2"]
	if !ok {
		t.Fatal("fastjson2 spec missing")
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	deps := resolveDeps(spec.depGlob)
	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)
	cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))
	outDir := t.TempDir()
	_, raw := treeCompileToDir(t, files, cp, outDir)
	count := 0
	for _, l := range strings.Split(raw, "\n") {
		// The JDKUtils:304 error is "cannot find symbol" (the detail "variable var31" is on the
		// next line, not the error line itself). Match the JDKUtils.java file + cannot find symbol.
		if strings.Contains(l, "JDKUtils.java") && strings.Contains(l, "cannot find symbol") {
			count++
		}
	}
	return count
}

// TestEmbedCatchColliderIsLoadBearing pins collectEmbeddedDeclInfos's catch-parameter collection as
// load-bearing on fastjson2's JDKUtils <clinit>: with the fix ON the "cannot find symbol: variable
// var31" is gone from the tree compile; disabling via the kill-switch must reintroduce it.
func TestEmbedCatchColliderIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := embedCatchColliderErrCount(t, false) // fix ON
	off := embedCatchColliderErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 tree JDKUtils 'cannot find symbol var31': ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("catch-param collider collection is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
