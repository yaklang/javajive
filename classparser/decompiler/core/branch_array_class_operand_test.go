package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDroppedBranchArrayKeepsEarlierClassResolutionAndHandlerOrder(t *testing.T) {
	for _, receiver := range []bool{false, true} {
		for _, change := range []string{"same domain", "unprotected", "handler boundary", "alternate entry", "branch", "later producer", "different literal", "duplicate producer", "another array use", "live definition"} {
			t.Run(map[bool]string{false: "argument", true: "receiver"}[receiver]+"/"+change, func(t *testing.T) {
				class := values.NewJavaClassValue(types.NewJavaClass("sample.Uninitialized"))
				class.OriginPC, class.HasOriginPC = 1, true
				arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
				array := values.NewNewArrayExpression(arrayType, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
				array.OriginPC, array.HasOriginPC, array.EvaluationEndPC, array.HasEvaluationEndPC = 3, true, 6, true
				array.Initializer = []values.JavaValue{values.JavaNull}
				ref := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType)
				call := &values.FunctionCallExpression{ClassName: "sample.Consumer", Object: values.NewJavaClassValue(types.NewJavaClass("sample.Consumer")), FunctionName: "consume", Descriptor: "(Ljava/lang/Class;[Ljava/lang/Object;)Ljava/lang/Object;", IsStatic: true, Kind: values.InvokeStatic, OriginPC: 7, HasOriginPC: true, Arguments: []values.JavaValue{class, ref}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{types.NewJavaClass("java.lang.Class"), arrayType}, ReturnType: types.NewJavaClass("java.lang.Object")}}
				arg := 1
				if receiver {
					call.IsStatic, call.Kind, call.Object = false, values.InvokeVirtual, class
					call.Arguments, call.FuncType.ParamTypes, arg = []values.JavaValue{ref}, []types.JavaType{arrayType}, 0
					call.Descriptor = "([Ljava/lang/Object;)Ljava/lang/Object;"
				}
				producer, size, alloc, dup, index, store, invoke := op(OP_LDC, 1), op(OP_ICONST_1, 2), op(OP_ANEWARRAY, 3), op(OP_DUP, 4), op(OP_ICONST_0, 5), op(OP_AASTORE, 6), op(OP_INVOKESTATIC, 7)
				ops := []*OpCode{producer, size, alloc, dup, index, store, invoke}
				for i := 0; i+1 < len(ops); i++ {
					ops[i].Target, ops[i+1].Source = []*OpCode{ops[i+1]}, []*OpCode{ops[i]}
				}
				producer.stackProduced = []values.JavaValue{class}
				store.stackConsumed = []values.JavaValue{values.JavaNull, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), ref}
				invoke.stackConsumed = []values.JavaValue{ref}
				d := &Decompiler{opCodes: ops, opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, FunctionContext: &class_context.ClassContext{}, branchArrayCalls: []branchArrayCall{{call: call, argIndex: arg, ref: ref, array: array}}, ExceptionTable: []*ExceptionTableEntry{{StartPc: 1, EndPc: 8, HandlerPc: 10, CatchType: 1}}}
				for _, opcode := range ops {
					d.opcodeToSimulateStack[opcode] = nil
				}
				body := []statements.Statement{&statements.ReturnStatement{JavaValue: call}}
				switch change {
				case "unprotected":
					d.ExceptionTable = nil
				case "handler boundary":
					d.ExceptionTable[0].StartPc = 2
				case "alternate entry":
					size.Source = append(size.Source, op(OP_NOP, 0))
				case "branch":
					producer.Target = append(producer.Target, op(OP_RETURN, 11))
				case "later producer":
					producer.CurrentOffset = 9
				case "different literal":
					producer.stackProduced[0] = values.NewJavaClassValue(class.JavaType)
				case "duplicate producer":
					size.stackProduced = []values.JavaValue{class}
				case "another array use":
					body = append(body, &statements.ReturnStatement{JavaValue: ref})
				case "live definition":
					body = append([]statements.Statement{statements.NewAssignStatement(ref, array, true)}, body...)
				}
				want := change == "same domain" || change == "unprotected"
				if changed := d.InlineDroppedBranchArrayCalls(body); (changed == 1) != want {
					t.Fatalf("array ownership transferred=%d, want %v", changed, want)
				}
				if (call.Arguments[arg] == array) != want {
					t.Fatal("a refused transfer changed its invocation operand")
				}
				if effect, _ := values.InspectValue(class); effect != values.EffectThrow {
					t.Fatal("the Class resolution must remain throwing without class initialization", effect)
				}
			})
		}
	}
}
