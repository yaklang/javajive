package javaclassparser

import (
	"bytes"
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A flattened enum keeps its binary identity but javac no longer knows that
// its declaration belonged to another class. JVMS 4.7.6 encodes that relation
// in a named InnerClasses row, independently of its constructors and Code.
// Restore only this one source-owned relation (including occurrences in its
// constant subclasses). Never infer ownership by splitting a dollar name.
func (a *sourceConstructorLoweringAuthority) restoreEnumLexicalOwner(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	fail := func(reason string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "enum lexical owner: "+reason)
	}
	if a == nil || a.jar == nil {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if want.AccessFlags&0x4000 == 0 || want.MajorVersion < 49 || want.MajorVersion > 52 || want.MinorVersion != 0 {
		return decline()
	}
	root, raw := want, original
	if want.GetSupperClassName() != "java/lang/Enum" {
		raw, err = a.jar.readJarEntry(want.GetSupperClassName() + ".class")
		if err != nil {
			return decline()
		}
		if len(raw) > 32<<20 || !nativeProofWork(work, int64(len(raw))+1) {
			return fail("parent budget")
		}
		root, err = ParseWithBudget(raw, work)
		if err != nil {
			return nil, err
		}
	}
	if root != want && root.GetClassName() != want.GetSupperClassName() {
		return fail("original parent identity")
	}
	if root.GetSupperClassName() != "java/lang/Enum" || root.AccessFlags&0x4000 == 0 || root.MajorVersion < 49 || root.MajorVersion > 52 || root.MinorVersion != 0 || !a.enumArraySourceReturned(root.GetClassName(), raw, work) {
		return decline()
	}
	key, ok := sourceParameterClassKey(root, root.ThisClass)
	if !ok {
		return fail("original identity")
	}
	rows, err := sourceAnonymousInnerRows(root, work)
	if err != nil {
		return nil, err
	}
	row, member := rows[key]
	if !member || row.outer == "" || row.name == "" {
		return decline()
	}
	// Enum members are static. Reject an ambiguous local/member declaration,
	// self cycles and flags outside the supported named enum grammar.
	if row.outer == key || row.flags&0x4410 != root.AccessFlags&0x4410 || row.flags&0x4008 != 0x4008 || row.flags & ^uint16(0x441f) != 0 || row.flags&7 != 0 && row.flags&7 != 1 && row.flags&7 != 2 && row.flags&7 != 4 {
		return fail("invalid original membership")
	}
	for _, attr := range root.Attributes {
		if raw, local := attr.(*UnparsedAttribute); local && raw.Name == "EnclosingMethod" {
			return fail("member has enclosing method")
		}
	}
	originalRows, err := sourceAnonymousInnerRows(want, work)
	if err != nil {
		return nil, err
	}
	originalRow, present := originalRows[key]
	if !present {
		return decline()
	}
	same := func(x, y sourceAnonymousInnerRow) bool {
		return x.outer == y.outer && x.name == y.name && x.flags == y.flags && x.anonymous == y.anonymous
	}
	if !same(originalRow, row) {
		return fail("original family disagrees")
	}
	reader, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(reader, nil, layout)
	if err != nil {
		return nil, err
	}
	if !sourceParameterClassIdentityClosed(want, got) || got.MajorVersion < 52 || got.MajorVersion > 65 || got.MinorVersion != 0 {
		return fail("fresh identity or version")
	}
	actual, err := sourceAnonymousInnerRows(got, work)
	if err != nil {
		return nil, err
	}
	if root.GetClassName() == got.GetClassName() {
		for _, attr := range got.Attributes {
			if raw, local := attr.(*UnparsedAttribute); local && raw.Name == "EnclosingMethod" {
				return fail("fresh local ownership")
			}
		}
	}
	if rebuilt, present := actual[key]; present {
		if !same(rebuilt, row) {
			return fail("fresh ownership conflicts")
		}
		return decline()
	}

	if work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool))*64) != nil {
		return fail("allocation budget")
	}
	// Add symbolic constants, preserving every old index and byte. A UTF8 may
	// also be an application string, so changing it in place is never sound.
	count := len(got.ConstantPool) + 1
	added := []byte{}
	appendEntry := func(tag byte, payload []byte) (uint16, bool) {
		if count >= 65535 || len(added)+len(payload)+1 > 1<<20 || !nativeProofWork(work, int64(len(payload))+1) {
			return 0, false
		}
		index := uint16(count)
		count++
		added = append(added, tag)
		added = append(added, payload...)
		return index, true
	}
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	text := func(raw string) (uint16, bool) {
		if len(raw) == 0 || len(raw) > 65535 {
			return 0, false
		}
		return appendEntry(CONSTANT_Utf8, append(word(uint16(len(raw))), []byte(raw)...))
	}
	class := func(raw string) (uint16, bool) {
		index, ok := text(raw)
		if !ok {
			return 0, false
		}
		return appendEntry(CONSTANT_Class, word(index))
	}
	inner, ik := class(key)
	outer, ok := class(row.outer)
	name, nk := text(row.name)
	if !ik || !ok || !nk {
		return fail("constant budget")
	}
	newRow := append(append(append(word(inner), word(outer)...), word(name)...), word(row.flags)...)
	var edits []compiledMetadataEdit
	if layout.innerClasses != nil {
		span := layout.innerClassSpan
		n := len(layout.innerClasses.Classes)
		if n >= 65535 || span.start < layout.classAttributesStart+2 || span.end > len(compiled) || span.end-span.start != 8+8*n {
			return fail("table span")
		}
		table := bytes.Clone(compiled[span.start:span.end])
		binary.BigEndian.PutUint32(table[2:], uint32(2+8*(n+1)))
		binary.BigEndian.PutUint16(table[6:], uint16(n+1))
		table = append(table, newRow...)
		edits = append(edits, compiledMetadataEdit{span, table})
	} else {
		at := layout.classAttributesStart
		if at < layout.poolEnd || at+2 > len(compiled) || len(got.Attributes) >= 65535 || int(binary.BigEndian.Uint16(compiled[at:])) != len(got.Attributes) {
			return fail("class attribute span")
		}
		attribute, ok := text("InnerClasses")
		if !ok {
			return fail("attribute constant")
		}
		table := append(word(attribute), 0, 0, 0, 10, 0, 1)
		table = append(table, newRow...)
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{at, at + 2}, word(uint16(len(got.Attributes) + 1))}, compiledMetadataEdit{compiledMetadataSpan{len(compiled), len(compiled)}, table})
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, word(uint16(count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	return applyCompiledMetadataEdits(compiled, edits, work)
}
