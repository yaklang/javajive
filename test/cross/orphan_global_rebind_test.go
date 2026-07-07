package cross

// 承重测试:「同一 JVM 槽的 store 在嵌套作用域被铸出新 id, 但其同 VarUid 的 READ 落在兄弟作用域(非该
// 铸造作用域的后代)时, 每个 rewriteVar 作用域只重写自己的语句切片, 兄弟作用域里的读保留 pre-mint 旧 id
// → 旧 id 无声明、渲染裸 varN 拼写, 与另一处同拼写的无关局部撞名」治本(replayUnambiguousRebindings)。
//
// 治本把每个作用域 defer 的 (oldId->newId) 重绑累积到方法级共享表, 在 rewriteVar 递归结束后, 对
// 「映射唯一」的 oldId 再做一次全方法 ReplaceVar, 补回兄弟作用域的孤儿读; 不相交槽(一个 oldId 重绑到
// 多个 newId)自动跳过。kill-switch JDEC_ORPHAN_GLOBAL_REBIND_OFF=1 关掉必复现。
//
// 早期本测试以 fastjson2 JSONPathParser.parseFilter 的 16 处 "cannot be converted to String" 为靶,
// 但 reachingRefSlotNullReassignMerge(条件 null 重赋值 phi)落地后, 该 parseFilter 的 slot-7 null 现在
// 在栈模拟阶段就并回单一变量, JSONPathParser 家族无论开关都已清零 —— 该单类不再能复现 orphan 缺陷。
// orphan-rebind 仍对整 jar 承重(JSONReaderJSONB / ObjectReaderImplListStr / ObjectWriterCreator 等的
// 孤儿读只有整树上下文才显形), 故改为整 jar 树编译口径: 关掉 kill-switch, fastjson2 整树 error 行数必
// 严格增多。

import (
	"os"
	"strings"
	"testing"
)

// TestOrphanGlobalRebindIsLoadBearing pins the cross-scope orphan-read rebind as load-bearing on the
// WHOLE fastjson2 jar. The residual it targets (a slot store minted in a nested scope whose sibling-scope
// reads keep the pre-mint id and render a bare varN that collides with an unrelated local) only surfaces
// once the entire jar is compiled as one tree, so a single-class exemplar is no longer sufficient (the
// former JSONPathParser exemplar is now additionally covered by reachingRefSlotNullReassignMerge). With
// the fix ON the orphan reads are rebound to their declared id and compile; disabling the replay via the
// kill-switch must reintroduce strictly more tree errors.
func TestOrphanGlobalRebindIsLoadBearing(t *testing.T) {
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

	const sw = "JDEC_ORPHAN_GLOBAL_REBIND_OFF"
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
		t.Fatalf("orphan-rebind is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
