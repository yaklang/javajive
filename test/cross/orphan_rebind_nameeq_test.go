package cross

// 承重测试:「同一 JVM 槽的逻辑变量在多个兄弟/嵌套作用域各自被 rewriteVar 铸出新 *VariableId, 但这些
// 铸出的 id 渲染相同的 varN 拼写」时, replayUnambiguousRebindings 的指针严格唯一性测试把该 oldId 判为
// 多目标(ambiguous)而跳过, 遂令跨作用域的读使用点(如 invokeinterface 的 receiver)保留 pre-mint 旧 id,
// 渲染成与另一槽位同名的 varN → 撞名 → javac「cannot find symbol: method ... location: variable varN of
// type Object」。治本把多目标判别从「指针唯一」放宽到「渲染名唯一」: 同名目标等价、任取其一重绑即可,
// 因为渲染名相同即代表同一变量; 渲染名仍不同的目标保持真 ambiguous、照旧跳过。
//
// 靶子: fastjson2 JSONPathSegmentName.eval offset-345 `Collection.addAll` —— slot5(JSONArray 累加器)的
// receiver 读保留 pre-mint `var8` id(其 allReplace 条目列出两个兄弟臂各自铸出的 `var9` id), 修前渲染
// `var8.addAll((Collection)var8)` javac 报「找不到符号: 方法 addAll(Collection), 位置: 类型为 Object 的
// 变量 var8」; 修后 receiver 重绑到 `var9` → `var9_1.addAll((Collection)var8)` 编译通过。kill-switch
// JDEC_ORPHAN_REBIND_NAMEEQ_OFF=1 关掉必复现。
//
// 承重口径: 整 fastjson2 jar 树编译。关掉 kill-switch, fastjson2 整树错误行数必严格增多(本修清掉
// JSONPathSegmentName 恰好 2 行)。

import (
	"os"
	"strings"
	"testing"
)

// TestOrphanRebindNameEqIsLoadBearing pins the name-equivalence relaxation of replayUnambiguousRebindings
// as load-bearing on the whole fastjson2 jar. The residual it targets (a slot store minted in two
// sibling/nested scopes under distinct *VariableIds that render the same varN, whose cross-scope read
// use-site — the invokeinterface receiver — keeps the pre-mint colliding id) only surfaces once the
// entire jar is compiled as one tree. With the fix ON the colliding read is rebound to its declared id
// and compiles; disabling the name-equivalence branch via the kill-switch must reintroduce strictly
// more tree errors (the 2 JSONPathSegmentName addAll/add lines).
func TestOrphanRebindNameEqIsLoadBearing(t *testing.T) {
	lookJavac(t)
	spec, ok := jarSpecs["fastjson2"]
	if !ok {
		t.Skip("fastjson2 spec missing; skipping")
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	deps := resolveDeps(spec.depGlob)
	cp := withJfr(t, withSunMisc(t, strings.Join(deps, string(os.PathListSeparator))))

	const sw = "JDEC_ORPHAN_REBIND_NAMEEQ_OFF"
	treeErrs := func(killOff bool) int {
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
		root := t.TempDir()
		files, _, _ := decompileAll(t, jarPath, root, 0)
		outDir := t.TempDir()
		_, out := treeCompileToDir(t, files, cp, outDir)
		return len(parseTreeErrors(out, root))
	}

	on := treeErrs(false) // fix ON
	off := treeErrs(true) // fix OFF (kill-switch)
	t.Logf("fastjson2 tree error lines: ON=%d OFF=%d", on, off)

	if off <= on {
		t.Fatalf("orphan-rebind name-equivalence is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
