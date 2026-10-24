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

func TestLambdaLexicalFieldCopyElisionKeepsEagerFactoryAndOtherOperands(t *testing.T) {
	for _, descriptor := range []string{"J", "D", "Ljava/lang/Object;"} {
		t.Run(descriptor, func(t *testing.T) {
			for _, variant := range []string{"original", "initialized constructor", "uninitialized constructor", "ordinary field without source port", "no body name constraint", "missing source identity", "missing factory", "wrong factory", "wrong operand", "changed seed", "changed field", "foreign receiver", "receiver store", "duplicate receiver", "foreign owner", "static context", "wrong expected type", "changed field and expected type", "missing expected type", "already declared", "neighbor effect retained", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					typ, err := types.ParseDescriptor(descriptor)
					if err != nil {
						t.Fatal(err)
					}
					receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("fixture.CapturedOwner"))
					receiver.IsParam, receiver.IsThis = true, true
					receiver.MarkOriginalParameter(0)
					receiver.MarkOriginalReceiver()
					field := values.NewRefMember(receiver, "captured", typ)
					field.OriginPC, field.HasOriginPC = 2, true
					field.MarkOriginalFieldRead(values.NewJavaClassMember("fixture.CapturedOwner", "captured", descriptor, typ), 2)
					snapshot := values.NewJavaRef(utils.NewRootVariableId(), field, typ.Copy())
					snapshot.MarkOriginalDynamicOperand(10, 0, field)
					port := values.NewLambdaLexicalFieldCapture(snapshot, "fixture.CapturedOwner", "captured", descriptor, 2)
					if port == nil {
						t.Fatal("original frontend field port")
					}
					lambda := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("proof rendered deferred body") }, func() types.JavaType { panic("proof queried deferred type") })
					lambda.Flag, lambda.OriginPC, lambda.HasOriginPC, lambda.CapturesKnown = "lambda", 10, true, true
					lambda.Captures = []values.JavaValue{snapshot}
					if variant != "no body name constraint" {
						lambda.RequireLambdaParameterNames()
					}
					if variant != "missing source identity" {
						lambda.BindLambdaSourceIdentity(values.NewLambdaSourceIdentity())
					}
					if variant != "ordinary field without source port" {
						bound := lambda.BindLambdaLexicalFieldCaptures([]*values.LambdaLexicalFieldCapture{port})
						if bound != (variant != "no body name constraint" && variant != "missing source identity") {
							t.Fatal("source role bound without its original declaration")
						}
					}
					op := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}}
					factory := values.NewJavaRef(utils.NewRootVariableId(), lambda, types.NewJavaClass("java.util.function.Supplier"))
					d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "fixture.CapturedOwner", FunctionName: "factory"}, Params: []values.JavaValue{receiver}, opCodes: []*OpCode{{Instr: &Instruction{OpCode: OP_NOP}}}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{op: {{Ref: snapshot, Value: field, Operand: true, ExpectedType: typ}, {Ref: factory, Value: lambda}}}}
					if variant == "neighbor effect retained" {
						call := &values.FunctionCallExpression{ClassName: "fixture.Effects", FunctionName: "next", Descriptor: "()Ljava/lang/Object;", IsStatic: true, Kind: values.InvokeStatic, FuncType: types.NewJavaFuncType("()Ljava/lang/Object;", nil, types.NewJavaClass("java.lang.Object"))}
						other := values.NewJavaRef(utils.NewRootVariableId(), call, types.NewJavaClass("java.lang.Object"))
						other.MarkOriginalDynamicOperand(10, 1, call)
						lambda.Captures = append(lambda.Captures, other)
						d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: other, Value: call, Operand: true, ExpectedType: other.Type()})
					}
					if variant != "missing factory" {
						lambda.MarkOriginalLambdaFactory(10)
					}
					switch variant {
					case "initialized constructor", "uninitialized constructor":
						d.FunctionContext.FunctionName = "<init>"
						d.constructorInitialized = variant == "initialized constructor"
					case "wrong factory":
						op.CurrentOffset++
					case "wrong operand":
						lambda.Captures[0] = receiver
					case "changed seed":
						snapshot.Val = values.NewSlotValue(field, typ)
					case "changed field":
						field.Member = "other"
					case "foreign receiver":
						field.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type())
					case "receiver store":
						d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_0}})
					case "duplicate receiver":
						d.Params = append(d.Params, receiver)
					case "foreign owner":
						d.FunctionContext.ClassName = "fixture.Other"
					case "static context":
						d.FunctionContext.IsStatic = true
					case "wrong expected type":
						d.evaluationSnapshots[op][0].ExpectedType = types.NewJavaPrimer(types.JavaInteger)
					case "changed field and expected type":
						field.JavaType = types.NewJavaPrimer(types.JavaInteger)
						snapshot.ResetVarType(field.JavaType)
						d.evaluationSnapshots[op][0].ExpectedType = field.JavaType
					case "missing expected type":
						d.evaluationSnapshots[op][0].ExpectedType = nil
					case "already declared":
						snapshot.MarkOriginalDynamicOperandDeclaration(10, field)
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
					want := variant == "original" || variant == "initialized constructor" || variant == "neighbor effect retained"
					_, _, inlined := snapshot.OriginalInlinedDynamicOperandWitness(field)
					if inlined != want || !want && snapshot.StackVar != nil {
						t.Fatalf("lexical capture inlined=%v want=%v", inlined, want)
					}
					keptFactory, keptOperand, keptOther := false, false, false
					for _, retained := range d.evaluationSnapshots[op] {
						keptFactory = keptFactory || retained.Ref == factory
						keptOperand = keptOperand || retained.Ref == snapshot
						keptOther = keptOther || retained.Operand && retained.Ref != snapshot
					}
					if !keptFactory || keptOperand == want || variant == "neighbor effect retained" && !keptOther {
						t.Fatal("eager factory or unrelated operand changed")
					}
				})
			}
		})
	}
}
