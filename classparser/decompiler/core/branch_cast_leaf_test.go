package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBranchPureCastLeafProof(t *testing.T) {
	for _, kind := range []string{"load", "cast entry", "direct merge", "null", "cast before arm", "handler boundary", "alternate entry", "effect before cast", "effectful operand", "different producer", "shared local", "effect after cast"} {
		t.Run(kind, func(t *testing.T) {
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 10}
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 13}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 16}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ARETURN}, CurrentOffset: 20}
			load.Target, check.Source = []*OpCode{check}, []*OpCode{load}
			check.Target, jump.Source = []*OpCode{jump}, []*OpCode{check}
			jump.Target, merge.Source = []*OpCode{merge}, []*OpCode{jump}
			param := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			param.IsParam = true
			cast := &values.CastExpression{Value: param, TargetType: types.NewJavaClass("java.lang.Number"), OriginPC: 13}
			ref := values.NewJavaRef(utils.NewRootVariableId(), cast, cast.Type())
			check.stackProduced = []values.JavaValue{ref}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{load: nil, check: nil, jump: nil, merge: nil}}
			entry, leaf := load, jump
			want := false
			switch kind {
			case "load":
				want = true
			case "cast entry":
				entry, want = check, true
			case "direct merge":
				check.Target, merge.Source = []*OpCode{merge}, []*OpCode{check}
				leaf, want = check, true
			case "null":
				load.Instr.OpCode, cast.Value, want = OP_ACONST_NULL, values.JavaNull, true
			case "cast before arm":
				entry = jump
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20, HandlerPc: 30}}
			case "alternate entry":
				check.Source = append(check.Source, &OpCode{})
			case "effect before cast":
				load.Instr.OpCode = OP_INVOKESTATIC
			case "effectful operand":
				cast.Value = &values.FunctionCallExpression{}
			case "different producer":
				check.stackProduced = nil
			case "shared local":
				other := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 6, stackProduced: []values.JavaValue{ref}}
				d.opcodeToSimulateStack[other] = nil
			case "effect after cast":
				jump.Instr.OpCode = OP_INVOKESTATIC
			}
			if got := d.branchPureCastLeaf(ref, cast, entry, leaf, merge); (got == check) != want {
				t.Fatalf("proof accepted=%t, want=%t", got != nil, want)
			}
		})
	}
}

func TestBranchCallCastLeafProof(t *testing.T) {
	for _, kind := range []string{"valid", "handler boundary", "call before arm", "other effect", "alternate entry", "different producer", "unmatched witness", "static argument", "static before arm", "unmatched field", "field before later argument"} {
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
			if kind == "static argument" || kind == "static before arm" || kind == "unmatched field" || kind == "field before later argument" {
				field := &values.JavaClassMember{Name: "Probe", Member: "KEY", JavaType: types.NewJavaClass("java.lang.Object")}
				load := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 9, Target: []*OpCode{invoke}, stackProduced: []values.JavaValue{field}}
				invoke.Source = []*OpCode{load}
				d.opcodeToSimulateStack[load] = nil
				call.Arguments = []values.JavaValue{field}
				entry = load
				if kind == "static before arm" {
					entry = invoke
				}
				if kind == "unmatched field" {
					load.stackProduced = []values.JavaValue{values.JavaNull}
				}
				if kind == "field before later argument" {
					call.Arguments = append(call.Arguments, values.JavaNull)
				}
			}
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
			if (got == check) != (kind == "valid" || kind == "static argument") {
				t.Fatalf("proof accepted=%t", got != nil)
			}
		})
	}
}
