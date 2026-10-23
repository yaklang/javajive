package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialMixedReferenceRegistrationPreservesImplementationRoles(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Fatal("original native Java 8 compiler required")
	}
	for _, shape := range []string{"separate methods", "same method", "repeated ordinary target"} {
		t.Run(shape, func(t *testing.T) {
			const referenceMethods = `
 public static java.util.function.ToIntFunction<String> before(){return String::length;}
 public static java.util.function.Function<Object,Object> after(){return ReferenceOperations::requireNonNull;}
`
			fixture := strings.Replace(localVarargsReceiverFixture, "public class LocalReceiverOwner {", "public class LocalReceiverOwner {"+referenceMethods, 1)
			fixture = strings.Replace(fixture, "for(int mode=0;", `if(LocalReceiverOwner.before().applyAsInt("abc")!=3||LocalReceiverOwner.after().apply(token)!=token)throw new AssertionError("ordinary target");
 boolean caught=false;try{LocalReceiverOwner.before().applyAsInt(null);}catch(NullPointerException expected){caught=true;}if(!caught)throw new AssertionError("unbound invocation null timing");
 for(int mode=0;`, 1)
			body := "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;"
			if shape != "separate methods" {
				body = `java.util.function.ToIntFunction<String> first=String::length;LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);Consumer<Object> reference=receiver::append;
 java.util.function.Function<Object,Object> last=ReferenceOperations::requireNonNull;
 if(first.applyAsInt("abc")!=3||last.apply(receiver)!=receiver)throw new AssertionError("ordinary factory order");return reference;`
			}
			if shape == "repeated ordinary target" {
				body = strings.Replace(body, "return reference;", `java.util.function.ToIntFunction<String> again=String::length;if(again.applyAsInt("abcd")!=4)throw new AssertionError("repeated reference");return reference;`, 1)
			}
			fixture = strings.Replace(fixture, "$BODY", body, 1)
			fixture += `
class ReferenceOperations {public static <T>T requireNonNull(T value){if(value==null)throw new NullPointerException();return value;}}
`
			testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "LocalReceiverOwner", debug, true)
			}, NativeJavac8, compiler, []string{"LocalReceiverOwner"}, "LocalReceiverDriver", "9:local:bound:reference:evaluation:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS {
				return nativeEnumOwnUnitArchive(t, files, NativeJavac8)
			}, []string{"normal"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
				if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
					t.Fatalf("original adapter metadata changed %s\n%s\n%s", name, x, y)
				}
			})
		})
	}
}
