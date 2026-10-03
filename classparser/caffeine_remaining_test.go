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

func TestT19LocalAsyncCacheCallTargetIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LocalAsyncCache.class", "JDEC_POLY_CALL_TARGET_OFF",
		"BiFunction<? super K, Executor, CompletableFuture<V>> var5 =",
		// With original class formal parameters available, disabling the poly
		// target leaves an erased interface local, rather than invented concrete
		// type arguments. The ON target contract is unchanged.
		"BiFunction var5 =")
}

func TestLocalLoadingCacheLoadAllIsLoadBearing(t *testing.T) {

	path := "testdata/regression/LocalLoadingCache.class"
	desc := "(Lcom/github/benmanes/caffeine/cache/CacheLoader;)Ljava/util/function/Function;"
	raw, _, _ := reviewedFixtureMethod(t, path, "newBulkMappingFunction", desc)
	assertReviewedTypeVarMethod(t, raw, "newBulkMappingFunction", desc, "<K:Ljava/lang/Object;V:Ljava/lang/Object;>(Lcom/github/benmanes/caffeine/cache/CacheLoader<-TK;TV;>;)Ljava/util/function/Function<Ljava/lang/Iterable<+TK;>;Ljava/util/Map<TK;TV;>;>;")
	assertReviewedTypeVarInvoke(t, path, "lambda$newBulkMappingFunction$3", "(Lcom/github/benmanes/caffeine/cache/CacheLoader;Ljava/lang/Iterable;)Ljava/util/Map;", 2, 185, "com/github/benmanes/caffeine/cache/CacheLoader", "loadAll", "(Ljava/lang/Iterable;)Ljava/util/Map;")
	assertReviewedSeedSAM(t, raw, "(Ljava/lang/Object;)Ljava/lang/Object;", "(Ljava/lang/Iterable;)Ljava/util/Map;")
	reviewedSeedSources(t, path, "JDEC_CAFFEINE_REMAINING_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `newBulkMappingFunction\(CacheLoader<\? super K, V> [^)]*\)`)
		carrier := reviewedFunctionalCarrier(t, body, "Function<Iterable<? extends K>, Map<K, V>>", "(Function) ((Function<Iterable, Map>)")
		mapped := requireReviewedPattern(t, body, `Map\s+(\w+)\s*=\s*\w+\.loadAll\(\w+\);`)[1]
		if !strings.Contains(body, "return "+mapped+";") {
			t.Fatal("bulk callback changed map result identity")
		}

		if !strings.Contains(body, "return "+carrier+";") {
			t.Fatal("lost typed bulk callback return")
		}
	})
}

func TestBoundedLocalCachePutVar6InitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BoundedLocalCache.class", "JDEC_CAFFEINE_REMAINING_OFF",
		"Node var6 = null;",
		"Node var6;")
}
