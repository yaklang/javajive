package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNumericWebJoinUsesCompleteDefinitionAndDomainEvidence(t *testing.T) {
	for _, kind := range []string{"double", "float", "long", "mixed", "int", "entry", "missing store", "missing value"} {
		t.Run(kind, func(t *testing.T) {
			name := types.JavaDouble
			if kind == "float" {
				name = types.JavaFloat
			}
			if kind == "long" {
				name = types.JavaLong
			}
			if kind == "int" {
				name = types.JavaInteger
			}
			typ := types.NewJavaPrimer(name)
			a, b := op(OP_DSTORE_1, 1), op(OP_DSTORE_1, 2)
			load := op(OP_DLOAD_1, 3)
			x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			y := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			x.Id.SetName("var3")
			y.Id.SetName("var8")
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2.0, typ)}
			rhs := typ
			if kind == "mixed" {
				rhs = types.NewJavaPrimer(types.JavaLong)
			}
			b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(3.0, rhs)}
			slot := values.NewSlotValue(x, typ)
			load.stackProduced = []values.JavaValue{slot}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1, load: 1}, entryWeb: map[int]int{0: 2}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: []*OpCode{a, b, load}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{a: {{x, true}}, b: {{y, true}}}}
			if kind == "entry" {
				webs.entryWeb[1] = 1
			}
			if kind == "missing store" {
				webs.webOf[op(OP_DSTORE_1, 4)] = 1
			}
			if kind == "missing value" {
				b.stackConsumed = nil
			}
			d.unifyNumericExitWebs()
			joined := kind == "double" || kind == "float" || kind == "long"
			got := d.opcodeIdToRef[a][0][0].(*values.JavaRef)
			if (got == d.opcodeIdToRef[b][0][0]) != joined {
				t.Fatalf("joined=%v expected=%v", got == d.opcodeIdToRef[b][0][0], joined)
			}
			if joined && (got.SolvedWebIdentity == nil || got.Id.String() != "var3" || slot.GetValue() != got || x.Id == got.Id || y.Id == got.Id) {
				t.Fatal("load or identity evidence lost; old alias mutated")
			}
		})
	}
}

func TestNumericWebRepairDoesNotReplaceConsistentIdentity(t *testing.T) {
	for _, name := range []string{"consistent", "stale load", "foreign web", "nil id"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaDouble)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			a, b, load := op(OP_DSTORE_1, 1), op(OP_DSTORE_1, 2), op(OP_DLOAD_1, 3)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1.0, typ)}
			b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2.0, typ)}
			slot := values.NewSlotValue(ref, typ)
			load.stackProduced = []values.JavaValue{slot}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1, load: 1}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: []*OpCode{a, b, load}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{a: {{ref, true}}, b: {{ref, false}}}}
			if name == "stale load" {
				slot.ResetValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ))
			}
			if name == "foreign web" {
				other := op(OP_DSTORE_2, 4)
				webs.webOf[other] = 2
				d.opcodeIdToRef[other] = [][2]any{{ref, true}}
			}
			if name == "nil id" {
				ref.Id = nil
			}
			d.unifyNumericExitWebs()
			repaired := d.opcodeIdToRef[a][0][0] != ref
			if repaired != (name == "stale load" || name == "foreign web") {
				t.Fatalf("repaired=%v", repaired)
			}
			if repaired && slot.GetValue() != d.opcodeIdToRef[a][0][0] {
				t.Fatal("stale load retained")
			}
		})
	}
}

func TestIntegerExitWebRequiresClosedNonBooleanConstants(t *testing.T) {
	for _, name := range []string{"proved", "boolean domain", "nonconstant", "byte domain", "entry", "increment"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			y := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			a, b, load := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 2), op(OP_ILOAD_1, 3)
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, typ)}
			b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, typ)}
			slot := values.NewSlotValue(y, typ)
			load.stackProduced = []values.JavaValue{slot}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1, load: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: []*OpCode{a, b, load}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{a: {{x, true}}, b: {{y, true}}}}
			switch name {
			case "increment":
				inc := op(OP_IINC, 4)
				inc.Data = []byte{1, 1}
				d.opCodes = append(d.opCodes, inc)
			case "boolean domain":
				b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, typ)}
			case "nonconstant":
				b.stackConsumed = []values.JavaValue{y}
			case "byte domain":
				b.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaByte))}
			case "entry":
				webs.entryWeb[0] = 1
			}
			d.unifyNumericExitWebs()
			got := d.opcodeIdToRef[a][0][0].(*values.JavaRef)
			if (got.SolvedWebIdentity != nil) != (name == "proved") {
				t.Fatal("wrong finite domain proof", name)
			}
			if name == "proved" && (got != d.opcodeIdToRef[b][0][0] || slot.GetValue() != got || got.Id == x.Id || got.Id == y.Id) {
				t.Fatal("definitions and use did not receive one fresh identity")
			}
		})
	}
}
