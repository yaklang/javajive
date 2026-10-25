package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source-level anonymous constructor declares no capture parameters. Prove
// the entire physical descriptor is a bijection onto final synthetic captures,
// with a separate zero-argument SUPER edge. Slot widths matter; arity and field
// spelling cannot establish this projection. Named locals, explicit SUPER
// operands and missing lexical declarations retain their own obligations.
func (c *ClassObjectDumper) anonymousCapturedConstructor(method *MemberInfo) *constructorLoweringPacket {
	obj := c.obj
	if obj == nil || method == nil || obj.MajorVersion < 49 || obj.MajorVersion > 52 || obj.MinorVersion != 0 || len(obj.Methods) > 4096 || len(obj.Fields) > 4096 {
		return nil
	}
	name, known := sourceBridgeUTF8(obj, method.NameIndex)
	if !known || name != "<init>" {
		return nil
	}
	owner, _, anonymous := originalAnonymousOwner(obj)
	key, present, valid := sourceEnclosingMethodIdentity(obj, c.Work)
	if !anonymous || !present || !valid {
		return nil
	}
	parent, known := c.nativeAnnotationDeclarationResolver()(owner)
	if !known || parent == nil || len(parent.Methods) > 4096 {
		return nil
	}
	parentKey, known := sourceParameterClassKey(parent, parent.ThisClass)
	if !known || parentKey != key.owner {
		return nil
	}
	matches := 0
	for _, m := range parent.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, nk := sourceParameterUTF8Key(parent, m.NameIndex)
		d, dk := sourceParameterUTF8Key(parent, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == key.name && d == key.descriptor {
			matches++
		}
	}
	if matches != 1 {
		return nil
	}
	constructors := 0
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, known := sourceBridgeUTF8(obj, m.NameIndex)
		if !known {
			return nil
		}
		if n == "<init>" {
			if m != method {
				return nil
			}
			constructors++
		}
	}
	sig, _, known := sourceConstructorSignature(obj, method)
	if constructors != 1 || !known || sig != "" && sig != "()V" {
		return nil
	}
	p := constructorLoweringPacketFor(obj, method, true, true, c.Work)
	if p == nil || p.plan.superDescriptor != "()V" || len(p.plan.arguments) != 0 || len(p.plan.prelude) != 0 || p.plan.qualified != nil {
		return nil
	}
	params, _, err := callbinding.Descriptor(p.plan.descriptor)
	if err != nil || len(params) == 0 || len(params) != len(p.plan.captures) {
		return nil
	}
	slots := constructorParameterSlots(params)
	used := map[int]bool{}
	for _, capture := range p.plan.captures {
		index, exists := slots[capture.slot]
		if !exists || used[capture.slot] || params[index] != capture.field.descriptor {
			return nil
		}
		used[capture.slot] = true
	}
	return p
}

// Completion restores only the independently proved declared-type attribute.
// It neither imports original Code nor changes physical descriptor/slots. The
// same source-return receipt used for enclosing binding is revalidated here;
// a guessed anonymous name or an unreturned speculative body grants no edit.
func (a *sourceConstructorLoweringAuthority) restoreAnonymousCapturedConstructorSignature(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	unchanged := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	fail := func(s string) ([]byte, error) {
		return nil, constructorLoweringFailure(work, "anonymous constructor signature: "+s)
	}
	if a == nil || a.jar == nil {
		return unchanged()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if !a.constructorBindingSourceReturned(want.GetClassName(), original, work) {
		return unchanged()
	}
	d := NewClassObjectDumper(want)
	d.Work, d.foldSiblingResolver, d.declarationResolver = work, a.jar.enumSiblingResolver(), a.jar.declarationResolver
	var before *MemberInfo
	var prior *constructorLoweringPacket
	for _, m := range want.Methods {
		if p := d.anonymousCapturedConstructor(m); p != nil {
			before, prior = m, p
			break
		}
	}
	if prior == nil {
		return unchanged()
	}
	// Java cannot spell ACC_SYNTHETIC on any field, including unrelated
	// assertion storage. Complete that independent whole-field bijection first
	// instead of treating its owed marker as a changed capture contract. The
	// restorer checks every field identity and every other flag, and edits no
	// Code or shared constant. Final artifact metadata rechecks it idempotently.
	compiled, err = RestoreSyntheticFieldMetadata(original, compiled, work)
	if err != nil {
		return nil, err
	}
	r, err := NewClassReaderWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
	got, err := parseWithMetadataLayout(r, nil, layout)
	if err != nil {
		return nil, err
	}
	target := sourceConstructorMetadataPlan{sourceMajor: uint16(a.jar.targetSourceVersion + 44)}
	if !target.targetMatches(want, got, reconstructionCaptureMetadataTarget(a.jar)) || !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(want.Fields) != len(got.Fields) {
		return fail("class/member universe")
	}
	// The independent field transaction above already closed the whole field
	// universe, including legacy Synthetic attributes. Do not reinterpret its
	// normalized flags as raw access bits or grant a second capture-only mask.
	var after *MemberInfo
	for _, m := range got.Methods {
		n, nk := sourceBridgeUTF8(got, m.NameIndex)
		desc, dk := sourceBridgeUTF8(got, m.DescriptorIndex)
		if !nk || !dk {
			return fail("method identity")
		}
		if n == "<init>" {
			if after != nil || desc != prior.plan.descriptor {
				return fail("constructor universe")
			}
			after = m
		}
	}
	if after == nil || after.AccessFlags != before.AccessFlags {
		return fail("constructor flags")
	}
	next := constructorLoweringPacketFor(got, after, true, false, work, len(prior.plan.captures))
	if next == nil {
		next = constructorLoweringPacketFor(got, after, false, false, work, len(prior.plan.captures))
	}
	if next == nil || !reflect.DeepEqual(prior.plan, next.plan) {
		return fail("typed capture/SUPER packet changed")
	}
	we, wk := originalMethodExceptions(want, before)
	ge, gk := originalMethodExceptions(got, after)
	if !wk || !gk || !slices.Equal(we, ge) {
		return fail("constructor throws changed")
	}
	ws, wp, wv := sourceConstructorSignature(want, before)
	gs, gp, gv := sourceConstructorSignature(got, after)
	if !wv || !gv || wp && ws != "()V" || gp && gs != "()V" {
		return fail("signature outside empty declared type")
	}
	if wp == gp {
		return unchanged()
	}
	if !wp {
		edit, e := compiledMethodWithoutAttributes(compiled, got, after, layout, map[string]bool{"Signature": true}, work)
		if e != nil {
			return nil, e
		}
		return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{edit}, work)
	}
	span, known := layout.methods[after]
	if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || len(after.Attributes) >= 65535 || len(got.ConstantPool)+3 >= 65535 || work.CheckAlloc(int64(len(compiled))*2+64) != nil {
		return fail("attribute budget")
	}
	raw := bytes.Clone(compiled[span.start:span.end])
	if int(binary.BigEndian.Uint16(raw[6:])) != len(after.Attributes) {
		return fail("attribute inventory")
	}
	nameIndex := len(got.ConstantPool) + 1
	sigIndex := nameIndex + 1
	binary.BigEndian.PutUint16(raw[6:], uint16(len(after.Attributes)+1))
	raw = append(raw, byte(nameIndex>>8), byte(nameIndex), 0, 0, 0, 2, byte(sigIndex>>8), byte(sigIndex))
	added := []byte{CONSTANT_Utf8, 0, 9}
	added = append(added, []byte("Signature")...)
	added = append(added, CONSTANT_Utf8, 0, 3, '(', ')', 'V')
	count := sigIndex + 1
	return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{{span, raw}, {compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, {compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added}}, work)
}
