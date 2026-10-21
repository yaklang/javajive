package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// JVMS5.4.3.5/5.4.4 allow a class's own lookup to retain its directly
// declared private ordinary method. This is distinct from foreign/nestmate
// privilege and from changing a constructor protocol. Corroborate the indexed
// observer with the actual original CP and that exact root's declaration view.
// Body consumption, constructor access and final family metadata stay separate.
func nativeMemberOwnPrivateHandleSourceClosed(object *ClassObject, source *nativeMemberClass, method *MemberInfo, target nativeMemberHandleTarget, work *workbudget.Budget) bool {
	if object == nil || method == nil || method.AccessFlags&7 != 2 || method.AccessFlags&(0x0040|0x1000|0x0400) != 0 || object.AccessFlags&0x0200 != 0 || source == nil || source.object != object || source.lambdaContext.resolve == nil || source.lambdaContext.metadata == nil || len(object.ConstantPool) > 65535 || len(object.Methods) > 4096 || !nativeProofWork(work, 1) {
		return false
	}
	owner := object.GetClassName()
	if target.referencer != owner || !target.methodRef || target.name == "<init>" || target.name == "<clinit>" || !class_context.PreservesJava8Identifier(target.name) || target.kind != 5 && target.kind != 6 && target.kind != 7 || (method.AccessFlags&8 != 0) != (target.kind == 6) || !nativeMemberTopLevelEvidence(object, work) {
		return false
	}
	if _, _, err := callbinding.Descriptor(target.descriptor); err != nil {
		return false
	}
	name, named := sourceBridgeUTF8(object, method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
	if !named || !typed || name != target.name || descriptor != target.descriptor {
		return false
	}
	owned := 0
	for _, original := range object.Methods {
		if !nativeProofWork(work, 1) {
			return false
		}
		if original == method {
			owned++
		}
	}
	if owned != 1 {
		return false
	}
	// The declaration provider may materialize the complete original table.
	// Charge its known bound before invoking it, even for direct callers.
	if work != nil && work.CheckAlloc(int64(len(object.Methods))*96+128) != nil {
		return false
	}
	resolved, known := source.lambdaContext.resolve(owner)
	if !known || resolved != object {
		return false
	}
	for _, attribute := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if _, generic := attribute.(*SignatureAttribute); generic {
			return false
		}
	}
	metadata, known := source.lambdaContext.metadata(owner)
	if !known || metadata.Name != owner || metadata.IsInterface || !metadata.MembersComplete || len(metadata.Methods) > 4096 || !nativeProofWork(work, int64(len(metadata.Methods))+1) {
		return false
	}
	declarations := 0
	for _, declaration := range metadata.Methods {
		if declaration.Name != target.name || declaration.Desc != target.descriptor {
			continue
		}
		if declaration.Public || declaration.Generic || declaration.Bridge || declaration.Static != (method.AccessFlags&8 != 0) || declaration.Varargs != (method.AccessFlags&0x0080 != 0) {
			return false
		}
		declarations++
	}
	if declarations != 1 {
		return false
	}
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		handle, ok := constant.(*ConstantMethodHandleInfo)
		if !ok || handle == nil || handle.ReferenceKind != target.kind {
			continue
		}
		original, known := nativeLambdaOriginalHandleTarget(object, handle)
		if known && !original.interfaceRef && original.owner == owner && original.name == target.name && original.descriptor == target.descriptor {
			return true
		}
	}
	return false
}
