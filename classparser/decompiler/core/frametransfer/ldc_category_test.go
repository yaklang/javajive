package frametransfer

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"reflect"
	"testing"
)

func TestLDCWidthMustMatchOriginalConstantCategory(t *testing.T) {
	for _, op := range []int{core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W} {
		for _, constant := range []methodir.Const{{}, {Kind: methodir.ConstInt, Int: 7}, {Kind: methodir.ConstFloat, FloatBits: 0x80000000}, {Kind: methodir.ConstString, String: "s"}, {Kind: methodir.ConstClass, Class: "java/lang/String"}, {Kind: methodir.ConstLong, Long: 7}, {Kind: methodir.ConstDouble, DoubleBits: 0x8000000000000000}} {
			in := NewFrame(1)
			in.Locals[0] = IntConst(99)
			before := in.Clone()
			out, _, err := Transfer(in, Instr{Op: op, Const: constant})
			cat2 := constant.Kind == methodir.ConstLong || constant.Kind == methodir.ConstDouble
			wantOK := constant.Kind != methodir.ConstNone && ((op == core.OP_LDC2_W) == cat2)
			if (err == nil) != wantOK {
				t.Errorf("op0x%x kind%d: err%v wantOK%t stack%v", op, constant.Kind, err, wantOK, out.Stack)
			}
			if !wantOK && constant.Kind == methodir.ConstNone && !IsUnsupported(err) {
				t.Fatalf("unknown CP kind classified as %v instead of unsupported", err)
			}
			if !wantOK && constant.Kind != methodir.ConstNone && !IsInvalid(err) {
				t.Fatalf("illegal opcode/constant width classified as %v instead of invalid", err)
			}
			if !reflect.DeepEqual(in, before) {
				t.Fatal("rejected or accepted constant mutated incoming frame")
			}
			if wantOK {
				width := 1
				if cat2 {
					width = 2
				}
				if len(out.Stack) != width {
					t.Errorf("kind%d output width%d want%d", constant.Kind, len(out.Stack), width)
				}
			}
		}
	}
}
