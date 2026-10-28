package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestPrimitiveWebSplitRetainsResidualDeclarationWithoutRebinding(t *testing.T) {
	for _, kind := range []string{types.JavaInteger, types.JavaBoolean, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, scenario := range []string{"transfer", "already declared", "other declaration", "not displaced", "no residual definition", "missing operand", "ambiguous record", "second residual"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				typ := types.NewJavaPrimer(kind)
				old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				old.Id.SetName("old")
				id, uid := old.Id, old.VarUid
				a, b, c, load, later := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 2), op(OP_ISTORE_1, 3), op(OP_ILOAD_1, 4), op(OP_ISTORE_1, 5)
				rhs := values.NewJavaLiteral(0, typ)
				for _, st := range []*OpCode{a, b, c, later} {
					st.stackConsumed = []values.JavaValue{rhs}
				}
				own, other := values.NewSlotValue(old, typ), values.NewSlotValue(old, typ)
				selectedLoad := op(OP_ILOAD_1, 2)
				selectedLoad.stackProduced = []values.JavaValue{own}
				load.stackProduced = []values.JavaValue{other}
				webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1, selectedLoad: 1, c: 2, load: 2, later: 2}}
				d := &Decompiler{opCodes: []*OpCode{a, b, selectedLoad, c, load, later}, opcodeIdToRef: map[*OpCode][][2]any{a: {{old, true}}, b: {{old, false}}, c: {{old, false}}}}
				switch scenario {
				case "already declared":
					d.opcodeIdToRef[c][0][1] = true
				case "other declaration":
					d.opcodeIdToRef[op(OP_DUP, 0)] = [][2]any{{old, true}}
				case "not displaced":
					d.opcodeIdToRef[a][0][1] = false
				case "no residual definition":
					delete(d.opcodeIdToRef, c)
				case "missing operand":
					c.stackConsumed = nil
				case "ambiguous record":
					d.opcodeIdToRef[c] = append(d.opcodeIdToRef[c], [2]any{old, false})
				case "second residual":
					d.opcodeIdToRef[later] = [][2]any{{old, false}}
				}
				canon := d.bindProvedPrimitiveWeb(webs, 1, []*OpCode{a, b}, kind)
				expected := scenario == "transfer" || scenario == "already declared" || scenario == "second residual"
				got := len(d.opcodeIdToRef[c]) > 0 && d.opcodeIdToRef[c][0][1] == true
				if got != expected {
					t.Fatalf("residual declaration=%v want=%v", got, expected)
				}
				if own.GetValue() != canon || old.Id != id || old.VarUid != uid || other.GetValue() != old || c.stackConsumed != nil && c.stackConsumed[0] != rhs || a.stackConsumed[0] != rhs || b.stackConsumed[0] != rhs || canon == old || d.opcodeIdToRef[a][0][0] != canon || d.opcodeIdToRef[b][0][0] != canon {
					t.Fatal("identity, binding or evaluation changed")
				}
				if scenario == "second residual" && d.opcodeIdToRef[later][0][1] != false {
					t.Fatal("duplicate residual declaration")
				}
			})
		}
	}
}
