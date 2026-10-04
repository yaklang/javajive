package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBranchNestedCastExpressionOwnership(t *testing.T) {
	for _, kind := range []string{"receiver", "argument", "shared use", "wrong input", "wrong origin", "second producer", "external entrance", "handler change", "store", "dup", "discard", "cast before arm", "different leaf", "cycle", "snapshot", "expression disabled", "immediate disabled"} {
		t.Run(kind, func(t *testing.T) {
			input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 10, stackProduced: []values.JavaValue{input}}
			cast := &values.CastExpression{Value: input, TargetType: types.NewJavaClass("java.lang.String"), OriginPC: 11}
			ref := values.NewJavaRef(utils.NewRootVariableId(), cast, cast.Type())
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 11, stackConsumed: []values.JavaValue{input}, stackProduced: []values.JavaValue{ref}}
			call := &values.FunctionCallExpression{Object: ref, ClassName: "java.lang.String", FunctionName: "length", Descriptor: "()I", OriginPC: 14}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 14, stackConsumed: []values.JavaValue{ref}, stackProduced: []values.JavaValue{call}}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 17}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ISTORE_1}, CurrentOffset: 20}
			path := []*OpCode{load, check, invoke, jump, merge}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, checkcastInnerArg: map[*OpCode]values.JavaValue{check: input}, inlineCheckcast: map[*OpCode]bool{}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
			eligible := func(*values.JavaRef) bool { return true }
			want := kind == "receiver" || kind == "argument"
			entry, leaf := load, jump
			switch kind {
			case "expression disabled":
				t.Setenv("JDEC_BRANCH_EXPRESSION_CAST_OFF", "1")
			case "immediate disabled":
				t.Setenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF", "1")
			case "argument":
				call.IsStatic, call.Object, call.Arguments, call.Descriptor = true, nil, []values.JavaValue{ref}, "(Ljava/lang/String;)I"
				invoke.Instr.OpCode = OP_INVOKESTATIC
			case "shared use":
				eligible = func(*values.JavaRef) bool { return false }
			case "wrong input":
				check.stackConsumed = []values.JavaValue{values.JavaNull}
			case "wrong origin":
				cast.OriginPC = 9
			case "second producer":
				d.opcodeToSimulateStack[&OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 5, stackProduced: []values.JavaValue{ref}}] = nil
			case "store":
				invoke.Instr.OpCode = OP_ASTORE_1
			case "dup":
				invoke.Instr.OpCode = OP_DUP
			case "discard":
				invoke.Instr.OpCode = OP_POP
			case "cast before arm":
				entry = invoke
			case "different leaf":
				leaf = check
			case "cycle":
				cast.Value = ref
				check.stackConsumed = []values.JavaValue{ref}
				d.checkcastInnerArg[check] = ref
				load.stackProduced = []values.JavaValue{ref}
			case "snapshot":
				check.Instr.OpCode = OP_ALOAD_1
			}
			for i, op := range path {
				d.opcodeToSimulateStack[op] = nil
				if i > 0 {
					op.Source = []*OpCode{path[i-1]}
				}
				if i+1 < len(path) {
					op.Target = []*OpCode{path[i+1]}
				}
			}
			if kind == "external entrance" {
				check.Source = append(check.Source, &OpCode{})
			}
			if kind == "handler change" {
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 14, HandlerPc: 30}}
			}
			planned, casts := d.branchNestedCastExpression(call, entry, leaf, merge, eligible)
			if (len(casts) > 0) != want {
				t.Fatalf("accepted=%v", len(casts) > 0)
			}
			if want {
				out := planned.(*values.FunctionCallExpression)
				actual := out.Object
				if kind == "argument" {
					actual = out.Arguments[0]
				}
				got, ok := actual.(*values.CastExpression)
				if !ok || got.Value != input || got.OriginPC != 11 || casts[check] != ref {
					t.Fatal("cast/evaluation witness lost")
				}
				if call.Object != ref && kind == "receiver" || call.Arguments != nil && call.Arguments[0] != ref {
					t.Fatal("mutated original call")
				}
			} else if planned != call {
				t.Fatal("failed plan changed expression")
			}
			if d.inlineCheckcast[check] {
				t.Fatal("published before routing accepted")
			}
		})
	}
}
