package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestUpdateIntrinsicsPreserveEffectsAndLocalDependencies(t *testing.T) {
	for _, op := range []string{INC, DEC} {
		for _, prefix := range []bool{false, true} {
			name := op + "/postfix"
			if prefix {
				name = op + "/prefix"
			}
			t.Run(name, func(t *testing.T) {
				x, y := motionRef(), motionRef()
				one := NewJavaLiteral(1, x.Type())
				var update JavaValue = NewBinaryExpression(x, one, op, x.Type())
				if prefix {
					update = NewUnaryExpression(NewSlotValue(x, x.Type()), op, x.Type())
				}
				effect, reads := InspectValue(update)
				if effect&EffectWriteMemory == 0 || !reads[x] || IsPure(update) || MayFold(update) {
					t.Fatal("typed update lost its observable write")
				}
				a := InspectAccess(update)
				if !a.Writes[x] || !a.Reads[x] || a.Effects != 0 {
					t.Fatalf("local update access: %+v", a)
				}
				alias := *x
				for _, other := range []JavaValue{x, &alias, NewAssignmentExpression(x, one, 1, nil), update} {
					b := InspectAccess(other)
					if CanSwap(a, b) || CanSwap(b, a) {
						t.Fatal("read/write or write/write order was lost")
					}
				}
				if !CanSwap(a, InspectAccess(y)) {
					t.Fatal("different original local identity conflated by its spelling")
				}
				if CanMoveAcrossPrefix(a, InspectAccess(x)) || CanMoveAcrossPrefix(InspectAccess(y), a) {
					t.Fatal("update crossed a dependent read or an effectful prefix")
				}
				want := x.String(nil)
				token := "++"
				if op == DEC {
					token = "--"
				}
				if prefix {
					want = token + "(" + want + ")"
				} else {
					want += token
				}
				if got := update.String(nil); got != want {
					t.Fatalf("update position: %s want %s", got, want)
				}
			})
		}
	}
}

func TestUpdateIntrinsicsRetainHeapWriteBarriers(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	field := NewJavaClassMember("AnyOwner", "state", "I", typ)
	for _, op := range []string{INC, DEC} {
		for _, prefix := range []bool{false, true} {
			var update JavaValue = NewBinaryExpression(field, NewJavaLiteral(1, typ), op, typ)
			if prefix {
				update = NewUnaryExpression(field, op, typ)
			}
			a := InspectAccess(update)
			if a.Effects&EffectWriteMemory == 0 || CanSwap(a, InspectAccess(motionRef())) || IsPure(update) {
				t.Fatal("field update lost its heap barrier", op, prefix)
			}
		}
	}
}
