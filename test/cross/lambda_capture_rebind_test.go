package cross

// 承重测试:「内联 lambda / 方法引用的『捕获变量』只存活在渲染闭包里, 不在语句树上, 故 RewriteVar 遍历树、
// 对每个引用施加变量 id 改写(尤其是 disjoint 槽拆分: 同一 JVM 槽先后被两种类型复用时给后一段铸新 id、
// 令 varN → varN_1)时够不到它们——捕获引用遂停留在旧的槽基名上, 渲染出错误变量」治本
// (CODEC_TODO 泛型/槽位·lambda 捕获变量 ReplaceVar 转发)。
//
// 真实 fastjson2:`BeanUtils.getField` 里槽 9 先作 `char`(方法名第 2 字符)、后作 `Class`(字段类型)
// 复用, 拆分为 `var9`(char) / `var9_1`(Class)。其内联 lambda 捕获的是那个 `Class`(应渲染 `var9_1`),
// 但捕获引用没跟上拆分, 渲染成 `var9`(char) → lambda 体 `var9.isAssignableFrom(...)` 即
// `char cannot be dereferenced`(外加 `(l0.getType()) != (var9)` 的 `!=` 操作数类型错)。
// 治法: 给 LambdaMetafactory 造出的 lambda / 方法引用 CustomValue 装上 ReplaceFunc, 把 ReplaceVar
// 转发到每个 captured 值, 令捕获引用与树上其它引用一样参与 id 改写。ReplaceVar 仅当 ref.Id==oldId 才生效,
// 故只有确属该逻辑变量的改写会命中——对捕获从不被改写的 lambda 结构性无副作用。
// JDEC_LAMBDA_CAPTURE_REBIND_OFF=1 关掉转发必复现 `char cannot be dereferenced`。
//
// 该缺陷是逐文件(iso)可复现的真错(与扁平 $ 假阳性无关): 单编 BeanUtils.java 对原始 jar 即可暴露,
// 故本测试走整 jar 反编译 + 单文件(BeanUtils.java)iso 重编译口径, 用唯一错误签名
// "char cannot be dereferenced" 精确隔离本缺陷。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lambdaCaptureCharDerefCount decompiles the WHOLE fastjson2 jar under the kill-switch, then compiles
// BeanUtils.java ALONE against the original jar (iso), and counts the "char cannot be dereferenced"
// error. That substring is unique to the getField lambda's mis-named captured Class variable, so it
// isolates precisely this defect (unaffected by flat-$ false positives or newly-unmasked latent errors).
func lambdaCaptureCharDerefCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_LAMBDA_CAPTURE_REBIND_OFF"
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

	// Find the decompiled BeanUtils.java among the flattened units.
	var beanUtils string
	for _, f := range files {
		if strings.HasSuffix(f, filepath.FromSlash("com/alibaba/fastjson2/util/BeanUtils.java")) {
			beanUtils = f
			break
		}
	}
	if beanUtils == "" {
		t.Fatal("decompiled BeanUtils.java not found")
	}

	// iso classpath: ORIGINAL jar + deps (so BeanUtils' own references resolve; the defect is a genuine
	// per-file type error, independent of sibling flattened units).
	cpParts := append([]string{jarPath}, deps...)
	cp := withSunMisc(t, strings.Join(cpParts, string(os.PathListSeparator)))

	outDir := t.TempDir()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", cp, "-d", outDir, beanUtils)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ": error:") && strings.Contains(line, "char cannot be dereferenced") {
			n++
		}
	}
	return n
}

// TestLambdaCaptureRebindIsLoadBearing pins fastjson2's BeanUtils.getField lambda: its captured Class
// variable must render as the post-split name (var9_1), not the stale char slot name (var9). Disabling
// the captured-value ReplaceVar forwarding via the kill-switch must reintroduce the
// "char cannot be dereferenced" error.
func TestLambdaCaptureRebindIsLoadBearing(t *testing.T) {
	lookJavac(t)
	if resolveJar(jarSpecs["fastjson2"].relPath) == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}

	on := lambdaCaptureCharDerefCount(t, false) // fix ON
	off := lambdaCaptureCharDerefCount(t, true) // fix OFF (kill-switch)
	t.Logf("fastjson2 BeanUtils char-deref errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the lambda captured-var char-deref error: ON=%d (want 0)", on)
	}
}
