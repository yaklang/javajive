package javaclassparser

// A method handle's reference kind and constant-pool tag jointly describe its
// invocation. In particular, JVMS 4.4.8 permits interface references for static
// and special handles starting at classfile 52; virtual handles remain class
// references, while interface handles require interface references.
type nativeLambdaHandleTarget struct {
	owner, name, descriptor string
	interfaceRef            bool
}

func nativeLambdaOriginalHandleTarget(object *ClassObject, handle *ConstantMethodHandleInfo) (nativeLambdaHandleTarget, bool) {
	var target nativeLambdaHandleTarget
	if object == nil || handle == nil || object.MajorVersion < 51 || handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(object.ConstantPool) {
		return target, false
	}
	var ref *ConstantMemberrefInfo
	switch entry := object.ConstantPool[handle.ReferenceIndex-1].(type) {
	case *ConstantMethodrefInfo:
		if entry != nil {
			ref = &entry.ConstantMemberrefInfo
		}
	case *ConstantInterfaceMethodrefInfo:
		if entry != nil {
			ref = &entry.ConstantMemberrefInfo
			target.interfaceRef = true
		}
	}
	if ref == nil {
		return target, false
	}
	switch handle.ReferenceKind {
	case 5, 8:
		if target.interfaceRef {
			return target, false
		}
	case 6, 7:
		if target.interfaceRef && object.MajorVersion < 52 {
			return target, false
		}
	case 9:
		if !target.interfaceRef {
			return target, false
		}
	default:
		return target, false
	}
	var known bool
	target.owner, known = sourceBridgeClassName(object, ref.ClassIndex)
	if !known || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
		return target, false
	}
	nt, valid := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !valid || nt == nil {
		return target, false
	}
	var named, typed bool
	target.name, named = sourceBridgeUTF8(object, nt.NameIndex)
	target.descriptor, typed = sourceBridgeUTF8(object, nt.DescriptorIndex)
	if !named || !typed || target.name == "<clinit>" || (target.name == "<init>") != (handle.ReferenceKind == 8) {
		return target, false
	}
	return target, true
}

// This is only invocation-shape validation for an already identified original
// declaration. The caller must still prove private synthetic ownership, exact
// metafactory use, and absence of direct calls or method-handle data uses.
func nativeLambdaImplementationHandle(object *ClassObject, method *MemberInfo, handle *ConstantMethodHandleInfo, target nativeLambdaHandleTarget) bool {
	if handle == nil {
		return false
	}
	return nativeLambdaImplementationKind(object, method, handle.ReferenceKind, target.interfaceRef)
}

// The original constant-pool reader and the archive's typed handle inventory
// must agree on class/interface reference tags. A factory-owned private body
// is validated before ordinary public interface-method rules are considered.
func nativeLambdaImplementationKind(object *ClassObject, method *MemberInfo, kind uint8, interfaceRef bool) bool {
	if object == nil || method == nil || interfaceRef != (object.AccessFlags&0x0200 != 0) {
		return false
	}
	if method.AccessFlags&StaticFlag != 0 {
		return kind == 6
	}
	if interfaceRef {
		return kind == 7 || kind == 9
	}
	return kind == 5 || kind == 7
}
