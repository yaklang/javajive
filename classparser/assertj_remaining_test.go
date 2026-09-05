package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestIntegerAssertDropsNumberCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AbstractIntegerAssert.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"this.integers.assertIsZero((AssertionInfo)(this.info),((Integer)(this.actual)))",
		"this.integers.assertIsZero((AssertionInfo)(this.info),(Number)(((Integer)(this.actual)))")
}

func TestBigDecimalAssertDropsStringCastIsLoadBearing(t *testing.T) {
	assertKillSwitchJarFS(t, assertjCoreJar,
		"org/assertj/core/api/AbstractBigDecimalAssert.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"this.isEqualTo(new BigDecimal(var1))",
		"this.isEqualTo((String)(new BigDecimal(var1)))")
}

func TestInstantAssertDropsStringParseCastIsLoadBearing(t *testing.T) {
	assertKillSwitchJarFS(t, assertjCoreJar,
		"org/assertj/core/api/AbstractInstantAssert.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"this.isEqualTo(this.parse(var1))",
		"this.isEqualTo((String)(this.parse(var1)))")
}

func TestObjectArrayAssertKeepsElementActualIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AbstractObjectArrayAssert.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"this.arrays.assertAre((AssertionInfo)(this.info),this.actual,var1)",
		"this.arrays.assertAre((AssertionInfo)(this.info),((Object[])(this.actual)),var1)")
}

func TestNaturalOrderComparatorUsesCompareToIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/NaturalOrderComparator.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"return var1.compareTo(var2)",
		"return Comparator.naturalOrder().compare(var1,var2)")
}

func TestJoinStreamCtorCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Join.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"this((Stream)(Arrays.stream(((Condition[])(checkNotNullConditions(var1))))))",
		"this(Arrays.stream(((Condition[])(checkNotNullConditions(var1)))))")
}

func TestSoftProxiesCacheFindOrInsertCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SoftProxies.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"return (Class)(CACHE.findOrInsert(",
		"return CACHE.findOrInsert(")
}

func TestPreferredAssumptionFlatMapIsLoadBearing(t *testing.T) {
	assertKillSwitchJarFS(t, assertjCoreJar,
		"org/assertj/core/configuration/PreferredAssumptionException.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"Optional opt = (Optional)(l0)",
		".flatMap((Function<Optional, Stream>)")
}

func TestMapsCloneDropsNSMEThrowsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Maps.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"private static <K, V> Map<K, V> clone(Map<K, V> var0) {",
		"private static <K, V> Map<K, V> clone(Map<K, V> var0) throws NoSuchMethodException {")
}

func TestAssumptionsGetConstructorCatchesNSMEIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Assumptions.class", "JDEC_ASSERTJ_REMAINING_OFF",
		"}catch(IllegalAccessException | InvocationTargetException | InstantiationException | NoSuchMethodException var3){",
		"}catch(IllegalAccessException | InvocationTargetException | InstantiationException var3){")
}

const assertjCoreJar = "testdata/regression/assertj-core-3.24.2.jar"

func assertKillSwitchJarFS(t *testing.T, jar, entry, env, onMust, offMust string) {
	t.Helper()
	if _, err := os.Stat(jar); err != nil {
		home := os.Getenv("HOME")
		jar = home + "/.m2/repository/org/assertj/assertj-core/3.24.2/assertj-core-3.24.2.jar"
		if _, err := os.Stat(jar); err != nil {
			t.Skipf("assertj jar not present: %v", err)
		}
	}
	dump := func() string {
		jfs, err := NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatal(err)
		}
		defer jfs.Close()
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	os.Unsetenv(env)
	on := dump()
	if !strings.Contains(on, onMust) {
		t.Errorf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv(env, "1")
	off := dump()
	if !strings.Contains(off, offMust) {
		t.Errorf("OFF missing %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}
