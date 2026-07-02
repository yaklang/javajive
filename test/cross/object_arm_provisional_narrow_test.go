package cross

// 承重测试:「一个 JVM 槽的 `java.lang.Object` 声明是『临时的』——它由 DFS 先访问的直通臂 `cur = prev` 铸出,
// 而 `prev` 是尚未定型的 null-init(`ObjectWriter prev = null`); Object-臂合并(reachingRefSlotObjectArmMerge)
// 遂把 `cur` 冻结在 Object, 令后续未造型的 `cur.writeXxx(...)` / `this.field = cur` 编不过」治本
// (CODEC_TODO 泛型/槽位·provisional-Object 收窄)。
//
// 真实 fastjson2:`ObjectWriterArray.write`/`writeJSONB` 等的缓存写出惯用法:
//   ObjectWriter prev = null;                       // 槽 P: null-init, 暂定 Object
//   ...循环...
//   ObjectWriter cur;
//   if (cls == prevCls) cur = prev;                 // DFS 首个直通臂: 拷贝 null-init prev → cur 铸成 Object
//   else { cur = jw.getObjectWriter(cls); prev = cur; }  // 计算 prev/cur 真实类型的臂
//   cur.write(var1,var11,Integer.valueOf(var10),this.itemType,var5);  // 未造型使用 → 需 ObjectWriter
// 若冻结 Object, `cur.write(...)` 报 `cannot find symbol`(method write(...) location: variable of type Object)。
// 治法: reachingRefSlotObjectArmMerge 命中「current 的铸值是未 adopt 的 null-init ref」时, 把 current 收窄到
// 具体臂类型(而非冻结 Object); null-init 源随后由自身 `prev = cur` 存储 adopt 同一类型。安全边界: 真正多态
// 的 Object 局部(`ObjectWriterAdapter.toJSONObject` 的 `var7`, 由 `getFieldValue()` 返回 Object、仅经
// `instanceof`/`(Map)`/`(Collection)` 造型使用)其铸值不是 null-init ref, 故不收窄, 保持 Object。
// JDEC_OBJECT_ARM_PROVISIONAL_NARROW_OFF=1 关掉收窄必复现 `cannot find symbol`。
//
// 该缺陷是逐文件(iso)可复现的真错(与扁平 $ 假阳性无关): 单编 ObjectWriterArray.java 对原始 jar 即可暴露,
// 且该文件 iso 无扁平 $ import 干扰(fix ON 时 cannot-find-symbol=0), 故用唯一错误签名精确隔离。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// objectArmNarrowFindSymbolCount decompiles the WHOLE fastjson2 jar under the kill-switch, then compiles
// ObjectWriterArray.java ALONE against the original jar (iso), and counts "cannot find symbol". With the
// fix ON that file is clean (0); with the fix OFF the cache-writer `cur.write(...)` calls on the frozen
// Object slot fail to resolve.
func objectArmNarrowFindSymbolCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_OBJECT_ARM_PROVISIONAL_NARROW_OFF"
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

	var target string
	for _, f := range files {
		if strings.HasSuffix(f, filepath.FromSlash("com/alibaba/fastjson2/writer/ObjectWriterArray.java")) {
			target = f
			break
		}
	}
	if target == "" {
		t.Fatal("decompiled ObjectWriterArray.java not found")
	}

	cpParts := append([]string{jarPath}, deps...)
	cp := withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator)))

	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", cp, "-d", outDir, target)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ": error:") && strings.Contains(line, "cannot find symbol") {
			n++
		}
	}
	return n
}

// TestObjectArmProvisionalNarrowIsLoadBearing pins fastjson2's ObjectWriterArray cache-writer: a slot
// whose Object declaration is provisional (copied from an un-adopted null-init) must be narrowed to the
// concrete arm type so the uncast writer calls resolve. Disabling the narrowing via the kill-switch must
// reintroduce the "cannot find symbol" errors.
func TestObjectArmProvisionalNarrowIsLoadBearing(t *testing.T) {
	lookJavac(t)
	if resolveJar(jarSpecs["fastjson2"].relPath) == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}

	on := objectArmNarrowFindSymbolCount(t, false) // fix ON
	off := objectArmNarrowFindSymbolCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 ObjectWriterArray cannot-find-symbol errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the provisional-Object cannot-find-symbol errors: ON=%d (want 0)", on)
	}
}
