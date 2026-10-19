package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIntegerAssertDropsNumberCastIsLoadBearing(t *testing.T) {
	// Keep the historical inventory entry, but retire its unsafe contract:
	// source text cannot license erasing the physical descriptor's Number
	// view. This single-class input lacks the receiver's declaration table.
	// The original-first numeric fixture independently checks actual overload
	// binding, receiver checks and full ABI with complete declarations.
	raw, err := os.ReadFile("testdata/regression/AbstractIntegerAssert.class")
	if err != nil {
		t.Fatal(err)
	}
	var before string
	for _, flag := range []string{"", "1"} {
		// The budget is shared by every method in this large historical class.
		// This source-binding regression needs the full class, rather than the
		// smaller default graph budget used by resource-limit tests.
		result, err := DecompileWithOptions(bytes.Clone(raw), DecompileOptions{
			Mode:        Compatibility,
			Limits:      Limits{MaxGraphScans: 10_000_000},
			EnvSnapshot: map[string]string{"JDEC_ASSERTJ_REMAINING_OFF": flag},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Source, "this.integers.assertIsZero((AssertionInfo)(this.info),(Number)(((Integer)(this.actual)))") {
			t.Fatalf("unresolved class binding lost its descriptor view:\n%s", result.Source)
		}
		if before != "" && result.Source != before {
			t.Fatal("class keyword recovery still changed a numeric binding")
		}
		before = result.Source
	}
}

func TestBigDecimalAssertDropsStringCastIsLoadBearing(t *testing.T) {
	assertReviewedAssertjObjectDelegation(t, "AbstractBigDecimalAssert", 9, `new BigDecimal\(\w+\)`)
}

func TestInstantAssertDropsStringParseCastIsLoadBearing(t *testing.T) {
	assertReviewedAssertjObjectDelegation(t, "AbstractInstantAssert", 10, `this\.parse\(\w+\)`)
}

func TestObjectArrayAssertKeepsElementActualIsLoadBearing(t *testing.T) {
	// Retain the inventory entry, but derive the contract from the saved class:
	// all eleven helpers consume Object[] after an actual field CHECKCAST.
	// A missing declaration family cannot authorize choosing a narrower rival.
	// The authored original-first fixture checks the complete family, binary
	// callers, exceptions, array effects and literal data in both modes/profiles.
	raw, err := os.ReadFile("testdata/regression/AbstractObjectArrayAssert.class")
	if err != nil {
		t.Fatal(err)
	}
	original := arrayProducerCheckedInvocations(t, raw)
	for _, op := range arrayFieldViewOperations() {
		tail := "Lorg/assertj/core/api/Condition;)V"
		if op.count == "count" {
			tail = "I" + tail
		}
		if strings.Contains(op.parameters, "Comparator") {
			tail = "Ljava/util/Comparator;)V"
		}
		invocation := "b6:org/assertj/core/internal/ObjectArrays." + op.name + "(Lorg/assertj/core/api/AssertionInfo;[Ljava/lang/Object;" + tail
		found := false
		for _, row := range strings.Split(original, "\n") {
			if at := strings.Index(row, invocation); at >= 0 {
				if !strings.Contains(row[:at], "checkcast:[Ljava/lang/Object;") {
					t.Fatal("original helper lost its physical array check", row)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("saved original helper/descriptor missing", invocation)
		}
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		var before []string
		for _, off := range []string{"", "1"} {
			// This full historical class contains over a hundred method bodies.
			// Give their shared proof budget room; resource-limit tests retain
			// the production defaults. Do not trim unrelated original methods.
			result, err := DecompileWithOptions(bytes.Clone(raw), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Limits: Limits{MaxGraphScans: 100_000_000}, EnvSnapshot: map[string]string{"JDEC_ASSERTJ_REMAINING_OFF": off}})
			if err != nil {
				t.Fatal(err)
			}
			var calls []string
			for _, op := range arrayFieldViewOperations() {
				var found string
				for _, line := range strings.Split(result.Source, "\n") {
					if strings.Contains(line, "this.arrays."+op.name+"(") {
						if found != "" {
							t.Fatal("ambiguous historical helper source", op.name)
						}
						found = strings.TrimSpace(line)
					}
				}
				if found == "" || !strings.Contains(found, "(Object[])") || !strings.Contains(found, "this.actual") {
					t.Fatalf("%s/%q lost original checked array view: %s", mode, off, found)
				}
				calls = append(calls, found)
			}
			if before != nil && strings.Join(before, "\n") != strings.Join(calls, "\n") {
				t.Fatal("source words still changed original array helper bindings")
			}
			before = calls
		}
	}
}

func TestNaturalOrderComparatorUsesCompareToIsLoadBearing(t *testing.T) {
	// Retain the historical inventory entry, but use the actual original
	// invocation graph. The generic receiver proof now restores this factory
	// through its original erasure, rather than replacing it by source spelling.
	// The independent authored factory-spelling fixture checks effects and
	// unchanged binary callers; the generic-factory fixture additionally checks
	// both compiler profiles, complete declarations and all lowering instructions.
	raw, err := os.ReadFile("testdata/regression/NaturalOrderComparator.class")
	if err != nil {
		t.Fatal(err)
	}
	original := arrayProducerCheckedInvocations(t, raw)
	if !strings.Contains(original, "b8:java/util/Comparator.naturalOrder()Ljava/util/Comparator;") ||
		!strings.Contains(original, "b9:java/util/Comparator.compare(Ljava/lang/Object;Ljava/lang/Object;)I") {
		t.Fatal("saved original no longer supplies the factory and interface dispatch", original)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		before := ""
		for _, off := range []string{"", "1"} {
			result, err := DecompileWithOptions(bytes.Clone(raw), DecompileOptions{Mode: mode, TargetSourceVersion: 8, EnvSnapshot: map[string]string{"JDEC_ASSERTJ_REMAINING_OFF": off}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(result.Source, ".naturalOrder()") || !strings.Contains(result.Source, ".compare(") || strings.Contains(result.Source, ".compareTo(") {
				t.Fatalf("%s/%q lost the original factory or selected operation:\n%s", mode, off, result.Source)
			}
			if before != "" && result.Source != before {
				t.Fatal("class text still changed the original factory binding")
			}
			before = result.Source
		}
	}
}

func TestJoinStreamCtorCastIsLoadBearing(t *testing.T) {
	// Verify the original selected constructor after real recompilation. The
	// former counterfactual disabled typed binding and assumed every original
	// parameter was named var1; neither assumption is a constructor contract.
	testReviewedJoinConstructorContract(t)
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
