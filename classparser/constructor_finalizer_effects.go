package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core"

// A constructor can initialize a further subclass. Finalizer dispatch is closed
// by a final allocated class or by the nearest original final override, including
// an inherited one. Prove that selected body empty, then validate the complete
// ancestry: an overridden parent body is not invoked implicitly, but overriding
// a final parent method cannot establish valid original dispatch evidence.
// The bootstrap Object finalizer must also be witnessed as an empty return.
// JLS 12.6/12.6.1: https://docs.oracle.com/javase/specs/jls/se8/html/jls-12.html#jls-12.6
// This removes only finalizer observation; the effect interpreter must still
// reject every publication, alias and moved-field observation of THIS.
func (c *ClassObjectDumper) constructorReceiverCannotObserveFinalization(remaining *int) bool {
	if c == nil || c.obj == nil || remaining == nil || c.obj.AccessFlags&0x0200 != 0 {
		return false
	}
	finalClass := c.obj.AccessFlags&0x0010 != 0
	selected := false
	obj := c.obj
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth <= 16; depth++ {
		*remaining--
		name := obj.GetClassName()
		if *remaining <= 0 || name == "" || seen[name] || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(seen)+1)*64) != nil {
			return false
		}
		seen[name] = true
		matches := 0
		var finalizer *MemberInfo
		for _, method := range obj.Methods {
			if method == nil || !nativeProofWork(c.Work, 1) {
				return false
			}
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
			if matches != 1 {
				return false
			}
			flags := finalizer.AccessFlags
			visibility := flags & 0x0007
			if visibility != 0x0001 && visibility != 0x0004 || flags&0x0008 != 0 {
				return false
			}
			if !selected {
				if !finalClass && flags&0x0010 == 0 || !constructorFinalizerEmpty(finalizer) {
					return false
				}
				selected = true
			} else if flags&0x0010 != 0 {
				return false
			}
		}
		if name == "java/lang/Object" {
			return selected && matches == 1 && constructorFinalizerEmpty(finalizer)
		}
		var ok bool
		obj, ok = c.constructorMotionClass(obj.GetSupperClassName())
		if !ok {
			return false
		}
	}
	return false
}

func constructorFinalizerEmpty(method *MemberInfo) bool {
	if method == nil || method.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 {
		return false
	}
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		if candidate, ok := attribute.(*CodeAttribute); ok {
			if candidate == nil || code != nil {
				return false
			}
			code = candidate
		}
	}
	return code != nil && code.MaxLocals >= 1 && len(code.ExceptionTable) == 0 && len(code.Code) == 1 && code.Code[0] == core.OP_RETURN
}
