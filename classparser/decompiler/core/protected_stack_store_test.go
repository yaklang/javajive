package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestProtectedStackStoreEdges(t *testing.T) {
	for _, kind := range []string{"valid", "subtype join", "missing handler", "conditional edge", "back edge", "extra target", "empty stack", "two values", "type mismatch", "reused local"} {
		t.Run(kind, func(t *testing.T) {
			store := &OpCode{Instr: &Instruction{OpCode: OP_ISTORE_1}, CurrentOffset: 40}
			normal := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 20, Target: []*OpCode{store}}
			caught := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 30, Target: []*OpCode{store}}
			handler := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_1}, CurrentOffset: 25, IsCatch: true, Target: []*OpCode{caught}}
			caught.Source = []*OpCode{handler}
			store.Source = []*OpCode{normal, caught}
			a := values.NewJavaLiteral(42, types.NewJavaPrimer(types.JavaInteger))
			b := values.NewJavaLiteral(-7, types.NewJavaPrimer(types.JavaInteger))
			normal.StackEntry, caught.StackEntry = newStackItem(NewEmptyStackEntry(), a), newStackItem(NewEmptyStackEntry(), b)
			ref := values.NewJavaRef(utils.NewRootVariableId(), a, a.Type())
			d := &Decompiler{opCodes: []*OpCode{normal, handler, caught, store},
				ExceptionTable: []*ExceptionTableEntry{{StartPc: 0, EndPc: 20, HandlerPc: 25}},
				opcodeIdToRef:  map[*OpCode][][2]any{store: {{ref, true}}}}
			accepted, invalid := true, false
			switch kind {
			case "subtype join":
				store.Instr.OpCode = OP_ASTORE_1
				a.Type().ResetType(types.NewJavaClass("p.Base"))
				b.Type().ResetType(types.NewJavaClass("p.Child"))
				d.FunctionContext = &class_context.ClassContext{SiblingSuperTypes: func(name string) ([]string, bool) {
					if name == "p/Child" {
						return []string{"p/Base"}, true
					}
					if name == "p/Base" {
						return []string{"java/lang/Object"}, true
					}
					return nil, false
				}}
			case "missing handler":
				d.ExceptionTable, accepted = nil, false
			case "conditional edge":
				normal.Instr.OpCode, accepted = OP_IFNE, false
			case "back edge":
				normal.CurrentOffset, accepted = 50, false
			case "extra target":
				normal.Target, accepted = append(normal.Target, handler), false
			case "empty stack":
				caught.StackEntry, invalid = NewEmptyStackEntry(), true
			case "two values":
				caught.StackEntry, invalid = newStackItem(caught.StackEntry, b), true
			case "type mismatch":
				caught.StackEntry.value, invalid = values.NewJavaLiteral("x", types.NewJavaClass("java.lang.String")), true
			case "reused local":
				d.opcodeIdToRef[store][0][1], invalid = false, true
			}
			if d.isProtectedStackStore(store) != accepted {
				t.Fatal("incorrect edge proof")
			}
			stores, edges, err := d.lowerProtectedStackStores()
			if (err != nil) != invalid {
				t.Fatalf("error=%v, invalid=%v", err, invalid)
			}
			if !accepted || invalid {
				if len(stores) != 0 || len(edges) != 0 {
					t.Fatal("partial lowering escaped a rejected plan")
				}
				return
			}
			if !stores[store] || edges[normal].JavaValue != a || edges[caught].JavaValue != b ||
				edges[normal].LeftValue != ref || edges[caught].LeftValue != ref || len(d.disFoldRef) != 1 || d.disFoldRef[0] != ref {
				t.Fatal("edge values or shared local identity were lost")
			}
		})
	}
}

func TestNopSplicePreservesControlEdges(t *testing.T) {
	for _, kind := range []string{"ordinary", "try anchor", "handler"} {
		t.Run(kind, func(t *testing.T) {
			branch := &OpCode{Id: 1, Instr: &Instruction{OpCode: OP_IFEQ}}
			nop := &OpCode{Id: 2, Instr: &Instruction{OpCode: OP_NOP}, Source: []*OpCode{branch}}
			body := &OpCode{Id: 3, Instr: &Instruction{OpCode: OP_RETURN}, Source: []*OpCode{nop}}
			other := &OpCode{Id: 4, Instr: &Instruction{OpCode: OP_RETURN}, Source: []*OpCode{branch}}
			branch.Target, branch.Jmp, nop.Target = []*OpCode{nop, other}, nop.Id, []*OpCode{body}
			nop.IsTryCatchParent, nop.IsCatch = kind == "try anchor", kind == "handler"
			nop.TryNode = body
			d := &Decompiler{opCodes: []*OpCode{branch, nop, body, other}}
			if err := d.DropUnreachableOpcode(); err != nil {
				t.Fatal(err)
			}
			want := nop
			if kind == "ordinary" {
				want = body
				if len(body.Source) != 1 || body.Source[0] != branch {
					t.Fatal("predecessor not spliced")
				}
			}
			if branch.Target[0] != want || branch.Target[1] != other || branch.Jmp != want.Id {
				t.Fatal("branch polarity or structural anchor changed")
			}
		})
	}
}
