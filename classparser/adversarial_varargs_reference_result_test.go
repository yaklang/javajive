package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialVarargsReferenceResultsPreserveOriginalReturnConversions(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, tc := range []struct{ name, sam, call string }{
		{"boxing result", "java.util.function.Function<Integer,Integer>", "ResultOwner.make(target).apply(value)"},
		{"primitive widening result", "java.util.function.IntToLongFunction", "ResultOwner.make(target).applyAsLong(value)"},
		{"boxing with result widening", "java.util.function.IntFunction<Number>", "ResultOwner.make(target).apply(value).longValue()"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `class ResultTarget{int calls;boolean fail;static Object pack;static final RuntimeException failure=new RuntimeException("original target");public int ints(int...values){if(pack==values)throw new AssertionError("old array");pack=values;calls++;if(fail)throw failure;return values[0];}}public class ResultOwner{static $S make(ResultTarget target){return target::ints;}}class ResultDriver{public static void main(String[]a){ResultTarget target=new ResultTarget();for(int value:new int[]{Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE}){long got=$C;if(got!=(long)value)throw new AssertionError("conversion result");}boolean npe=false;try{ResultOwner.make(null);}catch(NullPointerException e){npe=true;}if(!npe||target.calls!=5)throw new AssertionError("eager factory");target.fail=true;int value=1;Throwable caught=null;try{long ignored=$C;}catch(Throwable t){caught=t;}if(caught!=ResultTarget.failure||target.calls!=6)throw new AssertionError("target payload before return conversion");System.out.println("7:result:conversion:bound");}}`
			source = strings.NewReplacer("$S", tc.sam, "$C", tc.call).Replace(source)
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					compiler := modern
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Skip("JAVA8_JAVAC required")
						}
					}
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, source, "ResultOwner", debug, true)
					}, profile, compiler, []string{"ResultOwner"}, "ResultDriver", "7:result:conversion:bound\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("original result adapter metadata changed%s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}
