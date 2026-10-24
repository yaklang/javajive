package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The original call ignores the result, but Java source lookup still has to
// collapse overridden declarations and account for the compiler's covariant
// bridge. Distinct overloads remain candidates. Original dependency classes
// observe fresh packing, exact receiver evaluation and exception identity.
func TestAdversarialVarargsReferenceOverrideFamilyRetainsOriginalSelection(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("native compiler required")
			}
		}
		t.Run(string(profile), func(t *testing.T) {
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					for _, kind := range []string{"covariant bridge", "same return override", "distinct overload", "interface declaration", "transitive override", "covariant bridge chain"} {
						t.Run(kind, func(t *testing.T) {
							declaration := "public Object append(Object...items){throw new AssertionError(\"parent dispatch\");}"
							if kind == "same return override" {
								declaration = "public Object[] append(Object...items){throw new AssertionError(\"parent dispatch\");}"
							}
							if kind == "distinct overload" {
								declaration = "public Object append(CharSequence...items){throw new AssertionError(\"wrong overload\");}"
							}
							fixture := strings.Replace(localVarargsReceiverFixture, "class LocalReceiverTarget {", "class LocalReceiverBase {"+declaration+"}\nclass LocalReceiverTarget extends LocalReceiverBase {", 1)
							switch kind {
							case "interface declaration":
								fixture = strings.Replace(localVarargsReceiverFixture, "class LocalReceiverTarget {", "interface LocalReceiverBase {Object append(Object...items);}\nclass LocalReceiverTarget implements LocalReceiverBase {", 1)
							case "transitive override", "covariant bridge chain":
								middle := ""
								if kind == "covariant bridge chain" {
									middle = "public Cloneable append(Object...items){throw new AssertionError(\"intermediate dispatch\");}"
								}
								fixture = strings.Replace(fixture, "class LocalReceiverTarget extends LocalReceiverBase", "class LocalReceiverMiddle extends LocalReceiverBase {"+middle+"}\nclass LocalReceiverTarget extends LocalReceiverMiddle", 1)
							}
							fixture = strings.Replace(fixture, "$BODY", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;", 1)
							testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
								return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "LocalReceiverOwner", debug, named)
							}, profile, compiler, []string{"LocalReceiverOwner"}, "LocalReceiverDriver", "9:local:bound:reference:evaluation:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures)
						})
					}
				})
			}
		})
	}
}
