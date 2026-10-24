package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

type enumConstructorPortCall struct {
	method            *MemberInfo
	owner, descriptor string
	index             uint16
	pc                int
}

// A constant-specific subclass forwards its own typed parameters followed by
// one null access port. The port cannot be read, computed, or reached through
// another control-flow edge. This grammar is independent of its class name.
func enumConstructorPortForward(object *ClassObject, method *MemberInfo, parent string, work *workbudget.Budget) *enumConstructorPortCall {
	if object == nil || method == nil || method.AccessFlags != 0 || !nativeAccessorVersion(object, work) || object.MajorVersion > 54 {
		return nil
	}
	name, nk := sourceBridgeUTF8(object, method.NameIndex)
	desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, ret, err := callbinding.Descriptor(desc)
	code := constructorLoweringCode(method)
	if !nk || !dk || name != "<init>" || err != nil || ret != "V" || len(params) > 250 || code == nil || len(code.Code) > 512 || len(code.ExceptionTable) != 0 {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if !nativeProofWork(work, int64(len(code.Code))+1) || decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != len(params)+4 || !enumArrayInstructionSpans(code.Code, ops) || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[0]) != 0 {
		return nil
	}
	slot := 1
	for i, typ := range params {
		if !constructorMotionLoad(ops[i+1], typ) || core.GetRetrieveIdx(ops[i+1]) != slot {
			return nil
		}
		slot += constructorEffectType(typ).width()
	}
	call := ops[len(ops)-2]
	member := constructorMotionMember(object, call, core.OP_INVOKESPECIAL)
	if int(code.MaxLocals) < slot || int(code.MaxStack) < slot+1 || !nativeEnumOpcode(ops[len(ops)-3], core.OP_ACONST_NULL) || !nativeEnumOpcode(ops[len(ops)-1], core.OP_RETURN) || member == nil || member.Name != parent || member.Member != "<init>" {
		return nil
	}
	args, result, err := callbinding.Descriptor(member.Description)
	if err != nil || result != "V" || len(args) != len(params)+1 || !slices.Equal(args[:len(params)], params) || !strings.HasPrefix(args[len(params)], "L") || !strings.HasSuffix(args[len(params)], ";") {
		return nil
	}
	index, ok := nativeEnumCPIndex(call)
	if !ok {
		return nil
	}
	return &enumConstructorPortCall{method, parent, member.Description, index, int(call.CurrentOffset)}
}

// Change only the ignored port's declaration and proven null forwarding calls.
// The fresh port may be a real constant subclass, so no class or shared UTF8 is
// renamed or removed. Each class completes independently of traversal order;
// the caller still must complete and verify the entire fresh family.
func (a *sourceConstructorLoweringAuthority) lowerEnumConstructorPort(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if a == nil || a.jar == nil {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, constructorLoweringFailure(work, "enum constructor port input budget")
	}
	want, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	if want.AccessFlags&0x4000 == 0 || want.MajorVersion < 49 || want.MajorVersion > 52 || want.MinorVersion != 0 {
		return decline()
	}
	parent := want.GetClassName()
	root := want
	rootRaw := original
	isRoot := want.GetSupperClassName() == "java/lang/Enum"
	if !isRoot {
		parent = want.GetSupperClassName()
		rootRaw, err = a.jar.readJarEntry(parent + ".class")
		if err != nil {
			return decline()
		}
		if len(rootRaw) > 32<<20 || !nativeProofWork(work, int64(len(rootRaw))+1) {
			return nil, constructorLoweringFailure(work, "enum constructor parent budget")
		}
		root, err = ParseWithBudget(rootRaw, work)
		if err != nil {
			return nil, err
		}
	}
	if root.GetClassName() != parent || root.GetSupperClassName() != "java/lang/Enum" || root.AccessFlags&0x4000 == 0 || root.MajorVersion < 49 || root.MajorVersion > 52 || root.MinorVersion != 0 || !a.enumArraySourceReturned(parent, rootRaw, work) {
		return decline()
	}
	oldReader := NewClassObjectDumper(root)
	oldReader.Work = work
	bridges := oldReader.originalNativeConstructorAccessBridges()
	byTarget := map[string]*nativeConstructorAccessBridge{}
	for _, b := range bridges {
		if byTarget[b.target] != nil {
			return decline()
		}
		byTarget[b.target] = b
	}
	if len(byTarget) == 0 || len(byTarget) > 32 {
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
	if !sourceParameterClassIdentityClosed(want, got) || !nativeAccessorVersion(got, work) || got.MajorVersion > 54 || len(got.Methods) > 4096 || work.CheckAlloc(int64(len(compiled))*3+int64(len(got.ConstantPool))*64) != nil {
		return decline()
	}
	// Append new symbolic constants. Existing entries may serve unrelated
	// literals, field types, class identities or invocation sites.
	poolCount := len(got.ConstantPool) + 1
	var added []byte
	appendConstant := func(tag byte, payload []byte) (uint16, bool) {
		if poolCount >= 65535 || len(added)+len(payload)+1 > 1<<20 || !nativeProofWork(work, int64(len(payload))+1) {
			return 0, false
		}
		index := uint16(poolCount)
		poolCount++
		added = append(added, tag)
		added = append(added, payload...)
		return index, true
	}
	appendText := func(raw string) (uint16, bool) {
		if len(raw) == 0 || len(raw) > 65535 {
			return 0, false
		}
		return appendConstant(CONSTANT_Utf8, append([]byte{byte(len(raw) >> 8), byte(len(raw))}, []byte(raw)...))
	}
	word := func(n uint16) []byte { return []byte{byte(n >> 8), byte(n)} }
	var edits []compiledMetadataEdit
	// All executable uses of each changed symbolic method must be accounted
	// for. In particular a MethodHandle cannot borrow a direct null-call proof.
	changedCalls := map[string]map[int]bool{}
	if isRoot {
		freshReader := NewClassObjectDumper(got)
		freshReader.Work = work
		freshBridges := freshReader.originalNativeConstructorAccessBridges()
		seen := map[string]bool{}
		for _, b := range freshBridges {
			old := byTarget[b.target]
			if old == nil || seen[b.target] {
				return decline()
			}
			seen[b.target] = true
			if old.descriptor == b.descriptor {
				continue
			}
			oldKey, ok := sourceParameterUTF8Key(root, old.method.DescriptorIndex)
			freshKey, fk := sourceParameterUTF8Key(got, b.method.DescriptorIndex)
			// Use lossless raw target keys below; a display spelling cannot
			// establish identity for a surrogate or literal escape collision.
			var targetOld, targetFresh *MemberInfo
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				d, _ := sourceBridgeUTF8(root, m.DescriptorIndex)
				if n == "<init>" && d == old.target {
					targetOld = m
				}
			}
			for _, m := range got.Methods {
				n, _ := sourceBridgeUTF8(got, m.NameIndex)
				d, _ := sourceBridgeUTF8(got, m.DescriptorIndex)
				if n == "<init>" && d == b.target {
					targetFresh = m
				}
			}
			if targetOld == nil || targetFresh == nil || !enumArrayUTF8Equal(root, targetOld.DescriptorIndex, got, targetFresh.DescriptorIndex) {
				return decline()
			}
			targetKey, tk := sourceParameterUTF8Key(root, targetOld.DescriptorIndex)
			prefix := len(targetKey) - 2
			if !ok || !fk || !tk || prefix < 1 || !strings.HasPrefix(oldKey, targetKey[:prefix]) || !strings.HasPrefix(freshKey, targetKey[:prefix]) {
				return decline()
			}
			oldPort, freshPort := oldKey[prefix:len(oldKey)-2], freshKey[prefix:len(freshKey)-2]
			index, ok := appendText(oldKey)
			if !ok {
				return decline()
			}
			span, ok := layout.methods[b.method]
			if !ok || span.end-span.start < 8 {
				return decline()
			}
			edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{span.start + 4, span.start + 6}, word(index)})
			oldPortIndex, ok := appendText(oldPort)
			if !ok {
				return decline()
			}
			debug, err := enumConstructorPortDebugEdits(compiled, got, b.method, layout, oldPortIndex, freshPort, work)
			if err != nil {
				return nil, err
			}
			edits = append(edits, debug...)
			changedCalls[b.descriptor] = map[int]bool{}
		}
		if len(seen) != len(byTarget) {
			return decline()
		}
	} else {
		for _, m := range got.Methods {
			n, _ := sourceBridgeUTF8(got, m.NameIndex)
			if n != "<init>" {
				continue
			}
			d, _ := sourceBridgeUTF8(got, m.DescriptorIndex)
			bridge := byTarget[d]
			if bridge == nil {
				return decline()
			}
			var oldMethod *MemberInfo
			for _, candidate := range want.Methods {
				cn, _ := sourceBridgeUTF8(want, candidate.NameIndex)
				cd, _ := sourceBridgeUTF8(want, candidate.DescriptorIndex)
				if cn == n && cd == d {
					if oldMethod != nil {
						return decline()
					}
					oldMethod = candidate
				}
			}
			if oldMethod == nil || !enumArrayUTF8Equal(want, oldMethod.DescriptorIndex, got, m.DescriptorIndex) {
				return decline()
			}
			left, right := enumConstructorPortForward(want, oldMethod, parent, work), enumConstructorPortForward(got, m, parent, work)
			if left == nil || right == nil || left.descriptor != bridge.descriptor {
				return decline()
			}
			rootKey, rk := sourceParameterClassKey(root, root.ThisClass)
			oldOwner, oldDesc, ok := enumConstructorPortReferenceKeys(want, left.index)
			newOwner, newDesc, nk := enumConstructorPortReferenceKeys(got, right.index)
			bridgeKey, bk := sourceParameterUTF8Key(root, bridge.method.DescriptorIndex)
			ownKey, ck := sourceParameterUTF8Key(got, m.DescriptorIndex)
			if !rk || !ok || !nk || !bk || !ck || oldOwner != rootKey || newOwner != rootKey || oldDesc != bridgeKey || len(ownKey) < 3 || !strings.HasPrefix(newDesc, ownKey[:len(ownKey)-2]) || !strings.HasPrefix(bridgeKey, ownKey[:len(ownKey)-2]) {
				return decline()
			}
			if right.descriptor == left.descriptor {
				continue
			}
			ref := nativeConstantMember(got.ConstantPool[right.index-1])
			if ref == nil {
				return decline()
			}
			nt, ok := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return decline()
			}
			raw, ok := sourceParameterUTF8Key(root, bridge.method.DescriptorIndex)
			if !ok {
				return decline()
			}
			descriptorIndex, ok := appendText(raw)
			if !ok {
				return decline()
			}
			nat, ok := appendConstant(CONSTANT_NameAndType, append(word(nt.NameIndex), word(descriptorIndex)...))
			if !ok {
				return decline()
			}
			index, ok := appendConstant(CONSTANT_Methodref, append(word(ref.ClassIndex), word(nat)...))
			if !ok {
				return decline()
			}
			payload, err := enumArrayCodePayload(compiled, got, layout.methods[m], work)
			if err != nil {
				return nil, err
			}
			pc := payload.span.start + 6 + 8 + right.pc + 1
			edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{pc, pc + 2}, word(index)})
			if changedCalls[right.descriptor] == nil {
				changedCalls[right.descriptor] = map[int]bool{}
			}
			changedCalls[right.descriptor][pc] = true
		}
	}
	if len(edits) == 0 {
		return decline()
	}
	for _, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		if h, ok := constant.(*ConstantMethodHandleInfo); ok && h != nil {
			if h.ReferenceIndex == 0 || int(h.ReferenceIndex) > len(got.ConstantPool) {
				return decline()
			}
			ref := nativeConstantMember(got.ConstantPool[h.ReferenceIndex-1])
			if ref == nil {
				continue
			}
			owner, ok := sourceBridgeClassName(got, ref.ClassIndex)
			if !ok {
				return decline()
			}
			if owner == parent {
				if ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(got.ConstantPool) {
					return decline()
				}
				nt, ok := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return decline()
				}
				n, _ := sourceBridgeUTF8(got, nt.NameIndex)
				d, _ := sourceBridgeUTF8(got, nt.DescriptorIndex)
				if n == "<init>" && changedCalls[d] != nil {
					return decline()
				}
			}
		}
	}
	for _, m := range got.Methods {
		code := constructorLoweringCode(m)
		if code == nil {
			continue
		}
		d := core.NewDecompiler(code.Code, nil)
		d.Work = work
		if !nativeProofWork(work, int64(len(code.Code))+1) || d.ParseOpcode() != nil {
			return decline()
		}
		payload, err := enumArrayCodePayload(compiled, got, layout.methods[m], work)
		if err != nil {
			return nil, err
		}
		for _, op := range constructorMotionOps(d) {
			if op.Instr.OpCode != core.OP_INVOKESPECIAL && op.Instr.OpCode != core.OP_INVOKESTATIC && op.Instr.OpCode != core.OP_INVOKEVIRTUAL && op.Instr.OpCode != core.OP_INVOKEINTERFACE {
				continue
			}
			member := constructorMotionMember(got, op, op.Instr.OpCode)
			if member != nil && member.Name == parent && member.Member == "<init>" && changedCalls[member.Description] != nil {
				pc := payload.span.start + 6 + 8 + int(op.CurrentOffset) + 1
				if op.Instr.OpCode != core.OP_INVOKESPECIAL || !changedCalls[member.Description][pc] {
					return decline()
				}
				delete(changedCalls[member.Description], pc)
			}
		}
	}
	for _, remaining := range changedCalls {
		if len(remaining) != 0 {
			return decline()
		}
	}
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{8, 10}, word(uint16(poolCount))}, compiledMetadataEdit{compiledMetadataSpan{layout.poolEnd, layout.poolEnd}, added})
	return applyCompiledMetadataEdits(compiled, edits, work)
}

func enumConstructorPortReferenceKeys(object *ClassObject, index uint16) (string, string, bool) {
	if index == 0 || int(index) > len(object.ConstantPool) {
		return "", "", false
	}
	ref := nativeConstantMember(object.ConstantPool[index-1])
	if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
		return "", "", false
	}
	nt, ok := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return "", "", false
	}
	owner, ok := sourceParameterClassKey(object, ref.ClassIndex)
	descriptor, dk := sourceParameterUTF8Key(object, nt.DescriptorIndex)
	return owner, descriptor, ok && dk
}

func enumConstructorPortDebugEdits(compiled []byte, object *ClassObject, method *MemberInfo, layout *compiledMetadataLayout, oldPortIndex uint16, freshPort string, work *workbudget.Budget) ([]compiledMetadataEdit, error) {
	fail := func() ([]compiledMetadataEdit, error) {
		return nil, constructorLoweringFailure(work, "enum constructor unused-port debug domain")
	}
	payload, err := enumArrayCodePayload(compiled, object, layout.methods[method], work)
	if err != nil {
		return nil, err
	}
	raw := payload.data
	code := constructorLoweringCode(method)
	if code == nil || len(raw) < 12 || int(binary.BigEndian.Uint32(raw[4:])) != len(code.Code) || !bytes.Equal(raw[8:8+len(code.Code)], code.Code) {
		return fail()
	}
	end := 8 + len(code.Code)
	if len(raw) < end+4 || binary.BigEndian.Uint16(raw[end:]) != 0 {
		return fail()
	}
	desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, _, err := callbinding.Descriptor(desc)
	if err != nil || len(params) == 0 {
		return fail()
	}
	slot := 1
	for _, p := range params[:len(params)-1] {
		slot += constructorEffectType(p).width()
	}
	positions := map[int]bool{len(code.Code): true}
	ops, ok := nativeEnumMethodOps(object, method, work)
	if !ok {
		return fail()
	}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = true
	}
	pos := end + 4
	seen := map[string]bool{}
	var edits []compiledMetadataEdit
	for i := 0; i < int(binary.BigEndian.Uint16(raw[end+2:])); i++ {
		if len(raw)-pos < 6 {
			return fail()
		}
		size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if size < 2 || size > int64(len(raw)-pos-6) || !nativeProofWork(work, size+1) {
			return fail()
		}
		name, ok := sourceBridgeUTF8(object, binary.BigEndian.Uint16(raw[pos:]))
		if !ok || seen[name] {
			return fail()
		}
		seen[name] = true
		body := raw[pos+6 : pos+6+int(size)]
		count := int(binary.BigEndian.Uint16(body))
		switch name {
		case "LineNumberTable":
			if len(body) != 2+4*count {
				return fail()
			}
			for n := 0; n < count; n++ {
				pc := int(binary.BigEndian.Uint16(body[2+4*n:]))
				if !positions[pc] || pc == len(code.Code) {
					return fail()
				}
			}
		case "StackMapTable":
			if !bytes.Equal(body, []byte{0, 0}) {
				return fail()
			}
		case "LocalVariableTable":
			if len(body) != 2+10*count {
				return fail()
			}
			for n := 0; n < count; n++ {
				at := 2 + 10*n
				start := int(binary.BigEndian.Uint16(body[at:]))
				limit := start + int(binary.BigEndian.Uint16(body[at+2:]))
				local := int(binary.BigEndian.Uint16(body[at+8:]))
				if !positions[start] || start == len(code.Code) || !positions[limit] || local >= int(code.MaxLocals) {
					return fail()
				}
				if local == slot {
					key, ok := sourceParameterUTF8Key(object, binary.BigEndian.Uint16(body[at+6:]))
					if !ok || key != freshPort {
						return fail()
					}
					offset := payload.span.start + 6 + pos + 6 + at + 6
					edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{offset, offset + 2}, []byte{byte(oldPortIndex >> 8), byte(oldPortIndex)}})
				}
			}
		default:
			return fail()
		}
		pos += 6 + int(size)
	}
	if pos != len(raw) {
		return fail()
	}
	return edits, nil
}
