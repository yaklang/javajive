package javaclassparser

import (
	"fmt"
	"testing"
)

func TestNativeLambdaPlatformPrimitiveTargetsRequireOriginalExactSAM(t *testing.T) {
	files := nativeCompileClasses(t, `class PrimitivePlatformWitness {}`)
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		for _, target := range []struct{ owner, name, descriptor string }{
			{"java/util/function/LongUnaryOperator", "applyAsLong", "(J)J"},
			{"java/util/function/DoubleUnaryOperator", "applyAsDouble", "(D)D"},
			{"java/util/function/LongBinaryOperator", "applyAsLong", "(JJ)J"},
			{"java/util/function/IntToDoubleFunction", "applyAsDouble", "(I)D"},
			{"java/util/function/LongToIntFunction", "applyAsInt", "(J)I"},
			{"java/util/function/DoubleToLongFunction", "applyAsLong", "(D)J"},
		} {
			t.Run(fmt.Sprintf("%d/%s", release, target.owner), func(t *testing.T) {
				object, err := Parse(files["PrimitivePlatformWitness.class"])
				if err != nil {
					t.Fatal(err)
				}
				reader := NewClassObjectDumper(object)
				reader.options.TargetSourceVersion = release
				resolve := reader.nativeAnnotationDeclarationResolver()
				if !nativeLambdaFunctionalTarget("L"+target.owner+";", target.name, target.descriptor, resolve, nil) {
					t.Fatal("complete original primitive SAM")
				}
				if nativeLambdaFunctionalTarget("L"+target.owner+";", target.name, "()Ljava/lang/Object;", resolve, nil) {
					t.Fatal("primitive SAM borrowed a reference signature")
				}
			})
		}
	}
}
