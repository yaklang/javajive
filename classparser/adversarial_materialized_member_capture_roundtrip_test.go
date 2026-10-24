package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A named member allocation in a nested anonymous body can first be stored in
// a local or consumed as a collection operand. Neither optimization use is a
// source-rendering phase: its hidden enclosing word has no independent Java
// spelling. The unchanged binary driver checks argument/exception identity,
// evaluation order, constructor overload, publication and member metadata.
func TestAdversarialMaterializedMemberCapturePreservesEvaluation(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"MemberArgsRoot", "RenamedMaterializedOwner"} {
			for _, layout := range []string{"local", "collection"} {
				t.Run(compiler.name+"/"+owner+"/"+layout, func(t *testing.T) {
					source := strings.ReplaceAll(anonymousMemberArgumentsFixture, "MemberArgsRoot", owner)
					allocation := "new Element(MemberArgsEffects.ref(1,first),MemberArgsEffects.wide(salt),MemberArgsEffects.ref(3,last))"
					body := "Element entry=" + allocation + ";return entry;"
					if layout == "collection" {
						body = "java.util.ArrayList<Element> entries=new java.util.ArrayList<>();entries.add(" + allocation + ");return entries.get(0);"
					}
					original := "return " + allocation + ";"
					if strings.Count(source, original) != 1 {
						t.Fatal("fixture allocation is not unique")
					}
					source = strings.Replace(source, original, body, 1)
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return nativePrivateEnumCompile(t, source, owner, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, "MemberArgsDriver", "900:anonymous:member:arguments:identity:effects\n", nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
