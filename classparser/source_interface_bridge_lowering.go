package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Pre-default-method interfaces express covariant/generic overrides without
// executable bridges. New javac can add default bridges, changing both the
// declared method universe and initialization of implementing classes (JVMS
// 5.4.3.4, 5.5). Only the original abstract protocol grants this obligation.
func legacyAbstractInterfaceDomain(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil || obj.MajorVersion < 49 || obj.MajorVersion >= 52 || obj.MinorVersion != 0 || hasOriginalNestAttribute(obj, "NestHost") || hasOriginalNestAttribute(obj, "NestMembers") || obj.AccessFlags&^uint16(1) != 0x0600 || obj.GetSupperClassName() != "java/lang/Object" || len(obj.Methods) > 4096 {
		return false
	}
	methods := 0
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, known := sourceBridgeUTF8(obj, m.NameIndex)
		if !known {
			return false
		}
		if n == "<clinit>" {
			if m.AccessFlags != 8 {
				return false
			}
			continue
		}
		if strings.HasPrefix(n, "<") || m.AccessFlags&^uint16(0x0080) != 0x0401 || constructorLoweringCode(m) != nil {
			return false
		}
		methods++
	}
	return methods > 0
}

// An omitted declaration must still resolve to an original abstract method.
// Inspect every ancestor, so an unresolved branch, cycle or competing default
// cannot be mistaken for an inherited abstract contract.
func inheritedAbstractInterfaceMethod(d *ClassObjectDumper, name, descriptor string) bool {
	resolve := d.nativeAnnotationDeclarationResolver()
	seen, active := map[string]bool{}, map[string]bool{}
	found := false
	var visit func(string) bool
	visit = func(owner string) bool {
		if active[owner] || len(seen) >= 256 || !nativeProofWork(d.Work, 1) {
			return false
		}
		if seen[owner] {
			return true
		}
		obj, known := resolve(owner)
		if !known || obj == nil || obj.GetClassName() != owner || obj.AccessFlags&0x0200 == 0 || len(obj.Methods) > 4096 {
			return false
		}
		seen[owner], active[owner] = true, true
		matches := 0
		for _, m := range obj.Methods {
			if m == nil || !nativeProofWork(d.Work, 1) {
				return false
			}
			n, nk := sourceBridgeUTF8(obj, m.NameIndex)
			desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
			if !nk || !dk {
				return false
			}
			if n == name && desc == descriptor {
				matches++
				if matches > 1 || m.AccessFlags&^uint16(0x0080) != 0x0401 || constructorLoweringCode(m) != nil {
					return false
				}
				found = true
			}
		}
		for _, parent := range obj.GetInterfacesName() {
			if !visit(parent) {
				return false
			}
		}
		delete(active, owner)
		return true
	}
	active[d.obj.GetClassName()] = true
	for _, owner := range d.obj.GetInterfacesName() {
		if !visit(owner) {
			return false
		}
	}
	return found
}

func interfaceBridgeForwardTarget(obj *ClassObject, method *MemberInfo, provider callbinding.Provider, work *workbudget.Budget) string {
	if method == nil || method.AccessFlags != 0x1041 {
		return ""
	}
	name, nk := sourceBridgeUTF8(obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(desc)
	if !nk || !dk || strings.HasPrefix(name, "<") || err != nil || len(params) > 255 {
		return ""
	}
	code := constructorLoweringCode(method)
	if code == nil || len(code.Code) > 2048 || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))+1) {
		return ""
	}
	textWork := int64(0)
	if _, _, err := sourceParameterRows(obj, method, work, &textWork); err != nil {
		return ""
	}
	codeCount := 0
	for _, a := range method.Attributes {
		switch a := a.(type) {
		case *CodeAttribute:
			codeCount++
			if a == nil || codeCount != 1 {
				return ""
			}
		case *ExceptionsAttribute, *RuntimeVisibleAnnotationsAttribute:
			// These describe only the added declaration, absent in the original.
		case *RuntimeVisibleParameterAnnotationsAttribute:
			// javac copies both RUNTIME and CLASS parameter annotations to its
			// bridge. They belong to that added declaration, independently of
			// the cast/forwarding program; the original target keeps its bytes.
			// A bridge has no hidden constructor ports to justify a short table.
			if a == nil || int(a.NumParameters) != len(params) || len(a.ParameterAnnotations) != len(params) {
				return ""
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "MethodParameters" {
				return ""
			}
		default:
			return ""
		}
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return ""
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 3 || !enumArrayInstructionSpans(code.Code, ops) || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
		return ""
	}
	invoke := ops[len(ops)-2]
	call := constructorMotionMember(obj, invoke, core.OP_INVOKEINTERFACE)
	if call == nil || call.Name != obj.GetClassName() || call.Member != name || call.Description == desc || len(invoke.Data) != 4 || invoke.Data[3] != 0 {
		return ""
	}
	index := core.Convert2bytesToInt(invoke.Data[:2])
	if index <= 0 || int(index) > len(obj.ConstantPool) {
		return ""
	}
	if _, ok := obj.ConstantPool[index-1].(*ConstantInterfaceMethodrefInfo); !ok {
		return ""
	}
	targetParams, targetResult, err := callbinding.Descriptor(call.Description)
	if err != nil || len(targetParams) != len(params) || !callbinding.Assignable(targetResult, result, provider) {
		return ""
	}
	cursor, slot := 1, 1
	for i, param := range params {
		if cursor >= len(ops)-2 || core.GetRetrieveIdx(ops[cursor]) != slot || !constructorMotionLoad(ops[cursor], param) {
			return ""
		}
		cursor++
		if param != targetParams[i] {
			if !callbinding.Reference(param) || !callbinding.Reference(targetParams[i]) || !callbinding.Assignable(targetParams[i], param, provider) || cursor >= len(ops)-2 || ops[cursor].Instr.OpCode != core.OP_CHECKCAST || len(ops[cursor].Data) != 2 {
				return ""
			}
			typeName, known := sourceBridgeClassName(obj, uint16(core.Convert2bytesToInt(ops[cursor].Data)))
			if !strings.HasPrefix(typeName, "[") {
				typeName = "L" + typeName + ";"
			}
			if !known || typeName != targetParams[i] {
				return ""
			}
			cursor++
		}
		slot += constructorEffectType(param).width()
	}
	ret := core.OP_ARETURN
	switch result {
	case "V":
		ret = core.OP_RETURN
	case "J":
		ret = core.OP_LRETURN
	case "D":
		ret = core.OP_DRETURN
	case "F":
		ret = core.OP_FRETURN
	case "Z", "B", "C", "S", "I":
		ret = core.OP_IRETURN
	}
	stack := slot
	if width := constructorEffectType(targetResult).width(); width > stack {
		stack = width
	}
	if cursor != len(ops)-2 || int(invoke.Data[2]) != slot || !nativeEnumOpcode(ops[len(ops)-1], ret) || int(code.MaxLocals) < slot || int(code.MaxStack) < stack {
		return ""
	}
	return call.Description
}

// Remove only fresh forwarding methods absent from a sealed original legacy
// interface. Every retained method and CP byte stays fresh and unchanged. The
// ancestor declaration and exact typed call ensure ordinary invokeinterface
// resolution remains available; final metadata checks still own every other
// difference. This never deletes an original default method or class bridge.
func (a *sourceConstructorLoweringAuthority) lowerLegacyInterfaceBridges(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if a == nil || a.jar == nil || !a.bridgeSourceReturned(owner, original, work) {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "interface bridge input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if !legacyAbstractInterfaceDomain(want, work) {
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
	if !sourceParameterClassIdentityClosed(want, got) || got.MajorVersion < 52 || got.MajorVersion > 65 || got.MinorVersion != 0 || len(got.Methods) > 4096 || len(got.Methods) <= len(want.Methods) || want.GetClassName() != owner {
		return decline()
	}
	if work.CheckAlloc(int64(len(got.Methods))*512) != nil {
		return nil, work.Err()
	}
	key := func(obj *ClassObject, method *MemberInfo) string {
		n, _ := sourceBridgeUTF8(obj, method.NameIndex)
		d, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		return n + "\x00" + d
	}
	before, after := map[string]*MemberInfo{}, map[string]*MemberInfo{}
	for _, pair := range []struct {
		obj     *ClassObject
		methods map[string]*MemberInfo
	}{{want, before}, {got, after}} {
		for _, method := range pair.obj.Methods {
			if method == nil || !nativeProofWork(work, 1) || pair.methods[key(pair.obj, method)] != nil {
				return decline()
			}
			pair.methods[key(pair.obj, method)] = method
		}
	}
	for k, m := range before {
		if after[k] == nil || after[k].AccessFlags != m.AccessFlags || m.AccessFlags&0x0400 != 0 && constructorLoweringCode(after[k]) != nil {
			return decline()
		}
	}
	d := a.jar.nativeMemberReader(want)
	d.Work = work
	provider := d.buildInvocationMetadata()
	var edits []compiledMetadataEdit
	for k, method := range after {
		if before[k] != nil {
			continue
		}
		n, _ := sourceBridgeUTF8(got, method.NameIndex)
		desc, _ := sourceBridgeUTF8(got, method.DescriptorIndex)
		target := interfaceBridgeForwardTarget(got, method, provider, work)
		originalTarget := before[n+"\x00"+target]
		if target == "" || originalTarget == nil || originalTarget.AccessFlags&^uint16(0x0080) != 0x0401 || !inheritedAbstractInterfaceMethod(d, n, desc) {
			return decline()
		}
		edits = append(edits, compiledMetadataEdit{layout.methods[method], nil})
	}
	// Interface invocation can inherit the original abstract declaration.
	// A direct default-super consumer has different selection semantics and
	// is outside this obligation, even when its symbolic descriptor matches.
	if work.CheckAlloc(int64(len(got.ConstantPool))*32) != nil {
		return nil, work.Err()
	}
	removedRefs := map[uint16]bool{}
	for i, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		ref, ok := constant.(*ConstantInterfaceMethodrefInfo)
		if !ok || ref == nil {
			continue
		}
		refOwner, known := sourceBridgeClassName(got, ref.ClassIndex)
		if !known || int(ref.NameAndTypeIndex) > len(got.ConstantPool) || ref.NameAndTypeIndex == 0 {
			return decline()
		}
		nt, ok := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return decline()
		}
		n, nk := sourceBridgeUTF8(got, nt.NameIndex)
		desc, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
		if !nk || !dk {
			return decline()
		}
		if refOwner == owner && before[n+"\x00"+desc] == nil && after[n+"\x00"+desc] != nil {
			removedRefs[uint16(i+1)] = true
		}
	}
	for _, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		if handle, ok := constant.(*ConstantMethodHandleInfo); ok && handle != nil && removedRefs[handle.ReferenceIndex] {
			return decline()
		}
	}
	if len(removedRefs) > 0 {
		for _, method := range got.Methods {
			code := constructorLoweringCode(method)
			if code == nil {
				continue
			}
			if !nativeProofWork(work, int64(len(code.Code))+1) {
				return decline()
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(got.ConstantPool, i) })
			decoder.Work = work
			if decoder.ParseOpcode() != nil {
				return decline()
			}
			for _, op := range constructorMotionOps(decoder) {
				if (op.Instr.OpCode == core.OP_INVOKESPECIAL || op.Instr.OpCode == core.OP_INVOKESTATIC || op.Instr.OpCode == core.OP_INVOKEVIRTUAL) && len(op.Data) >= 2 && removedRefs[core.Convert2bytesToInt(op.Data[:2])] {
					return decline()
				}
			}
		}
	}
	first := len(compiled)
	for _, span := range layout.methods {
		if span.start < first {
			first = span.start
		}
	}
	if len(edits) != len(after)-len(before) || first < layout.poolEnd+2 || first > len(compiled) || binary.BigEndian.Uint16(compiled[first-2:]) != uint16(len(got.Methods)) {
		return decline()
	}
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(before)))
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{first - 2, first}, count})
	return applyCompiledMetadataEdits(compiled, edits, work)
}
