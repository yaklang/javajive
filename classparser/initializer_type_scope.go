package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// EnclosingMethod's zero method index identifies a class initializer context.
// Resolve only its type-variable static boundary from original creation sites.
// Several constructors can contain copies of the SAME instance initializer;
// this grants no source placement, effect ordering, access or capture proof.
func originalInitializerTypeScopeStatic(owner, child *ClassObject, work *workbudget.Budget) (bool, bool) {
	if owner == nil || child == nil || child.MajorVersion < 49 {
		return false, false
	}
	if owner.GetClassName() == child.GetClassName() || work != nil && work.CheckAlloc(int64(len(owner.Methods)+len(child.Methods))*128) != nil {
		return false, false
	}
	attributes := 0
	for _, a := range child.Attributes {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" {
			attributes++
			if attributes != 1 || raw.Length != 4 || len(raw.Info) != 4 || binary.BigEndian.Uint16(raw.Info[2:]) != 0 {
				return false, false
			}
			name, known := sourceBridgeClassName(child, binary.BigEndian.Uint16(raw.Info[:2]))
			if !known || name != owner.GetClassName() {
				return false, false
			}
		}
	}
	if attributes != 1 {
		return false, false
	}
	constructors := map[string]bool{}
	for _, m := range child.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false, false
		}
		n, nk := sourceBridgeUTF8(child, m.NameIndex)
		d, dk := sourceBridgeUTF8(child, m.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(work, int64(len(n)+len(d))) {
			return false, false
		}
		if n != "<init>" {
			continue
		}
		_, ret, err := callbinding.Descriptor(d)
		if err != nil || ret != "V" || m.AccessFlags&StaticFlag != 0 || constructors[d] {
			return false, false
		}
		constructors[d] = true
	}
	if len(constructors) == 0 {
		return false, false
	}
	static, contexts := false, 0
	seen := map[string]bool{}
	for _, m := range owner.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false, false
		}
		n, nk := sourceBridgeUTF8(owner, m.NameIndex)
		d, dk := sourceBridgeUTF8(owner, m.DescriptorIndex)
		if !nk || !dk || seen[n+d] || !nativeProofWork(work, int64(len(n)+len(d))) {
			return false, false
		}
		seen[n+d] = true
		var code *CodeAttribute
		for _, a := range m.Attributes {
			if !nativeProofWork(work, 1) {
				return false, false
			}
			if c, ok := a.(*CodeAttribute); ok {
				if c == nil || code != nil {
					return false, false
				}
				code = c
			}
		}
		if code == nil {
			continue
		}
		if !nativeProofWork(work, int64(len(code.Code))) {
			return false, false
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(owner.ConstantPool, i) })
		decoder.Work = work
		if decoder.ParseOpcode() != nil {
			return false, false
		}
		news, calls := 0, 0
		for _, op := range constructorMotionOps(decoder) {
			if !nativeProofWork(work, 1) {
				return false, false
			}
			if op.Instr.OpCode == core.OP_NEW {
				if len(op.Data) != 2 {
					return false, false
				}
				target, known := sourceBridgeClassName(owner, core.Convert2bytesToInt(op.Data))
				if !known {
					return false, false
				}
				if target == child.GetClassName() {
					news++
				}
			}
			if ref := constructorMotionMember(owner, op, core.OP_INVOKESPECIAL); ref != nil && ref.Name == child.GetClassName() && ref.Member == "<init>" {
				if !constructors[ref.Description] {
					return false, false
				}
				calls++
			}
		}
		if news == 0 && calls == 0 {
			continue
		}
		_, ret, err := callbinding.Descriptor(d)
		isStatic := m.AccessFlags&StaticFlag != 0
		if news == 0 || news != calls || err != nil || ret != "V" || n != "<init>" && n != "<clinit>" || isStatic != (n == "<clinit>") || n == "<clinit>" && d != "()V" || contexts > 0 && static != isStatic {
			return false, false
		}
		static = isStatic
		contexts++
	}
	return static, contexts > 0
}
