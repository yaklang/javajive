package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Ordinary references have a canonical typed source, but no synthetic body
// request. Retain their actual factory separately from neighboring lambdas.
// This receipt authorizes neither a new call binding nor changed eager order.
type interfaceInitializerMethodReference struct {
	factory  *lambdaSourceFactory
	value    *values.CustomValue
	identity values.MethodReferenceSource
}

func (c *ClassObjectDumper) interfaceInitializerOriginalMethodReference(value *values.CustomValue, code *CodeAttribute, pc int) *interfaceInitializerMethodReference {
	identity, known := value.UncapturedMethodReferenceSource()
	if !known {
		return nil
	}
	factory := &lambdaSourceFactory{object: c.obj, method: c.CurrentMethod, code: code, pc: pc}
	if !factory.seal(c.Work) {
		return nil
	}
	receipt := &interfaceInitializerMethodReference{factory: factory, value: value, identity: identity}
	if !receipt.closed(c) {
		return nil
	}
	return receipt
}

func (receipt *interfaceInitializerMethodReference) closed(c *ClassObjectDumper) bool {
	if c == nil || receipt == nil || receipt.value == nil || !receipt.factory.closed() || !receipt.factory.sealed || receipt.factory.object != c.obj || !nativeProofWork(c.Work, 1) {
		return false
	}
	factory := receipt.factory
	pc, captures, known := receipt.value.OriginalLambdaFactoryEvaluation()
	identity, sourceKnown := receipt.value.UncapturedMethodReferenceSource()
	if !known || !sourceKnown || pc != factory.pc || len(captures) != 0 || identity != receipt.identity || pc+5 > len(factory.code.Code) {
		return false
	}
	op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKEDYNAMIC}, CurrentOffset: uint16(pc), Data: factory.code.Code[pc+1 : pc+5]}
	remaining := 4096
	descriptor, known := c.constructorReceiverDynamicDescriptor(c.obj, factory.code, op, &remaining)
	if !known || descriptor != identity.CallDescriptor {
		return false
	}
	constant := func(index uint16) ConstantInfo {
		if index == 0 || int(index) > len(c.obj.ConstantPool) {
			return nil
		}
		return c.obj.ConstantPool[index-1]
	}
	site, valid := constant(binary.BigEndian.Uint16(op.Data[:2])).(*ConstantInvokeDynamicInfo)
	if !valid || site == nil {
		return false
	}
	nameType, valid := constant(site.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
	if !valid || nameType == nil {
		return false
	}
	name, known := sourceBridgeUTF8(c.obj, nameType.NameIndex)
	if !known || name != identity.CallName {
		return false
	}
	var table *BootstrapMethodsAttribute
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if candidate, ok := attribute.(*BootstrapMethodsAttribute); ok {
			if candidate == nil || table != nil {
				return false
			}
			table = candidate
		}
	}
	if table == nil || int(table.NumBootstrapMethods) != len(table.BootstrapMethods) || int(site.BootstrapMethodAttrIndex) >= len(table.BootstrapMethods) {
		return false
	}
	bootstrap := table.BootstrapMethods[site.BootstrapMethodAttrIndex]
	if bootstrap == nil || bootstrap.NumBootstrapArguments != 3 || len(bootstrap.BootstrapArguments) != 3 {
		return false
	}
	handle, valid := constant(bootstrap.BootstrapMethodRef).(*ConstantMethodHandleInfo)
	target, known := nativeLambdaOriginalHandleTarget(c.obj, handle)
	if !valid || !known || handle.ReferenceKind != 6 || target.interfaceRef || target.owner != "java/lang/invoke/LambdaMetafactory" || target.name != "metafactory" || target.descriptor != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
		return false
	}
	implementation, valid := constant(bootstrap.BootstrapArguments[1]).(*ConstantMethodHandleInfo)
	target, known = nativeLambdaOriginalHandleTarget(c.obj, implementation)
	if !valid || !known || implementation.ReferenceKind != identity.Kind || target.owner != identity.Owner || target.name != identity.Member || target.descriptor != identity.Descriptor {
		return false
	}
	for index, expected := range map[int]string{0: identity.ErasedSAM, 2: identity.Instantiated} {
		methodType, valid := constant(bootstrap.BootstrapArguments[index]).(*ConstantMethodTypeInfo)
		if !valid || methodType == nil {
			return false
		}
		descriptor, known := sourceBridgeUTF8(c.obj, methodType.DescriptorIndex)
		if !known || descriptor != expected || !nativeProofWork(c.Work, int64(len(descriptor))+1) {
			return false
		}
	}
	return true
}
