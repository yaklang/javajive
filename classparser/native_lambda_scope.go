package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
	"sort"
	"strings"
)

// EnclosingMethod identifies the lexical source scope. A lambda's NEW may
// physically live in its implementation. Join those scopes only through an
// actual original metafactory site with unchanged descriptor parameter words.
// Unreferenced handles and ordinary invocations cannot establish this proof.
func nativeAnonymousAllocationScope(object *ClassObject, child *nativeAnonymousClass, name, desc string, work *workbudget.Budget) bool {
	if object == nil || child == nil || child.object == nil {
		return false
	}
	owner, lexical, original := originalAnonymousOwner(child.object)
	if !original || owner != object.GetClassName() || lexical != child.method {
		return false
	}
	if child.method == "" {
		return name == "<init>" || name == "<clinit>"
	}
	if child.method == name+desc {
		return true
	}
	// Compatibility with the existing body emitter; never a scope certificate.
	if !strings.HasPrefix(name, "lambda$") {
		return false
	}
	var impl *MemberInfo
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(object, m.NameIndex)
		d, dk := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n+d == name+desc {
			if impl != nil {
				return false
			}
			impl = m
		}
	}
	if impl == nil || impl.AccessFlags&(0x0002|0x1000|0x0400|0x0100) != (0x0002|0x1000) {
		return false
	}
	for _, a := range impl.Attributes {
		if _, ok := a.(*SignatureAttribute); ok {
			return false
		}
	}
	params, result, err := callbinding.Descriptor(desc)
	if err != nil {
		return false
	}
	var boot *BootstrapMethodsAttribute
	for _, a := range object.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			if boot != nil || b == nil {
				return false
			}
			boot = b
		}
	}
	if boot == nil || int(boot.NumBootstrapMethods) != len(boot.BootstrapMethods) {
		return false
	}
	constant := func(index uint16) ConstantInfo {
		if index == 0 || int(index) > len(object.ConstantPool) {
			return nil
		}
		return object.ConstantPool[index-1]
	}
	member := func(index uint16) (string, string, string, bool) {
		ref, ok := constant(index).(*ConstantMethodrefInfo)
		if !ok || ref == nil {
			return "", "", "", false
		}
		owner, ok := sourceBridgeClassName(object, ref.ClassIndex)
		nt, valid := constant(ref.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
		if !ok || !valid || nt == nil {
			return "", "", "", false
		}
		n, nk := sourceBridgeUTF8(object, nt.NameIndex)
		d, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
		return owner, n, d, nk && dk
	}
	handles := map[uint16]bool{}
	for i, cp := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		h, ok := cp.(*ConstantMethodHandleInfo)
		if !ok {
			continue
		}
		if h == nil {
			return false
		}
		owner, n, d, valid := member(h.ReferenceIndex)
		if valid && owner == object.GetClassName() && n == name && d == desc {
			if impl.AccessFlags&StaticFlag != 0 && h.ReferenceKind != 6 || impl.AccessFlags&StaticFlag == 0 && h.ReferenceKind != 5 && h.ReferenceKind != 7 {
				return false
			}
			handles[uint16(i+1)] = true
		}
	}
	if len(handles) != 1 {
		return false
	}
	allowed := map[int][]string{}
	for index, site := range boot.BootstrapMethods {
		if !nativeProofWork(work, 1) || site == nil {
			return false
		}
		uses := 0
		for ordinal, arg := range site.BootstrapArguments {
			if handles[arg] {
				if ordinal != 1 {
					return false
				}
				uses++
			}
		}
		if uses == 0 {
			continue
		}
		if uses != 1 || len(site.BootstrapArguments) != 3 || site.NumBootstrapArguments != 3 {
			return false
		}
		h, ok := constant(site.BootstrapMethodRef).(*ConstantMethodHandleInfo)
		if !ok || h == nil || h.ReferenceKind != 6 {
			return false
		}
		o, n, d, ok := member(h.ReferenceIndex)
		if !ok || o != "java/lang/invoke/LambdaMetafactory" || n != "metafactory" || d != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
			return false
		}
		sam, ok := constant(site.BootstrapArguments[0]).(*ConstantMethodTypeInfo)
		inst, ik := constant(site.BootstrapArguments[2]).(*ConstantMethodTypeInfo)
		if !ok || !ik || sam == nil || inst == nil {
			return false
		}
		s, sk := sourceBridgeUTF8(object, sam.DescriptorIndex)
		in, ink := sourceBridgeUTF8(object, inst.DescriptorIndex)
		sp, _, se := callbinding.Descriptor(s)
		ip, ir, ie := callbinding.Descriptor(in)
		if !sk || !ink || se != nil || ie != nil || len(sp) != len(ip) || ir != result || len(params) < len(ip) || !slices.Equal(params[len(params)-len(ip):], ip) {
			return false
		}
		captures := slices.Clone(params[:len(params)-len(ip)])
		if impl.AccessFlags&StaticFlag == 0 {
			captures = append([]string{"L" + object.GetClassName() + ";"}, captures...)
		}
		if len(captures) == 0 {
			return false
		}
		allowed[index] = captures
	}
	if len(allowed) != 1 {
		return false
	}
	sites := 0
	implBodies := 0
	for _, m := range object.Methods {
		n, _ := sourceBridgeUTF8(object, m.NameIndex)
		md, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return false
			}
			ops := constructorMotionOps(decoder)
			entries, valid := nativeMemberLexicalControlEntries(decoder, code, work)
			if !valid {
				return false
			}
			if m == impl {
				implBodies++
				words := map[int]bool{}
				word, offset := 0, 0
				if m.AccessFlags&StaticFlag == 0 {
					words[0] = true
					word, offset = 1, 1
				}
				for _, captures := range allowed {
					for _, p := range captures[offset:] {
						words[word] = true
						word++
						if p == "J" || p == "D" {
							words[word] = true
							word++
						}
					}
				}
				if !nativeEnumSelectorParametersUnchanged(ops, words) {
					return false
				}
			}
			for i, op := range ops {
				if !nativeProofWork(work, 1) {
					return false
				}
				for _, kind := range []int{core.OP_INVOKESTATIC, core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE, core.OP_INVOKESPECIAL} {
					ref := constructorMotionMember(object, op, kind)
					if ref != nil && ref.Name == object.GetClassName() && ref.Member == name && ref.Description == desc {
						return false
					}
				}
				if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
					continue
				}
				if len(op.Data) != 4 || op.Data[2] != 0 || op.Data[3] != 0 {
					return false
				}
				dynamic, ok := constant(core.Convert2bytesToInt(op.Data[:2])).(*ConstantInvokeDynamicInfo)
				if !ok || dynamic == nil {
					return false
				}
				captures := allowed[int(dynamic.BootstrapMethodAttrIndex)]
				if captures == nil {
					continue
				}
				if n+md != child.method || i < len(captures) {
					return false
				}
				nt, ok := constant(dynamic.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return false
				}
				ds, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
				dp, dr, e := callbinding.Descriptor(ds)
				if !known || e != nil || !callbinding.Reference(dr) || !slices.Equal(dp, captures) {
					return false
				}
				fp, _, e := callbinding.Descriptor(md)
				if e != nil {
					return false
				}
				slots := map[int]string{}
				word := 0
				if m.AccessFlags&StaticFlag == 0 {
					slots[0] = "L" + object.GetClassName() + ";"
					word = 1
				}
				for _, p := range fp {
					slots[word] = p
					word++
					if p == "J" || p == "D" {
						word++
					}
				}
				used := map[int]bool{}
				start := i - len(captures)
				for j, p := range captures {
					load := ops[start+j]
					slot := core.GetRetrieveIdx(load)
					if slots[slot] != p || !constructorMotionLoad(load, p) || j == 0 && impl.AccessFlags&StaticFlag == 0 && slot != 0 {
						return false
					}
					used[slot] = true
					if p == "J" || p == "D" {
						used[slot+1] = true
					}
				}
				entry := sort.SearchInts(entries, int(ops[start].CurrentOffset)+1)
				if entry < len(entries) && entries[entry] <= int(op.CurrentOffset) {
					return false
				}
				if !nativeEnumSelectorParametersUnchanged(ops, used) {
					return false
				}
				sites++
			}
		}
	}
	return sites == 1 && implBodies == 1
}
