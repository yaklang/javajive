package javaclassparser

import (
	"strings"
	"testing"
)

// TestUnboundMethodRefObjectMethodCastIsLoadBearing pins the lambdaArgRawJDKReceiverCast
// fix for unbound instance method references that target Object-inherited methods (toString,
// hashCode, etc.) on a raw JDK generic receiver (Stream/Optional). When the method name IS on
// Object, javac tries to bind the reference to Object's version (0 args) instead of the unbound
// form (1 arg = receiver), causing "invalid method reference". The cast
// `(Function<Method, String>) Method::toString` re-targets the SAM. When the method name is NOT
// on Object (e.g. `MergedAnnotation::withNonMergedAttributes`), javac resolves correctly and
// the cast is skipped (it would break downstream type inference). Kill-switch:
// JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF. Real hit: commons-lang3 MethodUtils `map(Method::toString)`.
func TestUnboundMethodRefObjectMethodCastIsLoadBearing(t *testing.T) {
	raw := reviewedRemainingSAMRaw(t, "UnboundMethodRefSeed")
	assertReviewedTypeVarMethod(t, raw, "describeMethods", "(Ljava/util/List;)Ljava/lang/String;", "(Ljava/util/List<Ljava/lang/reflect/Method;>;)Ljava/lang/String;")
	assertReviewedRemainingSAMTarget(t, raw, "java/lang/reflect/Method", "toString", "()Ljava/lang/String;", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Ljava/lang/reflect/Method;)Ljava/lang/String;")
	reviewedSeedSources(t, "testdata/regression/UnboundMethodRefSeed.class", "JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `describeMethods\(`)
		requireReviewedPattern(t, body, `\.map\([^;]*Function<Method,\s*String>[^;]*Method::toString[^;]*\.collect\([^;]*Collectors\.joining\(", "\)`)
		if strings.Count(body, "Method::toString") != 1 {
			t.Fatal("unbound producer duplicated")
		}
	})
}
