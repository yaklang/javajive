package frametransfer

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// TestAdversarialArrayLUBAndAALOAD verifies JVMS 4.10.1.2 array LUB precision
// and AALOAD component preservation across joined reference array paths.
func TestAdversarialArrayLUBAndAALOAD(t *testing.T) {
	cases := []struct {
		name     string
		a, b     Type
		wantJoin Type
		wantElem *Type
	}{
		{
			name:     "String[] join Integer[] -> Object[] -> AALOAD Object",
			a:        RefOf("[Ljava/lang/String;"),
			b:        RefOf("[Ljava/lang/Integer;"),
			wantJoin: RefOf("[Ljava/lang/Object;"),
			wantElem: ptrType(RefOf("java/lang/Object")),
		},
		{
			name:     "String[][] join Integer[][] -> Object[][]",
			a:        RefOf("[[Ljava/lang/String;"),
			b:        RefOf("[[Ljava/lang/Integer;"),
			wantJoin: RefOf("[[Ljava/lang/Object;"),
			wantElem: ptrType(RefOf("[Ljava/lang/Object;")),
		},
		{
			name:     "int[][] join long[][] -> Object[]",
			a:        RefOf("[[I"),
			b:        RefOf("[[J"),
			wantJoin: RefOf("[Ljava/lang/Object;"),
			wantElem: ptrType(RefOf("java/lang/Object")),
		},
		{
			name:     "int[] join long[] -> java/lang/Object",
			a:        RefOf("[I"),
			b:        RefOf("[J"),
			wantJoin: RefOf("java/lang/Object"),
		},
		{
			name:     "Object[] join int[][] -> Object[]",
			a:        RefOf("[Ljava/lang/Object;"),
			b:        RefOf("[[I"),
			wantJoin: RefOf("[Ljava/lang/Object;"),
			wantElem: ptrType(RefOf("java/lang/Object")),
		},
		{
			name:     "String[] join null -> String[]",
			a:        RefOf("[Ljava/lang/String;"),
			b:        T(Null),
			wantJoin: RefOf("[Ljava/lang/String;"),
			wantElem: ptrType(RefOf("java/lang/String")),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := JoinTypes(tc.a, tc.b)
			if !ok || !got.Equal(tc.wantJoin) {
				t.Fatalf("JoinTypes(%v, %v) = (%v, %v), want %v", tc.a, tc.b, got, ok, tc.wantJoin)
			}
			rev, ok := JoinTypes(tc.b, tc.a)
			if !ok || !rev.Equal(got) {
				t.Fatalf("commutativity violated: JoinTypes(%v, %v) = %v vs %v", tc.b, tc.a, rev, got)
			}
			if tc.wantElem != nil {
				in := NewFrame(1)
				in.Stack = []Type{got, IntConst(0)}
				out, _, err := Transfer(in, Instr{Op: core.OP_AALOAD})
				if err != nil {
					t.Fatalf("AALOAD on %v failed: %v", got, err)
				}
				if !reflect.DeepEqual(out.Stack, []Type{*tc.wantElem}) {
					t.Fatalf("AALOAD stack = %v, want %v", out.Stack, *tc.wantElem)
				}
			}
		})
	}

	lattice := []Type{
		T(Null),
		RefOf("[Ljava/lang/String;"),
		RefOf("[Ljava/lang/Integer;"),
		RefOf("[Ljava/lang/Object;"),
		RefOf("[[Ljava/lang/String;"),
		RefOf("[[I"),
		RefOf("[[J"),
		RefOf("[I"),
		RefOf("[J"),
		RefOf("java/lang/Object"),
	}
	for _, x := range lattice {
		id, ok := JoinTypes(x, x)
		if !ok || !id.Equal(x.DropValue()) {
			t.Fatalf("idempotence failed for %v: got %v (%v)", x, id, ok)
		}
		for _, y := range lattice {
			for _, z := range lattice {
				xy, ok1 := JoinTypes(x, y)
				yz, ok2 := JoinTypes(y, z)
				if !ok1 || !ok2 {
					t.Fatalf("join failed on %v, %v, %v", x, y, z)
				}
				lhs, okL := JoinTypes(xy, z)
				rhs, okR := JoinTypes(x, yz)
				if !okL || !okR || !lhs.Equal(rhs) {
					t.Fatalf("associativity violated on (%v, %v, %v): (%v) != (%v)", x, y, z, lhs, rhs)
				}
			}
		}
	}
}

func ptrType(t Type) *Type { return &t }
