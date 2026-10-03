package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// javac accessibility bridges expose a public method inherited from a hidden
// superclass. The dumper omits bridges, so source lookup sees that superclass's
// generic declaration. Original JVM metadata remains untouched. Only an exact
// receiver-only, same-descriptor invokespecial forwarder supplies this separate
// source-view evidence; arbitrary bridges or rewritten bodies cannot supply it.
func (c *ClassObjectDumper) buildSourceBridgeTargets() func(string, string, string) (string, bool) {
	cache := map[string]map[string]string{}
	return func(owner, name, descriptor string) (string, bool) {
		methods, seen := cache[owner]
		if !seen {
			methods = map[string]string{}
			cache[owner] = methods
			if obj, ok := c.constructorMotionClass(owner); ok {
				counts := map[string]int{}
				for _, method := range obj.Methods {
					n, err := obj.getUtf8(method.NameIndex)
					if err != nil {
						continue
					}
					d, err := obj.getUtf8(method.DescriptorIndex)
					if err != nil {
						continue
					}
					key := class_context.MethodDescKey(n, d)
					counts[key]++
					if target, proved := sourceBridgeGetterTarget(obj, method); proved {
						methods[key] = target
					}
				}
				for key, count := range counts {
					if count != 1 {
						delete(methods, key)
					}
				}
			}
		}
		target, ok := methods[class_context.MethodDescKey(name, descriptor)]
		return target, ok
	}
}

func sourceBridgeGetterTarget(obj *ClassObject, method *MemberInfo) (string, bool) {
	if obj == nil || method == nil || method.AccessFlags != 0x1041 {
		return "", false
	}
	name, nerr := obj.getUtf8(method.NameIndex)
	desc, derr := obj.getUtf8(method.DescriptorIndex)
	if nerr != nil || derr != nil || name == "" || strings.HasPrefix(name, "<") || !strings.HasPrefix(desc, "()L") || !strings.HasSuffix(desc, ";") {
		return "", false
	}
	if params, result, err := callbinding.Descriptor(desc); err != nil || len(params) != 0 || !callbinding.Reference(result) {
		return "", false
	}
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		switch attr := attribute.(type) {
		case *SignatureAttribute:
			return "", false // An explicit declaration remains authoritative.
		case *CodeAttribute:
			if code != nil {
				return "", false
			}
			code = attr
		}
	}
	if code == nil || code.MaxLocals != 1 || code.MaxStack != 1 || len(code.ExceptionTable) != 0 || len(code.Code) != 5 || code.Code[0] != core.OP_ALOAD_0 || code.Code[1] != core.OP_INVOKESPECIAL || code.Code[4] != core.OP_ARETURN {
		return "", false
	}
	cp := NewConstantPoolWithConstant(&obj.ConstantPool)
	ref, ok := cp.IndexInfo(int(binary.BigEndian.Uint16(code.Code[2:4]))).(*ConstantMethodrefInfo)
	if !ok {
		return "", false
	}
	nt, ok := cp.IndexInfo(int(ref.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
	if !ok {
		return "", false
	}
	n, ne := obj.getUtf8(nt.NameIndex)
	d, de := obj.getUtf8(nt.DescriptorIndex)
	parent := cp.GetClassName(int(ref.ClassIndex))
	if ne != nil || de != nil || n != name || d != desc || parent == "" || parent != obj.GetSupperClassName() || parent == obj.GetClassName() {
		return "", false
	}
	return parent, true
}
