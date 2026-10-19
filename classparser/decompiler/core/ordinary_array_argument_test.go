package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A complete original packet has two initializer-only DUP uses and one eager
// invocation. The source declaration retains that allocation and its type.
func ordinaryArrayPacketFixture() (*Decompiler, *values.FunctionCallExpression, *values.JavaRef, *values.NewExpression, []*OpCode, *Node, *callbinding.Class) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	object := types.NewJavaClass("java.lang.Object")
	arrayType := types.NewJavaArrayType(object)
	length, zero, one := values.NewJavaLiteral(2, integer), values.NewJavaLiteral(0, integer), values.NewJavaLiteral(1, integer)
	first := values.NewJavaLiteral("first", types.NewJavaClass("java.lang.String"))
	second := values.NewJavaLiteral("second", types.NewJavaClass("java.lang.String"))
	array := values.NewNewArrayExpression(arrayType, length)
	array.OriginPC, array.HasOriginPC = 1, true
	array.EvaluationEndPC, array.HasEvaluationEndPC = 13, true
	array.Initializer = []values.JavaValue{first, second}
	ref := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType)
	call := &values.FunctionCallExpression{
		Object: values.NewJavaClassValue(types.NewJavaClass("sample.ArraySink")), ClassName: "sample.ArraySink", FunctionName: "consume",
		Descriptor: "([Ljava/lang/Object;)Ljava/lang/Object;", Kind: values.InvokeStatic, IsStatic: true,
		OriginPC: 14, HasOriginPC: true, Arguments: []values.JavaValue{ref},
		FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{arrayType}, ReturnType: object},
	}
	all := []*OpCode{op(OP_ANEWARRAY, 1), op(OP_DUP, 4), op(OP_ICONST_0, 5), op(OP_LDC, 6), op(OP_AASTORE, 8), op(OP_DUP, 9), op(OP_ICONST_1, 10), op(OP_LDC, 11), op(OP_AASTORE, 13), op(OP_INVOKESTATIC, 14)}
	for i := 0; i < len(all)-1; i++ {
		all[i].Target, all[i+1].Source = []*OpCode{all[i+1]}, []*OpCode{all[i]}
	}
	all[0].stackConsumed, all[0].stackProduced = []values.JavaValue{length}, []values.JavaValue{array}
	all[1].stackConsumed, all[1].stackProduced = []values.JavaValue{array}, []values.JavaValue{ref, ref}
	all[2].stackProduced, all[3].stackProduced = []values.JavaValue{zero}, []values.JavaValue{first}
	all[4].stackConsumed = []values.JavaValue{first, zero, ref}
	all[5].stackProduced = []values.JavaValue{ref}
	all[6].stackProduced, all[7].stackProduced = []values.JavaValue{one}, []values.JavaValue{second}
	all[8].stackConsumed = []values.JavaValue{second, one, ref}
	all[9].stackConsumed, all[9].stackProduced = []values.JavaValue{ref}, []values.JavaValue{call}
	metadata := &callbinding.Class{Name: "sample/ArraySink", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "consume", Desc: call.Descriptor, Static: true}}}
	ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) { return *metadata, name == metadata.Name }}
	start := &Node{Id: 0, Statement: statements.NewMiddleStatement("start", nil)}
	definition := &Node{Id: 4, Statement: statements.NewAssignStatement(ref, array, true)}
	consumer := &Node{Id: 14, Statement: statements.NewReturnStatement(call)}
	start.AddNext(definition)
	definition.AddNext(consumer)
	d := &Decompiler{RootNode: start, FunctionContext: ctx, opCodes: all, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{all[9]: call}, opcodeIdToRef: map[*OpCode][][2]any{all[1]: {{ref, true}}}, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}}
	for _, instruction := range all {
		d.opcodeToSimulateStack[instruction] = nil
	}
	return d, call, ref, array, all, definition, metadata
}

func TestOrdinaryStaticArraySourceViewRequiresOriginalFixedDeclaration(t *testing.T) {
	for _, scenario := range []string{"original", "missing metadata", "foreign metadata", "incomplete members", "generic declaration", "signature declaration", "bridge declaration", "instance declaration", "duplicate declaration", "missing declaration", "wrong invoke kind", "special call", "missing call origin", "foreign call identity", "wrong raw operand", "wrong formal count", "changed formal", "changed result", "non-array final formal", "long descriptor", "canceled", "memory", "work"} {
		t.Run(scenario, func(t *testing.T) {
			d, call, _, _, all, _, metadata := ordinaryArrayPacketFixture()
			switch scenario {
			case "missing metadata":
				d.FunctionContext.InvocationMetadata = nil
			case "foreign metadata":
				original := *metadata
				original.Name = "foreign/ArraySink"
				d.FunctionContext.InvocationMetadata = func(string) (callbinding.Class, bool) { return original, true }
			case "incomplete members":
				metadata.MembersComplete = false
			case "generic declaration":
				metadata.Methods[0].Generic = true
			case "signature declaration":
				metadata.Methods[0].Signature = "([TT;)TT;"
			case "bridge declaration":
				metadata.Methods[0].Bridge = true
			case "instance declaration":
				metadata.Methods[0].Static = false
			case "duplicate declaration":
				metadata.Methods = append(metadata.Methods, metadata.Methods[0])
			case "missing declaration":
				metadata.Methods = nil
			case "wrong invoke kind":
				all[9].Instr.OpCode = OP_INVOKEVIRTUAL
			case "special call":
				call.IsSpecialInvoke = true
			case "missing call origin":
				call.HasOriginPC = false
			case "foreign call identity":
				copy := *call
				copy.Descriptor = "([Ljava/lang/String;)Ljava/lang/Object;"
				d.invokeFuncCall[all[9]] = &copy
			case "wrong raw operand":
				all[9].stackConsumed = []values.JavaValue{values.JavaNull}
			case "wrong formal count":
				call.FuncType.ParamTypes = nil
			case "changed formal", "non-array final formal":
				call.FuncType.ParamTypes[0] = types.NewJavaClass("java.lang.Object")
				if scenario == "non-array final formal" {
					call.Descriptor, metadata.Methods[0].Desc = "(Ljava/lang/Object;)Ljava/lang/Object;", "(Ljava/lang/Object;)Ljava/lang/Object;"
				}
			case "changed result":
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.String")
			case "long descriptor":
				call.Descriptor = string(make([]byte, 8193))
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if got := d.ordinaryStaticArraySourceView(call, all[9]); got != (scenario == "original") {
				t.Fatalf("source view=%v", got)
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource classification")
			}
		})
	}
}

func TestOrdinaryStaticArrayTransferReopensWholeOriginalPacket(t *testing.T) {
	for _, scenario := range []string{"original", "definition at root", "jump predecessor", "metadata changed after discovery", "opaque reference after discovery", "inlined reference after discovery", "initializer changed", "store index changed", "extra original use", "local publication", "handler boundary", "alternate entry", "missing allocation origin", "missing initialization end", "missing DUP identity", "argument duplicated", "intervening source effect", "hidden original effect", "extra source use", "canceled", "work", "memory"} {
		t.Run(scenario, func(t *testing.T) {
			d, call, ref, array, all, definition, metadata := ordinaryArrayPacketFixture()
			d.RegisterNestedBranchArrayCalls()
			if len(d.branchArrayCalls) != 1 || !d.branchArrayCalls[0].requiresStaticSourceView {
				t.Fatal("original ordinary source call was not independently discovered")
			}
			switch scenario {
			case "definition at root":
				definition.RemoveAllSource()
				d.RootNode = definition
			case "jump predecessor":
				d.RootNode.JmpNode = definition
			case "metadata changed after discovery":
				metadata.Methods[0].Signature = "([TT;)TT;"
			case "opaque reference after discovery":
				ref.CustomValue = &values.CustomValue{}
			case "inlined reference after discovery":
				ref.StackVar = values.JavaNull
			case "initializer changed":
				array.Initializer[0] = values.JavaNull
			case "store index changed":
				all[4].stackConsumed[1] = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "extra original use", "local publication":
				extra := op(OP_INVOKESTATIC, 20)
				if scenario == "local publication" {
					extra.Instr.OpCode = OP_ASTORE
				}
				extra.stackConsumed = []values.JavaValue{ref}
				d.opCodes = append(d.opCodes, extra)
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 13, HandlerPc: 20}}
			case "alternate entry":
				all[4].Source = append(all[4].Source, op(OP_NOP, 0))
			case "missing allocation origin":
				array.HasOriginPC = false
			case "missing initialization end":
				array.HasEvaluationEndPC = false
			case "missing DUP identity":
				all[1].stackProduced = nil
			case "argument duplicated":
				call.Arguments = append(call.Arguments, ref)
			case "intervening source effect":
				next := definition.Next[0]
				definition.RemoveAllNext()
				barrier := &Node{Id: 50, Statement: statements.NewExpressionStatement(&values.FunctionCallExpression{IsStatic: true})}
				definition.AddNext(barrier)
				barrier.AddNext(next)
			case "hidden original effect":
				extra := op(OP_INVOKESTATIC, 12)
				extra.stackProduced = []values.JavaValue{&values.FunctionCallExpression{IsStatic: true, HasOriginPC: true, OriginPC: 12, FunctionName: "hidden"}}
				all[7].Target, extra.Source = []*OpCode{extra}, []*OpCode{all[7]}
				extra.Target, all[8].Source = []*OpCode{all[8]}, []*OpCode{extra}
				d.opCodes = append(d.opCodes, extra)
			case "extra source use":
				last := definition.Next[0]
				last.AddNext(&Node{Id: 21, Statement: statements.NewReturnStatement(ref)})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			got := d.InlinePrivateBranchArrayDefinitions()
			want := 0
			if scenario == "original" || scenario == "definition at root" || scenario == "jump predecessor" {
				want = 1
			}
			if got != want {
				t.Fatalf("transfer=%d want=%d", got, want)
			}
			if want == 0 && (values.UnpackSoltValue(call.Arguments[0]) != ref || len(definition.Source) == 0 || len(definition.Next) == 0) {
				t.Fatal("refused transfer changed source argument or detached its definition")
			}
			if want == 1 && (call.Arguments[0] != array || len(definition.Source) != 0 || len(definition.Next) != 0) {
				t.Fatal("proved transfer did not retain exactly one complete initializer at its consumer")
			}
			if want == 1 && (d.RootNode == definition || d.RootNode.JmpNode == definition) {
				t.Fatal("removed definition retained as a source entry or jump target")
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("lost resource classification")
			}
		})
	}
}
