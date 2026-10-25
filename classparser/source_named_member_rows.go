package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A member's self row and its declaring class's row form one original
// relationship. A flat owner may never mention a member in executable code,
// so javac cannot regenerate that row. Require returned sources at both ends
// and exact reciprocal original declarations; never infer a nest from names.
func (c *ClassObjectDumper) publishNamedMemberRows(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() || c.sourceDegraded || legacySourceHasFailureMarker(source) {
		return true
	}
	rows, err := sourceAnonymousInnerRows(c.obj, c.Work)
	key, known := sourceParameterClassKey(c.obj, c.obj.ThisClass)
	if err != nil || !known {
		return c.Work.Check() == nil
	}
	found := false
	for inner, row := range rows {
		if inner != key && row.outer == key && row.name != "" && !row.anonymous && row.flags&StaticFlag == 0 {
			found = true
		}
	}
	if !found {
		return true
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !nativeProofWork(c.Work, int64(len(raw)+len(source))+1) || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Work.CheckAlloc(int64(len(a.namedRowsPending)+1)*96) != nil {
		return false
	}
	if a.namedRowsPending == nil {
		a.namedRowsPending = map[string]constructorNativeSourceOrigin{}
	}
	a.namedRowsPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) namedMemberRowsReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.namedRowsActive[owner]
	_, pending := a.namedRowsPending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.namedRowsActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

func sameNamedMemberRow(a, b sourceAnonymousInnerRow) bool {
	return a.outer == b.outer && a.name == b.name && a.flags == b.flags && a.anonymous == b.anonymous
}

func (a *sourceConstructorLoweringAuthority) restoreNamedMemberRows(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	fail := func(s string) ([]byte, error) { return nil, constructorLoweringFailure(work, "named member rows: "+s) }
	decline := func() ([]byte, error) { return compiled, work.Check() }
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
	if want.MajorVersion < 49 || want.MajorVersion > 52 || want.MinorVersion != 0 || !a.namedMemberRowsReturned(want.GetClassName(), original, work) {
		return decline()
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
	key, known := sourceParameterClassKey(want, want.ThisClass)
	if !known {
		return fail("original identity")
	}
	wanted, err := sourceAnonymousInnerRows(want, work)
	if err != nil {
		return nil, err
	}
	actual, err := sourceAnonymousInnerRows(got, work)
	if err != nil {
		return nil, err
	}
	var table *InnerClassesAttribute
	for _, attr := range want.Attributes {
		if t, ok := attr.(*InnerClassesAttribute); ok {
			table = t
		}
	}
	var missing []string
	for inner, row := range wanted {
		if inner == key || row.outer != key || row.name == "" || row.anonymous || row.flags&StaticFlag != 0 {
			continue
		}
		if table == nil || row.ordinal < 0 || row.ordinal >= len(table.Classes) {
			return fail("original row ordinal")
		}
		decoded, known := sourceBridgeClassName(want, table.Classes[row.ordinal].InnerClassInfoIndex)
		if !known {
			return fail("original member name")
		}
		raw, e := a.jar.readJarEntry(decoded + ".class")
		if e != nil {
			continue
		}
		if len(raw) > 32<<20 || !nativeProofWork(work, int64(len(raw))+1) {
			return fail("member budget")
		}
		if !a.memberGenericSourceReturned(decoded, raw, work) {
			continue
		}
		child, e := ParseWithBudget(raw, work)
		if e != nil {
			return nil, e
		}
		childKey, valid := sourceParameterClassKey(child, child.ThisClass)
		rows, e := sourceAnonymousInnerRows(child, work)
		if e != nil {
			return nil, e
		}
		self, present := rows[inner]
		owner, _, flags, member := originalMemberOwner(child)
		if !valid || childKey != inner || child.GetClassName() != decoded || !present || !sameNamedMemberRow(row, self) || !member || owner != want.GetClassName() || flags != row.flags {
			return fail("original reciprocal declaration")
		}
		if fresh, present := actual[inner]; present {
			if !sameNamedMemberRow(row, fresh) {
				return fail("fresh declaration conflict")
			}
		} else {
			missing = append(missing, inner)
		}
	}
	if len(missing) == 0 {
		return decline()
	}
	sort.Strings(missing)
	if len(actual)+len(missing) > 65535 || work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool)+len(missing))*128) != nil {
		return fail("output budget")
	}
	count := len(got.ConstantPool) + 1
	var added, rows []byte
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	entry := func(tag byte, data []byte) (uint16, bool) {
		if count >= 65535 || len(added)+len(data)+1 > 1<<20 || !nativeProofWork(work, int64(len(data))+1) {
			return 0, false
		}
		i := uint16(count)
		count++
		added = append(append(added, tag), data...)
		return i, true
	}
	text := func(s string) (uint16, bool) {
		if len(s) == 0 || len(s) > 65535 {
			return 0, false
		}
		return entry(CONSTANT_Utf8, append(word(uint16(len(s))), []byte(s)...))
	}
	class := func(s string) (uint16, bool) {
		i, ok := text(s)
		if !ok {
			return 0, false
		}
		return entry(CONSTANT_Class, word(i))
	}
	for _, inner := range missing {
		row := wanted[inner]
		i, ik := class(inner)
		o, ok := class(row.outer)
		n, nk := text(row.name)
		if !ik || !ok || !nk {
			return fail("constant budget")
		}
		rows = append(rows, word(i)...)
		rows = append(rows, word(o)...)
		rows = append(rows, word(n)...)
		rows = append(rows, word(row.flags)...)
	}
	var edits []compiledMetadataEdit
	if layout.innerClasses != nil {
		span := layout.innerClassSpan
		n := len(layout.innerClasses.Classes)
		if span.start < layout.classAttributesStart+2 || span.end > len(compiled) || span.end-span.start != 8+8*n {
			return fail("table span")
		}
		table := bytes.Clone(compiled[span.start:span.end])
		binary.BigEndian.PutUint32(table[2:], uint32(2+8*(n+len(missing))))
		binary.BigEndian.PutUint16(table[6:], uint16(n+len(missing)))
		table = append(table, rows...)
		edits = append(edits, compiledMetadataEdit{span, table})
	} else {
		at := layout.classAttributesStart
		if at < layout.poolEnd || at+2 > len(compiled) || len(got.Attributes) >= 65535 || int(binary.BigEndian.Uint16(compiled[at:])) != len(got.Attributes) {
			return fail("class attribute span")
		}
		i, ok := text("InnerClasses")
		if !ok {
			return fail("attribute constant")
		}
		table := append(word(i), 0, 0, 0, 0)
		binary.BigEndian.PutUint32(table[2:], uint32(2+len(rows)))
		table = append(table, word(uint16(len(missing)))...)
		table = append(table, rows...)
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{at, at + 2}, word(uint16(len(got.Attributes) + 1))}, compiledMetadataEdit{compiledMetadataSpan{len(compiled), len(compiled)}, table})
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, word(uint16(count))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	return applyCompiledMetadataEdits(compiled, edits, work)
}
