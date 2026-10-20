package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestCompilerReferenceParameterCopyKeepsFactoryWithoutArrowNamePermission(t *testing.T) {
	for _, variant := range []string{"original", "no reference capture constraint", "not method reference", "unsealed source parameter", "wrong original slot", "later direct store", "unrelated store", "mutable local", "implicit receiver", "missing entry identity", "duplicate entry", "wrong expected type", "missing expected type", "wrong factory PC", "unsealed factory", "changed factory capture", "unsealed snapshot", "declared snapshot", "wrong position", "changed seed", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("ReferenceReceiver")
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			parameter.Id.SetName("original")
			parameter.IsParam = true
			if variant == "wrong original slot" {
				parameter.MarkOriginalParameter(1)
			} else if variant != "unsealed source parameter" {
				parameter.MarkOriginalParameter(0)
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), parameter, typ.Copy())
			snapshot.MarkOriginalDynamicOperand(10, 0, parameter)
			lambda := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("proof must not render lambda") }, func() types.JavaType { panic("proof must not query lambda type") })
			lambda.Flag, lambda.OriginPC, lambda.HasOriginPC, lambda.CapturesKnown = "lambda", 10, true, true
			lambda.Captures = []values.JavaValue{snapshot}
			lambda.IsMethodRef = variant != "not method reference"
			if variant != "no reference capture constraint" {
				lambda.RequireOriginalReferenceCapture()
			}
			if lambda.LambdaParameterNamesRequired() {
				t.Fatal("reference source borrowed arrow name permission")
			}
			if variant != "unsealed factory" {
				lambda.MarkOriginalLambdaFactory(10)
			}
			op := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{IsStatic: true}, Params: []values.JavaValue{parameter}, opCodes: []*OpCode{{Instr: &Instruction{OpCode: OP_NOP}}}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{op: {{Ref: snapshot, Value: parameter, Operand: true, ExpectedType: typ}}}}
			switch variant {
			case "later direct store":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_0}})
			case "wide interior store":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ISTORE_1}})
			case "wide interior iinc":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_IINC}, Data: []byte{1, 1}})
			case "unrelated store":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ISTORE_2}})
			case "mutable local":
				parameter.IsParam = false
			case "implicit receiver":
				parameter.IsThis = true
			case "missing entry identity":
				other := values.NewJavaRef(parameter.Id, nil, typ)
				other.IsParam = true
				snapshot.Val = other
			case "duplicate entry":
				d.Params = append(d.Params, parameter)
			case "wrong expected type":
				d.evaluationSnapshots[op][0].ExpectedType = types.NewJavaPrimer(types.JavaDouble)
			case "missing expected type":
				d.evaluationSnapshots[op][0].ExpectedType = nil
			case "wrong factory PC":
				op.CurrentOffset++
			case "changed factory capture":
				lambda.Captures[0] = parameter
			case "unsealed snapshot":
				other := values.NewJavaRef(snapshot.Id, parameter, typ)
				other.VarUid = snapshot.VarUid
				d.evaluationSnapshots[op][0].Ref = other
			case "declared snapshot":
				snapshot.MarkOriginalDynamicOperandDeclaration(10, parameter)
			case "wrong position":
				d.evaluationSnapshots[op][0].Ref = values.NewJavaRef(utils.NewRootVariableId(), parameter, typ)
				d.evaluationSnapshots[op][0].Ref.MarkOriginalDynamicOperand(10, 1, parameter)
			case "changed seed":
				snapshot.Val = values.NewJavaLiteral(int64(7), typ)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.inlineUnchangedLambdaParameterOperands(op, lambda)
			want := variant == "original" || variant == "unrelated store"
			pc, index, inlined := snapshot.OriginalInlinedDynamicOperandWitness(parameter)
			if inlined != want || want && (pc != 10 || index != 0 || len(d.evaluationSnapshots[op]) != 0) || !want && len(d.evaluationSnapshots[op]) != 1 {
				t.Fatalf("copy projection=%v, retained=%d", inlined, len(d.evaluationSnapshots[op]))
			}
			if !want && snapshot.StackVar != nil {
				t.Fatal("failed proof changed an operand")
			}
		})
	}
}
