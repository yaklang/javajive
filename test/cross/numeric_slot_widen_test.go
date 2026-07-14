package cross

// 承重测试: 当 JVM 槽位被复用为多个不兼容的 boxed numeric 子类型 (Integer/Long/...) 时,
// 槽位会解析到 java.lang.Number (它们的公共超类)。但 `IsFirst` 声明的渲染用的是
// 初始化值 (JavaValue) 的具体类型 (Integer), 而非槽位类型 (Number), 导致
// `Integer var11 = Integer.valueOf(0)` 与后续 `var11 = Long.valueOf(...)` 类型冲突
// (javac: "Long cannot be converted to Integer")。
//
// 治本 (numericSlotWiderThan): 当槽位类型是 Number 且初始化值是具体 numeric 子类型时,
// 用槽位类型渲染声明 (`Number var11 = Integer.valueOf(0)`) — Integer 是 Number, 合法赋值。
// Kill-switch: JDEC_NUMERIC_DECL_SLOT_TYPE_OFF=1。
//
// 真实 fastjson2 ObjectWriterCreatorASM.gwFieldName: slot 11 存 Integer.valueOf(0)
// (初始化) 和 Long.valueOf(...) (switch case 15), 读端用 .intValue()/.longValue() (Number 方法)。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// numericSlotWidenErrCount decompiles the WHOLE fastjson2 jar, then compiles
// ObjectWriterCreatorASM.java ALONE against the original jar (iso), and counts
// "Long cannot be converted to Integer" (the #OWCASM-2380 signature). With the fix ON
// that error is gone; with the fix OFF (kill-switch) the Integer-typed declaration rejects
// the Long reassignment.
func numericSlotWidenErrCount(t *testing.T, killOff bool) int {
	t.Helper()
	const sw = "JDEC_NUMERIC_DECL_SLOT_TYPE_OFF"
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
	var f string
	for _, ff := range files {
		if filepath.Base(ff) == "ObjectWriterCreatorASM.java" {
			f = ff
			break
		}
	}
	if f == "" {
		t.Fatal("ObjectWriterCreatorASM.java not produced")
	}
	javac := lookJavac(t)
	cp := withJfr(t, withSunMisc(t, jarPath))
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", cp, f)
	cmd := exec.Command(javac, args...)
	out, _ := cmd.CombinedOutput()
	return strings.Count(string(out), "Long cannot be converted to Integer")
}

// TestNumericSlotWidenIsLoadBearing pins numericSlotWiderThan as load-bearing on fastjson2's
// ObjectWriterCreatorASM.gwFieldName: with the fix ON the "Long cannot be converted to Integer"
// signature is gone from the iso compile; disabling the widen via the kill-switch must
// reintroduce it.
func TestNumericSlotWidenIsLoadBearing(t *testing.T) {
	lookJavac(t)
	on := numericSlotWidenErrCount(t, false) // fix ON
	off := numericSlotWidenErrCount(t, true) // fix OFF (kill-switch)
	t.Logf("ObjectWriterCreatorASM.java iso 'Long cannot be converted to Integer': ON=%d OFF=%d", on, off)
	if off <= on {
		t.Fatalf("numericSlotWiderThan is NOT load-bearing: ON=%d OFF=%d (OFF must be strictly greater)", on, off)
	}
}
