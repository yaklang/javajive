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

func TestBranchArrayLengthUsesOwnedStackAndOrigin(t *testing.T) {
	for _, kind := range []string{"owned prefix", "same protected range", "wrong origin", "absent origin", "different same-named operand", "wrong consumed operand", "opaque length", "materialized length", "missing consumed operand", "extra produced operand", "unowned first argument", "handler boundary", "alternate entry", "dup", "store"} {
		t.Run(kind, func(t *testing.T) {
			arrayType := types.NewJavaArrayType(types.NewJavaClass("Token"))
			array := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			array.Id.SetName("var0")
			other := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
			other.Id.SetName("var0")
			length := &values.ArrayLengthExpression{Array: array, OriginPC: 12, HasOriginPC: true}
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 10, stackProduced: []values.JavaValue{array}}
			loadAgain := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 11, stackProduced: []values.JavaValue{array}}
			readLength := &OpCode{Instr: &Instruction{OpCode: OP_ARRAYLENGTH}, CurrentOffset: 12, stackConsumed: []values.JavaValue{array}, stackProduced: []values.JavaValue{length}}
			call := &values.FunctionCallExpression{IsStatic: true, Arguments: []values.JavaValue{array, length}, OriginPC: 13, Descriptor: "([Ljava/lang/Object;I)[Ljava/lang/Object;"}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 13, stackConsumed: []values.JavaValue{length, array}, stackProduced: []values.JavaValue{call}}
			cast := &values.CastExpression{Value: call, TargetType: arrayType, OriginPC: 17}
			result := values.NewJavaRef(utils.NewRootVariableId(), cast, arrayType)
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 17, stackConsumed: []values.JavaValue{call}, stackProduced: []values.JavaValue{result}}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 20}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ARETURN}, CurrentOffset: 25}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}, checkcastInnerArg: map[*OpCode]values.JavaValue{check: call}}
			want := false
			path := []*OpCode{load, loadAgain, readLength, invoke, check, jump, merge}
			switch kind {
			case "owned prefix":
				want = true
			case "same protected range":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 26, HandlerPc: 50}}
				want = true
			case "wrong origin":
				length.OriginPC = 11
			case "absent origin":
				length.HasOriginPC = false
			case "different same-named operand":
				length.Array = other
			case "wrong consumed operand":
				readLength.stackConsumed[0] = other
			case "opaque length", "materialized length":
				var hidden values.JavaValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "var0.length" }, length.Type)
				if kind == "materialized length" {
					hidden = values.NewJavaRef(utils.NewRootVariableId(), length, length.Type())
				}
				readLength.stackProduced[0], call.Arguments[1], invoke.stackConsumed[0] = hidden, hidden, hidden
			case "missing consumed operand":
				readLength.stackConsumed = nil
			case "extra produced operand":
				readLength.stackProduced = append(readLength.stackProduced, array)
			case "unowned first argument":
				path = path[1:]
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 12, EndPc: 13, HandlerPc: 50}}
			case "dup":
				loadAgain.Instr.OpCode = OP_DUP
			case "store":
				loadAgain.Instr.OpCode = OP_ASTORE_1
			}
			for i, op := range path {
				d.opcodeToSimulateStack[op] = nil
				if i+1 < len(path) {
					op.Target, path[i+1].Source = []*OpCode{path[i+1]}, []*OpCode{op}
				}
			}
			if kind == "alternate entry" {
				readLength.Source = append(readLength.Source, &OpCode{})
			}
			if got := d.branchExpressionCastLeaf(result, cast, path[0], jump, merge); (got == check) != want {
				t.Fatalf("accepted=%v, want=%v", got != nil, want)
			}
		})
	}
}

func TestProtectedTerminalValueMergePreservesHandlerDomain(t *testing.T) {
	for _, kind := range []string{"protected return", "synthetic method exit", "retained enclosing handler", "selector outside try", "return after boundary", "new return handler", "foreign arm handler", "external region entrance", "external return entrance", "local store", "field store", "throw", "back edge", "nonterminal return", "exit with successor", "wrong consumer", "cancelled proof"} {
		t.Run(kind, func(t *testing.T) {
			selector := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 9}
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 10}
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 17}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 20}
			empty := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 24}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ARETURN}, CurrentOffset: 25}
			connect := func(from *OpCode, to ...*OpCode) {
				from.Target = to
				for _, next := range to {
					next.Source = append(next.Source, from)
				}
			}
			connect(selector, load, empty)
			connect(load, check)
			connect(check, jump)
			connect(jump, merge)
			connect(empty, merge)
			d := &Decompiler{ExceptionTable: []*ExceptionTableEntry{{StartPc: 9, EndPc: 25, HandlerPc: 50, CatchType: 1}}}
			want := false
			switch kind {
			case "protected return":
				want = true
			case "synthetic method exit", "exit with successor":
				exit := &OpCode{Instr: &Instruction{OpCode: OP_END}}
				connect(merge, exit)
				if kind == "exit with successor" {
					connect(exit, &OpCode{Instr: &Instruction{OpCode: OP_NOP}})
				} else {
					want = true
				}
			case "retained enclosing handler":
				d.ExceptionTable = append(d.ExceptionTable, &ExceptionTableEntry{StartPc: 9, EndPc: 30, HandlerPc: 60, CatchType: 2})
				want = true
			case "selector outside try":
				d.ExceptionTable[0].StartPc = 10
			case "return after boundary":
				d.ExceptionTable[0].EndPc = 24
			case "new return handler":
				d.ExceptionTable = append(d.ExceptionTable, &ExceptionTableEntry{StartPc: 25, EndPc: 30, HandlerPc: 60, CatchType: 2})
			case "foreign arm handler":
				d.ExceptionTable = append(d.ExceptionTable, &ExceptionTableEntry{StartPc: 24, EndPc: 25, HandlerPc: 60, CatchType: 2})
			case "external region entrance":
				check.Source = append(check.Source, &OpCode{})
			case "external return entrance":
				merge.Source = append(merge.Source, &OpCode{})
			case "local store":
				load.Instr.OpCode = OP_ASTORE_0
			case "field store":
				load.Instr.OpCode = OP_PUTFIELD
			case "throw":
				load.Instr.OpCode = OP_ATHROW
			case "back edge":
				connect(check, load)
			case "nonterminal return":
				connect(merge, &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 26})
			case "wrong consumer":
				merge.Instr.OpCode = OP_ASTORE_0
			case "cancelled proof":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.protectedTerminalValueMerge(selector, merge, check); got != want {
				t.Fatalf("accepted=%v, want=%v", got, want)
			}
		})
	}
}
