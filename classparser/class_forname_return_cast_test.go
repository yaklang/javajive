package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestClassForNameReturnCastIsLoadBearing pins classForNameReturnCast. `ClassForNameRetSeed.load` declares
// a `Class<Wrapper<T>>` return (a `Class<...>` parameterization mentioning the type variable T) and returns
// `Class.forName(name)`. The JDK signature is `Class<?> forName(String)`, so javac captures the wildcard to
// CAP#1 and rejects `Class<CAP#1>` -> `Class<Wrapper<T>>`; the source carried an unchecked `(Class<Wrapper<T>>)`
// cast the bytecode dropped. The fix re-inserts it; the kill-switch drops it, proving it load-bearing. Real
// hit: spring objenesis DelegatingToExoticInstantiator.instantiatorClass().
func TestClassForNameReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassForNameRetSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMethod(t, data, "load", "(Ljava/lang/String;)Ljava/lang/Class;", "(Ljava/lang/String;)Ljava/lang/Class<LClassForNameRetSeed$Wrapper<TT;>;>;")
	// The active switch remains a negative control for return binding.
	// A same-erasure raw bridge keeps the wildcard producer compilable without a new runtime check.
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_CLASS_FORNAME_RET_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		hasView := strings.Contains(compactReviewedGenericSource(source), "return(Class<ClassForNameRetSeed$Wrapper<T>>)(Class)(Class.forName(")
		if hasView != (setting == "") {
			t.Fatalf("switch=%q: missing declared generic view and same-erasure bridge: %s", setting, source)
		}
	}
}
