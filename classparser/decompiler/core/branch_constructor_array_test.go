package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBranchConstructorArrayPrivateStackProof(t *testing.T) {
	for _, change := range []string{"valid", "effectful element", "partial fill", "bad index", "different element", "escape", "alternate entry", "handler boundary", "later effect", "earlier effect", "missing invoke witness", "wrong descriptor", "foreign receiver", "wrong owner", "wrong origin", "wrong length", "wrong formal", "wrong descriptor array", "hidden effect", "duplicate mismatch"} {
		t.Run(change, func(t *testing.T) {
			obj := values.NewNewExpression(types.NewJavaClass("example.Failure"))
			obj.OriginPC, obj.HasOriginPC = 1, true
			cause := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			payload := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			lit := func(n int) values.JavaValue { return values.NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger)) }
			array := values.NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")), lit(1))
			array.OriginPC, array.HasOriginPC = 7, true
			array.EvaluationEndPC, array.HasEvaluationEndPC = 11, true
			array.Initializer = []values.JavaValue{payload}
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, array.Type())
			call := &values.FunctionCallExpression{Object: obj, Arguments: []values.JavaValue{cause, ref}, IsSpecialInvoke: true, FunctionName: "<init>", ClassName: "example.Failure", OriginPC: 12, Descriptor: "(Ljava/lang/Throwable;[Ljava/lang/Object;)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{cause.Type(), array.Type()}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}}
			newOp := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 1, stackProduced: []values.JavaValue{obj}}
			ctorDup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 2}
			loadCause := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_1}, CurrentOffset: 4, stackProduced: []values.JavaValue{cause}}
			length := &OpCode{Instr: &Instruction{OpCode: OP_ICONST_1}, CurrentOffset: 6, stackProduced: array.Length}
			alloc := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 7, stackConsumed: array.Length, stackProduced: []values.JavaValue{array}}
			dup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 8, stackConsumed: []values.JavaValue{array}, stackProduced: []values.JavaValue{ref, ref}}
			index := lit(0)
			idx := &OpCode{Instr: &Instruction{OpCode: OP_ICONST_0}, CurrentOffset: 9, stackProduced: []values.JavaValue{index}}
			item := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_2}, CurrentOffset: 10, stackProduced: []values.JavaValue{payload}}
			store := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 11, stackConsumed: []values.JavaValue{payload, index, ref}}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 12, stackConsumed: []values.JavaValue{ref, cause, obj}}
			path := []*OpCode{newOp, ctorDup, loadCause, length, alloc, dup, idx, item, store, invoke}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			want := false
			switch change {
			case "valid":
				want = true
			case "effectful element":
				f := &values.FunctionCallExpression{IsStatic: true, OriginPC: 10, Descriptor: "()Ljava/lang/Object;"}
				item.Instr.OpCode = OP_INVOKESTATIC
				item.stackProduced = []values.JavaValue{f}
				store.stackConsumed[0], array.Initializer[0], d.invokeFuncCall[item] = f, f, f
				want = true
			case "partial fill":
				array.HasEvaluationEndPC = false
			case "bad index":
				index.(*values.JavaLiteral).Data = 1
			case "different element":
				array.Initializer[0] = values.JavaNull
			case "escape":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_3}, stackConsumed: []values.JavaValue{ref}})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 7, EndPc: 11, HandlerPc: 30}}
			case "later effect":
				path = append(path[:len(path)-1], &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 12}, invoke)
				invoke.CurrentOffset, call.OriginPC = 13, 13
			case "earlier effect":
				call.Arguments[0] = &values.FunctionCallExpression{}
			case "missing invoke witness":
				delete(d.invokeFuncCall, invoke)
			case "wrong descriptor":
				call.Descriptor = "()Ljava/lang/Object;"
			case "foreign receiver":
				call.Object = cause
			case "wrong owner":
				call.ClassName = "example.OtherFailure"
			case "wrong origin":
				obj.OriginPC = 4
			case "wrong length":
				array.Length = []values.JavaValue{lit(2)}
			case "wrong formal":
				call.FuncType.ParamTypes[1] = types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			case "wrong descriptor array":
				call.Descriptor = "(Ljava/lang/Throwable;[Ljava/lang/String;)V"
			case "hidden effect":
				item.Instr.OpCode = OP_POP
			case "duplicate mismatch":
				dup.stackProduced[1] = values.JavaNull
			}
			for i, op := range path {
				d.opCodes = append(d.opCodes, op)
				d.opcodeToSimulateStack[op] = nil
				if i+1 < len(path) {
					op.Target, path[i+1].Source = []*OpCode{path[i+1]}, []*OpCode{op}
				}
			}
			if change == "alternate entry" {
				store.Source = append(store.Source, &OpCode{})
			}
			if got := d.branchConstructorArrayIsolated(obj, call, ref, array); got != want {
				t.Fatalf("accepted=%t, want=%t", got, want)
			}
		})
	}
}
