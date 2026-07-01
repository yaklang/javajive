//go:build !race

// race 检测器会把 CPU/分配密集的反编译拖慢约一个数量级, 令固定 40s 时限在共享 CI runner 上可能误伤;
// 故本性能守卫在 -race 构建下不编译。非 race 的 5 个 CI 矩阵组合(ubuntu 非 race / macOS / windows × Go 1.22/1.23)
// 仍会运行它, 足以在治理版 ~3s vs 病态版 ~73s 之间稳定区分, 抓住 O(names x depth) 回归。

package javaclassparser

// 承重测试(性能守卫): 巨型方法体的反编译不得退化回 O(names x depth) 的渲染爆炸。
// 种子 = fastjson2 reader.ObjectReaderBaseModule, 其单个方法体极大, 曾触发
// coverUndeclaredGeneratedLocals 对每个 generated-local 名字、在每层递归都重新
// 渲染整棵子树并用 regexp 做 whole-word 匹配, 导致该类反编译耗时 ~73s(GC 风暴)。
// 治理: (1) stmtRenderMemo 在单趟 pass 内缓存渲染, 树变更即失效;
//       (2) countWholeWord/containsWholeWord 用 strings.Index 手写 ASCII 词边界,
//           取代 regexp 回溯。治理后该类 ~3s, 且逐类字节级输出不变。
// 本测试用宽松时限守卫: 治理版 ~3s, 病态版 ~73s, 40s 阈值可稳定区分, 不误伤慢 CI。
// 无 kill-switch: 这是纯性能优化(字节级等价), 正确性由既有全量重编译基准兜底。

import (
	"os"
	"testing"
	"time"
)

func TestCoverUndeclaredPerfGuard(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Fastjson2ObjectReaderBaseModule.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	const budget = 40 * time.Second
	start := time.Now()
	out, err := Decompile(data)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("decompile produced empty output")
	}
	if elapsed > budget {
		t.Errorf("decompile took %s (> %s budget): coverUndeclaredGeneratedLocals O(names x depth) render blow-up likely regressed", elapsed, budget)
	}
	t.Logf("ObjectReaderBaseModule decompiled in %s (%d bytes)", elapsed, len(out))
}
