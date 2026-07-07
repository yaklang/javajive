package cross

// 承重测试 (Bug AL-web): 局部变量活跃区间 web 修复 (kill-switch JDEC_LIVEINTERVAL_WEB_OFF)。
//
// reachingSlotVersionByWeb / reachingSlotStoreContinuationByWeb 在「一个 JVM 槽位的多个到达定义
// 经 web 分析证明确属同一源变量(同 VarUid)」时, 把 load/store 重定向到该 web 的规范 ref, 修正
// DFS 遍历序把后到/不相交分支版本漏进槽位表导致的读错变量。历史上是 opt-in (默认关), 注释称
// 「iso delta +0, tree 略负」。重测当前 8-jar tree 口径后发现是严格改进: fastjson2 tree
// errLines 24->22 (ObjectReaderCreator 3->2, JSONPathParser 2->1), 其余 jar 全部持平, 故翻成默认开。
//
// 此测试用 fastjson2 ObjectReaderCreator 组(整组重编译)断言「修复 ON 的 javac 错误数 严格少于
// 修复 OFF (kill-switch)」, 即修复是 load-bearing 的。命中行: ObjectReaderCreator `Object cannot be
// converted to ObjectReader` (一个 Map.get 的 Object 读被错误绑定到了独立 ref)。

import (
	"os"
	"testing"
)

// TestLiveIntervalWebRepairIsLoadBearing pins the web load/store read-redirect repair on the fastjson2
// ObjectReaderCreator group: turning it OFF (JDEC_LIVEINTERVAL_WEB_OFF) must reproduce strictly more
// recompile errors than the default (ON).
func TestLiveIntervalWebRepairIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const prefix = "com/alibaba/fastjson2/reader/ObjectReaderCreator"

	on := groupRecompileErrors(t, jarPath, prefix, false) // fix ON (web default)
	off := webOffRecompileErrors(t, jarPath, prefix)      // fix OFF (JDEC_LIVEINTERVAL_WEB_OFF)
	t.Logf("ObjectReaderCreator group recompile errors: ON(web)=%d OFF(web)=%d", on, off)

	if off <= on {
		t.Errorf("web repair is NOT load-bearing: ON=%d OFF=%d (OFF must reproduce more errors)", on, off)
	}
}

// webOffRecompileErrors is groupRecompileErrors with the web kill-switch held OFF for the decompile.
// groupRecompileErrors already toggles JDEC_LIVEINTERVAL_OFF (the master) for its own measurement; we
// reuse it but additionally force JDEC_LIVEINTERVAL_WEB_OFF=1 so the web read-redirects are disabled
// even though slotWebs() (gated by the master) still computes.
func webOffRecompileErrors(t *testing.T, jarPath, classPrefix string) int {
	t.Helper()
	prev, had := os.LookupEnv("JDEC_LIVEINTERVAL_WEB_OFF")
	os.Setenv("JDEC_LIVEINTERVAL_WEB_OFF", "1")
	defer func() {
		if had {
			os.Setenv("JDEC_LIVEINTERVAL_WEB_OFF", prev)
		} else {
			os.Unsetenv("JDEC_LIVEINTERVAL_WEB_OFF")
		}
	}()
	return groupRecompileErrors(t, jarPath, classPrefix, false)
}
