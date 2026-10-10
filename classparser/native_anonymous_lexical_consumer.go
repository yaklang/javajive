package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
	"strings"
)

// An original producer/consumer certificate: an unnameable enclosing receiver has no standalone
// source expression. Only its original complete THIS chain and an exact lexical
// consumer may eliminate the operand. Final source and archive closure remain
// separate publication requirements.
type nativeAnonymousLexicalConsumer struct {
	read                    *nativeMemberLexicalRead
	getter                  *nativeMemberPrivateGetter
	declaredField           *MemberInfo
	declarationOwner        string
	owner, name, descriptor string
	kind, pc                int
}

func nativeAnonymousLexicalParent(f *nativeAnonymousForest, owner string) (string, bool) {
	if f == nil || f.objects[owner] == nil {
		return "", false
	}
	if unit := f.units[owner]; unit != nil && unit.object == f.objects[owner] {
		parent, _, ok := originalAnonymousOwner(unit.object)
		return parent, ok && unit.enclosingField != "" && f.objects[parent] != nil
	}
	if f.members != nil {
		if unit := f.members.children[owner]; unit != nil && unit.object == f.objects[owner] && !unit.static {
			return unit.owner, f.objects[unit.owner] != nil
		}
	}
	return "", false
}

// Every intervening source class and hierarchy must be known and free of a
// competing declaration. A binary prefix or the target's descriptor alone
// does not establish Java's nearest lexical lookup.
func nativeAnonymousLexicalConsumerLookup(f *nativeAnonymousForest, current, target, name string, field bool, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if f == nil || work != nil && work.CheckAlloc(512) != nil {
		return false
	}
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if current == "" || seen[current] || !nativeProofWork(work, 1) {
			return false
		}
		seen[current] = true
		obj := f.objects[current]
		if obj == nil || obj.GetClassName() != current {
			return false
		}
		if current == target {
			return f.units[target] != nil && f.units[target].object == obj
		}
		if field {
			// Captured locals have compiler fields val$name, but source lookup
			// sees name. Check the proved capture role before physical fields.
			if unit := f.units[current]; unit != nil {
				for captured := range unit.fields {
					if local, ok := strings.CutPrefix(captured, "val$"); ok && class_context.SafeIdentifier(local) == class_context.SafeIdentifier(name) {
						return false
					}
				}
			}
			names, ok := nativeAnonymousInitializerFieldNames(obj, resolve, work)
			if !ok || names[class_context.SafeIdentifier(name)] {
				return false
			}
		} else {
			hierarchy := map[string]bool{}
			active := map[string]bool{}
			var visit func(*ClassObject, int) bool
			visit = func(o *ClassObject, d int) bool {
				if o == nil || d >= 64 || active[o.GetClassName()] || !nativeProofWork(work, 1) {
					return false
				}
				if hierarchy[o.GetClassName()] {
					return true
				}
				if len(hierarchy)+len(active) >= 128 || work != nil && work.CheckAlloc(int64(len(hierarchy)+len(active)+1)*128) != nil {
					return false
				}
				active[o.GetClassName()] = true
				for _, m := range o.Methods {
					if m == nil || !nativeProofWork(work, 1) {
						return false
					}
					n, ok := sourceBridgeUTF8(o, m.NameIndex)
					if !ok || class_context.SafeIdentifier(n) == class_context.SafeIdentifier(name) {
						return false
					}
				}
				parents := append([]string{}, o.GetInterfacesName()...)
				if p := o.GetSupperClassName(); p != "" {
					parents = append(parents, p)
				}
				for _, p := range parents {
					if active[p] {
						return false
					}
					if hierarchy[p] {
						continue
					}
					if resolve == nil {
						return false
					}
					next, ok := resolve(p)
					if !ok || next == nil || next.GetClassName() != p || !visit(next, d+1) {
						return false
					}
				}
				delete(active, o.GetClassName())
				hierarchy[o.GetClassName()] = true
				return true
			}
			if !visit(obj, 0) {
				return false
			}
		}
		var ok bool
		current, ok = nativeAnonymousLexicalParent(f, current)
		if !ok {
			return false
		}
	}
	return false
}

func nativeAnonymousLexicalConsumerProof(f *nativeAnonymousForest, caller *ClassObject, read *nativeMemberLexicalRead, target string, op *core.OpCode, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeAnonymousLexicalConsumer {
	if work != nil && work.CheckAlloc(512) != nil {
		return nil
	}
	if f == nil || caller == nil || f.objects[caller.GetClassName()] != caller || read == nil || read.descriptor != "L"+target+";" || f.units[target] == nil || op == nil || op.Instr == nil || len(op.Data) != 2 || !nativeProofWork(work, 1) {
		return nil
	}
	index := int(core.Convert2bytesToInt(op.Data))
	if index < 1 || index > len(caller.ConstantPool) {
		return nil
	}
	if op.Instr.OpCode == core.OP_GETFIELD {
		ref, physical := caller.ConstantPool[index-1].(*ConstantFieldrefInfo)
		symbol := constructorMotionMember(caller, op, core.OP_GETFIELD)
		if !physical || ref == nil || symbol == nil || symbol.Name != target || class_context.SafeIdentifier(symbol.Member) != symbol.Member {
			return nil
		}
		decl, field, known := nativeAnonymousLexicalFieldDeclaration(f.objects[target], symbol.Member, symbol.Description, resolve, work)
		if !known || field.AccessFlags&(0x0002|0x0008) != 0 || field.AccessFlags&(0x0001|0x0004) == 0 && nativeAnonymousCallPackage(decl.GetClassName()) != nativeAnonymousCallPackage(caller.GetClassName()) || !nativeAnonymousLexicalConsumerLookup(f, caller.GetClassName(), target, symbol.Member, true, resolve, work) {
			return nil
		}
		return &nativeAnonymousLexicalConsumer{read: read, declaredField: field, declarationOwner: decl.GetClassName(), owner: target, name: symbol.Member, descriptor: symbol.Description, kind: core.OP_GETFIELD, pc: int(op.CurrentOffset)}
	}
	if ref, ok := caller.ConstantPool[index-1].(*ConstantMethodrefInfo); !ok || ref == nil {
		return nil
	}
	kind := op.Instr.OpCode
	symbol := constructorMotionMember(caller, op, kind)
	if symbol == nil || symbol.Name != target || class_context.SafeIdentifier(symbol.Member) != symbol.Member {
		return nil
	}
	c := &nativeAnonymousLexicalConsumer{read: read, owner: target, name: symbol.Member, descriptor: symbol.Description, kind: kind, pc: int(op.CurrentOffset)}
	if kind == core.OP_INVOKESTATIC && f.members != nil {
		g := f.members.getters[nativeMemberGetterKey(target, symbol.Member, symbol.Description)]
		if g == nil || !nativeAnonymousAccessorDeclaration(f, f.objects[target], g.method, work) || g.staticField || g.setter || g.update != nil || g.call != nil || !nativeAnonymousLexicalConsumerLookup(f, caller.GetClassName(), target, g.field, true, resolve, work) {
			return nil
		}
		c.getter = g
		if g.inheritedField {
			decl, field, known := nativeAnonymousLexicalFieldDeclaration(f.objects[target], g.field, g.fieldDescriptor, resolve, work)
			if !known {
				return nil
			}
			c.declaredField, c.declarationOwner = field, decl.GetClassName()
		}
		return c
	}
	if kind != core.OP_INVOKEVIRTUAL || symbol.Member == "<init>" || symbol.Member == "<clinit>" {
		return nil
	}
	ps, _, err := callbinding.Descriptor(symbol.Description)
	if err != nil || len(ps) != 0 {
		return nil
	}
	found := false
	for _, m := range f.objects[target].Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(f.objects[target], m.NameIndex)
		d, dk := sourceBridgeUTF8(f.objects[target], m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n != symbol.Member {
			continue
		}
		args, _, err := callbinding.Descriptor(d)
		if err != nil {
			return nil
		}
		if len(args) != 0 {
			continue
		}
		if found || d != symbol.Description || m.AccessFlags&(0x0002|0x0008|0x0040|0x0100|0x0400|0x1000) != 0 {
			return nil
		}
		for _, a := range m.Attributes {
			if _, ok := a.(*SignatureAttribute); ok {
				return nil
			}
		}
		found = true
	}
	if !found || !nativeAnonymousLexicalConsumerLookup(f, caller.GetClassName(), target, symbol.Member, false, resolve, work) {
		return nil
	}
	return c
}

func nativeAnonymousAccessorDeclaration(f *nativeAnonymousForest, o *ClassObject, m *MemberInfo, work *workbudget.Budget) bool {
	if work != nil && work.CheckAlloc(512) != nil {
		return false
	}
	if f == nil || f.members == nil || o == nil || m == nil || f.objects[o.GetClassName()] != o || f.units[o.GetClassName()] == nil || f.units[o.GetClassName()].object != o {
		return false
	}
	n, nk := sourceBridgeUTF8(o, m.NameIndex)
	d, dk := sourceBridgeUTF8(o, m.DescriptorIndex)
	if !nk || !dk {
		return false
	}
	g := f.members.getters[nativeMemberGetterKey(o.GetClassName(), n, d)]
	if g != nil && class_context.SafeIdentifier(g.field) != g.field {
		return false
	}
	fresh := nativeMemberGetterPacketProof(o, m, f.resolve, work)
	if g == nil || g.method != m || fresh == nil || fresh.staticField || g.setter || g.update != nil || g.call != nil || g.inheritedField != fresh.inheritedField || g.owner != fresh.owner || g.name != fresh.name || g.descriptor != fresh.descriptor || g.field != fresh.field || g.fieldDescriptor != fresh.fieldDescriptor || g.ordinal != fresh.ordinal || g.staticField != fresh.staticField || g.genericField != fresh.genericField {
		return false
	}
	if g.inheritedField {
		// The accessor packet and Java's lexical field lookup must agree on
		// one known, non-generic declaration. Recheck its actual CP field tag:
		// a Methodref with identical text is not a GETFIELD certificate.
		if _, _, known := nativeAnonymousLexicalFieldDeclaration(o, g.field, g.fieldDescriptor, f.resolve, work); !known {
			return false
		}
		for _, attr := range m.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				index := int(code.Code[2])<<8 | int(code.Code[3])
				if index < 1 || index > len(o.ConstantPool) {
					return false
				}
				if ref, ok := o.ConstantPool[index-1].(*ConstantFieldrefInfo); !ok || ref == nil {
					return false
				}
			}
		}
	}
	ps, result, err := callbinding.Descriptor(d)
	if err != nil {
		return false
	}
	for owned := range f.units {
		if strings.Contains(result, "L"+owned+";") {
			return false
		}
		for i, p := range ps {
			if strings.Contains(p, "L"+owned+";") && (i != 0 || owned != o.GetClassName() || p != "L"+owned+";" || g.staticField) {
				return false
			}
		}
	}
	return true
}

func nativeAnonymousAccessorNameType(f *nativeAnonymousForest, o *ClassObject, index int, work *workbudget.Budget) bool {
	if f == nil || f.members == nil || o == nil || f.objects[o.GetClassName()] != o || index < 1 || index > len(o.ConstantPool) || work != nil && work.CheckAlloc(int64(len(o.ConstantPool))*24+512) != nil {
		return false
	}
	nt, ok := o.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	n, nk := sourceBridgeUTF8(o, nt.NameIndex)
	desc, dk := sourceBridgeUTF8(o, nt.DescriptorIndex)
	if !nk || !dk || !nativeProofWork(work, int64(len(desc))+1) {
		return false
	}
	ps, _, err := callbinding.Descriptor(desc)
	if err != nil || len(ps) != 1 || !strings.HasPrefix(ps[0], "L") || !strings.HasSuffix(ps[0], ";") {
		return false
	}
	owner := ps[0][1 : len(ps[0])-1]
	g := f.members.getters[nativeMemberGetterKey(owner, n, desc)]
	if g == nil || !nativeAnonymousAccessorDeclaration(f, f.objects[owner], g.method, work) {
		return false
	}
	handles := map[int]bool{}
	for _, cp := range o.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if h, ok := cp.(*ConstantMethodHandleInfo); ok {
			if h == nil {
				return false
			}
			handles[int(h.ReferenceIndex)] = true
		}
	}
	used := false
	for i, cp := range o.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if m := nativeConstantMember(cp); m != nil && int(m.NameAndTypeIndex) == index {
			ref, ok := cp.(*ConstantMethodrefInfo)
			if !ok || ref == nil || handles[i+1] {
				return false
			}
			target, ok := sourceBridgeClassName(o, ref.ClassIndex)
			if !ok || target != owner {
				return false
			}
			used = true
		}
		switch cp := cp.(type) {
		case *ConstantDynamicInfo:
			if cp == nil || int(cp.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantInvokeDynamicInfo:
			if cp == nil || int(cp.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return used
}

func nativeAnonymousLexicalConsumerOperand(v any, c *nativeAnonymousLexicalConsumer, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if c == nil || ctx == nil || !nativeMemberLexicalReadOperand(v, c.read, work, ctx) {
		return false
	}
	vv, ok := v.(values.JavaValue)
	if !ok {
		return false
	}
	for node := c.read; node != nil; node = node.prior {
		vv, ok = nativeMemberEnclosingUnpack(vv, work)
		if !ok {
			return false
		}
		field, ok := vv.(*values.RefMember)
		if !ok || field == nil {
			return false
		}
		vv = field.Object
	}
	vv, ok = nativeMemberEnclosingUnpack(vv, work)
	if !ok {
		return false
	}
	ref, ok := vv.(*values.JavaRef)
	if !ok || ref == nil {
		return false
	}
	slot, original := ref.OriginalReceiverSlot()
	return original && slot == 0
}

// Re-open the physical producer/consumer interval at source consumption. The
// earlier planning map is an index, never permission for a changed Code body,
// method, field chain, target or lexical binding.
func nativeAnonymousLexicalConsumerSourceClosed(d *ClassObjectDumper, c *nativeAnonymousLexicalConsumer, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if d == nil || d.obj == nil || c == nil || ctx == nil || d.nativeAnonymousForest == nil || d.nativeAnonymousForest.objects[d.obj.GetClassName()] != d.obj || work != nil && work.CheckAlloc(1024) != nil {
		return false
	}
	f := d.nativeAnonymousForest
	if f.members == nil || f.members.anonymousForest != f || !nativeMemberJointAnonymousAccess(f.members, d.obj.GetClassName(), work) {
		return false
	}
	var method *MemberInfo
	for _, m := range d.obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(d.obj, m.NameIndex)
		desc, dk := sourceBridgeUTF8(d.obj, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n == ctx.FunctionName && desc == ctx.CurrentMethodDesc {
			if method != nil {
				return false
			}
			method = m
		}
	}
	if method == nil || method.AccessFlags&8 != 0 {
		return false
	}
	var code *CodeAttribute
	for _, a := range method.Attributes {
		if ca, ok := a.(*CodeAttribute); ok {
			if ca == nil || code != nil {
				return false
			}
			code = ca
		}
	}
	if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
		return false
	}
	dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.obj.ConstantPool, i) })
	dec.Work = work
	if dec.ParseOpcode() != nil {
		return false
	}
	ops := constructorMotionOps(dec)
	entries, ok := nativeMemberLexicalControlEntries(dec, code, work)
	if !ok {
		return false
	}
	var path []*nativeMemberLexicalRead
	for r := c.read; r != nil; r = r.prior {
		if len(path) >= 64 || !nativeProofWork(work, 1) {
			return false
		}
		path = append(path, r)
	}
	for i, op := range ops {
		if int(op.CurrentOffset) != c.pc {
			continue
		}
		if i < len(path)+1 || op.Instr.OpCode != c.kind {
			return false
		}
		for j, r := range path {
			producer := ops[i-1-j]
			s := constructorMotionMember(d.obj, producer, core.OP_GETFIELD)
			if s == nil || int(producer.CurrentOffset) != r.pc || s.Name != r.owner || s.Member != r.field || s.Description != r.descriptor {
				return false
			}
		}
		load := ops[i-1-len(path)]
		if core.GetRetrieveIdx(load) != 0 || !constructorMotionLoad(load, "Ljava/lang/Object;") {
			return false
		}
		entry := sort.SearchInts(entries, int(load.CurrentOffset)+1)
		if entry < len(entries) && entries[entry] <= c.pc {
			return false
		}
		fresh := nativeAnonymousLexicalConsumerProof(d.nativeAnonymousForest, d.obj, c.read, c.owner, op, d.nativeAnonymousForest.resolve, work)
		return fresh != nil && fresh.getter == c.getter && fresh.declaredField == c.declaredField && fresh.declarationOwner == c.declarationOwner && fresh.owner == c.owner && fresh.name == c.name && fresh.descriptor == c.descriptor && fresh.kind == c.kind && fresh.pc == c.pc
	}
	return false
}

func (d *ClassObjectDumper) wireNativeAnonymousLexicalConsumers(ctx *class_context.ClassContext) {
	f := d.nativeAnonymousForest
	if f == nil {
		return
	}
	prior := ctx.SourceLexicalInvocationReceiver
	ctx.SourceLexicalInvocationReceiver = func(v any) (string, bool) {
		call, ok := v.(*values.FunctionCallExpression)
		if ok && call != nil && call.HasOriginPC {
			c := f.consumers[d.obj.GetClassName()][ctx.FunctionName+ctx.CurrentMethodDesc][call.OriginPC]
			if c != nil && c.getter == nil && c.declaredField == nil {
				if !nativeAnonymousLexicalConsumerSourceClosed(d, c, ctx, d.Work) || call.ClassName != c.owner || call.FunctionName != c.name || call.Descriptor != c.descriptor || call.Kind != values.InvokeVirtual || len(call.Arguments) != 0 || !nativeAnonymousLexicalConsumerOperand(call.Object, c, ctx, d.Work) {
					d.nativeCaptureFailed = true
					return "", false
				}
				return "", true
			}
		}
		if prior != nil {
			return prior(v)
		}
		return "", false
	}
}
