package ssabuild

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

func originStep(t *testing.T, f frametransfer.Frame, origins []Origin, ins methodir.Instr) (frametransfer.Frame, []Origin, InstructionValues) {
	t.Helper()
	ft := frametransfer.FromIR(ins)
	ft.Local = effectiveLocal(ins)
	after, _, err := frametransfer.Transfer(f, ft)
	if err != nil {
		t.Fatal(err)
	}
	out, record, err := transferOrigins(origins, f, after, ins)
	if err != nil {
		t.Fatal(err)
	}
	return after, out, record
}

func TestOriginStackForms(t *testing.T) {
	// Inputs/expected permutations are independent JVMS bottom-to-top examples.
	cases := []struct {
		name         string
		op           int
		widths, want []int
	}{
		{"pop", core.OP_POP, []int{1, 1}, []int{0}},
		{"pop2 pair", core.OP_POP2, []int{1, 2}, []int{0}},
		{"pop2 singles", core.OP_POP2, []int{1, 1, 1}, []int{0}},
		{"dup", core.OP_DUP, []int{1, 1}, []int{0, 1, 1}},
		{"dup_x1", core.OP_DUP_X1, []int{1, 1, 1}, []int{0, 2, 1, 2}},
		{"dup_x2 singles", core.OP_DUP_X2, []int{1, 1, 1, 1}, []int{0, 3, 1, 2, 3}},
		{"dup_x2 pair", core.OP_DUP_X2, []int{1, 2, 1}, []int{0, 2, 1, 2}},
		{"dup2 singles", core.OP_DUP2, []int{1, 1, 1}, []int{0, 1, 2, 1, 2}},
		{"dup2 pair", core.OP_DUP2, []int{1, 2}, []int{0, 1, 1}},
		{"dup2_x1 singles", core.OP_DUP2_X1, []int{1, 1, 1, 1}, []int{0, 2, 3, 1, 2, 3}},
		{"dup2_x1 pair", core.OP_DUP2_X1, []int{1, 1, 2}, []int{0, 2, 1, 2}},
		{"dup2_x2 1111", core.OP_DUP2_X2, []int{1, 1, 1, 1, 1}, []int{0, 3, 4, 1, 2, 3, 4}},
		{"dup2_x2 211", core.OP_DUP2_X2, []int{1, 2, 1, 1}, []int{0, 2, 3, 1, 2, 3}},
		{"dup2_x2 112", core.OP_DUP2_X2, []int{1, 1, 1, 2}, []int{0, 3, 1, 2, 3}},
		{"dup2_x2 22", core.OP_DUP2_X2, []int{1, 2, 2}, []int{0, 2, 1, 2}},
		{"swap", core.OP_SWAP, []int{1, 1, 1}, []int{0, 2, 1}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := frametransfer.NewFrame(0)
			var origins []Origin
			for i, w := range tt.widths {
				typ := frametransfer.T(frametransfer.Int)
				if w == 2 {
					typ = frametransfer.T(frametransfer.Long)
				}
				f.Stack = append(f.Stack, typ)
				origins = append(origins, Origin{Kind: OriginParam, Slot: i})
				if w == 2 {
					f.Stack = append(f.Stack, frametransfer.TailOf(typ))
					origins = append(origins, origins[len(origins)-1])
				}
			}
			_, out, r := originStep(t, f, origins, methodir.Instr{PC: 90, Opcode: tt.op, Local: -1})
			var want []Origin
			for _, i := range tt.want {
				for j := 0; j < tt.widths[i]; j++ {
					want = append(want, Origin{Kind: OriginParam, Slot: i})
				}
			}
			if !reflect.DeepEqual(out, want) {
				t.Fatalf("got %v want %v", out, want)
			}
			if len(r.Results) != 0 {
				t.Fatal("stack alias invented definition")
			}
		})
	}
}

func TestOriginComputeAndLocalAliases(t *testing.T) {
	f := frametransfer.NewFrame(4)
	f.Stack = []frametransfer.Type{frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Int)}
	origins := paramOrigins(f)
	a, b := origins[5], origins[6]
	f, origins, r := originStep(t, f, origins, methodir.Instr{PC: 10, Opcode: core.OP_IADD, Local: -1})
	if origins[4] != (Origin{Kind: OriginParam, Slot: 4}) || !reflect.DeepEqual(r.Uses, []Origin{a, b}) || origins[5].PC != 10 {
		t.Fatalf("prefix/operands lost: %v %+v", origins, r)
	}
	f, origins, _ = originStep(t, f, origins, methodir.Instr{PC: 11, Opcode: core.OP_ISTORE_0, Local: 0})
	if origins[0].PC != 10 {
		t.Fatal("store changed value identity")
	}
	f, origins, _ = originStep(t, f, origins, methodir.Instr{PC: 12, Opcode: core.OP_ILOAD_0, Local: 0})
	if origins[len(origins)-1] != origins[0] {
		t.Fatal("load changed value identity")
	}
	_, out, r := originStep(t, f, origins, methodir.Instr{PC: 13, Opcode: core.OP_INEG, Local: -1})
	if out[len(out)-1].PC != 13 || len(r.Uses) != 1 || r.Uses[0].PC != 10 {
		t.Fatalf("bad unary definition: %+v", r)
	}
}

func TestOriginWideLocalAndOperandRebinding(t *testing.T) {
	f := frametransfer.NewFrame(4)
	if err := f.StoreLocal(0, frametransfer.T(frametransfer.Long)); err != nil {
		t.Fatal(err)
	}
	origins := paramOrigins(f)
	f, origins, _ = originStep(t, f, origins, methodir.Instr{PC: 1, Opcode: core.OP_LLOAD_0, Local: 0})
	if origins[4] != origins[5] || origins[4] != origins[0] {
		t.Fatal("wide load did not preserve alias")
	}
	f, origins, _ = originStep(t, f, origins, methodir.Instr{PC: 2, Opcode: core.OP_LSTORE_2, Local: 2})
	if origins[2] != origins[0] || origins[3] != origins[0] {
		t.Fatal("wide store did not preserve alias")
	}
	f, origins, _ = originStep(t, f, origins, methodir.Instr{PC: 3, Opcode: core.OP_LLOAD_2, Local: 2})
	ins := methodir.Instr{PC: 4, Opcode: core.OP_LNEG, Local: -1}
	_, out, first := originStep(t, f, origins, ins)
	origins[4] = Origin{Kind: OriginPhi, PC: 3, Slot: 4}
	origins[5] = origins[4]
	_, out2, second := originStep(t, f, origins, ins)
	if out[4] != out2[4] || first.Uses[0] == second.Uses[0] || second.Uses[0] != origins[4] {
		t.Fatal("unstable result or stale operands")
	}
}

func TestOriginCallsCheckedResultsAndArrays(t *testing.T) {
	f := frametransfer.NewFrame(0)
	f.Stack = []frametransfer.Type{frametransfer.T(frametransfer.Int), frametransfer.RefOf("T"), frametransfer.T(frametransfer.Long), frametransfer.T(frametransfer.LongTail)}
	origins := paramOrigins(f)
	_, out, r := originStep(t, f, origins, methodir.Instr{PC: 8, Opcode: core.OP_INVOKEVIRTUAL, Desc: "(J)I", Local: -1})
	if len(out) != 2 || out[0] != origins[0] || !reflect.DeepEqual(r.Uses, []Origin{origins[1], origins[2]}) {
		t.Fatalf("call effect: %v %+v", out, r)
	}
	f.Stack = []frametransfer.Type{frametransfer.RefOf("T")}
	origins = paramOrigins(f)
	_, out, r = originStep(t, f, origins, methodir.Instr{PC: 9, Opcode: core.OP_CHECKCAST, Class: "T", Local: -1})
	if out[0] == origins[0] || len(r.Results) != 1 || r.Uses[0] != origins[0] {
		t.Fatal("checkcast erased checked definition")
	}
	f.Stack = []frametransfer.Type{frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Int)}
	origins = paramOrigins(f)
	_, out, r = originStep(t, f, origins, methodir.Instr{PC: 10, Opcode: core.OP_MULTIANEWARRAY, Class: "[[I", Data: []byte{0, 1, 2}, Local: -1})
	if len(out) != 2 || out[0] != origins[0] || len(r.Uses) != 2 {
		t.Fatalf("array effect: %v %+v", out, r)
	}
}

func TestOriginRejectsMalformedAndUnknown(t *testing.T) {
	f := frametransfer.Frame{Stack: []frametransfer.Type{frametransfer.T(frametransfer.Long), frametransfer.T(frametransfer.LongTail)}}
	origins := []Origin{{Kind: OriginParam, Slot: 0}, {Kind: OriginParam, Slot: 1}}
	if _, _, err := transferOrigins(origins, f, f, methodir.Instr{Opcode: core.OP_DUP2}); err == nil {
		t.Fatal("mismatched pair accepted")
	}
	origins[1] = origins[0]
	if _, _, err := transferOrigins(origins, f, f, methodir.Instr{Opcode: core.OP_SWAP}); err == nil {
		t.Fatal("split pair accepted")
	}
	if _, _, err := transferOrigins(origins, f, f, methodir.Instr{Opcode: 255}); err == nil || !strings.Contains(err.Error(), "unsupported_feature") {
		t.Fatalf("wrong unknown-op diagnostic: %v", err)
	}
}
