package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

type memberScopeConstructorProof struct {
	capture *sourceConstructorLoweringPlan
	forward *memberScopeConstructorForward
}

type memberScopeConstructorForward struct {
	descriptor, target string
	flags              uint16
	arguments          []constructorLoweringArgument
}

// THIS delegation transports the enclosing instance to another constructor;
// the terminal SUPER constructor owns its storage. Close the entire finite
// constructor graph, requiring the same original outer slot on every edge and
// one proved capture at every leaf. No forwarding edge grants source ownership
// or permits a fresh target, operand tree, or declaration flag to change.
func memberScopeConstructorGraph(object *ClassObject, outer string, original bool, work *workbudget.Budget, provider callbinding.Provider) map[string]*memberScopeConstructorProof {
	if object == nil || len(object.Methods) > 4096 || work.CheckAlloc(int64(len(object.Methods)+1)*256) != nil {
		return nil
	}
	// The marker decoder assumes real member records. Validate the complete
	// input inventory before invoking that separate certificate.
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil
		}
		if _, ok := sourceBridgeUTF8(object, method.NameIndex); !ok {
			return nil
		}
		if _, ok := sourceBridgeUTF8(object, method.DescriptorIndex); !ok {
			return nil
		}
	}
	d := NewClassObjectDumper(object)
	d.Work = work
	markers := d.constructorForwardingPackets(syntheticMethodFlag)
	proofs := map[string]*memberScopeConstructorProof{}
	for _, method := range object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return nil
		}
		name, named := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !named || !typed {
			return nil
		}
		if name != "<init>" {
			continue
		}
		if marker := markers[descriptor]; marker != nil && marker.method == method {
			continue // its separate typed private-access certificate remains required
		}
		if proofs[descriptor] != nil {
			return nil
		}
		var packet *constructorLoweringPacket
		if original {
			packet = constructorLoweringTypedPacketFor(object, method, true, true, work, provider)
		} else {
			packet = constructorLoweringTypedPacketFor(object, method, true, false, work, provider, 1)
			if packet == nil {
				packet = constructorLoweringTypedPacketFor(object, method, false, false, work, provider, 1)
			}
		}
		if packet != nil && len(packet.plan.captures) == 1 && packet.plan.captures[0].slot == 1 && packet.plan.captures[0].field.descriptor == "L"+outer+";" {
			proofs[descriptor] = &memberScopeConstructorProof{capture: packet.plan}
			continue
		}
		forward := memberScopeConstructorForwarding(object, method, outer, original, work, provider)
		if forward == nil {
			return nil
		}
		proofs[descriptor] = &memberScopeConstructorProof{forward: forward}
	}
	if len(proofs) == 0 {
		return nil
	}
	state := map[string]uint8{}
	var visit func(string, int) bool
	visit = func(descriptor string, depth int) bool {
		if depth > 128 || !nativeProofWork(work, 1) || state[descriptor] == 1 {
			return false
		}
		if state[descriptor] == 2 {
			return true
		}
		proof := proofs[descriptor]
		if proof == nil {
			return false
		}
		state[descriptor] = 1
		if proof.capture == nil && (proof.forward == nil || !visit(proof.forward.target, depth+1)) {
			return false
		}
		state[descriptor] = 2
		return true
	}
	for descriptor := range proofs {
		if !visit(descriptor, 0) {
			return nil
		}
	}
	return proofs
}

func memberScopeConstructorForwarding(object *ClassObject, method *MemberInfo, outer string, original bool, work *workbudget.Budget, provider callbinding.Provider) *memberScopeConstructorForward {
	if object == nil || method == nil || object.MinorVersion != 0 || original && (object.MajorVersion < 49 || object.MajorVersion > 52) || !original && (object.MajorVersion < 52 || object.MajorVersion > 65) || method.AccessFlags&^uint16(0x0087) != 0 {
		return nil
	}
	name, named := sourceBridgeUTF8(object, method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(descriptor)
	code := constructorLoweringCode(method)
	if !named || name != "<init>" || !typed || err != nil || result != "V" || len(params) == 0 || params[0] != "L"+outer+";" || code == nil || len(code.Code) == 0 || len(code.Code) > 4096 || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))+1) || work.CheckAlloc(int64(len(code.Code))*192) != nil {
		return nil
	}
	words := nativeMemberParameterWidth(params) + 1
	if words > 255 || int(code.MaxLocals) < words {
		return nil
	}
	// This is an executable constructor-edge proof. Enum metadata grammar
	// cannot decode ordinary generic constructor Signatures. The caller
	// independently closes declaration metadata; decode all instructions here.
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if !enumArrayInstructionSpans(code.Code, ops) {
		return nil
	}
	at, peak := 0, 1
	var guards []constructorLoweringQualifierGuard
	arguments, _, target, known := constructorLoweringDelegateOperands(object, ops, &at, constructorParameterSlots(params), params, object.GetClassName(), newConstructorWideningQuery(provider), &peak, work, false, &guards)
	targetParams, targetResult, targetErr := callbinding.Descriptor(target)
	if !known || targetErr != nil || targetResult != "V" || target == descriptor || len(targetParams) == 0 || targetParams[0] != params[0] || len(arguments) == 0 || arguments[0].slot != 1 || arguments[0].descriptor != params[0] || arguments[0].invocation != nil || arguments[0].literal != nil || arguments[0].null || len(arguments[0].nullCasts) != 0 || at != len(ops)-1 || !nativeEnumOpcode(ops[at], core.OP_RETURN) || int(code.MaxStack) < peak {
		return nil
	}
	return &memberScopeConstructorForward{descriptor: descriptor, target: target, flags: method.AccessFlags, arguments: arguments}
}
