package dexparser

import (
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

// fillInnerClassOuters must link nested classes only from file evidence:
// a MemberClasses row declared by the outer class, or an EnclosingClass
// annotation. A "$" in the descriptor is not evidence and must not be parsed.
func TestTranslateSystemAnnotationsAssignsAnnotationDefaults(t *testing.T) {
	const owner = "Lpkg/Defaults;"
	file := &model.File{}
	class := model.Class{
		Descriptor: owner,
		VirtualMethods: []model.EncodedMethod{{
			MethodIndex: 7,
			Method:      model.MethodID{Class: owner, Name: "value"},
		}},
		Annotations: []model.Annotation{{
			Visibility: model.AnnotationSystem,
			Type:       "Ldalvik/annotation/AnnotationDefault;",
			Elements: []model.AnnotationElement{{
				Name: "value",
				Value: model.EncodedValue{Type: model.ValueAnnotation, Annotation: &model.EncodedAnnotation{
					Type:     owner,
					Elements: []model.AnnotationElement{{Name: "value", Value: model.EncodedValue{Type: model.ValueInt, Signed: 7}}},
				}},
			}},
		}},
	}
	p := &parser{file: file}
	if err := p.translateSystemAnnotations(&class); err != nil {
		t.Fatal(err)
	}
	if len(class.Annotations) != 0 || len(class.MethodAnnotations) != 1 {
		t.Fatalf("class = %#v", class)
	}
	annotation := class.MethodAnnotations[0]
	if annotation.MethodIndex != 7 || len(annotation.Annotations) != 1 || annotation.Annotations[0].Type != "Ldalvik/annotation/AnnotationDefault;" {
		t.Fatalf("method annotation = %#v", annotation)
	}
	value := annotation.Annotations[0].Elements[0].Value
	if value.Type != model.ValueInt || value.Signed != 7 {
		t.Fatalf("default value = %#v", value)
	}
}

func TestFillInnerClassOutersUsesFileEvidence(t *testing.T) {
	file := &model.File{Classes: []model.Class{
		{
			Descriptor: "Lpkg/Outer;",
			InnerClasses: []model.InnerClass{
				{Inner: "Lpkg/Outer$Member;", Outer: "Lpkg/Outer;", Name: "Member"},
			},
		},
		{
			// Member class: its own InnerClass annotation carries no outer; the
			// MemberClasses row above supplies it.
			Descriptor:   "Lpkg/Outer$Member;",
			InnerClasses: []model.InnerClass{{Inner: "Lpkg/Outer$Member;", Name: "Member"}},
		},
		{
			// Local/anonymous class: EnclosingClass supplies the declaring class.
			Descriptor:   "Lpkg/Outer$1;",
			Enclosing:    &model.EnclosingMethod{Class: "Lpkg/Outer;"},
			InnerClasses: []model.InnerClass{{Inner: "Lpkg/Outer$1;"}},
		},
		{
			// No evidence at all: the outer index must stay empty instead of
			// being guessed from the "$" in the descriptor.
			Descriptor:   "Lpkg/NoEvidence$X;",
			InnerClasses: []model.InnerClass{{Inner: "Lpkg/NoEvidence$X;"}},
		},
	}}
	(&parser{file: file}).fillInnerClassOuters()

	tests := []struct {
		class string
		want  string
	}{
		{"Lpkg/Outer$Member;", "Lpkg/Outer;"},
		{"Lpkg/Outer$1;", "Lpkg/Outer;"},
		{"Lpkg/NoEvidence$X;", ""},
	}
	for _, test := range tests {
		var found *model.InnerClass
		for index := range file.Classes {
			class := &file.Classes[index]
			if class.Descriptor != test.class {
				continue
			}
			if len(class.InnerClasses) != 1 {
				t.Fatalf("%s has %d inner class rows", test.class, len(class.InnerClasses))
			}
			found = &class.InnerClasses[0]
		}
		if found == nil {
			t.Fatalf("%s missing", test.class)
		}
		if found.Outer != test.want {
			t.Fatalf("%s outer = %q, want %q", test.class, found.Outer, test.want)
		}
	}
	// The outer class keeps its own MemberClasses row untouched.
	if outer := file.Classes[0].InnerClasses[0]; outer.Outer != "Lpkg/Outer;" {
		t.Fatalf("outer row = %#v", outer)
	}
}
