//go:build !javajive_full_jdk

package javaclassparser

import "testing"

func TestJDKConstructorBundleSlimDefault(t *testing.T) {
	if HasEmbeddedJDKConstructorBundle() {
		t.Fatal("default build should not embed the full JDK constructor bundle")
	}
	if raw, ok := jdkConstructorClassBytes("java/lang/Object", 8); ok || raw != nil {
		t.Fatalf("expected slim default jdkConstructorClassBytes to return (nil, false), got len=%d ok=%v", len(raw), ok)
	}
	// The compact invocation/hierarchy metadata catalog must still be available.
	if cls, ok := jdkInvocationMetadata("java/util/Map", 17); !ok || len(cls.Methods) == 0 {
		t.Fatal("compact JDK metadata catalog should remain available in default build")
	}
	if parents, ok := jdkReferenceSupertypes("java/security/SecureRandom", 17); !ok || len(parents) == 0 {
		t.Fatal("compact JDK reference hierarchy should remain available in default build")
	}
	if cls, ok := jdkThrowableAncestry("java/lang/AssertionError", 17); !ok || len(cls.Parents) == 0 {
		t.Fatal("compact JDK throwable hierarchy should remain available in default build")
	}
}
