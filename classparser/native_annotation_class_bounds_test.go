package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnnotationClassBoundsRequireOriginalDeclarations(t *testing.T) {
	files := nativeCompileClasses(t, annotationClassBoundsFixture)
	for _, scenario := range []string{"upper", "lower", "identity", "interface", "primitive", "void identity", "reference array", "primitive array", "array covariance", "empty array", "no default", "unbounded", "wrong upper", "wrong lower", "wrong identity", "wrong interface", "wrong tag", "void array", "parameterized bound", "type variable", "two arguments", "trailing text", "formal variable", "throws signature", "primitive bound", "nested wildcard", "wrong erased return", "missing bound", "foreign bound identity", "nil bound", "missing actual", "missing ancestor", "ancestor cycle", "foreign metadata identity", "incomplete metadata", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root, e := Parse(append([]byte(nil), files["ClassBoundOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok }
			original := d.nativeAnnotationDeclarationResolver()
			metadata := d.buildInvocationMetadata()
			objects := map[string]*ClassObject{}
			resolve := func(n string) (*ClassObject, bool) {
				if o, ok := objects[n]; ok {
					return o, true
				}
				if scenario == "missing bound" && n == "BoundBase" || scenario == "missing actual" && n == "BoundLeaf" || scenario == "missing ancestor" && n == "BoundBase" {
					return nil, false
				}
				return original(n)
			}
			provider := func(n string) (callbinding.Class, bool) {
				if scenario == "missing bound" && n == "BoundBase" || scenario == "missing actual" && n == "BoundLeaf" || scenario == "missing ancestor" && n == "BoundBase" {
					return callbinding.Class{}, false
				}
				m, ok := metadata(n)
				if scenario == "foreign metadata identity" && n == "java/lang/Integer" {
					m.Name = "Other"
				}
				if scenario == "incomplete metadata" && n == "java/lang/Integer" {
					m.ParentsComplete = false
				}
				return m, ok
			}
			desc := "Ljava/lang/Class;"
			sig := "()Ljava/lang/Class<+LBoundBase;>;"
			v := &ElementValuePairAttribute{Tag: 'c', Value: "LBoundLeaf;"}
			want := false
			var work *workbudget.Budget
			switch scenario {
			case "upper":
				want = true
			case "lower":
				sig = "()Ljava/lang/Class<-LBoundLeaf;>;"
				v.Value = "LBoundBase;"
				want = true
			case "identity":
				sig = "()Ljava/lang/Class<LBoundLeaf;>;"
				want = true
			case "interface":
				sig = "()Ljava/lang/Class<+LBoundMarker;>;"
				want = true
			case "primitive", "foreign metadata identity", "incomplete metadata":
				sig = "()Ljava/lang/Class<+Ljava/lang/Number;>;"
				v.Value = "I"
				want = scenario == "primitive"
			case "void identity":
				sig = "()Ljava/lang/Class<Ljava/lang/Void;>;"
				v.Value = "V"
				want = true
			case "reference array":
				sig = "()Ljava/lang/Class<+[Ljava/lang/Object;>;"
				v.Value = "[[Ljava/lang/String;"
				want = true
			case "primitive array":
				sig = "()Ljava/lang/Class<+Ljava/lang/Cloneable;>;"
				v.Value = "[I"
				want = true
			case "array covariance":
				sig = "()Ljava/lang/Class<+[LBoundBase;>;"
				v.Value = "[LBoundLeaf;"
				want = true
			case "empty array":
				desc = "[Ljava/lang/Class;"
				sig = "()[Ljava/lang/Class<+LBoundBase;>;"
				v = &ElementValuePairAttribute{Tag: '[', Value: []*ElementValuePairAttribute{}}
				want = true
			case "no default":
				v = nil
				want = true
			case "unbounded":
				sig = "()Ljava/lang/Class<*>;"
				v.Value = "V"
				want = true
			case "wrong upper", "wrong identity":
				v.Value = "Ljava/lang/String;"
				if scenario == "wrong identity" {
					sig = "()Ljava/lang/Class<LBoundLeaf;>;"
				}
			case "wrong lower":
				sig = "()Ljava/lang/Class<-LBoundBase;>;"
			case "wrong interface":
				sig = "()Ljava/lang/Class<+LBoundMarker;>;"
				v.Value = "Ljava/lang/String;"
			case "wrong tag":
				v.Tag = 's'
			case "void array":
				v.Value = "[V"
			case "parameterized bound":
				sig = "()Ljava/lang/Class<+Ljava/util/List<Ljava/lang/String;>;>;"
			case "type variable":
				sig = "()Ljava/lang/Class<TT;>;"
			case "two arguments":
				sig = "()Ljava/lang/Class<LBoundBase;LBoundLeaf;>;"
			case "trailing text":
				sig += "junk"
			case "formal variable":
				sig = "<T:LBoundBase;>" + sig
			case "throws signature":
				sig += "^Ljava/lang/Throwable;"
			case "primitive bound":
				sig = "()Ljava/lang/Class<+I>;"
			case "nested wildcard":
				sig = "()Ljava/lang/Class<++LBoundBase;>;"
			case "wrong erased return":
				desc = "Ljava/lang/Object;"
			case "foreign bound identity":
				objects["BoundBase"] = root
			case "nil bound":
				objects["BoundBase"] = nil
			case "ancestor cycle":
				o, e := Parse(append([]byte(nil), files["BoundLeaf.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				o.SuperClass = o.ThisClass
				objects["BoundLeaf"] = o
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnnotationClassSignatureMatches(desc, sig, v, work, resolve, provider); got != want {
				t.Fatalf("signature=%s literal=%v admitted=%v want=%v", sig, v, got, want)
			}
		})
	}
}
func TestNativeNestedAnnotationDefaultKeepsClassArgumentBounds(t *testing.T) {
	files := nativeCompileClasses(t, annotationClassBoundsFixture+`@interface BoundEnvelope {ClassBoundOwner.Spec value();}`)
	root, e := Parse(files["ClassBoundOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := NewClassObjectDumper(root)
	d.options.TargetSourceVersion = 8
	d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	resolve := d.nativeAnnotationDeclarationResolver()
	for _, scenario := range []string{"valid upper", "invalid upper", "valid lower", "invalid lower", "mixed array"} {
		t.Run(scenario, func(t *testing.T) {
			name, literal := "upper", "LBoundLeaf;"
			want := scenario == "valid upper" || scenario == "valid lower"
			if scenario == "invalid upper" {
				literal = "Ljava/lang/String;"
			}
			if scenario == "valid lower" {
				name, literal = "lower", "Ljava/lang/Object;"
			}
			if scenario == "invalid lower" {
				name, literal = "lower", "Ljava/lang/String;"
			}
			pair := &ElementValuePairAttribute{Name: name, Tag: 'c', Value: literal}
			if scenario == "mixed array" {
				pair = &ElementValuePairAttribute{Name: "many", Tag: '[', Value: []*ElementValuePairAttribute{{Tag: 'c', Value: "LBoundLeaf;"}, {Tag: 'c', Value: "Ljava/lang/String;"}}}
			}
			spec := &AnnotationAttribute{TypeName: "LClassBoundOwner$Spec;", ElementValuePairs: []*ElementValuePairAttribute{pair}}
			envelope := &ElementValuePairAttribute{Tag: '@', Value: &AnnotationAttribute{TypeName: "LBoundEnvelope;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "value", Tag: '@', Value: spec}}}}
			if got := nativeAnnotationDefaultMatches("LBoundEnvelope;", envelope, nil, resolve, d.buildInvocationMetadata()); got != want {
				t.Fatalf("nested bound admitted=%v want=%v", got, want)
			}
		})
	}
}
