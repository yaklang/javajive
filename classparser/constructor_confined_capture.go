package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Fresh confinement adds a source permission. A provider must not change the
// caller-owned root while that permission is established, including on the
// all-original-bytes path which has no later platform-profile replay.
func constructorConfinedRootSnapshot(object *ClassObject) (raw []byte, known bool) {
	defer func() {
		if recover() != nil {
			raw, known = nil, false
		}
	}()
	if object == nil {
		return nil, false
	}
	raw = object.Bytes()
	// The serializer reports unsupported/malformed trees with an empty result.
	// Two failed serializations must never stand for equal original evidence.
	return raw, len(raw) >= 10
}

// An exact NEW can copy its arguments into its own ordinary storage without
// publishing them. Keep the initialized result receiver-reaching, not THIS:
// later calls, field reads and publication cannot borrow THIS permissions.
// The caller separately proves the original receiver's finalization boundary.
func (c *ClassObjectDumper) constructorConfinedCapture(member *values.JavaClassMember, actuals []constructorEffectValue, remaining *int) (bool, bool) {
	return c.constructorConfinedCaptureChain(member, actuals, remaining, map[string]bool{}, 0, true)
}
func (c *ClassObjectDumper) constructorConfinedCaptureChain(member *values.JavaClassMember, actuals []constructorEffectValue, remaining *int, active map[string]bool, depth int, allocation bool) (bool, bool) {
	if depth > 16 || c == nil || member == nil || member.Member != "<init>" || len(member.Description) > 65535 || !c.constructorReceiverFinalizerSilent || remaining == nil || *remaining <= 0 || !nativeProofWork(c.Work, int64(len(member.Description))+1) {
		return false, false
	}
	key := member.Name + "\x00" + member.Description
	if active[key] {
		return false, false
	}
	active[key] = true
	defer delete(active, key)
	if member.Name == "java/lang/Object" && member.Description == "()V" {
		if c.FuncCtx != nil {
			exceptions, known := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, member.Name, "<init>", "()V")
			return false, known && len(exceptions) == 0 && len(actuals) == 0
		}
		exceptions, known := exactInvocationExceptions(c.buildInvocationMetadata(), member.Name, "<init>", "()V")
		return false, known && len(exceptions) == 0 && len(actuals) == 0
	}
	object, known := c.constructorMotionClassWithSnapshot(member.Name, true)
	if !known || object == nil || object.GetClassName() != member.Name || object.AccessFlags&(0x0200|0x2000|0x4000|0x8000) != 0 || allocation && object.AccessFlags&0x0400 != 0 {
		return false, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || result != "V" || len(params) != len(actuals) || len(params) > 128 {
		return false, false
	}
	for i, param := range params {
		value := actuals[i]
		if !constructorValidNullFact(value) || value.kind != constructorEffectType(param).kind || value.allocation != 0 || value.reachesReceiver() && value.kind != 'L' || value.receiver && value.retainsReceiver {
			return false, false
		}
	}
	var selected *MemberInfo
	for _, method := range object.Methods {
		*remaining--
		if method == nil || *remaining < 0 || !nativeProofWork(c.Work, 1) {
			return false, false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk || name == "finalize" && desc == "()V" {
			return false, false
		}
		if name == "<init>" && desc == member.Description {
			if selected != nil || method.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 {
				return false, false
			}
			selected = method
		}
	}
	if selected == nil {
		return false, false
	}
	var code *CodeAttribute
	for _, attr := range selected.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false, false
		}
		if body, ok := attr.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return false, false
			}
			code = body
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > *remaining || len(code.Code) == 0 || !nativeProofWork(c.Work, int64(len(code.Code))) {
		return false, false
	}
	locals := nativeMemberParameterWidth(params) + 1
	if locals > 255 || int(code.MaxLocals) < locals || code.MaxStack < 1 || c.Work != nil && c.Work.CheckAlloc(int64(locals)*32+int64(len(code.Code))*64) != nil {
		return false, false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 3 || !constructorEffectOriginalOperandFree(code, ops[len(ops)-1]) || ops[len(ops)-1].Instr.OpCode != core.OP_RETURN {
		return false, false
	}
	return c.constructorConfinedCaptureBody(object, code, ops, params, actuals, remaining, active, depth)
}
