package javaclassparser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorReceiverAndCheckcastSourceEvidence(t *testing.T) {
	for _, opcode := range []int{core.OP_INVOKEINTERFACE, core.OP_INVOKEVIRTUAL} {
		for _, mutation := range []string{"control", "receiver identity", "receiver missing", "receiver origin", "call pc", "call target", "call kind", "call static", "cast binding", "cast pc", "cast target", "cast absent", "cast operand", "argument count", "depth"} {
			t.Run(fmt.Sprintf("opcode-%x/%s", opcode, mutation), func(t *testing.T) {
				kind, ownerKind := values.InvokeInterface, values.MethodOwnerInterface
				if opcode == core.OP_INVOKEVIRTUAL {
					kind, ownerKind = values.InvokeVirtual, values.MethodOwnerClass
				}
				objectType := types.NewJavaClass("java.util.Map$Entry")
				param := values.NewJavaRef(utils.NewRootVariableId(), nil, objectType)
				param.IsParam = true
				param.MarkOriginalParameter(1)
				ctx := &class_context.ClassContext{}
				call := &values.FunctionCallExpression{ClassName: "java.util.Map$Entry", FunctionName: "getKey", Descriptor: "()Ljava/lang/Object;", Kind: kind, MethodOwnerKind: ownerKind, Object: param, HasOriginPC: true, OriginPC: 3}
				child := constructorLoweringArgument{descriptor: "Ljava/lang/Object;", invocation: &constructorLoweringInvocation{owner: "java/util/Map$Entry", name: "getKey", descriptor: call.Descriptor, opcode: opcode, ownerKind: ownerKind, receiver: &constructorLoweringArgument{slot: 1, descriptor: "Ljava/util/Map$Entry;"}}}
				argument := constructorLoweringArgument{descriptor: "Ljava/lang/Comparable;", checked: &child}
				origin := constructorLoweringOperandOrigin{pc: 8, arguments: []constructorLoweringOperandOrigin{{pc: 3, receiver: &constructorLoweringOperandOrigin{pc: 2}}}}
				cast := values.NewOriginalCheckCast(call, types.NewJavaClass("java.lang.Comparable"), 8)
				var value values.JavaValue = cast
				depth := 0
				switch mutation {
				case "receiver identity":
					call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, objectType)
				case "receiver missing":
					call.Object = nil
				case "receiver origin":
					origin.arguments[0].receiver = nil
				case "call pc":
					call.OriginPC++
				case "call target":
					call.FunctionName = "getValue"
				case "call kind":
					call.Kind = values.InvokeSpecial
				case "call static":
					call.IsStatic = true
				case "cast binding":
					cast.Binding = true
				case "cast pc":
					cast.OriginPC++
				case "cast target":
					cast.TargetType = types.NewJavaClass("java.lang.String")
				case "cast absent":
					value = call
				case "cast operand":
					cast.Value = param
				case "argument count":
					call.Arguments = []values.JavaValue{param}
				case "depth":
					depth = 32
				}
				if got := constructorLoweringSourceOperand(argument, origin, value, map[int]int{1: 0}, []values.JavaValue{param}, depth, ctx); got != (mutation == "control") {
					t.Fatalf("source proof=%v", got)
				}
			})
		}
	}
}

func TestAdversarialMemberEntryRejectsFreshReceiverAndCastChanges(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	_, fresh, artifact := constructorSuffixFamilyRoundTrip(t, memberEntrySignatureFixture, "source,lines,vars", []string{"EntryOwner$Entry", "EntryOwner"}, "16:member-entry:lexical:bridge:identity\n")
	for _, mutation := range []string{"control", "call target", "interface count", "interface reserved", "wrong call kind", "cast target", "cast removed", "receiver slot", "stack limit"} {
		t.Run(mutation, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if n == "<init>" && d == "(LEntryOwner;Ljava/util/Map$Entry;)V" {
					code = constructorLoweringCode(m)
				}
			}
			if code == nil {
				t.Fatal("constructor absent")
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if e := decoder.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			var getKey, getValue, cast, receiver *core.OpCode
			ops := constructorMotionOps(decoder)
			for i, op := range ops {
				if member := constructorMotionMember(obj, op, core.OP_INVOKEINTERFACE); member != nil {
					if member.Member == "getKey" {
						getKey = op
						receiver = ops[i-1]
					}
					if member.Member == "getValue" {
						getValue = op
					}
				}
				if op.Instr.OpCode == core.OP_CHECKCAST {
					cast = op
				}
			}
			if getKey == nil || getValue == nil || cast == nil || receiver == nil {
				t.Fatal("fixture operand shape")
			}
			at := int(getKey.CurrentOffset)
			cp := int(cast.CurrentOffset)
			switch mutation {
			case "call target":
				copy(code.Code[at+1:at+3], getValue.Data[:2])
			case "interface count":
				code.Code[at+3]++
			case "interface reserved":
				code.Code[at+4] = 1
			case "wrong call kind":
				code.Code[at] = core.OP_INVOKEVIRTUAL
			case "cast target":
				code.Code[cp+1], code.Code[cp+2] = byte(obj.ThisClass>>8), byte(obj.ThisClass)
			case "cast removed":
				code.Code[cp], code.Code[cp+1], code.Code[cp+2] = core.OP_NOP, core.OP_NOP, core.OP_NOP
			case "receiver slot":
				code.Code[receiver.CurrentOffset] = core.OP_ALOAD_1
			case "stack limit":
				code.MaxStack = 1
			}
			input := obj.Bytes()
			saved := bytes.Clone(input)
			output, err := artifact.FinishClass("EntryOwner$Entry.class", input)
			if !bytes.Equal(saved, input) {
				t.Fatal("input mutated")
			}
			if mutation == "control" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || output != nil {
				t.Fatal("unproved fresh operand graph accepted", err)
			}
		})
	}
}
