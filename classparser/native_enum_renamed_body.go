package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// renamedEnumConstantBody certifies a constant-specific class whose binary
// name is not Owner$N and whose InnerClasses/EnclosingMethod rows were
// removed. Javac 8 still emits the access bridge <init>(String, int, Marker),
// and every such body forwards to it with a null marker. The marker class is
// the first body. A present enclosing row stays on the strict anonymous
// certificate; this path does not override a contradictory one.
func (c *ClassObjectDumper) renamedEnumConstantBody(body *ClassObject) (marker, descriptor string, ok bool) {
	if c == nil || c.obj == nil || body == nil || !nativeProofWork(c.Work, 1) {
		return "", "", false
	}
	owner := c.obj.GetClassName()
	if body.AccessFlags != 0x4030 || body.GetSupperClassName() != owner || len(body.Fields) != 0 || len(body.Interfaces) != 0 {
		return "", "", false
	}
	for _, attribute := range body.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return "", "", false
		}
		switch attribute := attribute.(type) {
		case *SourceFileAttribute:
			if attribute == nil {
				return "", "", false
			}
		default:
			return "", "", false
		}
	}
	var ctor *MemberInfo
	for _, method := range body.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return "", "", false
		}
		name, named := sourceBridgeUTF8(body, method.NameIndex)
		desc, described := sourceBridgeUTF8(body, method.DescriptorIndex)
		if !named || !described || name == "<clinit>" {
			return "", "", false
		}
		if name != "<init>" {
			continue
		}
		if ctor != nil || desc != "(Ljava/lang/String;I)V" || method.AccessFlags != 0 {
			return "", "", false
		}
		ctor = method
	}
	if ctor == nil {
		return "", "", false
	}
	var code *CodeAttribute
	for _, attribute := range ctor.Attributes {
		switch attribute := attribute.(type) {
		case *CodeAttribute:
			if attribute == nil || code != nil {
				return "", "", false
			}
			code = attribute
		case *UnparsedAttribute:
			if attribute == nil || attribute.Name != "MethodParameters" || !renamedEnumBridgeParameters(attribute) {
				return "", "", false
			}
		default:
			return "", "", false
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 64 || code.MaxLocals != 3 || code.MaxStack != 4 || !nativeProofWork(c.Work, int64(len(code.Code))) {
		return "", "", false
	}
	for _, attribute := range code.Attributes {
		switch attribute := attribute.(type) {
		case *LineNumberTableAttribute:
			if attribute == nil {
				return "", "", false
			}
		case *UnparsedAttribute:
			if attribute == nil || attribute.Name != "LocalVariableTable" {
				return "", "", false
			}
		default:
			return "", "", false
		}
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return "", "", false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != 6 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[1], "Ljava/lang/String;") || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[2], "I") || core.GetRetrieveIdx(ops[2]) != 2 || !nativeEnumOpcode(ops[3], core.OP_ACONST_NULL) || !nativeEnumOpcode(ops[5], core.OP_RETURN) {
		return "", "", false
	}
	target := constructorMotionMember(body, ops[4], core.OP_INVOKESPECIAL)
	if target == nil || target.Name != owner || target.Member != "<init>" || !nativeEnumMemberOperand(body, ops[4], core.OP_INVOKESPECIAL, owner, "<init>", target.Description) {
		return "", "", false
	}
	bridge := c.originalNativeConstructorAccessBridges()[target.Description]
	if bridge == nil || bridge.target != "(Ljava/lang/String;I)V" || bridge.marker == "" {
		return "", "", false
	}
	return bridge.marker, target.Description, true
}

func renamedEnumBridgeParameters(attribute *UnparsedAttribute) bool {
	if attribute.Length != 9 || len(attribute.Info) != 9 || attribute.Info[0] != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		packet := attribute.Info[1+4*i : 5+4*i]
		if packet[0] != 0 || packet[1] != 0 || packet[3] != 0 || (packet[2] != 0 && packet[2] != 0x10) {
			return false
		}
	}
	return true
}
