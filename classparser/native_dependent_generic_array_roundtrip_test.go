package javaclassparser

import (
	"strings"
	"testing"
)

// T erases to Number through U, while the original array can still be Integer[].
// An exact erasure view must preserve AASTORE's check, producer order and partial
// state; it must not introduce an earlier Integer[] or element payload cast.
func TestNativeDependentLexicalArraysKeepOriginalStoreChecks(t *testing.T) {
	fixture := strings.Replace(nativeLexicalGenericArrayStoreFixture, "RowOwner<T extends Number>", "RowOwner<U extends Number,T extends U>", 1)
	fixture = strings.ReplaceAll(fixture, "RowOwner<Integer>", "RowOwner<Number,Integer>")
	for _, owner := range []string{"RowOwner", "DifferentDependentScope"} {
		t.Run(owner, func(t *testing.T) {
			f := strings.ReplaceAll(fixture, "RowOwner", owner)
			testNativeIndependentFamilyFixture(t, f, []string{owner}, "RowDriver", "256:lexical:generic:arrays:original-order\n", nativeLexicalExactSignatures)
		})
	}
}
