package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Flattening changes javac's lexical assertion-status class, but must not
// change when or where an enum root or constant subclass caches that status. Reopen the
// original allocation/constructor/assertion proofs, then change only the
// fresh initializer's class literal. Its field, reads, branches and cache
// remain owned by the same physical enum class.
func (a *sourceConstructorLoweringAuthority) restoreEnumAssertionOwner(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	fail := func(reason string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "enum assertion owner: "+reason)
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
	if want.AccessFlags&0x4000 != 0 && want.GetSupperClassName() == "java/lang/Enum" {
		if !nativeAccessorVersion(want, work) || !a.enumArraySourceReturned(want.GetClassName(), original, work) {
			return decline()
		}
		d := a.jar.nativeMemberReader(want)
		d.Work = work
		proof, known := nativeEnumAssertionInitialization(want, d.nativeAnnotationDeclarationResolver(), work)
		if !known || proof == nil {
			return decline()
		}
		return retargetEnumAssertionStatus(original, compiled, want, proof, want.GetClassName(), true, work)
	}
	if want.AccessFlags != 0x4030 || want.GetSupperClassName() == "java/lang/Enum" || !nativeAccessorVersion(want, work) {
		return decline()
	}
	present := false
	for _, f := range want.Fields {
		n, ok := sourceBridgeUTF8(want, f.NameIndex)
		if ok && n == nativeAssertionField {
			present = true
		}
	}
	if !present {
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
		return fail("original enum parent")
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
	ordinal := 0
	var proof *nativeEnumConstantBody
	bridges := d.originalNativeConstructorAccessBridges()
	resolve := d.nativeAnnotationDeclarationResolver()
	for _, p := range ordered {
		if p.allocatedClass == parent {
			continue
		}
		ordinal++
		if p.allocatedClass == want.GetClassName() {
			if proof != nil {
				return fail("repeated original allocation")
			}
			proof = nativeEnumConstantBodyProof(root, p, ordinal, bridges, resolve, work)
		}
	}
	if proof == nil || proof.assertions == nil || !proof.assertions.pureInitializer || !bytes.Equal(proof.object.Bytes(), original) {
		return fail("original source packet")
	}
	return retargetEnumAssertionStatus(original, compiled, want, proof.assertions, parent, false, work)
}

// Project only the independently regenerated cache's class literal. The JVM
// prefix and all allocation/user instructions retain their relative order.
func retargetEnumAssertionStatus(original, compiled []byte, want *ClassObject, proof *nativeMemberAssertion, freshOwner string, suffix bool, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	fail := func(reason string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "enum assertion owner: "+reason)
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
	if !sourceParameterClassIdentityClosed(want, got) {
		return fail("fresh class identity")
	}
	status := proof.statusOwner
	if already, ok := nativeMemberAssertionProofMode(got, status, work, suffix); ok && already != nil && (suffix || already.pureInitializer) {
		return decline()
	}
	fresh, ok := nativeMemberAssertionProofMode(got, freshOwner, work, suffix)
	if !ok || fresh == nil || !suffix && !fresh.pureInitializer {
		return fail("fresh cache packet")
	}
	oldOps, ok := nativeEnumMethodOps(want, proof.initializer, work)
	if !ok || len(oldOps) < 8 || !suffix && len(oldOps) != 8 {
		return fail("original cache instructions")
	}
	oldIndex, ok := nativeEnumCPIndex(oldOps[0])
	if !ok {
		return fail("original status literal")
	}
	key, ok := sourceParameterClassKey(want, oldIndex)
	if !ok {
		return fail("original raw status identity")
	}
	body := constructorLoweringCode(fresh.initializer)
	ops, ok := nativeEnumMethodOps(got, fresh.initializer, work)
	if !ok || len(ops) < 8 || !suffix && len(ops) != 8 || !enumArrayInstructionSpans(body.Code, ops) {
		return fail("fresh cache instructions")
	}
	width := int(ops[1].CurrentOffset)
	if (width != 2 && width != 3) || !enumArraySuffixClosed(body, ops, width) {
		return fail("fresh cache branch boundary")
	}
	if work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool))*64) != nil {
		return fail("allocation budget")
	}
	count := len(got.ConstantPool) + 1
	index := uint16(0)
	for i, c := range got.ConstantPool {
		if _, isClass := c.(*ConstantClassInfo); isClass {
			raw, known := sourceParameterClassKey(got, uint16(i+1))
			if known && raw == key {
				index = uint16(i + 1)
				break
			}
		}
	}
	var added []byte
	if index == 0 {
		if len(key) > 65535 || count+2 > 65535 || !nativeProofWork(work, int64(len(key))+2) {
			return fail("constant pool budget")
		}
		added = append([]byte{CONSTANT_Utf8, byte(len(key) >> 8), byte(len(key))}, []byte(key)...)
		added = append(added, CONSTANT_Class, byte(count>>8), byte(count))
		index = uint16(count + 1)
		count += 2
	}
	instruction := []byte{core.OP_LDC, byte(index)}
	if width == 3 || index > 255 {
		instruction = []byte{core.OP_LDC_W, byte(index >> 8), byte(index)}
	}
	replacement := append(bytes.Clone(instruction), body.Code[width:]...)
	payload, err := enumArrayCodePayload(compiled, got, layout.methods[fresh.initializer], work)
	if err != nil {
		return nil, err
	}
	rewritten, err := enumRelocateInstructionSpanCode(got, payload.data, body, ops, replacement, 0, width, len(instruction)-width, nil, 0, body.MaxStack, work)
	if err != nil {
		return nil, err
	}
	attribute := make([]byte, 6)
	binary.BigEndian.PutUint16(attribute, payload.name)
	binary.BigEndian.PutUint32(attribute[2:], uint32(len(rewritten)))
	attribute = append(attribute, rewritten...)
	edits := []compiledMetadataEdit{{payload.span, attribute}}
	if len(added) > 0 {
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	}
	return applyCompiledMetadataEdits(compiled, edits, work)
}
