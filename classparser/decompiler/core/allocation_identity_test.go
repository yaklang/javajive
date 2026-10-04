package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestInitializedAllocationDupRequiresConstructorIdentity(t *testing.T) {
	for _, scenario := range []string{"initialized", "uninitialized", "other allocation", "ordinary call", "later constructor", "ambiguous constructor", "alternate entry", "handler boundary"} {
		t.Run(scenario, func(t *testing.T) {
			allocation := values.NewNewExpression(types.NewJavaClass("example.Mutable"))
			init := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 4}
			dup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 7}
			init.Target, dup.Source = []*OpCode{dup}, []*OpCode{init}
			call := &values.FunctionCallExpression{Object: allocation, FunctionName: "<init>"}
			// Stack simulation has not yet constructed opcodeToSimulateStack.
			d := &Decompiler{invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{init: call}}
			switch scenario {
			case "uninitialized":
				delete(d.invokeFuncCall, init)
			case "other allocation":
				call.Object = values.NewNewExpression(types.NewJavaClass("example.Mutable"))
			case "ordinary call":
				call.FunctionName = "mutate"
			case "later constructor":
				init.CurrentOffset = 10
			case "ambiguous constructor":
				d.invokeFuncCall[&OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}}] = call
			case "alternate entry":
				dup.Source = append(dup.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 4, EndPc: 7, HandlerPc: 20}}
			}
			if got := d.isInitializedAllocationAtDup(allocation, dup); got != (scenario == "initialized") {
				t.Fatalf("initialized=%v", got)
			}
		})
	}
}
