package dexparser

import (
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

// fillInnerClassOuters must link nested classes only from file evidence:
// a MemberClasses row declared by the outer class, or an EnclosingClass
// annotation. A "$" in the descriptor is not evidence and must not be parsed.
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
