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

func TestLambdaParameterCopyElisionKeepsFactoryAndRejectsMutableWords(t *testing.T) {
	for _, variant := range []string{"original", "no body name constraint", "unsealed source parameter", "wrong original slot", "later direct store", "wide interior store", "wide interior iinc", "unrelated store", "mutable local", "implicit receiver", "missing entry identity", "duplicate entry", "wrong expected type", "missing expected type", "wrong factory PC", "unsealed factory", "changed factory capture", "unsealed snapshot", "declared snapshot", "wrong position", "changed seed", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
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
			if variant != "no body name constraint" {
				lambda.RequireLambdaParameterNames()
			}
			if variant != "unsealed factory" {
				lambda.MarkOriginalLambdaFactory(10)
			}
			op := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, Params: []values.JavaValue{parameter}, opCodes: []*OpCode{{Instr: &Instruction{OpCode: OP_NOP}}}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{op: {{Ref: snapshot, Value: parameter, Operand: true, ExpectedType: typ}}}}
			switch variant {
			case "later direct store":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_LSTORE_0}})
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

func TestLambdaReceiverCopyElisionRequiresInitializedOriginalEntry(t *testing.T) {
	for _, variant := range []string{"original", "initialized constructor", "uninitialized constructor", "static context", "forged receiver", "wrong owner", "wrong ordinal", "duplicate receiver", "receiver store", "wrong expected type", "missing factory", "wrong factory", "declared snapshot", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("fixture.Receiver")
			receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			receiver.IsParam, receiver.IsThis = true, true
			receiver.MarkOriginalParameter(0)
			if variant != "forged receiver" {
				receiver.MarkOriginalReceiver()
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), receiver, typ.Copy())
			ordinal := 0
			if variant == "wrong ordinal" {
				ordinal = 1
			}
			snapshot.MarkOriginalDynamicOperand(10, ordinal, receiver)
			lambda := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("proof rendered lambda") }, func() types.JavaType { panic("proof queried lambda type") })
			lambda.Flag, lambda.OriginPC, lambda.HasOriginPC, lambda.CapturesKnown = "lambda", 10, true, true
			lambda.Captures = []values.JavaValue{snapshot}
			if ordinal == 1 {
				lambda.Captures = []values.JavaValue{receiver, snapshot}
			}
			lambda.RequireLambdaParameterNames()
			if variant != "missing factory" {
				lambda.MarkOriginalLambdaFactory(10)
			}
			op := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "fixture.Receiver", FunctionName: "factory"}, Params: []values.JavaValue{receiver}, opCodes: []*OpCode{{Instr: &Instruction{OpCode: OP_NOP}}}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{op: {{Ref: snapshot, Value: receiver, Operand: true, ExpectedType: typ}}}}
			switch variant {
			case "initialized constructor", "uninitialized constructor":
				d.FunctionContext.FunctionName = "<init>"
				d.constructorInitialized = variant == "initialized constructor"
			case "static context":
				d.FunctionContext.IsStatic = true
			case "wrong owner":
				d.FunctionContext.ClassName = "fixture.Other"
			case "duplicate receiver":
				d.Params = append(d.Params, receiver)
			case "receiver store":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_0}})
			case "wrong expected type":
				d.evaluationSnapshots[op][0].ExpectedType = types.NewJavaClass("fixture.Other")
			case "wrong factory":
				op.CurrentOffset++
			case "declared snapshot":
				snapshot.MarkOriginalDynamicOperandDeclaration(10, receiver)
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
			pc, index, inlined := snapshot.OriginalInlinedDynamicOperandWitness(receiver)
			want := variant == "original" || variant == "initialized constructor"
			if inlined != want || want && (pc != 10 || index != 0 || len(d.evaluationSnapshots[op]) != 0) || !want && (snapshot.StackVar != nil || len(d.evaluationSnapshots[op]) != 1) {
				t.Fatalf("implicit receiver inlined=%v retained=%d", inlined, len(d.evaluationSnapshots[op]))
			}
		})
	}
}
