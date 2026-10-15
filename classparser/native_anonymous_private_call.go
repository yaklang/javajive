package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An anonymous receiver cannot be named by casting to its binary class name.
// A lexical call needs its own certificate, rather than the field-read one:
// re-prove the original nonvirtual bridge, complete target declaration and
// physical receiver descriptor. Operand/source binding is a separate proof;
// generic/inherited/static packets are not granted here.
func nativeAnonymousPrivateCallAccessorDeclaration(f *nativeAnonymousForest, obj *ClassObject, method *MemberInfo, work *workbudget.Budget) bool {
	if f == nil || f.members == nil || obj == nil || method == nil ||
		f.objects[obj.GetClassName()] != obj || f.units[obj.GetClassName()] == nil ||
		f.units[obj.GetClassName()].object != obj || work != nil && work.CheckAlloc(512) != nil {
		return false
	}
	name, nameKnown := sourceBridgeUTF8(obj, method.NameIndex)
	desc, descKnown := sourceBridgeUTF8(obj, method.DescriptorIndex)
	if !nameKnown || !descKnown {
		return false
	}
	g := f.members.getters[nativeMemberGetterKey(obj.GetClassName(), name, desc)]
	if g == nil || g.method != method || g.call == nil || g.setter || g.update != nil ||
		g.staticField || g.genericField || g.inheritedField || !class_context.PreservesJava8Identifier(g.field) {
		return false
	}
	fresh := nativeMemberCallPacketProof(obj, method, f.resolve, work, f.objects)
	if fresh == nil || fresh.call == nil || fresh.owner != g.owner || fresh.name != g.name ||
		fresh.descriptor != g.descriptor || fresh.field != g.field || fresh.fieldDescriptor != g.fieldDescriptor ||
		fresh.ordinal != g.ordinal || fresh.call.argumentCount != g.call.argumentCount ||
		fresh.call.methodFormalCount != g.call.methodFormalCount || fresh.call.static != g.call.static ||
		fresh.call.inherited != g.call.inherited || fresh.call.rawGeneric != g.call.rawGeneric ||
		len(fresh.call.rawLexicalOwner) != 0 || len(g.call.rawLexicalOwner) != 0 ||
		g.call.argumentCount < 1 || g.call.argumentCount > 128 || g.call.methodFormalCount != 0 || g.call.static || g.call.inherited || g.call.rawGeneric {
		return false
	}
	parameters, result, err := callbinding.Descriptor(desc)
	targetParameters, targetResult, targetErr := callbinding.Descriptor(g.fieldDescriptor)
	if err != nil || targetErr != nil || len(parameters) != g.call.argumentCount || parameters[0] != "L"+obj.GetClassName()+";" ||
		len(targetParameters)+1 != len(parameters) || result != targetResult {
		return false
	}
	for i, parameter := range targetParameters {
		if parameters[i+1] != parameter {
			return false
		}
	}
	for owned := range f.units {
		if !nativeProofWork(work, int64(len(result))+1) || strings.Contains(result, "L"+owned+";") {
			return false
		}
		for i, parameter := range parameters {
			if !nativeProofWork(work, int64(len(parameter))+1) || i > 0 && strings.Contains(parameter, "L"+owned+";") {
				return false
			}
		}
	}
	return true
}
