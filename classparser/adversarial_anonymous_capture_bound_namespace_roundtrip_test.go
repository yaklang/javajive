package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The target's bound is a physical class. A caller may declare a distinct
// formal with the same spelling; its Number erasure cannot change that bound.
func TestAdversarialAnonymousCaptureKeepsNominalBoundNamespace(t *testing.T) {
	source := strings.ReplaceAll(anonymousCapturedParentFixture, "CharSequence", "NominalCaptureValue")
	source = strings.Replace(source, "class CapturedParentBox<T>", "class CapturedParentBox<T extends NominalCaptureRoot>", 1)
	source = strings.Replace(source, "class CapturedParentOwner{", "class CapturedParentOwner<NominalCaptureRoot extends Number>{", 1)
	source = strings.Replace(source, `new NominalCaptureValue[]{null,new String("same"),new StringBuilder("different")}`, `new NominalCaptureValue[]{null,new NominalCaptureValue(),new NominalCaptureValue()}`, 1)
	source = strings.ReplaceAll(source, "new CapturedParentOwner().make(token)", "new CapturedParentOwner<Integer>().make(token)")
	source = "class NominalCaptureRoot{} class NominalCaptureValue extends NominalCaptureRoot{}\n" + source
	javac, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := javac
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("actual javac8 required")
				}
			}
			compile := func(debug string) map[string][]byte {
				if profile == NativeJavac8 {
					return nativePrivateEnumCompile(t, source, "CapturedParentOwner", debug)
				}
				return nativeCompileDebugClasses(t, source, debug)
			}
			original := t04RunJava(t, java, adversarialAssertionClasses(t, compile("none")), "CapturedParentDriver")
			if original != "6:anonymous:parent:binding:early:identity:failure\n" {
				t.Fatal("original nominal-bound oracle", original)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"CapturedParentOwner"}, "CapturedParentDriver", original, nil, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
		})
	}
}
