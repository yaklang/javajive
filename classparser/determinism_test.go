package javaclassparser

// 承重测试: 反编译必须是确定性的——同一 .class 字节、同一进程内多次反编译，产出源码必须逐字节相等。
//
// 背景: 核心 CFG 结构化 (parser.ParseSourceCode 栈模拟 / statement_wrap.go walkIfStatement 环检测)
// 历史上依赖 Go map 的遍历顺序，导致同输入多次反编译产出不同结果 (实测 fastjson2 JSONReader 6 跑
// 出 3 种), 表现为循环体被随机塌缩成 break (静默 miscompile)。这同时让「逐文件 javac 错误数 delta」
// 验证门禁失效 (±噪声)。本测试钉死确定性: 任何重新引入非确定性的改动都会让它变红。
//
// 种子 nondeterministic_loop_structuring.class = fastjson2-2.0.43 的 com/alibaba/fastjson2/JSONReader,
// 其 readObject 的 switch+do-while(true) 是已知的非确定性触发点。

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// decompileHashes decompiles data n times and returns the set of distinct output hashes. A
// deterministic decompiler yields exactly one distinct hash.
func decompileHashes(t *testing.T, data []byte, n int) map[string]int {
	t.Helper()
	out := map[string]int{}
	for i := 0; i < n; i++ {
		src, err := Decompile(data)
		if err != nil {
			t.Fatalf("decompile run %d failed: %v", i, err)
		}
		sum := sha256.Sum256([]byte(src))
		out[hex.EncodeToString(sum[:])]++
	}
	return out
}

// TestDecompileIsDeterministic is the load-bearing determinism guard. It decompiles the pinned
// loop-structuring seed many times and asserts every run produces byte-identical source.
func TestDecompileIsDeterministic(t *testing.T) {
	seed := filepath.Join(regressionDir, "nondeterministic_loop_structuring.class")
	data, err := os.ReadFile(seed)
	if err != nil {
		t.Skipf("seed %s missing: %v", seed, err)
	}
	const runs = 12
	hashes := decompileHashes(t, data, runs)
	if len(hashes) != 1 {
		t.Errorf("decompile is NON-DETERMINISTIC: %d distinct outputs over %d runs (want 1): %v",
			len(hashes), runs, hashes)
	}
}

// TestRegressionSeedsAreDeterministic extends the determinism guard to every committed regression
// seed, so any seed pinned by a future fix also locks determinism in for free.
func TestRegressionSeedsAreDeterministic(t *testing.T) {
	classes, err := filepath.Glob(filepath.Join(regressionDir, "*.class"))
	if err != nil {
		t.Fatalf("glob seeds: %v", err)
	}
	if len(classes) == 0 {
		t.Skipf("no regression seeds under %s yet", regressionDir)
	}
	for _, classPath := range classes {
		name := filepath.Base(classPath)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(classPath)
			if err != nil {
				t.Fatalf("read seed: %v", err)
			}
			const runs = 8
			hashes := decompileHashes(t, data, runs)
			if len(hashes) != 1 {
				t.Errorf("%s is NON-DETERMINISTIC: %d distinct outputs over %d runs (want 1)",
					name, len(hashes), runs)
			}
		})
	}
}
