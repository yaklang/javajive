package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A compiler bridge is a virtual forwarder, not another Java overload. Prove
// its physical body and its regeneration from the actual instantiated source
// interface before removing it from source-level lookup alternatives.
// This domain has no argument adaptation, method inference, raw interface,
// parent-interface diamond or bound substitution.
func nativeAnonymousZeroArgSourceBridge(f *nativeAnonymousForest, owner *ClassObject, target, bridge *MemberInfo, work *workbudget.Budget) bool {
	if f == nil || owner == nil || target == nil || bridge == nil || f.resolve == nil {
		return false
	}
	ownerName, ownerKnown := sourceBridgeClassName(owner, owner.ThisClass)
	parentName, parentKnown := sourceBridgeClassName(owner, owner.SuperClass)
	if !ownerKnown || !parentKnown || f.objects[ownerName] != owner || bridge.AccessFlags != 0x1041 || target.AccessFlags != 1 || len(owner.Interfaces) != 1 || parentName != "java/lang/Object" || !nativeProofWork(work, int64(len(owner.Methods))+1) {
		return false
	}
	targetSig, tsk := nativeMethodLocalOriginalSignature(owner, target.Attributes, work)
	if !tsk || targetSig != "" || !nativeAnonymousBridgeNoThrows(target, work) {
		return false
	}
	name, nk := sourceBridgeUTF8(owner, target.NameIndex)
	desc, dk := sourceBridgeUTF8(owner, target.DescriptorIndex)
	bn, bnk := sourceBridgeUTF8(owner, bridge.NameIndex)
	bd, bdk := sourceBridgeUTF8(owner, bridge.DescriptorIndex)
	if len(name) > 4096 || len(desc) > 4096 || len(bd) > 4096 || !nativeProofWork(work, int64(len(name)+len(desc)+len(bd))*130+1) || work != nil && work.CheckAlloc(int64(len(name)+len(desc)+len(bd))*256+128) != nil {
		return false
	}
	args, ret, err := callbinding.Descriptor(desc)
	bargs, bret, berr := callbinding.Descriptor(bd)
	if !nk || !dk || !bnk || !bdk || name != bn || desc == bd || err != nil || berr != nil || len(args) != 0 || len(bargs) != 0 || !callbinding.Reference(ret) || !callbinding.Reference(bret) {
		return false
	}
	var code *CodeAttribute
	for _, a := range bridge.Attributes {
		if !nativeProofWork(work, 1) || sourceProofNil(a) {
			return false
		}
		c, ok := a.(*CodeAttribute)
		if !ok || c == nil || code != nil {
			return false
		}
		code = c
	}
	if code == nil || code.MaxLocals != 1 || code.MaxStack != 1 || len(code.ExceptionTable) != 0 || len(code.Code) != 5 || code.Code[0] != core.OP_ALOAD_0 || code.Code[1] != core.OP_INVOKEVIRTUAL || code.Code[4] != core.OP_ARETURN {
		return false
	}
	index := binary.BigEndian.Uint16(code.Code[2:4])
	if int(index) < 1 || int(index) > len(owner.ConstantPool) {
		return false
	}
	ref, ok := owner.ConstantPool[index-1].(*ConstantMethodrefInfo)
	if !ok || ref == nil {
		return false
	}
	calledOwner, cok := sourceBridgeClassName(owner, ref.ClassIndex)
	if int(ref.NameAndTypeIndex) < 1 || int(ref.NameAndTypeIndex) > len(owner.ConstantPool) {
		return false
	}
	nt, ok := owner.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	calledName, cnk := sourceBridgeUTF8(owner, nt.NameIndex)
	calledDesc, cdk := sourceBridgeUTF8(owner, nt.DescriptorIndex)
	if !cok || !cnk || !cdk || calledOwner != ownerName || calledName != name || calledDesc != desc {
		return false
	}
	physicalTarget, physicalBridge := 0, 0
	symbols := map[string]bool{}
	for _, m := range owner.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		mn, mnk := sourceBridgeUTF8(owner, m.NameIndex)
		md, mdk := sourceBridgeUTF8(owner, m.DescriptorIndex)
		if !mnk || !mdk || len(mn) > 65535 || len(md) > 65535 || !nativeProofWork(work, int64(len(mn)+len(md))+1) || work != nil && work.CheckAlloc(int64(len(symbols)+1)*256+int64(len(mn)+len(md))) != nil || symbols[mn+"\x00"+md] {
			return false
		}
		symbols[mn+"\x00"+md] = true
		if m.AccessFlags&0x40 != 0 && m != bridge {
			return false
		}
		if m == target {
			physicalTarget++
		}
		if m == bridge {
			physicalBridge++
		}
	}
	if physicalTarget != 1 || physicalBridge != 1 {
		return false
	}
	// Source's sole implemented interface must have the exact original edge.
	sig, known := nativeMethodLocalOriginalSignature(owner, owner.Attributes, work)
	if !known || len(sig) > 4096 || !nativeProofWork(work, int64(len(sig))*130+1) || work != nil && work.CheckAlloc(int64(len(sig))*256+128) != nil {
		return false
	}
	own, refs, valid := types.SignatureTypeVariableReferences(sig)
	if !valid || len(own) != 0 || len(refs) != 0 {
		return false
	}
	if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(sig, "()V"); !valid {
		return false
	}
	super, interfaces := types.ParseClassSignatureSupers(sig)
	superName, sok := types.RawClassFQN(super)
	if !sok || superName != "java.lang.Object" || len(interfaces) != 1 {
		return false
	}
	view, pok := types.AsParameterizedType(interfaces[0])
	iname, ink := sourceBridgeClassName(owner, owner.Interfaces[0])
	if !pok || !ink || len(view.OwnerSegments) > 1 {
		return false
	}
	raw, rk := types.RawClassFQN(interfaces[0])
	if !rk || strings.ReplaceAll(raw, ".", "/") != iname {
		return false
	}
	iface, iok := f.resolve(iname)
	if !iok || iface == nil {
		return false
	}
	ifaceName, ifaceNameKnown := sourceBridgeClassName(iface, iface.ThisClass)
	ifaceParent, ifaceParentKnown := sourceBridgeClassName(iface, iface.SuperClass)
	if !ifaceNameKnown || !ifaceParentKnown || ifaceName != iname || iface.AccessFlags != 0x0601 || len(iface.Interfaces) != 0 || ifaceParent != "java/lang/Object" {
		return false
	}
	isig, known := nativeMethodLocalOriginalSignature(iface, iface.Attributes, work)
	if !known || len(isig) > 4096 || !nativeProofWork(work, int64(len(isig))*130+1) || work != nil && work.CheckAlloc(int64(len(isig))*256+128) != nil {
		return false
	}
	if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(isig, "()V"); !valid {
		return false
	}
	formals := types.ClassFormalTypeParamNames(isig)
	if len(formals) == 0 || len(formals) != len(view.TypeArgs) || len(types.ClassFormalTypeParamBounds(isig, nil)) != 0 {
		return false
	}
	ip, ii := types.ParseClassSignatureSupers(isig)
	ipname, ipk := types.RawClassFQN(ip)
	if !ipk || ipname != "java.lang.Object" || len(ii) != 0 {
		return false
	}
	sigma := map[string]types.JavaType{}
	for i, n := range formals {
		if view.TypeArgs[i] == nil || types.IsWildcardType(view.TypeArgs[i]) || !nativeProofWork(work, 1) {
			return false
		}
		sigma[n] = view.TypeArgs[i]
	}
	// The sole class ancestor is still part of source lookup: a bridge cannot
	// override a final Object declaration or borrow its return-only alternatives.
	object, ook := f.resolve(parentName)
	if !ook || object == nil {
		return false
	}
	objectName, objectNameKnown := sourceBridgeClassName(object, object.ThisClass)
	if !objectNameKnown || objectName != parentName || object.SuperClass != 0 || len(object.Interfaces) != 0 {
		return false
	}
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(object, m.NameIndex)
		d, dk := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		p, _, e := callbinding.Descriptor(d)
		if e != nil {
			return false
		}
		if n == name && len(p) == 0 {
			return false
		}
	}
	matches := 0
	for _, m := range iface.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		mn, mnk := sourceBridgeUTF8(iface, m.NameIndex)
		md, mdk := sourceBridgeUTF8(iface, m.DescriptorIndex)
		if !mnk || !mdk {
			return false
		}
		// A simple abstract generic interface supplies the complete override family.
		if m.AccessFlags != 0x0401 || mn != name || md != bd {
			return false
		}
		matches++
		ms, mk := nativeMethodLocalOriginalSignature(iface, m.Attributes, work)
		if !mk || len(ms) > 4096 || !nativeProofWork(work, int64(len(ms))*130+1) || work != nil && work.CheckAlloc(int64(len(ms))*256+128) != nil {
			return false
		}
		erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(isig, ms)
		if !valid || erased != bd || len(throws) != 0 || !nativeAnonymousBridgeNoThrows(m, work) {
			return false
		}
		methodFormals, methodRefs, valid := types.SignatureTypeVariableReferences(ms)
		if !valid || len(methodFormals) != 0 {
			return false
		}
		for _, n := range methodRefs {
			if sigma[n] == nil {
				return false
			}
		}
		params, result := types.ParseMethodSignature(ms)
		if len(params) != 0 || result == nil {
			return false
		}
		actual := types.SubstituteTypeVars(result, sigma)
		if actual == nil || !nativeAnonymousBridgeReturnSubstitutable(actual, ret, f.resolve, work) {
			return false
		}
	}
	return matches == 1
}

// Empty generic Signature throws does not assert an empty Exceptions attribute.
// This certificate grants no checked-exception adaptation in either source view.
func nativeAnonymousBridgeNoThrows(method *MemberInfo, work *workbudget.Budget) bool {
	if method == nil {
		return false
	}
	for _, a := range method.Attributes {
		if sourceProofNil(a) || !nativeProofWork(work, 1) {
			return false
		}
		if _, exceptions := a.(*ExceptionsAttribute); exceptions {
			return false
		}
	}
	return true
}
