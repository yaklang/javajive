package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Lower only fresh compiler adapters covered by the published lambda plan.
// Reintroducing the checked Object entry keeps the original helper ABI and
// metafactory input checks. Every instruction comes from fresh Code or the
// proved typed CHECKCAST operation; no original executable bytes are copied.
func lowerLambdaCheckedInputs(original, compiled []byte, plan *sourceLambdaMetadataPlan, work *workbudget.Budget) ([]byte, *sourceLambdaMetadataPlan, error) {
	fail := func(reason string) ([]byte, *sourceLambdaMetadataPlan, error) {
		return nil, nil, sourceLambdaMetadataFailure(work, "checked lambda input: "+reason)
	}
	if plan == nil {
		return compiled, plan, nil
	}
	changed := false
	for _, p := range plan.methods {
		changed = changed || p.original.descriptor != p.generated.descriptor
	}
	if !changed {
		return compiled, plan, nil
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || len(plan.methods) > 4096 || sha256.Sum256(original) != plan.originalHash || plan.sourceHash == ([32]byte{}) || !nativeProofWork(work, int64(len(original)+len(compiled))+1) || work.CheckAlloc(int64(len(compiled))*4+1<<20) != nil {
		return fail("input identity or budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, nil, err
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, nil, err
	}
	if want.GetClassName() != plan.owner || !sourceParameterClassIdentityClosed(want, got) || want.MajorVersion != 52 || got.MajorVersion != 52 || want.MinorVersion != 0 || got.MinorVersion != 0 || len(want.Methods) != len(got.Methods) {
		return fail("class or method universe")
	}
	methods := func(o *ClassObject) (map[sourceLambdaMethodIdentity]*MemberInfo, bool) {
		out := map[sourceLambdaMethodIdentity]*MemberInfo{}
		for _, m := range o.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return nil, false
			}
			n, nk := sourceBridgeUTF8(o, m.NameIndex)
			d, dk := sourceBridgeUTF8(o, m.DescriptorIndex)
			key := sourceLambdaMethodIdentity{n, d}
			if !nk || !dk || out[key] != nil {
				return nil, false
			}
			out[key] = m
		}
		return out, true
	}
	oldMethods, ok := methods(want)
	if !ok {
		return fail("original declarations")
	}
	freshMethods, ok := methods(got)
	if !ok {
		return fail("fresh declarations")
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	var edits []compiledMetadataEdit
	appendEntry := func(data []byte) (uint16, bool) {
		if count >= 65535 || len(added)+len(data) > 1<<20 || !nativeProofWork(work, int64(len(data))+1) {
			return 0, false
		}
		index := uint16(count)
		count++
		added = append(added, data...)
		return index, true
	}
	utf8 := map[string]uint16{}
	classes := map[string]uint16{}
	for i, entry := range got.ConstantPool {
		if u, ok := entry.(*ConstantUtf8Info); ok && u != nil {
			utf8[u.Value] = uint16(i + 1)
		}
		if _, ok := entry.(*ConstantClassInfo); ok {
			n, valid := sourceBridgeClassName(got, uint16(i+1))
			if !valid {
				return fail("class constant")
			}
			classes[n] = uint16(i + 1)
		}
	}
	text := func(value string) (uint16, bool) {
		if n := utf8[value]; n != 0 {
			return n, true
		}
		payload, e := NewUtf8FromString(value).mutf8BytesChecked()
		if e != nil || len(payload) > 65535 {
			return 0, false
		}
		data := []byte{CONSTANT_Utf8, byte(len(payload) >> 8), byte(len(payload))}
		n, ok := appendEntry(append(data, payload...))
		if ok {
			utf8[value] = n
		}
		return n, ok
	}
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	normalized := *plan
	normalized.methods = slices.Clone(plan.methods)
	seen := map[sourceLambdaMethodIdentity]bool{}
	for i, p := range plan.methods {
		if p.original.descriptor == p.generated.descriptor {
			continue
		}
		if seen[p.generated] {
			return fail("duplicate projection")
		}
		seen[p.generated] = true
		om := oldMethods[p.original]
		// Reopen a completed adapter by its physical declaration and live
		// factory, never by the presence of a spare constant. At most one of
		// the published fresh, input-normalized or fully restored identities
		// may exist. Completion is therefore idempotent without a cache.
		actual := p.generated
		candidates := []sourceLambdaMethodIdentity{p.generated, {p.generated.name, p.original.descriptor}, p.original}
		unique := map[sourceLambdaMethodIdentity]bool{}
		var fm *MemberInfo
		for _, key := range candidates {
			if unique[key] {
				continue
			}
			unique[key] = true
			if m := freshMethods[key]; m != nil {
				if fm != nil {
					return fail("competing implementations")
				}
				fm = m
				actual = key
			}
		}
		if om == nil || fm == nil {
			return fail("missing implementation")
		}
		completed := actual.descriptor == p.original.descriptor
		of, os, known := lambdaCheckedInputFactory(want, p.original.name, p.original.descriptor, work)
		if !known {
			return fail("original factory")
		}
		ff, fs, known := lambdaCheckedInputFactory(got, actual.name, actual.descriptor, work)
		if !known || fs.actual != os.actual || fs.erased != os.erased || fs.functional != os.functional || fs.samName != os.samName || p.generated.descriptor != os.actual {
			return fail("fresh factory adaptation")
		}
		on, nk := sourceBridgeUTF8(want, of.method.NameIndex)
		od, dk := sourceBridgeUTF8(want, of.method.DescriptorIndex)
		fn, fk := sourceBridgeUTF8(got, ff.method.NameIndex)
		fd, td := sourceBridgeUTF8(got, ff.method.DescriptorIndex)
		parent := sourceLambdaMethodIdentity{on, od}
		for _, q := range plan.methods {
			if q.original == parent {
				if freshMethods[q.generated] != nil {
					parent = q.generated
				}
				break
			}
		}
		if !nk || !dk || !fk || !td || parent != (sourceLambdaMethodIdentity{fn, fd}) {
			return fail("factory declaration moved")
		}
		old := lambdaCheckedInputPacket(want, om, os.actual, true, work)
		fresh := lambdaCheckedInputPacket(got, fm, fs.actual, completed, work)
		if old == nil || fresh == nil || old.member != fresh.member || old.owner != fresh.owner {
			return fail("typed forwarding packet")
		}
		if completed {
			normalized.methods[i].generated = actual
			continue
		}
		descriptor, ok := text(p.original.descriptor)
		if !ok {
			return fail("descriptor pool")
		}
		span, ok := layout.methods[fm]
		if !ok {
			return fail("method span")
		}
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start + 4, span.start + 6}, word(descriptor)})
		codeSpan, err := enumArrayCodePayload(compiled, got, span, work)
		if err != nil {
			return nil, nil, err
		}
		code := bytes.Clone(codeSpan.data)
		if !fresh.checked {
			class := classes[fresh.owner]
			if class == 0 {
				n, ok := text(fresh.owner)
				if !ok {
					return fail("cast name")
				}
				class, ok = appendEntry(append([]byte{CONSTANT_Class}, word(n)...))
				if !ok {
					return fail("cast class")
				}
				classes[fresh.owner] = class
			}
			replacement := append([]byte{core.OP_ALOAD_0, core.OP_CHECKCAST, byte(class >> 8), byte(class)}, fresh.code.Code[1:]...)
			decoder := core.NewDecompiler(fresh.code.Code, nil)
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return fail("fresh instruction spans")
			}
			code, err = enumRelocateInstructionSpanCode(got, code, fresh.code, constructorMotionOps(decoder), replacement, 0, 1, 3, nil, 0, 1, work)
			if err != nil {
				return nil, nil, err
			}
		}
		// The unchanged local word now has Object as its declaration type.
		// Adjust only the full-scope parameter row; every other debug shape was
		// rejected by the independently reopened forwarding packet above.
		objectType, ok := text("Ljava/lang/Object;")
		if !ok {
			return fail("debug type")
		}
		end := 8 + int(binary.BigEndian.Uint32(code[4:]))
		pos := end + 4
		for a := 0; a < int(binary.BigEndian.Uint16(code[end+2:])); a++ {
			if pos+6 > len(code) {
				return fail("debug span")
			}
			length := int(binary.BigEndian.Uint32(code[pos+2:]))
			if length > len(code)-pos-6 {
				return fail("debug length")
			}
			name, known := sourceBridgeUTF8(got, binary.BigEndian.Uint16(code[pos:]))
			if !known {
				return fail("debug name")
			}
			if name == "LocalVariableTable" {
				if length != 12 || binary.BigEndian.Uint16(code[pos+6:]) != 1 {
					return fail("debug row")
				}
				binary.BigEndian.PutUint16(code[pos+14:], objectType)
			}
			pos += 6 + length
		}
		if pos != len(code) {
			return fail("debug trailing bytes")
		}
		attribute := append(word(codeSpan.name), make([]byte, 4)...)
		binary.BigEndian.PutUint32(attribute[2:], uint32(len(code)))
		attribute = append(attribute, code...)
		edits = append(edits, compiledMetadataEdit{codeSpan.span, attribute})
		refs := 0
		for n, entry := range got.ConstantPool {
			ref, ok := entry.(*ConstantMethodrefInfo)
			if !ok || ref == nil {
				continue
			}
			owner, known := sourceBridgeClassName(got, ref.ClassIndex)
			if !known {
				return fail("reference owner")
			}
			if owner != plan.owner {
				continue
			}
			if ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(got.ConstantPool) {
				return fail("reference identity")
			}
			nt, ok := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return fail("reference table")
			}
			name, nk := sourceBridgeUTF8(got, nt.NameIndex)
			desc, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
			if !nk || !dk {
				return fail("reference text")
			}
			if (sourceLambdaMethodIdentity{name, desc}) != p.generated {
				continue
			}
			index, ok := appendEntry(append(append([]byte{CONSTANT_NameAndType}, word(nt.NameIndex)...), word(descriptor)...))
			if !ok {
				return fail("reference pool")
			}
			s := layout.constants[n]
			if s.end-s.start != 5 {
				return fail("reference span")
			}
			raw := bytes.Clone(compiled[s.start:s.end])
			binary.BigEndian.PutUint16(raw[3:], index)
			edits = append(edits, compiledMetadataEdit{s, raw})
			refs++
		}
		if refs == 0 {
			return fail("missing live implementation reference")
		}
		normalized.methods[i].generated.descriptor = p.original.descriptor
	}
	if len(edits) == 0 {
		if err := work.Check(); err != nil {
			return nil, nil, err
		}
		return compiled, &normalized, nil
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, word(uint16(count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	out, err := applyCompiledMetadataEdits(compiled, edits, work)
	if err != nil {
		return nil, nil, err
	}
	return out, &normalized, nil
}
