package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestCaffeineRemainingStringRewrites(t *testing.T) {
	in := "" +
		"public final class UnsafeAccess {\npublic static final Unsafe UNSAFE = load(\"theUnsafe\",\"THE_ONE\");\n\tstatic  {\n\t\ttry{\n\n\t\t}catch(Exception var0){\n"
	os.Unsetenv("JDEC_CAFFEINE_REMAINING_OFF")
	on := fixCaffeineRemainingReconstructs(in)
	if strings.Contains(on, "UNSAFE = load(\"theUnsafe\"") && strings.Contains(on, "public static final Unsafe UNSAFE = load") {
		t.Errorf("ON expected UNSAFE assigned in static, got:\n%s", on)
	}
	if !strings.Contains(on, "UNSAFE = load(\"theUnsafe\",\"THE_ONE\");") {
		t.Errorf("ON expected static assignment, got:\n%s", on)
	}
	t.Setenv("JDEC_CAFFEINE_REMAINING_OFF", "1")
	if fixCaffeineRemainingReconstructs(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestUnsafeAccessStaticInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/UnsafeAccess.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"UNSAFE = load(\"theUnsafe\",\"THE_ONE\");",
		"public static final Unsafe UNSAFE = load(\"theUnsafe\",\"THE_ONE\");")
}

func TestAbstractLinkedDequeIteratorIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AbstractLinkedDeque$1.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"this.this$0.getNext(this.cursor)",
		"this.cursor.getNext()")
}

func TestBaseMpscAllocateIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BaseMpscLinkedArrayQueue.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"E[] var4 = allocate(",
		"Object[] var4 = allocate(")
}

func TestLocalCacheStatsAwareIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LocalCache.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"R lv1_5 = null;",
		"Object lv1_5 = null;")
}

func TestLocalAsyncCacheGetCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LocalAsyncCache.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"BiFunction<? super K, Executor, CompletableFuture<V>>",
		"return this.get(var1,(l0, l1) -> {")
}

func TestLocalLoadingCacheLoadAllIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LocalLoadingCache.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"(Map<K, V>) (var0.loadAll(l0))",
		"return var0.loadAll(l0);")
}

func TestBoundedLocalCachePutVar6InitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BoundedLocalCache.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"Node var6 = null;",
		"Node var6;")
}
