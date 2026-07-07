package cross

// 承重测试:「AtomicReference<V> 的 V 形参方法(compareAndSet/getAndSet/set...)缺 (V) 实参造型」治本
// (CODEC_TODO 泛型擦除·JDK 方法形参造型族)。
//
// 真实 commons-lang3 AtomicInitializer<T>.get():
//   Object var1 = this.reference.get();      // reference 是 AtomicReference<T>, get() 返 Object
//   if (var1 == null) {
//       var1 = this.initialize();
//       if (!(this.reference.compareAndSet(null, var1))) {   // 源码带 (T) 造型
//           var1 = this.reference.get();
//       }
//   }
//   return (T) var1;
// compareAndSet 的形参在字节码里被擦成 Object, 反编译不补造型; javac 却按
// AtomicReference<V>.compareAndSet(V, V) 定型判 "Object cannot be converted to T"。治本
// (jdkMethodParamTypeArgIndex 增 AtomicReference 分支, 使 instantiatedParamType 把
// compareAndSet 形参 0/1 解析为接收者 V, 既有 arg-cast 路径遂重下 (T) 造型)。
// JDEC_ATOMIC_REF_PARAM_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// atomicRefVParamErrCount decompiles AtomicInitializer under the kill-switch, recompiles it against
// the jar, and counts the AtomicReference V-parameter cast defect: the Object-typed argument to
// `reference.compareAndSet(...)` passed without the `(T)` cast, so javac rejects "Object cannot be
// converted to T".
func atomicRefVParamErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_ATOMIC_REF_PARAM_OFF"
	const entry = "org/apache/commons/lang3/concurrent/AtomicInitializer.class"
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
	src, err := jfs.ReadFile(entry)
	if err != nil {
		t.Fatalf("read %s: %v", entry, err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "AtomicInitializer.java")
	if err := os.WriteFile(dst, src, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}

	javac := lookJavac(t)
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-Xmaxerrs", "100000",
		"-cp", jarPath, "-d", t.TempDir(), dst)
	out, _ := exec.Command(javac, args...).CombinedOutput()

	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "AtomicInitializer.java") {
			continue
		}
		if strings.Contains(line, "Object cannot be converted to T") {
			n++
		}
	}
	return n
}

// TestAtomicRefVParamCastIsLoadBearing pins commons-lang3 AtomicInitializer.get: an Object-typed
// argument to `reference.compareAndSet(...)` (receiver recovered as AtomicReference<T>) must be wrapped
// in a `(T)` cast, compareAndSet's value parameter resolved to the receiver's V via the AtomicReference
// parameter table. Disabling the fix via the kill-switch must reintroduce the "Object cannot be converted
// to T" error.
func TestAtomicRefVParamCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["commons-lang3"].relPath)
	if jarPath == "" {
		t.Skip("commons-lang3 jar not found under ~/.m2; skipping")
	}

	on := atomicRefVParamErrCount(t, jarPath, false) // fix ON
	off := atomicRefVParamErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("AtomicInitializer AtomicReference V-param errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the AtomicReference V-param error: ON=%d (want 0)", on)
	}
}
