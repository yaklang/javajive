package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Raw NEW carries no source type arguments. Its independently bound allocation
// can nevertheless supply a source supertype by the raw-inheritance rule. Keep
// the initializer and all constructor operands unchanged; a local's original
// synthetic capture descriptor constrains the requested erasure. The enclosing
// declaration proof still checks every identity copy before committing a view.
func (c *ClassObjectDumper) nativeCapturedRawAllocationType(declaration *statements.AssignStatement, actual, expected string, provider callbinding.Provider) (types.JavaType, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || declaration == nil || provider == nil {
		return nil, false
	}
	allocation, ok := declaration.JavaValue.(*values.NewExpression)
	if !ok || allocation == nil || allocation.JavaType == nil || !allocation.HasOriginPC || allocation.ConstructorCall == nil {
		return nil, false
	}
	if _, solved := types.AsParameterizedType(allocation.JavaType); solved {
		return nil, false
	}
	call := allocation.ConstructorCall
	receiver, known := nativeMemberEnclosingUnpack(call.Object, c.Work)
	if !known || receiver != allocation {
		return nil, false
	}
	owner := callbinding.Name(actual)
	if call.FunctionName != "<init>" || call.IsStatic || call.Kind != values.InvokeSpecial || !call.HasOriginPC || strings.ReplaceAll(call.ClassName, ".", "/") != owner {
		return nil, false
	}
	if erased, known := values.SourceTypeErasure(allocation.JavaType, c.FuncCtx); !known || erased != actual {
		return nil, false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	definition, known := resolve(owner)
	if !known || definition == nil || definition.GetClassName() != owner || !nativeMemberTopLevelEvidence(definition, c.Work) || definition.AccessFlags&1 == 0 && nativeBinaryPackage(owner) != nativeBinaryPackage(c.obj.GetClassName()) {
		return nil, false
	}
	var original *MemberInfo
	var code *CodeAttribute
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
		desc, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
		if !nk || !dk {
			return nil, false
		}
		if name != c.FuncCtx.FunctionName || desc != c.FuncCtx.CurrentMethodDesc {
			continue
		}
		if original != nil {
			return nil, false
		}
		original = method
		for _, attribute := range method.Attributes {
			if body, ok := attribute.(*CodeAttribute); ok {
				if body == nil || code != nil {
					return nil, false
				}
				code = body
			}
		}
	}
	bindings, known := c.nativeMemberAllocationInvocations(original, code)
	binding, found := bindings[allocation.OriginPC]
	if !known || !found || binding.pc != call.OriginPC || binding.owner != owner || binding.descriptor != call.Descriptor {
		return nil, false
	}
	correlated := func(name string) (callbinding.Class, bool) {
		meta, known := provider(name)
		if !known {
			return callbinding.Class{}, false
		}
		if _, agreed := nativeAnnotationOriginalDeclaration(name, c.Work, resolve, func(string) (callbinding.Class, bool) { return meta, true }); !agreed {
			return callbinding.Class{}, false
		}
		if original, available := resolve(name); available {
			signature, valid := nativeMethodLocalOriginalSignature(original, original.Attributes, c.Work)
			if !valid || signature != meta.Signature || meta.IsInterface != (original.AccessFlags&0x0200 != 0) || meta.Public != (original.AccessFlags&1 != 0 || strings.ReplaceAll(nativeBinaryPackage(name), "/", ".") == c.PackageName) {
				return callbinding.Class{}, false
			}
		}
		return meta, true
	}
	return types.ProjectRawAllocationSupertype(c.FuncCtx, allocation.JavaType, callbinding.Name(expected), correlated)
}
