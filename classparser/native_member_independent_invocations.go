package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"sort"
)

// A flattened independent declaration has no source access to its physical
// outer's synthetic methods. javac hides those binary methods from source
// lookup, even when their package access flags otherwise permit the call. Only
// this family's original owned declarations may contribute an accessor rewrite.
// Inspect actual instruction references, not unused constant-pool entries or
// accessor-name conventions. This is an admission guard, not a bridge rewrite.
func (z *JarFS) nativeMemberIndependentInvocationsClosed(prepared *nativeMemberPrepared) bool {
	if z == nil || prepared == nil || prepared.family == nil || prepared.reader == nil {
		return false
	}
	p, d := prepared.family, prepared.reader
	if p.independentRoot == nil {
		return true
	}
	if len(prepared.objects) == 0 || len(prepared.objects) > 4096 || !nativeProofWork(d.Work, 1) ||
		d.Work != nil && d.Work.CheckAlloc(int64(len(prepared.objects))*512) != nil {
		return false
	}
	names := make([]string, 0, len(prepared.objects))
	for name := range prepared.objects {
		names = append(names, name)
	}
	sort.Strings(names)
	loaded := map[string]*ClassObject{}
	missing := map[string]bool{}
	var total int64
	for _, name := range names {
		obj := prepared.objects[name]
		if obj == nil || obj.GetClassName() != name || p.lexicalObjects[name] != obj || !nativeProofWork(d.Work, 1) {
			return false
		}
		for _, method := range obj.Methods {
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
				decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, index) })
				decoder.Work = d.Work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(d.Work, 1) {
						return false
					}
					if op.Instr.OpCode != core.OP_INVOKESTATIC {
						continue
					}
					call := constructorMotionMember(obj, op, core.OP_INVOKESTATIC)
					if call == nil {
						return false
					}
					if p.lexicalObjects[call.Name] != nil || missing[call.Name] {
						continue
					}
					other := loaded[call.Name]
					if other == nil {
						if len(loaded)+len(missing) >= 4096 {
							return false
						}
						raw, found := z.enumSiblingResolver()(call.Name)
						if !found {
							missing[call.Name] = true
							continue
						}
						if len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-total || d.Work != nil && d.Work.CheckAlloc(total+int64(len(raw))) != nil {
							return false
						}
						var err error
						other, err = d.parseResolved(raw)
						if err != nil || other.GetClassName() != call.Name {
							return false
						}
						total += int64(len(raw))
						loaded[call.Name] = other
					}
					for _, target := range other.Methods {
						if target == nil || !nativeProofWork(d.Work, 1) {
							return false
						}
						n, nok := sourceBridgeUTF8(other, target.NameIndex)
						desc, dok := sourceBridgeUTF8(other, target.DescriptorIndex)
						if !nok || !dok {
							return false
						}
						if n == call.Member && desc == call.Description && target.AccessFlags&0x1008 == 0x1008 {
							return false
						}
					}
				}
			}
		}
	}
	return true
}
