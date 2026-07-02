package cross

// 承重测试:「值擦除为字段擦除的真子类型、字段为无界型变参数化, 字段存值缺 (X<typevar>) 造型」治本
// (CODEC_TODO 泛型擦除·子类型字段存值造型)。
//
// 真实 guava EndpointPairIterator<N extends Object> 构造器:
//   this.successorIterator = ImmutableSet.of().iterator();
// 字段 successorIterator 为 Iterator<N>; 值 ImmutableSet.of().iterator() —— 零参工厂 of() 的类型见证
// 被 .iterator() 链打断、推断为 <Object>, 返回 UnmodifiableIterator<Object>(Iterator 的真子类型)。字段
// 赋值遂被 javac 判 "UnmodifiableIterator<Object> cannot be converted to Iterator<N>"。治本 (新
// subtypeValueFieldStoreCast: 值擦除经 SiblingSuperTypes/IsSubtypeVia 证为字段擦除的真子类型、且字段顶层
// 实参均为无界型变/通配时, 补 (Iterator<N>) unchecked 造型; 把子类型造到「型变实参的父接口」永远合法)。
// 关键安全边界: 字段实参须无界(N extends Object)——有界 C extends Comparable 会令 Iterator<Object> →
// Iterator<C> 可证不同=不可造型, 且会破坏原本靠赋值目标定型工作的直接工厂调用, 故排除。
// JDEC_SUBTYPE_FIELD_STORE_CAST_OFF=1 关掉治本必复现。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// subtypeFieldStoreErrCount decompiles EndpointPairIterator under the kill-switch, recompiles it against
// the jar, and counts the subtype-field-store defect: the subtype value (UnmodifiableIterator) assigned
// into the `Iterator<N>` field without the `(Iterator<N>)` cast, so javac rejects
// "UnmodifiableIterator<Object> cannot be converted to Iterator<N>". The substring isolates precisely
// this defect (the file's other errors are flattened-`$` symbol lookups that never carry it).
func subtypeFieldStoreErrCount(t *testing.T, jarPath string, killOff bool) int {
	t.Helper()
	const sw = "JDEC_SUBTYPE_FIELD_STORE_CAST_OFF"
	const entry = "com/google/common/graph/EndpointPairIterator.class"
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
	src, err := jfs.ReadFile(entry) // JarFS.ReadFile decompiles on read (honoring the kill-switch).
	if err != nil {
		t.Fatalf("read %s: %v", entry, err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "EndpointPairIterator.java")
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
		if !strings.Contains(line, ": error:") || !strings.Contains(line, "EndpointPairIterator.java") {
			continue
		}
		if strings.Contains(line, "cannot be converted to Iterator<N>") {
			n++
		}
	}
	return n
}

// TestSubtypeFieldStoreCastIsLoadBearing pins guava EndpointPairIterator ctor: the subtype value
// `ImmutableSet.of().iterator()` (a UnmodifiableIterator, proper subtype of Iterator) assigned into the
// unbounded-type-variable field `Iterator<N>` must be wrapped in a `(Iterator<N>)` cast. Disabling the
// fix via the kill-switch must reintroduce the "cannot be converted to Iterator<N>" error.
func TestSubtypeFieldStoreCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}

	on := subtypeFieldStoreErrCount(t, jarPath, false) // fix ON
	off := subtypeFieldStoreErrCount(t, jarPath, true) // fix OFF (kill-switch)
	t.Logf("EndpointPairIterator subtype-field-store errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear the subtype-field-store error: ON=%d (want 0)", on)
	}
}
