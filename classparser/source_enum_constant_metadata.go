package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// InnerClasses reports source modifiers separately from ClassFile runtime
// flags. Different javac generations encode constant subclasses differently.
// Restore the original self row only after the published allocation ownership
// and both forwarding constructors are proved. No Code or shared CP byte changes.
func (a *sourceConstructorLoweringAuthority) restoreEnumConstantModifiers(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	fail := func(reason string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "enum constant modifiers: "+reason)
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
	if want.AccessFlags != 0x4030 || want.GetSupperClassName() == "java/lang/Enum" || want.MajorVersion < 49 || want.MajorVersion > 52 || want.MinorVersion != 0 {
		return decline()
	}
	parent := want.GetSupperClassName()
	raw, err := a.jar.readJarEntry(parent + ".class")
	if err != nil {
		return decline()
	}
	if len(raw) > 32<<20 || !nativeProofWork(work, int64(len(raw))+1) {
		return fail("parent budget")
	}
	if !a.enumArraySourceReturned(parent, raw, work) {
		return decline()
	}
	root, err := ParseWithBudget(raw, work)
	if err != nil {
		return nil, err
	}
	if root.GetClassName() != parent || root.GetSupperClassName() != "java/lang/Enum" {
		return fail("parent identity")
	}
	d := a.jar.nativeMemberReader(root)
	d.Work = work
	plans, err := d.nativeEnumConstantInitializations()
	if err != nil {
		return nil, err
	}
	ordered := make([]nativeEnumConstantAllocation, 0, len(plans))
	for _, p := range plans {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ordinal < ordered[j].ordinal })
	bridges := d.originalNativeConstructorAccessBridges()
	resolve := d.nativeAnnotationDeclarationResolver()
	ordinal := 0
	var proof *nativeEnumConstantBody
	for _, plan := range ordered {
		if plan.allocatedClass == parent {
			continue
		}
		ordinal++
		if plan.allocatedClass == want.GetClassName() {
			if proof != nil {
				return fail("duplicate allocation")
			}
			proof = flatEnumConstantBodyProof(root, plan, ordinal, bridges, resolve, work)
		}
	}
	if proof == nil || !bytes.Equal(proof.object.Bytes(), original) {
		return fail("original forwarding ownership")
	}
	key, known := sourceParameterClassKey(want, want.ThisClass)
	if !known {
		return fail("original identity")
	}
	originalRows, err := sourceAnonymousInnerRows(want, work)
	if err != nil {
		return nil, err
	}
	row, found := originalRows[key]
	if !found || row.outer != "" || row.name != "" || !row.anonymous || row.flags & ^uint16(0x4018) != 0 {
		return fail("original self row")
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
		return fail("fresh identity")
	}
	rows, err := sourceAnonymousInnerRows(got, work)
	if err != nil {
		return nil, err
	}
	actual, found := rows[key]
	if !found || actual.outer != "" || actual.name != "" || !actual.anonymous || actual.flags != 0x4010 && actual.flags != row.flags {
		return fail("fresh self row")
	}
	// Equal metadata needs no constructor rewrite authority. In particular a
	// newer target's nestmate constructor need not use the pre-nest bridge.
	if actual.flags == row.flags {
		return decline()
	}
	if got.MajorVersion > 54 {
		return fail("fresh constructor target outside forwarding proof")
	}
	owner, method, owned := originalAnonymousOwner(got)
	if !owned || owner != parent || method != "" {
		return fail("fresh enclosing owner")
	}
	constructors := 0
	for _, m := range got.Methods {
		name, named := sourceBridgeUTF8(got, m.NameIndex)
		if !named {
			return fail("fresh method name")
		}
		if name != "<init>" {
			continue
		}
		constructors++
		desc, known := sourceBridgeUTF8(got, m.DescriptorIndex)
		forward := enumConstructorPortForward(got, m, parent, work)
		if !known || desc != proof.descriptor || forward == nil || forward.descriptor != proof.superDescriptor {
			return fail("fresh forwarding packet")
		}
	}
	if constructors != 1 {
		return fail("fresh constructor family")
	}
	if layout.innerClasses == nil || work.CheckAlloc(int64(len(compiled))*2) != nil {
		return fail("fresh table or allocation budget")
	}
	span := layout.innerClassSpan
	if span.start < layout.classAttributesStart+2 || span.end > len(compiled) || span.end-span.start != 8+8*len(layout.innerClasses.Classes) {
		return fail("fresh table span")
	}
	position := -1
	for i, entry := range layout.innerClasses.Classes {
		name, known := sourceParameterClassKey(got, entry.InnerClassInfoIndex)
		if !known {
			return fail("fresh row identity")
		}
		if name == key {
			if position >= 0 {
				return fail("duplicate self row")
			}
			position = span.start + 8 + 8*i + 6
		}
	}
	if position < 0 || position+2 > span.end || binary.BigEndian.Uint16(compiled[position:]) != actual.flags {
		return fail("fresh flag span")
	}
	out := bytes.Clone(compiled)
	binary.BigEndian.PutUint16(out[position:], row.flags)
	return out, nil
}
