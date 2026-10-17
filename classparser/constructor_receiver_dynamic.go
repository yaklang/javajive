package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"strings"
)

// This receipt binds the original typed call site and selected bootstrap
// metadata. It does not predict successful linkage, interpret a bootstrap,
// whitelist a library or grant a source-lowering capability. The enclosing
// receiver-effect domain keeps the opaque operation and its failure in place;
// only receiver-free operands may cross its unpublished-THIS boundary.
func (c *ClassObjectDumper) constructorReceiverDynamicDescriptor(obj *ClassObject, code *CodeAttribute, op *core.OpCode, remaining *int) (string, bool) {
	if c == nil || obj == nil || code == nil || remaining == nil || *remaining <= 0 || len(obj.Attributes) > 65535 || obj.MajorVersion < 51 || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEDYNAMIC || op.IsWide || len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 || !nativeProofWork(c.Work, 1) {
		return "", false
	}
	// Attribute, parameter and bootstrap scans share the original chain bound.
	// A small opcode body cannot hide an arbitrarily large metadata proof.
	*remaining -= len(obj.Attributes) + 1
	if *remaining < 0 {
		return "", false
	}
	pc := int(op.CurrentOffset)
	if pc < 0 || pc+5 > len(code.Code) || int(code.Code[pc]) != core.OP_INVOKEDYNAMIC {
		return "", false
	}
	for i := 0; i < 4; i++ {
		if code.Code[pc+1+i] != op.Data[i] {
			return "", false
		}
	}
	index := int(core.Convert2bytesToInt(op.Data[:2]))
	if index < 1 || index > len(obj.ConstantPool) {
		return "", false
	}
	site, ok := obj.ConstantPool[index-1].(*ConstantInvokeDynamicInfo)
	if !ok || site == nil || site.NameAndTypeIndex == 0 || int(site.NameAndTypeIndex) > len(obj.ConstantPool) {
		return "", false
	}
	nt, ok := obj.ConstantPool[site.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return "", false
	}
	name, nk := sourceBridgeUTF8(obj, nt.NameIndex)
	descriptor, dk := sourceBridgeUTF8(obj, nt.DescriptorIndex)
	if !nk || !dk || name == "" || strings.ContainsAny(name, ".;[/<>") || len(descriptor) > 65535 || !nativeProofWork(c.Work, int64(len(name)+len(descriptor))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(descriptor))*8+128) != nil {
		return "", false
	}
	parameters, _, err := callbinding.Descriptor(descriptor)
	if err != nil {
		return "", false
	}
	*remaining -= len(parameters)
	if *remaining < 0 {
		return "", false
	}
	words := 0
	for _, parameter := range parameters {
		words += constructorEffectType(parameter).width()
		if words > 255 {
			return "", false
		}
	}
	var table *BootstrapMethodsAttribute
	for _, a := range obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			if table != nil || b == nil {
				return "", false
			}
			table = b
		}
	}
	if table == nil || int(table.NumBootstrapMethods) != len(table.BootstrapMethods) || int(site.BootstrapMethodAttrIndex) >= len(table.BootstrapMethods) {
		return "", false
	}
	boot := table.BootstrapMethods[site.BootstrapMethodAttrIndex]
	if boot == nil || int(boot.NumBootstrapArguments) != len(boot.BootstrapArguments) || boot.BootstrapMethodRef == 0 || int(boot.BootstrapMethodRef) > len(obj.ConstantPool) {
		return "", false
	}
	h, ok := obj.ConstantPool[boot.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
	if !ok || h == nil || h.ReferenceKind != 6 && h.ReferenceKind != 8 {
		return "", false
	}
	opcode := core.OP_INVOKESTATIC
	if h.ReferenceKind == 8 {
		opcode = core.OP_INVOKESPECIAL
	}
	member := constructorMotionMember(obj, &core.OpCode{Instr: &core.Instruction{OpCode: opcode}, Data: []byte{byte(h.ReferenceIndex >> 8), byte(h.ReferenceIndex)}}, opcode)
	if member == nil || member.Member == "<clinit>" || h.ReferenceKind == 6 && member.Member == "<init>" || h.ReferenceKind == 8 && member.Member != "<init>" {
		return "", false
	}
	if _, r, e := callbinding.Descriptor(member.Description); e != nil || h.ReferenceKind == 8 && r != "V" {
		return "", false
	}
	*remaining -= len(boot.BootstrapArguments)
	if *remaining < 0 {
		return "", false
	}
	for _, arg := range boot.BootstrapArguments {
		if !nativeProofWork(c.Work, 1) || arg == 0 || int(arg) > len(obj.ConstantPool) || sourceProofNil(obj.ConstantPool[arg-1]) {
			return "", false
		}
		switch obj.ConstantPool[arg-1].(type) {
		case *ConstantIntegerInfo, *ConstantFloatInfo, *ConstantLongInfo, *ConstantDoubleInfo, *ConstantStringInfo, *ConstantClassInfo, *ConstantMethodHandleInfo, *ConstantMethodTypeInfo:
		default:
			return "", false
		}
	}
	return descriptor, true
}
