package cross

// 承重测试:「一个具体类型声明的局部被 java.lang.Object 重赋, 合流读是 Object 级(return/cast/instanceof/
// RHS 进 Object 局部)」治本(widenConcreteDeclToObject, narrowNullInitObjectDecl 的逆)。
//
// 真实 fastjson2: `JSON.parse(String)` slot 6 持 JSONObject(首存 new JSONObject())/JSONArray(ObjectReader 读)/
// Object(`var6 = var5`, var5 是 Object), 合流 `return var6`(方法返回 Object)。具体声明 `JSONObject var6` 拒绝
// `var6 = var5`(Object)→ javac「Object cannot be converted to JSONObject」。治本: 具体 ref 声明 + Object 重赋值
// + 所有使用 Object-safe 时 widen 声明到 Object。
//
// 该缺陷 iso 可复现: 单编 JSON.java 对原始 jar(原始 jar 提供 JSONObject/JSONReader/... 兄弟类)即暴露
// `Object cannot be converted to JSONObject` at parse()。fix ON 该错误消失; fix OFF(关 widen)必复现。
// Kill-switch: JDEC_WIDEN_CONCRETE_TO_OBJECT_OFF=1。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// widenConcreteJSONObjectErrCount decompiles the WHOLE fastjson2 jar under the kill-switch, then
// compiles JSON.java ALONE against the original jar (iso), and counts "Object cannot be converted to
// JSONObject" (the #13 signature). With the fix ON that error is gone; with the fix OFF (widen
// disabled) the concrete `JSONObject var6` declaration rejects the Object reassignment.
func widenConcreteJSONObjectErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_WIDEN_CONCRETE_TO_OBJECT_OFF"
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
	root := t.TempDir()
	files, _, _ := decompileAll(t, jarPath, root, 0)
	var jsonFile string
	for _, f := range files {
		if filepath.Base(f) == "JSON.java" {
			jsonFile = f
			break
		}
	}
	if jsonFile == "" {
		t.Fatal("JSON.java not produced")
	}
	javac := lookJavac(t)
	cp := withJfr(t, withSunMisc(t, jarPath))
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", cp, jsonFile)
	cmd := exec.Command(javac, args...)
	out, _ := cmd.CombinedOutput()
	return strings.Count(string(out), "Object cannot be converted to JSONObject")
}

// TestWidenConcreteToObjectIsLoadBearing pins widenConcreteDeclToObject as load-bearing on fastjson2's
// JSON.parse: with the fix ON the `Object cannot be converted to JSONObject` signature is gone from
// JSON.java's iso compile; disabling the widen via the kill-switch must reintroduce it.
func TestWidenConcreteToObjectIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := widenConcreteJSONObjectErrCount(t, false) // fix ON
	off := widenConcreteJSONObjectErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("JSON.java iso 'Object cannot be converted to JSONObject': ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("widenConcreteDeclToObject is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
