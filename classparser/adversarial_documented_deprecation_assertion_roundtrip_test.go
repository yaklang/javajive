package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// A Deprecated classfile attribute need not have a runtime annotation: javac
// also emits it for the declaration's Javadoc tag. Losing that distinction can
// reject an otherwise closed lexical family, flatten its assertion-status
// owner, and make the child's explicit compiler flag collide with its parent.
// Neither the inherited assertion nor the original metadata may be discarded.
func TestAdversarialDocumentedDeprecationKeepsAssertionScopeAndMetadata(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"Owner", "RenamedDocumentedScope"} {
			for _, site := range []string{"root", "child", "both"} {
				for _, enabled := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/enabled=%t", compiler.name, owner, site, enabled), func(t *testing.T) {
						fixture := strings.NewReplacer("BASE_STATUS", fmt.Sprint(!enabled), "CHILD_STATUS", fmt.Sprint(enabled)).Replace(nativeStandaloneAssertionFixture)
						if site != "child" {
							fixture = strings.Replace(fixture, "class OwnerFamily{", "/** @deprecated original documentation marker */ class OwnerFamily{", 1)
						}
						if site != "root" {
							fixture = strings.Replace(fixture, "static class Child", "/** @deprecated original child documentation marker */ static class Child", 1)
						}
						fixture = strings.Replace(fixture, `System.out.println("root-assertion:independent-status:effects:metadata");`, `if(OwnerFamily.class.isAnnotationPresent(Deprecated.class)||OwnerFamily.Child.class.isAnnotationPresent(Deprecated.class)||!OwnerFamily.Child.class.isMemberClass()||OwnerFamily.Child.class.getEnclosingClass()!=OwnerFamily.class)throw new AssertionError("documentation marker must not invent annotation or lose scope");java.lang.reflect.Field childFlag=OwnerFamily.Child.class.getDeclaredField("$assertionsDisabled");if(!childFlag.isSynthetic()||childFlag.getModifiers()!=0x1018)throw new AssertionError("child flag metadata");System.out.println("documented:assertions:metadata:scope");`, 1)
						fixture = strings.ReplaceAll(fixture, "Owner", owner)
						compile := func(debug string) map[string][]byte {
							var files map[string][]byte
							if compiler.profile == NativeJavac8 {
								files = localGetterCompileJavac8(t, map[string]string{owner + "Family.java": fixture}, debug)
							} else {
								files = nativeCompileDebugClasses(t, fixture, debug)
							}
							return files
						}
						testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner + "Base", owner + "Family"}, "RootAssertionDriver", "documented:assertions:metadata:scope\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures, documentedClassDeprecationMetadata)
					})
				}
			}
		}
	}
}

func documentedClassDeprecationMetadata(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	snapshot := func(raw []byte) string {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		markers, annotations := 0, 0
		for _, attr := range obj.Attributes {
			switch a := attr.(type) {
			case *DeprecatedAttribute:
				if a == nil || a.AttrLen != 0 {
					t.Fatal("invalid deprecation attribute")
				}
				markers++
			case *RuntimeVisibleAnnotationsAttribute:
				for _, annotation := range a.Annotations {
					if annotation.TypeName == "Ljava/lang/Deprecated;" {
						annotations++
					}
				}
			}
		}
		return fmt.Sprintf("markers=%d/annotations=%d", markers, annotations)
	}
	if before, after := snapshot(original), snapshot(rebuilt); before != after {
		t.Fatalf("declaration metadata changed %s: %s != %s", name, before, after)
	}
}
