package javaclassparser

import (
	"strings"
	"testing"
)

// Fixed-head native regressions are parsed only. The trusted authored oracle
// independently verifies effect ordering and error/cause identity.
func TestOriginalAssertjUncheckedPrefixAndDeclaredReflectionBoundary(t *testing.T) {
	const jar = "org/assertj/assertj-core/3.24.2/assertj-core-3.24.2.jar"
	raw := originalJarClassForReview(t, jar, "org/assertj/core/api/ThrowableAssert.class")
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, method := range object.Methods {
		name, _ := object.getUtf8(method.NameIndex)
		desc, _ := object.getUtf8(method.DescriptorIndex)
		if name == "buildThrowableAssertFromCallable" && desc == "(Ljava/util/concurrent/Callable;)Ljava/lang/Throwable;" {
			exceptions, known := originalMethodExceptions(object, method)
			if !known || len(exceptions) != 1 || exceptions[0] != "java/lang/AssertionError" {
				t.Fatal("original prefix throws changed", exceptions)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("original generic prefix declaration absent")
	}
	reviewedOriginalFamilySources(t, raw, jar, "JDEC_ASSERTJ_REMAINING_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+(?:<V>\s+)?ThrowableAssert\(Callable(?:<[^>]+>)?\s+\w+\)`)
		if strings.Contains(body, DecompileStubMarker) {
			t.Fatal("canonical unchecked prefix rejected", body)
		}
		requireReviewedPattern(t, body, `super\([^;]*buildThrowableAssertFromCallable\(`)
	})
	raw = originalJarClassForReview(t, jar, "org/assertj/core/api/junit/jupiter/SoftlyExtension.class")
	reviewedOriginalFamilySources(t, raw, jar, "JDEC_ASSERTJ_REMAINING_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+void\s+postProcessTestInstance\(`)
		if strings.Contains(body, DecompileStubMarker) {
			t.Fatal("declared Exception did not cover original reflective subclasses", body)
		}
		requireReviewedPattern(t, body, `throws\s+Exception`)
		requireReviewedPattern(t, body, `initSoftAssertionsField\(`)
	})
}
func TestOriginalByteBuddyUncheckedHandlerAbsorbsCheckedBody(t *testing.T) {
	const jar = "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar"
	raw := originalJarClassForReview(t, jar, "net/bytebuddy/asm/ClassVisitorFactory$CreateClassVisitorFactory.class")
	reviewedOriginalFamilySources(t, raw, jar, "JDEC_HARDJAR_SHAPE_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+ClassVisitorFactory<S>\s+run\(\)`)
		if strings.Contains(body, DecompileStubMarker) {
			t.Fatal("original unchecked handler forced unnecessary helper", body)
		}
		requireReviewedPattern(t, body, `catch\(Exception\s+\w+\)`)
		requireReviewedPattern(t, body, `throw new IllegalArgumentException\(`)
	})
}
