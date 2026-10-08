package frametransfer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// The constant-pool class is ANEWARRAY's component, including when that
// component is itself an array. AALOAD removes exactly one reference rank.
// These are descriptor rules; neither instruction consults a library catalog.
func TestReferenceArrayTransferRetainsOriginalComponentAndRank(t *testing.T) {
	for _, component := range []string{"example/Element", "java/lang/Object", "[Ljava/lang/String;", "[[Ljava/lang/Object;", "[I", "[[J", strings.Repeat("[", 254) + "D"} {
		t.Run(component, func(t *testing.T) {
			in := NewFrame(1)
			in.Locals[0] = IntConst(42)
			in.Stack = []Type{IntConst(3)}
			before := in.Clone()
			out, exceptional, err := Transfer(in, Instr{Op: core.OP_ANEWARRAY, Class: component})
			if err != nil || exceptional == nil {
				t.Fatalf("allocation: %v", err)
			}
			want := "[L" + component + ";"
			if strings.HasPrefix(component, "[") {
				want = "[" + component
			}
			if !reflect.DeepEqual(out.Stack, []Type{RefOf(want)}) || !reflect.DeepEqual(in, before) {
				t.Fatalf("original component %s: stack %v want %s; mutated input=%v", component, out.Stack, want, !reflect.DeepEqual(in, before))
			}
			out.Stack = append(out.Stack, IntConst(1))
			loaded, loadExceptional, err := Transfer(out, Instr{Op: core.OP_AALOAD})
			if err != nil || loadExceptional == nil || !reflect.DeepEqual(loaded.Stack, []Type{RefOf(component)}) {
				t.Fatalf("array read must remove one rank: %v %v", loaded.Stack, err)
			}
		})
	}
}

func TestReferenceArrayTransferRefusesMissingOrMalformedComponents(t *testing.T) {
	for _, component := range []string{"", "[", "[V", "[L;", "[Ljava/lang/String;trailing", strings.Repeat("[", 255) + "I"} {
		t.Run("allocation/"+component, func(t *testing.T) {
			in := NewFrame(1)
			in.Stack = []Type{IntConst(1)}
			before := in.Clone()
			if _, _, err := Transfer(in, Instr{Op: core.OP_ANEWARRAY, Class: component}); err == nil {
				t.Fatal("missing, malformed or excessive-rank component accepted")
			}
			if !reflect.DeepEqual(in, before) {
				t.Fatal("failed allocation mutated its input frame")
			}
		})
	}
	for _, array := range []string{"[I", "[J", "[Z", "[", "[V", "[L;", "[Ljava/lang/String;trailing"} {
		t.Run("read/"+array, func(t *testing.T) {
			in := NewFrame(1)
			in.Locals[0] = IntConst(42)
			in.Stack = []Type{RefOf(array), IntConst(0)}
			before := in.Clone()
			if _, _, err := Transfer(in, Instr{Op: core.OP_AALOAD}); err == nil {
				t.Fatal("non-reference or malformed component accepted by AALOAD")
			}
			if !reflect.DeepEqual(in, before) {
				t.Fatal("failed read mutated its input frame")
			}
		})
	}
}
