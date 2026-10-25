package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestDescriptorIntJoinRequiresClosedOriginalNumericDefinitions(t *testing.T) {
	for _, name := range []string{"char", "byte", "short", "int", "boolean descriptor", "long descriptor", "reference descriptor", "missing descriptor", "malformed descriptor", "wrong origin", "no origin", "undecoded invoke", "wrong invoke", "entry", "missing store", "missing load", "wrong load", "wrong store", "increment other web", "boolean seed", "boolean annotated constant", "local seed", "parameter", "handler", "dynamic", "work cap", "allocation cap"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			a, b, load, invoke, foreign := op(OP_ISTORE_1, 1), op(OP_ISTORE_1, 3), op(OP_ILOAD_1, 4), op(OP_INVOKESTATIC, 2), op(OP_ISTORE_1, 5)
			x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			x.Id.SetName("word")
			y := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaChar))
			y.Id.SetName("arm")
			desc := "()C"
			switch name {
			case "byte":
				desc = "()B"
			case "short":
				desc = "()S"
			case "int":
				desc = "()I"
			case "boolean descriptor":
				desc = "()Z"
			case "long descriptor":
				desc = "()J"
			case "reference descriptor":
				desc = "()Ljava/lang/Object;"
			case "malformed descriptor":
				desc = "()Ctrailing"
			case "missing descriptor":
				desc = ""
			}
			call := &values.FunctionCallExpression{Descriptor: desc, HasOriginPC: true, OriginPC: 2, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaChar)}}
			a.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, typ)}
			b.stackConsumed = []values.JavaValue{call}
			view := values.NewSlotValue(x, typ)
			load.stackProduced = []values.JavaValue{view}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1, load: 1, foreign: 2}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: []*OpCode{a, invoke, b, load, foreign}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{a: {{x, true}}, b: {{y, true}}, foreign: {{x, true}}}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			switch name {
			case "wrong origin":
				call.OriginPC = 20
			case "no origin":
				call.HasOriginPC = false
			case "undecoded invoke":
				d.opCodes = []*OpCode{a, b, load, foreign}
			case "wrong invoke":
				invoke.Instr = op(OP_IADD, 2).Instr
			case "entry":
				webs.entryWeb[1] = 1
			case "missing store":
				webs.webOf[op(OP_ISTORE_1, 6)] = 1
			case "missing load":
				load.stackProduced = nil
			case "wrong load":
				load.Instr = op(OP_ALOAD_1, 4).Instr
			case "wrong store":
				b.Instr = op(OP_ASTORE_1, 3).Instr
			case "increment other web":
				inc := op(OP_IINC, 6)
				inc.Data = []byte{1, 1}
				d.opCodes = append(d.opCodes, inc)
				webs.webOf[inc] = 3
			case "boolean annotated constant":
				a.stackConsumed[0] = values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaBoolean))
			case "boolean seed":
				a.stackConsumed[0] = values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean))
			case "local seed":
				a.stackConsumed[0] = x
			case "parameter":
				x.IsParam = true
			case "handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 5}}
			case "dynamic":
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{invoke: {{Value: x, Operand: true}}}
			case "work cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation cap":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			d.unifyDescriptorIntJoins()
			got := d.opcodeIdToRef[a][0][0].(*values.JavaRef)
			want := name == "boolean annotated constant" || name == "char" || name == "byte" || name == "short" || name == "int"
			if (got != x) != want {
				t.Fatalf("joined=%v want=%v", got != x, want)
			}
			if want && (d.opcodeIdToRef[b][0][0] != got || view.GetValue() != got || got.SolvedWebIdentity != got.Id || got.WebDeclType == nil || !isExactPrimer(got.Type(), types.JavaInteger)) {
				t.Fatal("joined declaration and reads disagree")
			}
			if d.opcodeIdToRef[foreign][0][0] != x || b.stackConsumed[0] != call || call.Descriptor != desc {
				t.Fatal("changed unrelated lifetime or invocation")
			}
		})
	}
}
