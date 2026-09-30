package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBranchExpressionCastStackProof(t *testing.T) {
	for _, kind := range []string{"field and argument call", "static field", "inline nested cast", "nop", "snapshot local", "missing call witness", "wrong call origin", "wrong staticness", "wrong descriptor count", "void call", "changed argument", "changed receiver", "changed stack operand", "missing operand", "extra produced value", "hidden load expression", "materialized nested cast", "cast input mismatch", "cast witness mismatch", "alternate entry", "handler boundary", "local store", "discarded call", "duplicated field", "allocation", "back edge", "fork", "leftover stack", "shared cast producer", "cast before arm", "effect after cast", "bounded prefix"} {
		t.Run(kind, func(t *testing.T) {
			ref := func(typ string) *values.JavaRef {
				return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(typ))
			}
			owner := ref("Probe")
			field := values.NewRefMember(owner, "map", types.NewJavaClass("java.util.Map"))
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 10, stackProduced: []values.JavaValue{owner}}
			read := &OpCode{Instr: &Instruction{OpCode: OP_GETFIELD}, CurrentOffset: 11, stackConsumed: []values.JavaValue{owner}, stackProduced: []values.JavaValue{field}}
			key := &values.FunctionCallExpression{IsStatic: true, OriginPC: 14, Descriptor: "()Ljava/lang/String;"}
			arg := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 14, stackProduced: []values.JavaValue{key}}
			call := &values.FunctionCallExpression{Object: field, Arguments: []values.JavaValue{key}, OriginPC: 17, Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;"}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEINTERFACE}, CurrentOffset: 17, stackConsumed: []values.JavaValue{key, field}, stackProduced: []values.JavaValue{call}}
			cast := &values.CastExpression{Value: call, TargetType: types.NewJavaClass("java.lang.String"), OriginPC: 22}
			local := values.NewJavaRef(utils.NewRootVariableId(), cast, cast.Type())
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 22, stackConsumed: []values.JavaValue{call}, stackProduced: []values.JavaValue{local}}
			jump := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 25}
			merge := &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_1}, CurrentOffset: 30}
			path := []*OpCode{load, read, arg, invoke, check, jump, merge}
			d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{arg: key, invoke: call}, checkcastInnerArg: map[*OpCode]values.JavaValue{check: call}, inlineCheckcast: map[*OpCode]bool{}}
			want := false
			switch kind {
			case "field and argument call":
				want = true
			case "static field":
				static := &values.JavaClassMember{Name: "Probe", Member: "MAP", JavaType: field.Type()}
				read.Instr.OpCode, read.stackConsumed, read.stackProduced = OP_GETSTATIC, nil, []values.JavaValue{static}
				call.Object, invoke.stackConsumed[1], path, want = static, static, path[1:], true
			case "inline nested cast", "materialized nested cast":
				inner := &values.CastExpression{Value: field, TargetType: field.Type(), OriginPC: 13}
				op := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 13, stackConsumed: []values.JavaValue{field}, stackProduced: []values.JavaValue{inner}}
				d.inlineCheckcast[op], d.checkcastInnerArg[op] = true, field
				call.Object, invoke.stackConsumed[1] = inner, inner
				path = append(append([]*OpCode{}, path[:2]...), append([]*OpCode{op}, path[2:]...)...)
				if kind == "materialized nested cast" {
					snapshot := values.NewJavaRef(utils.NewRootVariableId(), inner, inner.Type())
					op.stackProduced, call.Object, invoke.stackConsumed[1] = []values.JavaValue{snapshot}, snapshot, snapshot
				} else {
					want = true
				}
			case "nop":
				path = append(append([]*OpCode{}, path[:2]...), append([]*OpCode{{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 13}}, path[2:]...)...)
				want = true
			case "snapshot local":
				// Its old defining field must not be expanded or read again.
				local := values.NewJavaRef(utils.NewRootVariableId(), field, field.Type())
				load.stackProduced, call.Object, invoke.stackConsumed[1] = []values.JavaValue{local}, local, local
				path, want = append([]*OpCode{load}, path[2:]...), true
			case "missing call witness":
				delete(d.invokeFuncCall, arg)
			case "wrong call origin":
				key.OriginPC = 9
			case "wrong staticness":
				key.IsStatic = false
			case "wrong descriptor count":
				call.Descriptor = "(Ljava/lang/Object;I)Ljava/lang/Object;"
			case "void call":
				key.Descriptor = "()V"
			case "changed argument":
				call.Arguments[0] = values.JavaNull
			case "changed receiver":
				call.Object = owner
			case "changed stack operand":
				read.stackConsumed[0] = values.JavaNull
			case "missing operand":
				invoke.stackConsumed = invoke.stackConsumed[:1]
			case "extra produced value":
				read.stackProduced = append(read.stackProduced, values.JavaNull)
			case "hidden load expression":
				path = append([]*OpCode{read}, path[2:]...)
				read.Instr.OpCode, read.stackConsumed = OP_ALOAD_0, nil
			case "cast input mismatch":
				cast.Value = values.JavaNull
			case "cast witness mismatch":
				d.checkcastInnerArg[check] = values.JavaNull
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 14, HandlerPc: 50}}
			case "local store":
				arg.Instr.OpCode = OP_ASTORE_1
			case "discarded call":
				arg.Instr.OpCode = OP_POP
			case "duplicated field":
				arg.Instr.OpCode = OP_DUP
			case "allocation":
				arg.Instr.OpCode = OP_NEW
			case "leftover stack":
				path = append([]*OpCode{{Instr: &Instruction{OpCode: OP_ACONST_NULL}, CurrentOffset: 9, stackProduced: []values.JavaValue{values.JavaNull}}}, path...)
			case "shared cast producer":
				d.opcodeToSimulateStack[&OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 7, stackProduced: []values.JavaValue{local}}] = nil
			case "effect after cast":
				jump.Instr.OpCode = OP_INVOKESTATIC
			case "bounded prefix":
				prefix := make([]*OpCode, 257)
				for i := range prefix {
					prefix[i] = &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: uint16(i + 1)}
				}
				for _, op := range path {
					op.CurrentOffset += 300
				}
				key.OriginPC, call.OriginPC, cast.OriginPC = 314, 317, 322
				path = append(prefix, path...)
			}
			for i, op := range path {
				d.opcodeToSimulateStack[op] = nil
				if i+1 < len(path) {
					op.Target, path[i+1].Source = []*OpCode{path[i+1]}, []*OpCode{op}
				}
			}
			entry := path[0]
			switch kind {
			case "alternate entry":
				read.Source = append(read.Source, &OpCode{})
			case "back edge":
				arg.CurrentOffset = 10
			case "fork":
				read.Target = append(read.Target, merge)
			case "cast before arm":
				entry = jump
			}
			if got := d.branchExpressionCastLeaf(local, cast, entry, jump, merge); (got == check) != want {
				t.Fatalf("accepted=%t, want=%t", got != nil, want)
			}
		})
	}
}
