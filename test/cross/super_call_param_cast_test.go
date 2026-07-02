package cross

// 承重测试:「super.m(...) 调用缺型变实参造型」治本 (CODEC_TODO 泛型擦除·super 形参解析)。
//
// 真实 guava MutableClassToInstanceMap$1<B> extends ForwardingMapEntry<Class<? extends B>, B>:
//   public B setValue(B var1) {
//       return (B)(super.setValue(MutableClassToInstanceMap.access$000((Class)this.getKey(), var1)));
//   }
// 参数 `access$000(...)` 是合成访问器、返回 Object; `super.setValue` 绑定超类 ForwardingMapEntry 的
// setValue(V), V 经 extends 子句实参代换 = B。字节码把形参擦成 Object, 反编译不补造型; javac 却按
// setValue(B) 定型判 "Object cannot be converted to B"。治本 (resolvedParamType 增 super 分支: 由当前类
// Signature 的 extends 子句恢复超类参数化类型 ForwardingMapEntry<Class<? extends B>, B>, 经
// ResolveInstantiatedParamType 解析 setValue 形参为 B, 既有 arg-cast 路径遂重下 (B) 造型)。
// JDEC_SUPER_PARAM_RESOLVE_OFF=1 关掉治本必复现。
//
// 注: MutableClassToInstanceMap$1 单文件编译会先撞 `access$000` 合成访问器 `cannot find symbol`(需外层
// 类在场), 故本测试同时反编译外层 MutableClassToInstanceMap + 内层 $1 一起编译, 让 access$000 解析得到,
// 真实的 "Object cannot be converted to B" 缺陷方能浮现(与整树 tree 口径一致)。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// superCallParamErrCount decompiles guava MutableClassToInstanceMap (outer) + $1 (inner) under the
// kill-switch, recompiles them together against the jar, and counts the super-call param cast defect:
// the Object-typed synthetic-accessor argument to `super.setValue(...)` passed without the `(B)` cast,
// so javac rejects "Object cannot be converted to B".
func superCallParamErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_SUPER_PARAM_RESOLVE_OFF"
	entries := []string{
		"com/google/common/collect/MutableClassToInstanceMap.class",
		"com/google/common/collect/MutableClassToInstanceMap$1.class",
	}
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

	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	dir := t.TempDir()
	var files []string
	for _, entry := range entries {
		src, err := jfs.ReadFile(entry) // JarFS.ReadFile decompiles on read (honoring the kill-switch).
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		base := strings.TrimSuffix(filepath.Base(entry), ".class")
		dst := filepath.Join(dir, base+".java")
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		files = append(files, dst)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir())
	args = append(args, files...)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "MutableClassToInstanceMap$1.java") {
			continue
		}
		if strings.Contains(line, "Object cannot be converted to B") {
			n++
		}
	}
	return n
}

// TestSuperCallParamCastIsLoadBearing pins guava MutableClassToInstanceMap$1.setValue: the Object-typed
// argument to `super.setValue(...)` (super declared ForwardingMapEntry<Class<? extends B>, B>, so
// setValue's V resolves to B) must be wrapped in a `(B)` cast. Disabling the fix via the kill-switch
// must reintroduce the "Object cannot be converted to B" error.
func TestSuperCallParamCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := superCallParamErrCount(t, jarPath, false) // fix ON
	off := superCallParamErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("MutableClassToInstanceMap$1 super-call param errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the super-call param error: ON=%d (want 0)", on)
	}
}
