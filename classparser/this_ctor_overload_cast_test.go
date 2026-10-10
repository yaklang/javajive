package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Original constructor declarations and invokespecial targets define the
// contract. Both argument-binding planners now preserve the target with the
// older rendering control disabled; a recursive this(...) is never expected.
func TestThisCtorOverloadCastIsLoadBearing(t *testing.T) {
	path := "testdata/regression/TypeFactory.class"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	const owner = "com/fasterxml/jackson/databind/type/TypeFactory"
	const lru = "Lcom/fasterxml/jackson/databind/util/LRUMap"
	const lookup = "Lcom/fasterxml/jackson/databind/util/LookupCache"
	const actuals = "<Ljava/lang/Object;Lcom/fasterxml/jackson/databind/JavaType;>"
	const tail = "Lcom/fasterxml/jackson/databind/type/TypeParser;[Lcom/fasterxml/jackson/databind/type/TypeModifier;Ljava/lang/ClassLoader;"
	for _, kind := range []string{lru, lookup} {
		assertReviewedTypeVarMethod(t, data, "<init>", "("+kind+";)V", "("+kind+actuals+";)V")
		assertReviewedTypeVarMethod(t, data, "<init>", "("+kind+";"+tail+")V", "("+kind+actuals+";"+tail+")V")
	}
	assertReviewedTypeVarInvoke(t, path, "<init>", "()V", 5, 183, owner, "<init>", "("+lookup+";)V")
	assertReviewedTypeVarInvoke(t, path, "<init>", "("+lru+";)V", 2, 183, owner, "<init>", "("+lookup+";)V")
	assertReviewedTypeVarInvoke(t, path, "<init>", "("+lru+";"+tail+")V", 6, 183, owner, "<init>", "("+lookup+";"+tail+")V")
	_, noArg, _ := reviewedFixtureMethod(t, path, "<init>", "()V")
	assertReviewedOpcode(t, noArg, 2, 192)
	assertReviewedConstructorDelegations(t, path, "TypeFactory", "JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", 3)
	reviewedSeedSources(t, path, "JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", false, func(source string) {
		compact := compactReviewedGenericSource(source)
		one := requireReviewedPattern(t, compact, `protectedTypeFactory\(LRUMap<Object,JavaType>(\w+)\)\{this\(\(LookupCache\)\((\w+)\)\);\}`)
		if one[1] != one[2] {
			t.Fatal("single-argument delegation no longer passes its original parameter")
		}
		four := requireReviewedPattern(t, compact, `protectedTypeFactory\(LRUMap<Object,JavaType>(\w+),TypeParser(\w+),TypeModifier\[\](\w+),ClassLoader(\w+)\)\{this\(\(LookupCache\)\((\w+)\),(\w+),(\w+),(\w+)\);\}`)
		for i := 1; i <= 4; i++ {
			if four[i] != four[i+4] {
				t.Fatal("multi-argument delegation changed parameter binding/order")
			}
		}
	})
}

func snippetThis(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, "this(") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	s := b.String()
	if len(s) > 800 {
		return s[:800]
	}
	return s
}
