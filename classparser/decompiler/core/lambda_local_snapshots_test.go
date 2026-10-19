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

func TestLambdaLocalCopyElisionRequiresOneOriginalDominatingStableStore(t *testing.T) {
	for _, descriptor := range []string{"I", "J", "F", "D", "Z", "B", "C", "S", "Ljava/lang/Object;", "Ljava/lang/String;", "[I"} {
		t.Run(descriptor, func(t *testing.T) {
			for _, change := range []string{"original", "late source type mutation", "late source staticness mutation", "no source constraint", "no local witness", "wrong store PC", "wrong slot", "wrong store category", "changed seed", "later write", "unreachable write", "normal bypass", "exception bypass", "entry overlap", "missing graph", "missing function type", "wrong expected type", "missing factory", "changed capture", "declared snapshot", "wrong snapshot ordinal", "custom store", "custom factory", "work", "memory", "canceled"} {
				t.Run(change, func(t *testing.T) {
					methodType, err := types.ParseMethodDescriptor("()" + descriptor)
					if err != nil {
						t.Fatal(err)
					}
					typ := methodType.FunctionType().ReturnType
					constant, store, load := byte(OP_ICONST_0), byte(OP_ISTORE_0), byte(OP_ILOAD_0)
					switch descriptor[0] {
					case 'L', '[':
						constant, store, load = OP_ACONST_NULL, OP_ASTORE_0, OP_ALOAD_0
					case 'J':
						constant, store, load = OP_LCONST_0, OP_LSTORE_0, OP_LLOAD_0
					case 'F':
						constant, store, load = OP_FCONST_0, OP_FSTORE_0, OP_FLOAD_0
					case 'D':
						constant, store, load = OP_DCONST_0, OP_DSTORE_0, OP_DLOAD_0
					}
					code := []byte{constant, store, load, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_POP, OP_RETURN}
					storePC, factoryPC := 1, 3
					if change == "normal bypass" {
						code = append([]byte{OP_ICONST_0, OP_IFEQ, 0, 6}, code...)
						storePC, factoryPC = 5, 7
					}
					if change == "later write" {
						code = append(code[:9], constant, store, OP_RETURN)
					}
					if change == "unreachable write" {
						code = append(code, constant, store, OP_RETURN)
					}
					d := auditCFG(t, code)
					d.FunctionContext = &class_context.ClassContext{IsStatic: true}
					d.FunctionType = methodType.FunctionType()
					if change == "entry overlap" {
						function, _ := types.ParseMethodDescriptor("(J)V")
						d.FunctionType = function.FunctionType()
					}
					if change == "exception bypass" {
						d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 2, HandlerPc: uint16(factoryPC), CatchType: 1}}
						// LDC may throw before the original local is initialized.
						d.opCodes[0].Instr = &Instruction{OpCode: OP_LDC}
						d.opCodes[0].Data = []byte{1}
					}
					g, err := d.buildSemanticCFG()
					if err != nil {
						t.Fatal(err)
					}
					d.semanticCFG = g
					factory := d.opCodes[d.offsetToOpcodeIndex[uint16(factoryPC)]]
					definition := d.opCodes[d.offsetToOpcodeIndex[uint16(storePC)]]
					seed := values.NewJavaLiteral(nil, typ)
					local := values.NewJavaRef(utils.NewRootVariableId(), seed, typ)
					if change != "no local witness" {
						pc, slot := storePC, 0
						if change == "wrong store PC" {
							pc++
						}
						if change == "wrong slot" {
							slot++
						}
						local.MarkOriginalLocalDeclaration(pc, slot, seed)
					}
					snapshot := values.NewJavaRef(utils.NewRootVariableId(), local, typ.Copy())
					ordinal := 0
					if change == "wrong snapshot ordinal" {
						ordinal = 1
					}
					snapshot.MarkOriginalDynamicOperand(factoryPC, ordinal, local)
					lambda := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("proof rendered a lambda") }, func() types.JavaType { panic("proof queried the factory type") })
					lambda.Flag, lambda.HasOriginPC, lambda.OriginPC, lambda.CapturesKnown = "lambda", true, factoryPC, true
					lambda.Captures = []values.JavaValue{snapshot}
					if change != "no source constraint" {
						lambda.RequireLambdaParameterNames()
					}
					if change != "missing factory" {
						lambda.MarkOriginalLambdaFactory(factoryPC)
					}
					result := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.Supplier"))
					d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{factory: {{Ref: snapshot, Value: local, Operand: true, ExpectedType: typ}, {Ref: result, Value: lambda, OriginPC: factoryPC}}}
					switch change {
					case "late source type mutation":
						changed, _ := types.ParseMethodDescriptor("(JJJ)V")
						d.FunctionType = changed.FunctionType()
					case "late source staticness mutation":
						d.FunctionContext.IsStatic = false
					case "wrong store category":
						definition.Instr = &Instruction{OpCode: OP_FSTORE_0}
						if descriptor == "F" {
							definition.Instr.OpCode = OP_ASTORE_0
						}
					case "changed seed":
						local.Val = values.NewJavaLiteral(nil, typ)
					case "missing graph":
						d.semanticCFG = nil
					case "missing function type":
						d.FunctionType = nil
					case "wrong expected type":
						d.evaluationSnapshots[factory][0].ExpectedType = types.NewJavaClass("different.Target")
					case "changed capture":
						lambda.Captures[0] = local
					case "declared snapshot":
						snapshot.MarkOriginalDynamicOperandDeclaration(factoryPC, local)
					case "custom store":
						definition.IsCustom = true
					case "custom factory":
						factory.IsCustom = true
					case "work":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					d.inlineUnchangedLambdaLocalOperands(factory, lambda)
					pc, index, inlined := snapshot.OriginalInlinedDynamicOperandWitness(local)
					want := change == "original" || change == "late source type mutation" || change == "late source staticness mutation"
					if inlined != want || want && (pc != factoryPC || index != 0 || snapshot.StackVar != local) {
						t.Fatalf("projection=%v PC=%d ordinal=%d want=%v", inlined, pc, index, want)
					}
					retained := d.evaluationSnapshots[factory]
					expectedCount := 2
					if want {
						expectedCount = 1
					}
					if len(retained) != expectedCount || retained[len(retained)-1].Ref != result || retained[len(retained)-1].Value != lambda || retained[len(retained)-1].OriginPC != factoryPC || !want && snapshot.StackVar != nil {
						t.Fatal("failed proof changed an operand, or factory/result moved")
					}
				})
			}
		})
	}
}
