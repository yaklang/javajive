package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/mutf8"
	"github.com/yaklang/javajive/internal/workbudget"
)

type privateNestMemberKey struct{ owner, name, descriptor string }
type privateNestCompletionBinding struct {
	original       *ClassObject
	member         *MemberInfo
	helper, target privateNestMemberKey
}

func privateNestReferenceKey(o *ClassObject, index uint16) (privateNestMemberKey, bool) {
	var key privateNestMemberKey
	if index == 0 || int(index) > len(o.ConstantPool) {
		return key, false
	}
	r, ok := o.ConstantPool[index-1].(*ConstantMethodrefInfo)
	if !ok || r == nil || r.NameAndTypeIndex == 0 || int(r.NameAndTypeIndex) > len(o.ConstantPool) {
		return key, false
	}
	nt, ok := o.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return key, false
	}
	var a, b, c bool
	key.owner, a = sourceParameterClassKey(o, r.ClassIndex)
	key.name, b = sourceParameterUTF8Key(o, nt.NameIndex)
	key.descriptor, c = sourceParameterUTF8Key(o, nt.DescriptorIndex)
	return key, a && b && c
}

// The helper's only behavior is a private call on the identical THIS and
// ordered typed slots. No cast, synchronization, handler or other effect can be
// deleted. Fresh javac may use either own-class private invocation opcode.
func privateNestHelperForward(o *ClassObject, m *MemberInfo, b *privateNestCompletionBinding, work *workbudget.Budget) bool {
	if m == nil || m.AccessFlags != 0x10 {
		return false
	}
	desc, ok := sourceParameterUTF8Key(o, m.DescriptorIndex)
	if !ok || desc != b.helper.descriptor {
		return false
	}
	code := constructorLoweringCode(m)
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 512 || !nativeProofWork(work, int64(len(code.Code))+1) {
		return false
	}
	tw := int64(0)
	if _, _, e := sourceParameterRows(o, m, work, &tw); e != nil {
		return false
	}
	for _, a := range m.Attributes {
		switch a := a.(type) {
		case *CodeAttribute:
		case *ExceptionsAttribute:
		case *UnparsedAttribute:
			if a == nil || a.Name != "MethodParameters" {
				return false
			}
		default:
			return false
		}
	}
	before, bk := originalMethodExceptions(b.original, b.member)
	after, ak := originalMethodExceptions(o, m)
	if !bk || !ak || !slices.Equal(before, after) {
		return false
	}
	params, result, e := callbinding.Descriptor(desc)
	if e != nil {
		return false
	}
	d := core.NewDecompiler(code.Code, nil)
	d.Work = work
	if d.ParseOpcode() != nil {
		return false
	}
	ops := constructorMotionOps(d)
	if len(ops) != len(params)+3 || !enumArrayInstructionSpans(code.Code, ops) || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[0]) != 0 {
		return false
	}
	slot := 1
	for i, p := range params {
		if !constructorMotionLoad(ops[i+1], p) || core.GetRetrieveIdx(ops[i+1]) != slot {
			return false
		}
		slot += constructorEffectType(p).width()
	}
	call := ops[len(ops)-2]
	if call.Instr.OpCode != core.OP_INVOKEVIRTUAL && call.Instr.OpCode != core.OP_INVOKESPECIAL {
		return false
	}
	index, known := nativeEnumCPIndex(call)
	key, valid := privateNestReferenceKey(o, index)
	peak := slot
	if result == "J" || result == "D" {
		peak = max(peak, 2)
	}
	return known && valid && key == b.target && nativeEnumOpcode(ops[len(ops)-1], int(bridgeReturnOpcode(desc))) && int(code.MaxLocals) >= slot && int(code.MaxStack) >= peak
}

type privateNestPoolAppend struct {
	count int
	data  []byte
	work  *workbudget.Budget
}

func (p *privateNestPoolAppend) entry(tag byte, data []byte) (uint16, bool) {
	if p.count >= 65535 || len(p.data)+len(data)+1 > 1<<20 || !nativeProofWork(p.work, int64(len(data))+1) {
		return 0, false
	}
	index := uint16(p.count)
	p.count++
	p.data = append(p.data, tag)
	p.data = append(p.data, data...)
	return index, true
}
func privateNestWord(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
func (p *privateNestPoolAppend) text(raw string) (uint16, bool) {
	if len(raw) == 0 || len(raw) > 65535 {
		return 0, false
	}
	if _, e := mutf8.Decode([]byte(raw)); e != nil {
		return 0, false
	}
	return p.entry(CONSTANT_Utf8, append(privateNestWord(uint16(len(raw))), []byte(raw)...))
}
func (p *privateNestPoolAppend) class(raw string) (uint16, bool) {
	i, k := p.text(raw)
	if !k {
		return 0, false
	}
	return p.entry(CONSTANT_Class, privateNestWord(i))
}
func (p *privateNestPoolAppend) method(key privateNestMemberKey) (uint16, bool) {
	c, ck := p.class(key.owner)
	n, nk := p.text(key.name)
	d, dk := p.text(key.descriptor)
	if !ck || !nk || !dk {
		return 0, false
	}
	nt, k := p.entry(CONSTANT_NameAndType, append(privateNestWord(n), privateNestWord(d)...))
	if !k {
		return 0, false
	}
	return p.entry(CONSTANT_Methodref, append(privateNestWord(c), privateNestWord(nt)...))
}

// A class is either a fresh flat unit with no nest table, or an already
// completed unit with exactly the original symbolic nest. Mixed/conflicting
// tables grant no private access. Payload keys preserve original MUTF-8 units.
func privateNestTable(o *ClassObject, work *workbudget.Budget) (string, []string, bool) {
	var name string
	var keys []string
	for _, a := range o.Attributes {
		if !nativeProofWork(work, 1) {
			return "", nil, false
		}
		raw, ok := a.(*UnparsedAttribute)
		if !ok || raw == nil || raw.Name != "NestHost" && raw.Name != "NestMembers" {
			continue
		}
		if name != "" || len(raw.Info) < 2 || int(raw.Length) != len(raw.Info) {
			return "", nil, false
		}
		name = raw.Name
		count, start := 1, 0
		if name == "NestMembers" {
			count, start = int(binary.BigEndian.Uint16(raw.Info)), 2
		}
		if count > 128 || len(raw.Info) != start+2*count {
			return "", nil, false
		}
		seen := map[string]bool{}
		for i := 0; i < count; i++ {
			key, k := sourceParameterClassKey(o, binary.BigEndian.Uint16(raw.Info[start+2*i:]))
			if !k || seen[key] {
				return "", nil, false
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return name, keys, true
}

func (a *sourceConstructorLoweringAuthority) lowerPrivateNestAdapters(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	unchanged := func() ([]byte, error) { return compiled, work.Check() }
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "private nest completion: "+s)
	}
	if a == nil || a.jar == nil || a.jar.targetSourceVersion < 11 {
		return unchanged()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, e := ParseWithBudget(original, work)
	if e != nil {
		return nil, e
	}
	if !a.privateNestSourceReturned(want.GetClassName(), original, work) {
		return unchanged()
	}
	d := NewClassObjectDumper(want)
	d.Work = work
	d.foldSiblingResolver = a.jar.enumSiblingResolver()
	d.declarationResolver = a.jar.declarationResolver
	family := d.privateNestCompletionFamily(a.jar.readJarEntry)
	if family == nil {
		return fail("original reciprocal graph")
	}
	for _, name := range family.names {
		if !a.privateNestSourceReturned(name, family.raw[name], work) {
			return fail("unreturned original nest source")
		}
	}
	reader, e := NewClassReaderWithBudget(compiled, work)
	if e != nil {
		return nil, e
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, e := parseWithMetadataLayout(reader, nil, layout)
	if e != nil {
		return nil, e
	}
	if !sourceParameterClassIdentityClosed(want, got) || got.MajorVersion != uint16(a.jar.targetSourceVersion+44) || got.MajorVersion < 55 || got.MajorVersion > 65 || got.MinorVersion != 0 {
		return fail("fresh class identity/version")
	}
	if work.CheckAlloc(int64(len(compiled))*4+int64(len(got.ConstantPool)+len(got.Methods)+1)*512) != nil {
		return nil, work.Err()
	}
	oldTable, oldKeys, ok := privateNestTable(want, work)
	newTable, newKeys, nk := privateNestTable(got, work)
	// NestMembers is a set of reciprocal private-access grants, not source
	// declaration order. A compiler can enumerate the identical members in a
	// different order. The reader already rejects duplicates; compare the
	// complete sets without permitting any missing or additional membership.
	oldSet, newSet := slices.Clone(oldKeys), slices.Clone(newKeys)
	if !nativeProofWork(work, int64(len(oldSet)+len(newSet))*8+1) {
		return fail("nest membership budget")
	}
	slices.Sort(oldSet)
	slices.Sort(newSet)
	if !ok || !nk || oldTable == "" || newTable != "" && (newTable != oldTable || !slices.Equal(oldSet, newSet)) {
		return fail("changed symbolic nest")
	}
	hasNestTable := newTable != ""
	own, _ := sourceParameterClassKey(want, want.ThisClass)
	helpers := map[privateNestMemberKey]*privateNestCompletionBinding{}
	targets := map[privateNestMemberKey]*privateNestCompletionBinding{}
	targetDeclarations := map[privateNestMemberKey]*privateNestCompletionBinding{}
	for _, name := range family.names {
		p := family.plans[name]
		if p == nil {
			continue
		}
		o := family.objects[name]
		owner, k := sourceParameterClassKey(o, o.ThisClass)
		if !k {
			return fail("original owner")
		}
		for _, m := range o.Methods {
			n, nk := sourceBridgeUTF8(o, m.NameIndex)
			s, sk := sourceBridgeUTF8(o, m.DescriptorIndex)
			if !nk || !sk {
				return fail("original declaration")
			}
			helper := p.bridges[privateNestTarget{n, s}]
			if helper == "" {
				continue
			}
			rawName, rn := sourceParameterUTF8Key(o, m.NameIndex)
			rawDesc, rd := sourceParameterUTF8Key(o, m.DescriptorIndex)
			if !rn || !rd {
				return fail("original symbolic declaration")
			}
			b := &privateNestCompletionBinding{original: o, member: m, helper: privateNestMemberKey{owner, helper, rawDesc}, target: privateNestMemberKey{owner, rawName, rawDesc}}
			helpers[b.helper], targets[b.target] = b, b
			targetDeclarations[privateNestMemberKey{name, n, s}] = b
		}
	}
	methodKey := func(o *ClassObject, m *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		n, nk := sourceParameterUTF8Key(o, m.NameIndex)
		s, sk := sourceParameterUTF8Key(o, m.DescriptorIndex)
		return sourceLambdaMethodIdentity{n, s}, nk && sk
	}
	originalMethods := map[sourceLambdaMethodIdentity]*MemberInfo{}
	freshMethods := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, item := range []struct {
		o     *ClassObject
		table map[sourceLambdaMethodIdentity]*MemberInfo
	}{{want, originalMethods}, {got, freshMethods}} {
		for _, m := range item.o.Methods {
			k, ok := methodKey(item.o, m)
			if !ok || item.table[k] != nil || !nativeProofWork(work, 1) {
				return fail("method universe")
			}
			item.table[k] = m
		}
	}
	originalConsumers := map[sourceLambdaMethodIdentity]sourceLambdaMethodIdentity{}
	for key, m := range originalMethods {
		n, nk := sourceBridgeUTF8(want, m.NameIndex)
		s, sk := sourceBridgeUTF8(want, m.DescriptorIndex)
		decoded := sourceLambdaMethodIdentity{n, s}
		if !nk || !sk {
			return fail("original consumer spelling")
		}
		if prior, exists := originalConsumers[decoded]; exists && prior != key {
			return fail("ambiguous decoded consumer")
		}
		originalConsumers[decoded] = key
	}
	for key, b := range targets {
		if key.owner == own {
			m := freshMethods[sourceLambdaMethodIdentity{key.name, key.descriptor}]
			if m == nil || m.AccessFlags != b.member.AccessFlags || m.AccessFlags&2 == 0 {
				return fail("private target flags")
			}
		}
	}
	for k := range originalMethods {
		if freshMethods[k] == nil {
			return fail("missing original declaration")
		}
	}
	removed := map[*MemberInfo]bool{}
	var edits []compiledMetadataEdit
	for k, m := range freshMethods {
		if originalMethods[k] != nil {
			continue
		}
		b := helpers[privateNestMemberKey{own, k.name, k.descriptor}]
		if b == nil || !privateNestHelperForward(got, m, b, work) {
			return fail("unproved temporary declaration")
		}
		target := freshMethods[sourceLambdaMethodIdentity{b.target.name, b.target.descriptor}]
		if target == nil || target.AccessFlags != b.member.AccessFlags || target.AccessFlags&2 == 0 {
			return fail("private target flags")
		}
		removed[m] = true
		edits = append(edits, compiledMetadataEdit{layout.methods[m], nil})
	}
	if !hasNestTable {
		for key := range helpers {
			if key.owner == own && freshMethods[sourceLambdaMethodIdentity{key.name, key.descriptor}] == nil {
				return fail("missing published helper")
			}
		}
	}
	for _, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return fail("method handle scan budget")
		}
		if h, ok := constant.(*ConstantMethodHandleInfo); ok && h != nil {
			key, k := privateNestReferenceKey(got, h.ReferenceIndex)
			if k && helpers[key] != nil {
				return fail("helper method handle")
			}
		}
	}
	type consumer struct {
		method sourceLambdaMethodIdentity
		target privateNestMemberKey
	}
	expected, observed := map[consumer]int{}, map[consumer]int{}
	for _, p := range family.plans {
		for site := range p.sites {
			if !nativeProofWork(work, 1) {
				return fail("original call graph budget")
			}
			if site.caller != want.GetClassName() {
				continue
			}
			mk, known := originalConsumers[sourceLambdaMethodIdentity{site.method, site.descriptor}]
			b := targetDeclarations[privateNestMemberKey{p.owner.GetClassName(), site.target.name, site.target.descriptor}]
			if !known || b == nil {
				return fail("original consumer declaration")
			}
			expected[consumer{mk, b.target}]++
		}
	}
	pool := privateNestPoolAppend{count: len(got.ConstantPool) + 1, work: work}
	references := map[privateNestMemberKey]uint16{}
	for _, m := range got.Methods {
		if removed[m] {
			continue
		}
		code := constructorLoweringCode(m)
		if code == nil {
			continue
		}
		if !nativeProofWork(work, int64(len(code.Code))+1) {
			return fail("consumer code budget")
		}
		decoder := core.NewDecompiler(code.Code, nil)
		decoder.Work = work
		if decoder.ParseOpcode() != nil {
			return fail("consumer code")
		}
		ops := constructorMotionOps(decoder)
		if !enumArrayInstructionSpans(code.Code, ops) {
			return fail("consumer instruction boundaries")
		}
		mk, k := methodKey(got, m)
		if !k {
			return fail("fresh consumer identity")
		}
		var replacement []byte
		for _, op := range ops {
			switch op.Instr.OpCode {
			case core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE:
			default:
				continue
			}
			index, k := sourceInvocationCPIndex(got, op, work)
			if !k {
				return fail("consumer reference")
			}
			key, k := privateNestReferenceKey(got, index)
			if !k {
				continue
			}
			b := helpers[key]
			direct := false
			if b == nil && key.owner != own {
				b = targets[key]
				direct = b != nil
			}
			if b == nil {
				continue
			}
			if direct && !hasNestTable || op.Instr.OpCode != core.OP_INVOKEVIRTUAL {
				return fail("private consumer invocation kind")
			}
			edge := consumer{mk, b.target}
			observed[edge]++
			if observed[edge] > expected[edge] {
				return fail("unowned helper consumer")
			}
			if direct {
				continue
			}
			ref := references[b.target]
			if ref == 0 {
				var ok bool
				ref, ok = pool.method(b.target)
				if !ok {
					return fail("fresh constant budget")
				}
				references[b.target] = ref
			}
			if replacement == nil {
				replacement = bytes.Clone(code.Code)
			}
			at := int(op.CurrentOffset)
			if at < 0 || at+3 > len(replacement) {
				return fail("consumer PC")
			}
			binary.BigEndian.PutUint16(replacement[at+1:], ref)
		}
		if replacement != nil {
			payload, e := enumArrayCodePayload(compiled, got, layout.methods[m], work)
			if e != nil {
				return nil, e
			}
			data := bytes.Clone(compiled[payload.span.start:payload.span.end])
			if len(data) < 14+len(code.Code) || !bytes.Equal(data[14:14+len(code.Code)], code.Code) {
				return fail("fresh Code span")
			}
			copy(data[14:14+len(code.Code)], replacement)
			edits = append(edits, compiledMetadataEdit{payload.span, data})
		}
	}
	if len(expected) != len(observed) {
		return fail("missing original call edge")
	}
	for key, n := range expected {
		if observed[key] != n {
			return fail("original call multiplicity")
		}
	}
	if len(removed) > 0 {
		first := len(compiled)
		for _, span := range layout.methods {
			if span.start < first {
				first = span.start
			}
		}
		if first < layout.poolEnd+2 || int(binary.BigEndian.Uint16(compiled[first-2:])) != len(got.Methods) {
			return fail("method count span")
		}
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{first - 2, first}, privateNestWord(uint16(len(got.Methods) - len(removed)))})
	}
	if hasNestTable {
		if len(edits) == 0 {
			return unchanged()
		}
		if len(pool.data) != 0 {
			edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, privateNestWord(uint16(pool.count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, pool.data})
		}
		return applyCompiledMetadataEdits(compiled, edits, work)
	}
	at := layout.classAttributesStart
	if at < layout.poolEnd || at+2 > len(compiled) || int(binary.BigEndian.Uint16(compiled[at:])) != len(got.Attributes) || len(got.Attributes) >= 65535 {
		return fail("class attribute span")
	}
	name, ok := pool.text(oldTable)
	if !ok {
		return fail("nest attribute constant")
	}
	var body []byte
	if oldTable == "NestMembers" {
		body = privateNestWord(uint16(len(oldKeys)))
	}
	for _, key := range oldKeys {
		i, k := pool.class(key)
		if !k {
			return fail("nest class constant")
		}
		body = append(body, privateNestWord(i)...)
	}
	attribute := append(privateNestWord(name), 0, 0, 0, 0)
	binary.BigEndian.PutUint32(attribute[2:], uint32(len(body)))
	attribute = append(attribute, body...)
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{at, at + 2}, privateNestWord(uint16(len(got.Attributes) + 1))}, compiledMetadataEdit{compiledMetadataSpan{len(compiled), len(compiled)}, attribute}, compiledMetadataEdit{compiledMetadataSpan{8, 10}, privateNestWord(uint16(pool.count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, pool.data})
	return applyCompiledMetadataEdits(compiled, edits, work)
}
