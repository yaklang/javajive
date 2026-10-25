package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java cannot declare an erased-return bridge beside an inherited final generic
// method, even though their JVM descriptors differ. Compile an unused private
// forwarding declaration, then restore just its original name and flags after
// proving the exact fresh super-call. This domain has no argument inference:
// the unchanged implicit receiver supplies all inputs and the result widens.
func (c *ClassObjectDumper) finalInheritedBridgeOriginal(method *MemberInfo) *inheritedBridgePacket {
	if c == nil || c.obj == nil || method == nil || c.obj.MajorVersion < 49 || c.obj.MajorVersion > 52 || method.AccessFlags != 0x1041 && method.AccessFlags != 0x1044 {
		return nil
	}
	p := inheritedBridgeForwardPacket(c.obj, method, core.OP_INVOKESPECIAL, c.Work)
	if p == nil || p.targetOwner != c.obj.GetSupperClassName() || p.descriptor == p.targetDescriptor {
		return nil
	}
	inputs, result, err := callbinding.Descriptor(p.descriptor)
	targetInputs, targetResult, targetErr := callbinding.Descriptor(p.targetDescriptor)
	if err != nil || targetErr != nil || len(inputs) != 0 || len(targetInputs) != 0 || !callbinding.Reference(result) || !callbinding.Reference(targetResult) {
		return nil
	}
	resolver := c.nativeAnnotationDeclarationResolver()
	parent, known := resolver(p.targetOwner)
	if !known || parent == nil || parent.GetClassName() != p.targetOwner {
		return nil
	}
	declaring, target := nativeMemberCallTarget(parent, p.name, p.targetDescriptor, true, resolver, c.Work)
	if target == nil || target.AccessFlags != 0x11 && target.AccessFlags != 0x14 || target.AccessFlags&1 != 0 && method.AccessFlags&1 == 0 {
		return nil
	}
	signature, valid := nativeMethodLocalOriginalSignature(declaring, target.Attributes, c.Work)
	classSignature, classValid := nativeOriginalClassSignature(declaring, c.Work)
	size := len(signature) + len(classSignature)
	if !valid || !classValid || size > 16384 || !nativeProofWork(c.Work, int64(size)*260+1) || c.Work.CheckAlloc(int64(size)*256) != nil {
		return nil
	}
	if signature != "" {
		erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(classSignature, signature)
		if !valid || erased != p.targetDescriptor || !nativeOriginalSignatureThrows(declaring, target, throws, c.Work) || !nativeOriginalClassSignatureSupersClosed(declaring, classSignature, c.Work) {
			return nil
		}
	}
	provider := c.buildInvocationMetadata()
	if !newConstructorWideningQuery(provider).assignable(targetResult, result) {
		return nil
	}
	names := &ClassObjectDumper{obj: c.obj, FuncCtx: &class_context.ClassContext{InvocationMetadata: provider}}
	reserved, complete := names.checkedEscapeInheritedNames()
	if !complete {
		return nil
	}
	index := -1
	matches := 0
	for i, m := range c.obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		reserved[class_context.SafeIdentifier(n)] = true
		if n == p.name && d == p.descriptor {
			matches++
			if m == method {
				index = i
			}
		}
	}
	if matches != 1 || index < 0 {
		return nil
	}
	// Owner bytes and member index make separately emitted helper families
	// disjoint. Reserve every original/inherited source name as well.
	if len(c.obj.GetClassName()) > 16000 {
		return nil
	}
	name := fmt.Sprintf("jdec$super$%x$%d", []byte(c.obj.GetClassName()), index)
	for reserved[name] {
		if len(name) >= 65535 || !nativeProofWork(c.Work, 1) {
			return nil
		}
		name += "$"
	}
	p.sourceName, p.sourceDescriptor, p.visibility = name, p.descriptor, method.AccessFlags&7
	return p
}

func (a *sourceConstructorLoweringAuthority) lowerFinalInheritedBridges(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if !a.bridgeSourceReturned(owner, original, work) {
		return decline()
	}
	fail := func(reason string) ([]byte, error) {
		return nil, sourceLambdaMetadataFailure(work, "final super bridge: "+reason)
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return fail("input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
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
	d := NewClassObjectDumper(want)
	d.Work = work
	d.foldSiblingResolver = a.jar.enumSiblingResolver()
	d.declarationResolver = a.jar.declarationResolver
	plans := map[*MemberInfo]*inheritedBridgePacket{}
	for _, m := range want.Methods {
		if p := d.finalInheritedBridgeOriginal(m); p != nil {
			plans[m] = p
		}
	}
	if len(plans) == 0 {
		return decline()
	}
	if !sourceParameterClassIdentityClosed(want, got) || len(want.Methods) != len(got.Methods) || len(got.Methods) > 4096 || work.CheckAlloc(int64(len(compiled))*2+int64(len(got.Methods))*256) != nil {
		return fail("class or method universe")
	}
	byKey := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, m := range got.Methods {
		name, nk := sourceBridgeUTF8(got, m.NameIndex)
		desc, dk := sourceBridgeUTF8(got, m.DescriptorIndex)
		key := sourceLambdaMethodIdentity{name, desc}
		if !nk || !dk || byKey[key] != nil || !nativeProofWork(work, 1) {
			return fail("fresh declaration identity")
		}
		byKey[key] = m
	}
	count := len(got.ConstantPool) + 1
	var added []byte
	var edits []compiledMetadataEdit
	for _, before := range want.Methods {
		p := plans[before]
		if p == nil {
			continue
		}
		named := byKey[sourceLambdaMethodIdentity{p.sourceName, p.descriptor}]
		completed := byKey[sourceLambdaMethodIdentity{p.name, p.descriptor}]
		if named != nil && completed != nil || named == nil && completed == nil {
			return fail("missing or competing declaration")
		}
		after := named
		if after == nil {
			after = completed
		}
		expectedFlags := uint16(2)
		if named == nil {
			expectedFlags = before.AccessFlags
		}
		fresh := inheritedBridgeNamedForwardPacket(got, after, core.OP_INVOKESPECIAL, work, p.name)
		if after.AccessFlags != expectedFlags || fresh == nil || fresh.targetOwner != p.targetOwner || fresh.targetDescriptor != p.targetDescriptor || !slices.Equal(fresh.throws, p.throws) || !inheritedBridgeAnnotationsEqual(before, p.descriptor, after, p.descriptor, work) {
			return fail("fresh super forwarding changed")
		}
		if named == nil {
			continue
		}
		users := &inheritedBridgePacket{name: p.sourceName, sourceDescriptor: p.descriptor}
		if !inheritedBridgeConsumersClosed(got, nil, users, work) {
			return fail("helper has consumers")
		}
		span, known := layout.methods[after]
		if !known || span.start < layout.poolEnd || span.end > len(compiled) || span.end-span.start < 8 || binary.BigEndian.Uint16(compiled[span.start:]) != after.AccessFlags {
			return fail("method span")
		}
		name, known := sourceParameterUTF8Key(want, before.NameIndex)
		if !known || len(name) > 65535 || count >= 65535 || len(added)+len(name) > 1<<20 || !nativeProofWork(work, int64(len(name))+1) {
			return fail("name constant budget")
		}
		index := uint16(count)
		count++
		added = append(added, CONSTANT_Utf8, byte(len(name)>>8), byte(len(name)))
		added = append(added, name...)
		edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start, span.start + 4}, []byte{byte(before.AccessFlags >> 8), byte(before.AccessFlags), byte(index >> 8), byte(index)}})
	}
	if len(edits) == 0 {
		return decline()
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, []byte{byte(count >> 8), byte(count)}}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	return applyCompiledMetadataEdits(compiled, edits, work)
}
