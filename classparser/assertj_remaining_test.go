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
	assertReviewedAssertjObjectDelegation(t, "AbstractBigDecimalAssert", 9, `new BigDecimal\(\w+\)`)
}

func TestInstantAssertDropsStringParseCastIsLoadBearing(t *testing.T) {
	assertReviewedAssertjObjectDelegation(t, "AbstractInstantAssert", 10, `this\.parse\(\w+\)`)
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
	// This test owns the legacy class-source rewrite. The typed constructor-binding
	// pass now handles the same bytecode path under its own kill switch.
	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
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
	jar := "org/assertj/assertj-core/3.24.2/assertj-core-3.24.2.jar"
	entry := "org/assertj/core/configuration/PreferredAssumptionException.class"
	raw := originalJarClassForReview(t, jar, "org/assertj/core/configuration/PreferredAssumptionException$1.class")
	assertReviewedRemainingSAMTarget(t, raw, "org/assertj/core/configuration/PreferredAssumptionException$1", "lambda$autoDetectAssumptionExceptionClass$1", "(Ljava/util/Optional;)Ljava/util/stream/Stream;", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Ljava/util/Optional;)Ljava/util/stream/Stream;")
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	found := false
	for _, method := range object.Methods {
		if cp.GetUtf8(int(method.NameIndex)).Value != "autoDetectAssumptionExceptionClass" {
			continue
		}
		for _, attr := range method.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				assertReviewedOpcode(t, code, 40, 185)
				found = true
			}
		}
	}
	if !found {
		t.Fatal("original flatMap path missing")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	var on string
	for _, flag := range []string{"", "1"} {
		t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", flag)
		fs, err := NewJarFSFromLocal(home + "/.m2/repository/" + jar)
		if err != nil {
			t.Fatal(err)
		}
		data, err := fs.ReadFile(entry)
		fs.Close()
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		carrier := requireReviewedPattern(t, source, `Function\s+(\w+)\s*=\s*\(Function\)\s*\(\(Function<Optional,\s*Stream>\)`)[1]
		requireReviewedPattern(t, source, `\.map\(\(Function\)[^;]*Function<Class,\s*Stream>[^;]*Stream::of[^;]*\.orElse\(\(Object\)\(Stream\.empty\(\)\)\)`)
		if !strings.Contains(source, ".flatMap("+carrier+").findFirst()") || !strings.Contains(source, "AUTO_DETECT((String)(null)) {") {
			t.Fatal("optional stream SAM carrier or original enum override detached")
		}
		if flag == "" {
			on = source
		} else if source != on {
			t.Fatal("superseded source patch still changes original flatMap")
		}
	}
}

func TestMapsClonePreservesDeclaredException(t *testing.T) {
	const declaration = "private static <K, V> Map<K, V> clone(Map<K, V> var0) throws NoSuchMethodException {"
	assertKillSwitchDecompile(t, "testdata/regression/Maps.class", "JDEC_ASSERTJ_REMAINING_OFF", declaration, declaration)
}

func TestAssumptionsGetConstructorCatchesNSMEIsLoadBearing(t *testing.T) {
	// The four types share one original handler PC. Its membership must be
	// derived from the exception table even when the source workaround is off.
	raw, err := os.ReadFile("testdata/regression/Assumptions.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedHandlerMultiplicity(t, raw, "JDEC_ASSERTJ_REMAINING_OFF")
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
