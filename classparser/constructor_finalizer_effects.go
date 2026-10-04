package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core"

// A constructor can initialize a further subclass, so absence of finalize in
// a non-final class does not close its receiver behavior. Require original
// ACC_FINAL and complete original ancestry, with no finalizer override. The
// bootstrap Object finalizer must also be witnessed as an empty return.
// JLS 12.6/12.6.1: https://docs.oracle.com/javase/specs/jls/se8/html/jls-12.html#jls-12.6
// This removes only finalizer observation; the effect interpreter must still
// reject every publication, alias and moved-field observation of THIS.
func (c *ClassObjectDumper) constructorReceiverCannotObserveFinalization(remaining *int) bool {
	if c.obj == nil || c.obj.AccessFlags&0x0010 == 0 {
		return false
	}
	obj := c.obj
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth <= 16; depth++ {
		*remaining--
		name := obj.GetClassName()
		if *remaining <= 0 || seen[name] {
			return false
		}
		seen[name] = true
		matches := 0
		var finalizer *MemberInfo
		for _, method := range obj.Methods {
			n, err := obj.getUtf8(method.NameIndex)
			if err != nil {
				return false
			}
			if n != "finalize" {
				continue
			}
			desc, err := obj.getUtf8(method.DescriptorIndex)
			if err != nil {
				return false
			}
			if desc == "()V" {
				matches++
				finalizer = method
			}
		}
		if matches != 0 {
			if name != "java/lang/Object" || matches != 1 || finalizer.AccessFlags&(0x0008|0x0100|0x0400) != 0 {
				return false
			}
			var code *CodeAttribute
			for _, attribute := range finalizer.Attributes {
				if candidate, ok := attribute.(*CodeAttribute); ok {
					if code != nil {
						return false
					}
					code = candidate
				}
			}
			return code != nil && code.MaxLocals >= 1 && len(code.ExceptionTable) == 0 && len(code.Code) == 1 && code.Code[0] == core.OP_RETURN
		}
		if name == "java/lang/Object" {
			return false
		}
		var ok bool
		obj, ok = c.constructorMotionClass(obj.GetSupperClassName())
		if !ok {
			return false
		}
	}
	return false
}
