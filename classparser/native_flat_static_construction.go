package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A terminal flat declaration never completed a native family's indexed-caller
// closure. Check its original constructor accesses before another family relies
// on that declaration. Source nesting must not erase a valid IllegalAccessError
// or invent a missing constructor. An unused CP reference is not an invocation;
// constructor handles keep the archive's conservative original-handle boundary.
func (z *JarFS) flatStaticOriginalConstructorAccessClosed(prepared *nativeMemberPrepared, target *ClassObject) bool {
	if z == nil || prepared == nil || prepared.reader == nil || target == nil || len(target.Methods) > 65535 {
		return false
	}
	d := prepared.reader
	constructors := map[string]uint16{}
	for _, method := range target.Methods {
		if method == nil || !nativeProofWork(d.Work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(target, method.NameIndex)
		descriptor, dk := sourceBridgeUTF8(target, method.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(d.Work, int64(len(name)+len(descriptor))+1) {
			return false
		}
		if name != "<init>" {
			continue
		}
		_, result, err := callbinding.Descriptor(descriptor)
		access := method.AccessFlags & 7
		if err != nil || result != "V" || access != 0 && access != 1 && access != 2 && access != 4 || method.AccessFlags&0x0528 != 0 {
			return false
		}
		if _, duplicate := constructors[descriptor]; duplicate || len(constructors) >= 256 || d.Work != nil && d.Work.CheckAlloc(int64(len(constructors)+1)*128+int64(len(descriptor))) != nil {
			return false
		}
		constructors[descriptor] = method.AccessFlags
	}
	index := z.originalMemberIndex()
	if index == nil || !index.valid {
		return false
	}
	name := target.GetClassName()
	for _, handle := range index.handleTargets[name] {
		if !nativeProofWork(d.Work, 1) {
			return false
		}
		if handle.name == "<init>" {
			flags, known := constructors[handle.descriptor]
			if !known || flags&2 != 0 && handle.referencer != name {
				return false
			}
		}
	}
	for user := range index.constructors[name] {
		if !nativeProofWork(d.Work, 1) {
			return false
		}
		object := target
		if user != name {
			raw, known := z.enumSiblingResolver()(user)
			if !known || len(raw) > 2<<20 || d.Work != nil && d.Work.CheckAlloc(int64(len(raw))*4) != nil {
				return false
			}
			var err error
			object, err = d.parseResolved(raw)
			if err != nil || object == nil || object.GetClassName() != user {
				return false
			}
		}
		for _, method := range object.Methods {
			if method == nil || !nativeProofWork(d.Work, 1) {
				return false
			}
			seen := false
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || seen || !nativeProofWork(d.Work, int64(len(code.Code))) {
					return false
				}
				seen = true
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
				decoder.Work = d.Work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range constructorMotionOps(decoder) {
					if !nativeProofWork(d.Work, 1) {
						return false
					}
					if op.Instr.OpCode != core.OP_INVOKESPECIAL {
						continue
					}
					call := constructorMotionMember(object, op, core.OP_INVOKESPECIAL)
					if call == nil {
						return false
					}
					if call.Member == "<init>" && call.Name == name {
						flags, known := constructors[call.Description]
						if !known || flags&2 != 0 && user != name {
							return false
						}
					}
				}
			}
		}
	}
	return true
}
