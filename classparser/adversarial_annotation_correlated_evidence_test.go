package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestAdversarialAnnotationPhysicalCodeCannotBypassExplicitMetadata(t *testing.T) {
	files := nativeCompileClasses(t, annotationClassBoundsFixture)
	for _, variant := range []string{"original", "physical only", "metadata only", "foreign identity", "incomplete hierarchy", "missing metadata", "wrong parent", "duplicate parent", "ambiguous providers", "nil provider"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["ClassBoundOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok }
			original := d.nativeAnnotationDeclarationResolver()
			if object, known := original("java/lang/Integer"); !known || object == nil || object.GetClassName() != "java/lang/Integer" {
				t.Fatal("original indexed platform code required")
			}
			resolve := original
			if variant == "metadata only" {
				resolve = func(n string) (*ClassObject, bool) { return nil, false }
			}
			metadata := d.buildInvocationMetadata()
			provider := func(n string) (callbinding.Class, bool) {
				m, known := metadata(n)
				if n == "java/lang/Integer" {
					switch variant {
					case "foreign identity":
						m.Name = "Foreign"
					case "incomplete hierarchy":
						m.ParentsComplete = false
					case "missing metadata":
						return callbinding.Class{}, false
					case "wrong parent":
						m.Parents = []string{"java/lang/Object"}
					case "duplicate parent":
						m.Parents = append(m.Parents, m.Parents[0])
					}
				}
				return m, known
			}
			providers := []callbinding.Provider{provider}
			if variant == "physical only" {
				providers = nil
			}
			if variant == "ambiguous providers" {
				providers = append(providers, metadata)
			}
			if variant == "nil provider" {
				providers[0] = nil
			}
			v := &ElementValuePairAttribute{Tag: 'c', Value: "I"}
			got := nativeAnnotationClassSignatureMatches("Ljava/lang/Class;", "()Ljava/lang/Class<+Ljava/lang/Number;>;", v, nil, resolve, providers...)
			want := variant == "original" || variant == "physical only" || variant == "metadata only"
			if got != want {
				t.Fatalf("correlated original declarations %v expected %v", got, want)
			}
		})
	}
}
