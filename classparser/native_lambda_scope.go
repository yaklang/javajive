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
	return nativeLambdaImplementationScope(object, name, desc, child.method, true, work)
}

// The implementation's source ownership is an original metafactory use, rather
// than a private/synthetic flag or a spelling. A named member has no imposed
// EnclosingMethod scope; anonymous allocations additionally require that scope
// and at least one unchanged capture. Both paths retain physical descriptors.
func nativeLambdaImplementationScope(object *ClassObject, name, desc, lexical string, requireCapture bool, work *workbudget.Budget, contexts ...nativeLambdaImplementationContext) bool {
	if len(contexts) > 1 || object == nil || len(object.Methods) > 4096 || len(object.ConstantPool) > 65535 || !nativeProofWork(work, int64(len(object.Methods)+len(object.ConstantPool))) {
		return false
	}
	var context nativeLambdaImplementationContext
	if len(contexts) == 1 {
		context = contexts[0]
		if context.resolve == nil {
			return false
		}
	}
	widening := newConstructorWideningQuery(context.metadata)
	bytes := 0
	for _, m := range object.Methods {
		if m == nil {
			return false
		}
		for _, attr := range m.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				if code == nil || len(code.Code) > (1<<20)-bytes {
					return false
				}
				bytes += len(code.Code)
			}
		}
	}
	if work != nil && work.CheckAlloc(int64(bytes)*32+int64(len(object.ConstantPool))*32) != nil {
		return false
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
	erased := map[int]string{}
	for index, site := range boot.BootstrapMethods {
		if !nativeProofWork(work, 1) || site == nil {
			return false
		}
		if handles[site.BootstrapMethodRef] {
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
		sp, sr, se := callbinding.Descriptor(s)
		ip, ir, ie := callbinding.Descriptor(in)
		if !sk || !ink || se != nil || ie != nil || len(sp) != len(ip) || ir != result || len(params) < len(ip) || !slices.Equal(params[len(params)-len(ip):], ip) {
			return false
		}
		// Metafactory's instantiated signature specializes erased reference
		// positions. Primitive positions cannot silently change width or kind.
		for i := range sp {
			if sp[i] != ip[i] && (!callbinding.Reference(sp[i]) || !callbinding.Reference(ip[i]) || !widening.assignable(ip[i], sp[i])) {
				return false
			}
		}
		if sr != ir && (!callbinding.Reference(sr) || !callbinding.Reference(ir) || !widening.assignable(ir, sr)) {
			return false
		}
		captures := slices.Clone(params[:len(params)-len(ip)])
		if impl.AccessFlags&StaticFlag == 0 {
			captures = append([]string{"L" + object.GetClassName() + ";"}, captures...)
		}
		if requireCapture && len(captures) == 0 {
			return false
		}
		allowed[index] = captures
		erased[index] = s
	}
	if len(allowed) != 1 {
		return false
	}
	for _, cp := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if dynamic, ok := cp.(*ConstantDynamicInfo); ok {
			if dynamic == nil {
				return false
			}
			if _, consumed := allowed[int(dynamic.BootstrapMethodAttrIndex)]; consumed {
				return false
			}
		}
	}
	sites := 0
	implBodies := 0
	var localSite *nativeLambdaLocalCaptureSite
	var factories []*nativeLambdaLocalCaptureSite
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
				if op == nil || op.Instr == nil {
					return false
				}
				if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W || op.Instr.OpCode == core.OP_LDC2_W {
					index := uint16(0)
					if len(op.Data) == 1 {
						index = uint16(op.Data[0])
					} else if len(op.Data) == 2 {
						index = core.Convert2bytesToInt(op.Data)
					} else {
						return false
					}
					if handles[index] {
						return false
					}
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
				captures, belongs := allowed[int(dynamic.BootstrapMethodAttrIndex)]
				if !belongs {
					continue
				}
				if lexical != "" && n+md != lexical || i < len(captures) {
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
				if len(contexts) == 1 {
					samName, known := sourceBridgeUTF8(object, nt.NameIndex)
					if !known || !nativeLambdaFunctionalTarget(dr, samName, erased[int(dynamic.BootstrapMethodAttrIndex)], context.resolve, work) {
						return false
					}
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
				if len(captures) > 64 || work != nil && work.CheckAlloc(int64(len(captures))*192+512) != nil {
					return false
				}
				operands := make([]*nativeEnumSelectorProducer, len(captures))
				// A hidden anonymous capture is read as THIS/GETFIELD, not one
				// parameter LOAD. Partition the actual stack producers backwards
				// without guessing from equal descriptors or variable spelling.
				loads := make([]int, len(captures))
				start := i
				for j := len(captures) - 1; j >= 0; j-- {
					if start == 0 {
						return false
					}
					start--
					loads[j] = start
					var field *nativeEnumSelectorProducer
					if m.AccessFlags&StaticFlag == 0 && !(j == 0 && impl.AccessFlags&StaticFlag == 0) {
						field = nativeAnonymousLambdaCaptureField(object, context.anonymousCaptures, ops, start, captures[j], work)
					}
					if field != nil {
						operands[j] = field
						start--
					} else if !constructorMotionLoad(ops[start], captures[j]) {
						return false
					}
				}
				var localReads map[int]*nativeEnumLocalRead
				var localFlow *nativeEnumParameterFlow
				hasLocal := false
				referenceDomain := nativeLocalReferenceDomain{descriptors: map[int]string{}, metadata: context.metadata}
				for j, descriptor := range captures {
					if operands[j] != nil {
						continue
					}
					load := ops[loads[j]]
					if !constructorMotionLoad(load, descriptor) {
						return false
					}
					if callbinding.Reference(descriptor) {
						referenceDomain.descriptors[int(load.CurrentOffset)] = descriptor
					}
				}
				for j, p := range captures {
					if operands[j] != nil {
						used[0] = true
						hasLocal = true // Requires the same post-render operand proof.
						continue
					}
					load := ops[loads[j]]
					slot := core.GetRetrieveIdx(load)
					if !constructorMotionLoad(load, p) || j == 0 && impl.AccessFlags&StaticFlag == 0 && slot != 0 {
						return false
					}
					operand := &nativeEnumSelectorProducer{opcode: load.Instr.OpCode, pc: int(load.CurrentOffset), slot: slot, result: p}
					if slots[slot] != p {
						if slots[slot] != "" || context.localCaptures == nil {
							return false
						}
						if localReads == nil {
							localFlow = nativeEnumParameterOriginalFlow(decoder, code, work)
							if localFlow == nil {
								return false
							}
							reader := NewClassObjectDumper(object)
							reader.Work = work
							var known bool
							localReads, known = reader.nativeTypedLocalReads(m, code, localFlow, slots, true, referenceDomain)
							if !known {
								return false
							}
						}
						read := localReads[int(load.CurrentOffset)]
						if read == nil || read.slot != slot || read.descriptor != p || !nativeLambdaLocalDefinitionsClosed(ops, read, work) && !nativeLambdaLocalDefinitionWebClosed(localFlow, ops, read, work) {
							return false
						}
						operand.local = read
						hasLocal = true
					} else {
						used[slot] = true
						// A bound anonymous receiver needs the same retained
						// source-operand proof as a captured field or local.
						if j == 0 && impl.AccessFlags&StaticFlag == 0 && context.anonymousCaptures != nil {
							hasLocal = true
						}
						if p == "J" || p == "D" {
							used[slot+1] = true
						}
					}
					operands[j] = operand
				}
				entry := sort.SearchInts(entries, int(ops[start].CurrentOffset)+1)
				if entry < len(entries) && entries[entry] <= int(op.CurrentOffset) {
					return false
				}
				if !nativeEnumSelectorParametersUnchanged(ops, used) {
					return false
				}
				factory := &nativeLambdaLocalCaptureSite{method: m, code: code, pc: int(op.CurrentOffset), operands: operands}
				if hasLocal {
					localSite = factory
				}
				if sites >= 64 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(sites+1)*128) != nil {
					return false
				}
				factories = append(factories, factory)
				sites++
			}
		}
	}
	if sites == 0 || implBodies != 1 || sites != 1 && context.factorySites == nil {
		return false
	}
	// Allocation-scope callers without source certificates retain their unique
	// lexical site rule. Only the member renderer can close multiple factories.
	if sites == 1 && localSite != nil {
		context.localCaptures[name+desc] = localSite
	}
	if context.factorySites != nil {
		context.factorySites[name+desc] = factories
	}
	return true
}
