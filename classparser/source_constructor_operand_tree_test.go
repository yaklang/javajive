package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Same text/type does not establish a producer identity. Mutate each edge of
// the source operand tree independently, including NEW's uninitialized token
// and a static invocation's unevaluated qualifier.
func TestAdversarialConstructorOperandTreeRequiresExactSourceProducers(t *testing.T) {
	for _, shape := range []string{"static", "allocation"} {
		for _, variant := range []string{"control", "call pc", "missing call pc", "owner", "name", "descriptor", "owner kind", "invocation kind", "argument count", "parameter identity", "child pc", "child target", "evaluated qualifier", "wrong qualifier", "literal qualifier", "missing qualifier", "allocation pc", "allocation type", "allocation identity", "array initializer", "depth"} {
			if shape == "static" && (strings.HasPrefix(variant, "allocation ") || variant == "array initializer") {
				continue
			}
			t.Run(shape+"/"+variant, func(t *testing.T) {
				wide := types.NewJavaPrimer(types.JavaLong)
				param := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, wide)
				param.IsParam = true
				param.MarkOriginalParameter(1)
				params := []values.JavaValue{param}
				slots := map[int]int{1: 0}
				child := &values.FunctionCallExpression{ClassName: "mvp.Operations", FunctionName: "scale", Descriptor: "(J)J", MethodOwnerKind: values.MethodOwnerClass, Kind: values.InvokeStatic, IsStatic: true, Object: values.NewJavaClassValue(types.NewJavaClass("mvp.Operations")), OriginPC: 4, HasOriginPC: true, Arguments: []values.JavaValue{param}}
				inner := constructorLoweringArgument{descriptor: "J", invocation: &constructorLoweringInvocation{owner: "mvp/Operations", name: "scale", descriptor: "(J)J", opcode: core.OP_INVOKESTATIC, ownerKind: values.MethodOwnerClass, arguments: []constructorLoweringArgument{{slot: 1, descriptor: "J"}}}}
				origin := constructorLoweringOperandOrigin{pc: 7, allocationPC: 0, arguments: []constructorLoweringOperandOrigin{{pc: 4, arguments: []constructorLoweringOperandOrigin{{pc: 3}}}}}
				outer := &values.FunctionCallExpression{ClassName: "mvp.Operations", FunctionName: "box", Descriptor: "(J)Lmvp/Payload;", MethodOwnerKind: values.MethodOwnerClass, Kind: values.InvokeStatic, IsStatic: true, Object: values.NewJavaClassValue(types.NewJavaClass("mvp.Operations")), OriginPC: 7, HasOriginPC: true, Arguments: []values.JavaValue{child}}
				arg := constructorLoweringArgument{descriptor: "Lmvp/Payload;", invocation: &constructorLoweringInvocation{owner: "mvp/Operations", name: "box", descriptor: outer.Descriptor, opcode: core.OP_INVOKESTATIC, ownerKind: values.MethodOwnerClass, arguments: []constructorLoweringArgument{inner}}}
				allocation := values.NewNewExpression(types.NewJavaClass("mvp.Payload"))
				allocation.OriginPC, allocation.HasOriginPC = 0, true
				var value values.JavaValue = outer
				if shape == "allocation" {
					outer.ClassName, outer.FunctionName, outer.Descriptor = "mvp.Payload", "<init>", "(J)V"
					outer.Kind, outer.IsStatic, outer.IsSpecialInvoke, outer.Object = values.InvokeSpecial, false, true, allocation
					allocation.ConstructorCall = outer
					arg.invocation.owner, arg.invocation.name, arg.invocation.descriptor, arg.invocation.opcode = "mvp/Payload", "<init>", "(J)V", core.OP_NEW
					value = allocation
				}
				depth := 0
				// Qualifier mutations target the nested static call in both trees.
				switch variant {
				case "call pc":
					outer.OriginPC++
				case "missing call pc":
					outer.HasOriginPC = false
				case "owner":
					outer.ClassName = "mvp.Other"
				case "name":
					outer.FunctionName = "other"
				case "descriptor":
					outer.Descriptor = "(I)V"
				case "owner kind":
					outer.MethodOwnerKind = values.MethodOwnerInterface
				case "invocation kind":
					outer.Kind = values.InvokeVirtual
				case "argument count":
					outer.Arguments = nil
				case "parameter identity":
					child.Arguments[0] = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, wide)
				case "child pc":
					child.OriginPC++
				case "child target":
					child.FunctionName = "otherScale"
				case "evaluated qualifier":
					child.Object = param
				case "wrong qualifier":
					child.Object = values.NewJavaClassValue(types.NewJavaClass("mvp.Other"))
				case "literal qualifier":
					child.Object.(*values.JavaClassValue).HasOriginPC = true
				case "missing qualifier":
					child.Object = nil
				case "allocation pc":
					allocation.OriginPC++
				case "allocation type":
					allocation.JavaType = types.NewJavaClass("mvp.Other")
				case "allocation identity":
					allocation.ConstructorCall = new(values.FunctionCallExpression)
					*allocation.ConstructorCall = *outer
					allocation.ConstructorCall.Object = values.NewNewExpression(types.NewJavaClass("mvp.Payload"))
				case "array initializer":
					allocation.Initializer = []values.JavaValue{param}
				case "depth":
					depth = 32
				}
				want := variant == "control"
				if got := constructorLoweringSourceOperand(arg, origin, value, slots, params, depth, nil); got != want {
					t.Fatalf("source producer %s: got %v want %v", variant, got, want)
				}
			})
		}
	}
}

func TestAdversarialConstructorOperandTreeRejectsFreshInvocationAndAllocationRebinding(t *testing.T) {
	fixture := strings.Replace(constructorOperandTreeFixture, "OPERAND", "new SuffixPayload(SuffixOps.number(seed))", 1)
	_, fresh, artifact := constructorSuffixFamilyAtTargetRoundTrip(t, fixture, "source,lines,vars", []string{"SuffixOwner$Member"}, "216:constructor:operand:tree:independent:oracle\n", 8, 16)
	for _, variant := range []string{"control", "call owner", "call name", "call descriptor", "instance call", "field reference", "constructor owner", "constructor descriptor", "allocation owner", "uninitialized duplicate", "argument type"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var packet *constructorLoweringPacket
			for _, method := range obj.Methods {
				if p := constructorLoweringPacketFor(obj, method, false, false, nil, 1); p != nil {
					if packet != nil {
						t.Fatal("ambiguous fresh constructor packet")
					}
					packet = p
				}
			}
			if packet == nil {
				t.Fatal("missing fresh constructor packet")
			}
			var call, init, allocation, argument *core.OpCode
			for _, op := range packet.ops {
				if int(op.CurrentOffset) >= packet.firstEnd {
					break
				}
				if m := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); m != nil && m.Member == "number" {
					call = op
				}
				if m := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "SuffixPayload" {
					init = op
				}
				if op.Instr.OpCode == core.OP_NEW {
					allocation = op
				}
				if constructorMotionLoad(op, "J") {
					argument = op
				}
			}
			if call == nil || init == nil || allocation == nil || argument == nil {
				t.Fatal("incomplete fresh operand control")
			}
			code := packet.code.Code
			rebind := func(op *core.OpCode, owner, name, descriptor string) {
				ref := obj.ConstantPoolManager.AddNewMethodInfo(owner, name, descriptor)
				binary.BigEndian.PutUint16(code[int(op.CurrentOffset)+1:], uint16(ref))
			}
			switch variant {
			case "call owner":
				rebind(call, "OtherOps", "number", "(J)J")
			case "call name":
				rebind(call, "SuffixOps", "different", "(J)J")
			case "call descriptor":
				rebind(call, "SuffixOps", "number", "(J)D")
			case "instance call":
				code[int(call.CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "field reference":
				ref := obj.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: *obj.ConstantPoolManager.newMemberrefInfo("SuffixOps", "number", "J")})
				binary.BigEndian.PutUint16(code[int(call.CurrentOffset)+1:], uint16(ref))
			case "constructor owner":
				rebind(init, "OtherPayload", "<init>", "(J)V")
			case "constructor descriptor":
				rebind(init, "SuffixPayload", "<init>", "(D)V")
			case "allocation owner":
				// A different well-formed class constant must not acquire this
				// allocation's initialization identity by sharing its PC.
				member := constructorMotionMember(obj, call, core.OP_INVOKESTATIC)
				ref := obj.ConstantPoolManager.AddNewMethodInfo(member.Name, member.Member, member.Description)
				method := obj.ConstantPool[ref-1].(*ConstantMethodrefInfo)
				binary.BigEndian.PutUint16(code[int(allocation.CurrentOffset)+1:], method.ClassIndex)
			case "uninitialized duplicate":
				code[int(allocation.CurrentOffset)+3] = core.OP_DUP_X1
			case "argument type":
				code[int(argument.CurrentOffset)] = core.OP_DLOAD_3
			}
			_, err := artifact.FinishClass("SuffixOwner$Member.class", obj.Bytes())
			if (err == nil) != (variant == "control") {
				t.Fatalf("fresh operand binding %s: %v", variant, err)
			}
		})
	}
}
