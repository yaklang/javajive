package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Authored original compilers establish the conversion boundary before rebuilding.
// Each independent owner has one original factory, so shared-body compiler
// normalization cannot conceal or substitute a typed entry-adaptation defect.
func TestAdversarialVarargsReferenceEntryConversionsPreserveInvocationBoundary(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, tc := range []struct{ name, sam, target, argument, assertion string }{
		{"unboxing", "java.util.function.Consumer<Integer>", "int", "Integer.valueOf(-7)", "((int[])EntryTarget.pack)[0]==-7"},
		{"unboxing widening", "java.util.function.Consumer<Byte>", "int", "Byte.valueOf((byte)-7)", "((int[])EntryTarget.pack)[0]==-7"},
		{"primitive widening", "java.util.function.IntConsumer", "double", "-7", "Double.doubleToRawLongBits(((double[])EntryTarget.pack)[0])==Double.doubleToRawLongBits(-7.0)"},
		{"boxing", "java.util.function.IntConsumer", "Object", "-7", "((Object[])EntryTarget.pack)[0].equals(Integer.valueOf(-7))"},
		{"reference widening", "java.util.function.Consumer<String>", "Object", "\"payload\"", "((Object[])EntryTarget.pack)[0].equals(\"payload\")"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `class EntryTarget{static Object pack;static int calls;public void accept($T...values){pack=values;calls++;}}public class EntryOwner{static $S make(EntryTarget x){return x::accept;}}class EntryDriver{public static void main(String[]a){EntryTarget x=new EntryTarget();EntryOwner.make(x).accept($A);if(!($P)||EntryTarget.calls!=1)throw new AssertionError("typed original entry");boolean npe=false;try{EntryOwner.make(null);}catch(NullPointerException e){npe=true;}if(!npe||EntryTarget.calls!=1)throw new AssertionError("eager factory timing");System.out.println("2:entry:conversion:factory");}}`
			source = strings.NewReplacer("$T", tc.target, "$S", tc.sam, "$A", tc.argument, "$P", tc.assertion).Replace(source)
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
						return compileLambdaParameterMetadataFixture(t, compiler, profile, source, "EntryOwner", debug, true)
					}, profile, compiler, []string{"EntryOwner"}, "EntryDriver", "2:entry:conversion:factory\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("physical original entry changed %s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}
