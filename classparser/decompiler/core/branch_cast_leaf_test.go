package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBranchCallCastLeafProof(t *testing.T) {
	for _, kind := range []string{"valid", "handler boundary", "call before arm", "other effect", "alternate entry", "different producer", "unmatched witness"} {
		t.Run(kind, func(t *testing.T) {
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 10}
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 13}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 16}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 20}
			invoke.Target, check.Source = []*OpCode{check}, []*OpCode{invoke}
			check.Target, jump.Source = []*OpCode{jump}, []*OpCode{check}
			jump.Target, merge.Source = []*OpCode{merge}, []*OpCode{jump}
			call := &values.FunctionCallExpression{OriginPC: 10, Descriptor: "()Ljava/lang/Object;"}
			cast := &values.CastExpression{Value: call, TargetType: types.NewJavaClass("java.lang.String"), OriginPC: 13}
			ref := values.NewJavaRef(utils.NewRootVariableId(), cast, cast.Type())
			check.stackProduced = []values.JavaValue{ref}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{invoke: nil, check: nil, jump: nil, merge: nil}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			entry := invoke
			switch kind {
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20, HandlerPc: 30}}
			case "call before arm":
				entry = check
			case "other effect":
				call.Arguments = []values.JavaValue{&values.FunctionCallExpression{}}
			case "alternate entry":
				check.Source = append(check.Source, &OpCode{})
			case "different producer":
				check.stackProduced = nil
			case "unmatched witness":
				d.invokeFuncCall[invoke] = &values.FunctionCallExpression{}
			}
			got := d.branchCallCastLeaf(ref, cast, entry, jump, merge)
			if (got == check) != (kind == "valid") {
				t.Fatalf("proof accepted=%t", got != nil)
			}
		})
	}
}
