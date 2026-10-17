package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNumericConditionalWebKeepsWholeWordAndSeparateLifetime(t *testing.T) {
	for _, name := range []string{"char", "short", "byte", "entry", "multiple definitions", "incomplete definitions", "missing operand", "missing load", "foreign category", "increment", "parameter", "already solved", "narrow result", "boolean", "handlers", "dynamic operand", "foreign dynamic operand", "dynamic result"} {
		t.Run(name, func(t *testing.T) {
			kind := types.JavaChar
			if name == "short" {
				kind = types.JavaShort
			}
			if name == "byte" {
				kind = types.JavaByte
			}
			if name == "boolean" {
				kind = types.JavaBoolean
			}
			typ := types.NewJavaPrimer(kind)
			old := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			old.Id.SetName("local")
			before, store, load, earlierLoad := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 3), op(OP_ILOAD_1, 4), op(OP_ILOAD_1, 2)
			earlier := values.NewSlotValue(old, typ)
			earlierLoad.stackProduced = []values.JavaValue{earlier}
			current := values.NewSlotValue(old, typ)
			load.stackProduced = []values.JavaValue{current}
			arm := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaChar))
			rhs := values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), arm, values.NewJavaLiteral(-1, types.NewJavaPrimer(types.JavaInteger)))
			store.stackConsumed = []values.JavaValue{rhs}
			before.stackConsumed = []values.JavaValue{arm}
			webs := &slotWeb{webOf: map[*OpCode]int{before: 0, earlierLoad: 0, store: 1, load: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{opCodes: []*OpCode{before, earlierLoad, store, load}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{before: {{old, true}}, store: {{old, false}}}}
			switch name {
			case "entry":
				webs.entryWeb[1] = 1
			case "multiple definitions":
				webs.webOf[before] = 1
			case "incomplete definitions":
				webs.webOf[op(OP_ISTORE_1, 5)] = 1
			case "missing operand":
				store.stackConsumed = nil
			case "missing load":
				load.stackProduced = nil
			case "foreign category":
				load.Instr = op(OP_ALOAD_1, 4).Instr
			case "increment":
				inc := op(OP_IINC, 5)
				inc.Data = []byte{1, 1}
				webs.webOf[inc] = 1
				d.opCodes = append(d.opCodes, inc)
			case "parameter":
				old.IsParam = true
			case "already solved":
				old.WebDeclType = typ
			case "handlers":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 3, HandlerPc: 4}}
			case "dynamic operand", "foreign dynamic operand", "dynamic result":
				seed := *old
				if name == "foreign dynamic operand" {
					seed.Id = utils.NewRootVariableId()
				}
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op(OP_INVOKEDYNAMIC, 6): {{Value: values.NewSlotValue(&seed, typ), Operand: name != "dynamic result"}}}
			case "narrow result":
				rhs.FalseValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			}
			d.restoreNumericConditionalWebs()
			got := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
			want := name == "char" || name == "short" || name == "byte" || name == "foreign dynamic operand" || name == "dynamic result"
			if (got != old) != want {
				t.Fatalf("rebound=%v want%v", got != old, want)
			}
			if old.Type().String(nil) != kind || earlier.GetValue() != old || d.opcodeIdToRef[before][0][0] != old {
				t.Fatal("unrelated earlier lifetime changed")
			}
			if want && (current.GetValue() != got || got.Type().String(nil) != types.JavaInteger || got.WebDeclType == nil || got.SolvedWebIdentity != got.Id || got.Id == old.Id) {
				t.Fatal("owned int declaration/load not sealed")
			}
		})
	}
}
