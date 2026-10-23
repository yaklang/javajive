package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The original backing-array protocol is proved before enum source is
// published. A speculative native reader cannot enroll this flat-source
// transaction. As with constructor lowering, permission becomes active only
// on the exact public source return, including its physical release namespace.
func (c *ClassObjectDumper) publishEnumArrayLowering(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || c.nativeEnumBacking == nil {
		return true
	}
	if c.sourceDegraded || legacySourceHasFailureMarker(source) || c.nativeEnumBacking.object != c.obj || !nativeProofWork(c.Work, int64(len(source))+1) {
		return false
	}
	raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
	if err != nil || !bytes.Equal(raw, c.obj.Bytes()) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.enumPending == nil {
		a.enumPending = map[string]constructorNativeSourceOrigin{}
	}
	a.enumPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	return true
}

func (a *sourceConstructorLoweringAuthority) enumArraySourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.enumActive[owner]
	_, pending := a.enumPending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.enumActive[owner]
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// A newer compiler can add a private array helper to a Java8 enum whose
// original initializer constructed that same array inline. The first domain
// is a complete straight-line initializer: declaration-ordered no-extra-arg
// constants, the unique backing producer, RETURN. The fresh helper is inlined
// from its OWN bytes and removed only after proving its only consumer. All
// remaining methods/fields keep their complete identities and other flags.
// No original executable byte is copied, nor is an extra method ignored by the
// metadata gate. Branches, handlers, user initializer effects and opaque PC
// attributes remain outside this executable lowering domain.
func (a *sourceConstructorLoweringAuthority) lowerEnumArrayHelper(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	if !a.enumArraySourceReturned(owner, original, work) {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) {
		return nil, sourceLambdaMetadataFailure(work, "enum lowering input budget")
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
	if want.GetClassName() != owner || want.MajorVersion != 52 || want.MinorVersion != 0 || got.MinorVersion != 0 || got.MajorVersion < 52 || got.MajorVersion > 65 || want.AccessFlags&0x4030 != 0x4030 || want.GetSupperClassName() != "java/lang/Enum" || !sourceParameterClassIdentityClosed(want, got) || len(got.Methods) != len(want.Methods)+1 || len(got.Methods) > 4096 {
		return decline()
	}
	fields, err := sourceFieldIdentities(want, work)
	if err != nil {
		return nil, err
	}
	freshFields, err := sourceFieldIdentities(got, work)
	if err != nil {
		return nil, err
	}
	if len(fields) != len(freshFields) {
		return decline()
	}
	for key, flags := range fields {
		actual, found := freshFields[key]
		if !found || actual != flags {
			return decline()
		}
	}
	constants := []string{}
	for _, field := range want.Fields {
		name, nk := sourceBridgeUTF8(want, field.NameIndex)
		desc, dk := sourceBridgeUTF8(want, field.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(work, 1) {
			return decline()
		}
		if field.AccessFlags&0x4000 != 0 {
			if field.AccessFlags != 0x4019 || desc != "L"+owner+";" || len(constants) == 256 {
				return decline()
			}
			constants = append(constants, name)
		}
	}
	identity := func(object *ClassObject, method *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if method == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		return sourceLambdaMethodIdentity{name, desc}, nk && dk
	}
	expected := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, method := range want.Methods {
		key, known := identity(want, method)
		if !known || expected[key] != nil || !nativeProofWork(work, 1) {
			return decline()
		}
		expected[key] = method
	}
	var helper, initializer, originalInit *MemberInfo
	var helperKey sourceLambdaMethodIdentity
	seen := map[sourceLambdaMethodIdentity]bool{}
	for _, method := range got.Methods {
		key, known := identity(got, method)
		if !known || seen[key] || !nativeProofWork(work, 1) {
			return decline()
		}
		seen[key] = true
		if old := expected[key]; old != nil {
			if old.AccessFlags != method.AccessFlags {
				return decline()
			}
			if key.name == "<clinit>" && key.descriptor == "()V" {
				initializer, originalInit = method, old
			}
		} else {
			if helper != nil || method.AccessFlags != 0x100a || key.name == "<init>" || key.name == "<clinit>" || key.descriptor != "()[L"+owner+";" {
				return decline()
			}
			helper, helperKey = method, key
		}
	}
	if helper == nil || initializer == nil || len(seen) != len(expected)+1 {
		return decline()
	}
	oldOps, oldOK := nativeEnumMethodOps(want, originalInit, work)
	initOps, initOK := nativeEnumMethodOps(got, initializer, work)
	arrayOps, arrayOK := nativeEnumMethodOps(got, helper, work)
	oldCode, initCode, arrayCode := constructorLoweringCode(originalInit), constructorLoweringCode(initializer), constructorLoweringCode(helper)
	if !oldOK || !initOK || !arrayOK || oldCode == nil || initCode == nil || arrayCode == nil || oldCode.MaxLocals != 0 || initCode.MaxLocals != 0 || arrayCode.MaxLocals != 0 || len(arrayOps) == 0 || !nativeEnumOpcode(arrayOps[len(arrayOps)-1], core.OP_ARETURN) || !nativeEnumValuesArrayPacket(got, arrayOps[:len(arrayOps)-1], constants, work) {
		return decline()
	}
	peak := uint16(1)
	if len(constants) != 0 {
		peak = 4
	}
	if oldCode.MaxStack < peak || initCode.MaxStack < peak || arrayCode.MaxStack < peak {
		return decline()
	}
	prefix := len(constants) * 6
	if !enumArrayConstantPrefix(want, oldOps, constants, work) || !enumArrayConstantPrefix(got, initOps, constants, work) || len(initOps) != prefix+3 || len(oldOps) != prefix+4+4*len(constants) || !nativeEnumValuesArrayPacket(want, oldOps[prefix:len(oldOps)-2], constants, work) || !nativeEnumMemberOperand(got, initOps[prefix], core.OP_INVOKESTATIC, owner, helperKey.name, helperKey.descriptor) || !nativeEnumOpcode(initOps[len(initOps)-1], core.OP_RETURN) || !nativeEnumOpcode(oldOps[len(oldOps)-1], core.OP_RETURN) {
		return decline()
	}
	oldStore := constructorMotionMember(want, oldOps[len(oldOps)-2], core.OP_PUTSTATIC)
	if oldStore == nil || !nativeEnumMemberOperand(want, oldOps[len(oldOps)-2], core.OP_PUTSTATIC, owner, oldStore.Member, "[L"+owner+";") || !nativeEnumMemberOperand(got, initOps[prefix+1], core.OP_PUTSTATIC, owner, oldStore.Member, "[L"+owner+";") || fields[sourceFieldIdentity{oldStore.Member, "[L" + owner + ";"}] != 0x101a {
		return decline()
	}
	// A surviving method handle would make deletion observable even without a
	// direct call. Decode all method uses instead of searching operand bytes.
	for _, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		if handle, ok := constant.(*ConstantMethodHandleInfo); ok && handle != nil {
			if handle.ReferenceIndex == 0 || int(handle.ReferenceIndex) > len(got.ConstantPool) {
				return decline()
			}
			ref := nativeConstantMember(got.ConstantPool[handle.ReferenceIndex-1])
			if ref == nil {
				return decline()
			}
			callee, known := sourceBridgeClassName(got, ref.ClassIndex)
			if !known || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(got.ConstantPool) {
				return decline()
			}
			nt, known := got.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !known || nt == nil {
				return decline()
			}
			name, nk := sourceBridgeUTF8(got, nt.NameIndex)
			desc, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
			if !nk || !dk || callee == owner && name == helperKey.name && desc == helperKey.descriptor {
				return decline()
			}
		}
	}
	calls := 0
	for _, method := range got.Methods {
		code := constructorLoweringCode(method)
		if code == nil {
			continue
		}
		decoder := core.NewDecompiler(code.Code, nil)
		decoder.Work = work
		if !nativeProofWork(work, int64(len(code.Code))) || decoder.ParseOpcode() != nil {
			return decline()
		}
		for _, op := range constructorMotionOps(decoder) {
			if nativeEnumMemberOperand(got, op, core.OP_INVOKESTATIC, owner, helperKey.name, helperKey.descriptor) {
				if method != initializer || int(op.CurrentOffset) != int(initOps[prefix].CurrentOffset) {
					return decline()
				}
				calls++
			}
		}
	}
	if calls != 1 {
		return decline()
	}
	return enumArrayFreshMethodEdits(compiled, got, layout, initializer, helper, initOps, arrayOps, prefix, work)
}

// These prefixes end at an empty stack. Each no-extra-argument allocation
// stores its exact original name/ordinal instance before the array is built.
func enumArrayConstantPrefix(object *ClassObject, ops []*core.OpCode, constants []string, work *workbudget.Budget) bool {
	if len(ops) < len(constants)*6 || !nativeProofWork(work, int64(len(constants))*6+1) {
		return false
	}
	owner := object.GetClassName()
	for i, name := range constants {
		at := i * 6
		index, known := nativeEnumCPIndex(ops[at+2])
		literal := ""
		if known && index > 0 && int(index) <= len(object.ConstantPool) && (nativeEnumOpcode(ops[at+2], core.OP_LDC) || nativeEnumOpcode(ops[at+2], core.OP_LDC_W)) {
			if value, ok := object.ConstantPool[index-1].(*ConstantStringInfo); ok && value != nil {
				literal, known = sourceBridgeUTF8(object, value.StringIndex)
			} else {
				known = false
			}
		} else {
			known = false
		}
		ordinal, ordinalOK := nativeEnumOriginalInt(object, ops[at+3])
		if !nativeEnumOpcode(ops[at], core.OP_NEW) || !nativeEnumClassOperand(object, ops[at], owner) || !nativeEnumOpcode(ops[at+1], core.OP_DUP) || !known || literal != name || !ordinalOK || ordinal != i || !nativeEnumMemberOperand(object, ops[at+4], core.OP_INVOKESPECIAL, owner, "<init>", "(Ljava/lang/String;I)V") || !nativeEnumMemberOperand(object, ops[at+5], core.OP_PUTSTATIC, owner, name, "L"+owner+";") {
			return false
		}
	}
	return true
}

func enumArrayFreshMethodEdits(compiled []byte, object *ClassObject, layout *compiledMetadataLayout, initializer, helper *MemberInfo, initOps, arrayOps []*core.OpCode, prefix int, work *workbudget.Budget) ([]byte, error) {
	fail := func(reason string) ([]byte, error) {
		return nil, sourceLambdaMetadataFailure(work, "enum lowering: "+reason)
	}
	initSpan, initKnown := layout.methods[initializer]
	helperSpan, helperKnown := layout.methods[helper]
	first := len(compiled)
	for _, span := range layout.methods {
		if span.start < first {
			first = span.start
		}
	}
	if !initKnown || !helperKnown || first < layout.poolEnd+2 || first > len(compiled) || binary.BigEndian.Uint16(compiled[first-2:]) != uint16(len(object.Methods)) {
		return fail("method inventory byte span")
	}
	initCode, arrayCode := constructorLoweringCode(initializer), constructorLoweringCode(helper)
	callPC := int(initOps[prefix].CurrentOffset)
	arrayEnd := int(arrayOps[len(arrayOps)-1].CurrentOffset)
	if callPC < 0 || callPC+3 > len(initCode.Code) || arrayEnd <= 0 || arrayEnd >= len(arrayCode.Code) || len(initCode.Code)-3+arrayEnd > 65535 || work.CheckAlloc(int64(len(initCode.Code)+arrayEnd)*8) != nil {
		return fail("fresh Code size")
	}
	newCode := append(bytes.Clone(initCode.Code[:callPC]), arrayCode.Code[:arrayEnd]...)
	newCode = append(newCode, initCode.Code[callPC+3:]...)
	initRaw, err := enumArrayCodePayload(compiled, object, initSpan, work)
	if err != nil {
		return nil, err
	}
	helperRaw, err := enumArrayCodePayload(compiled, object, helperSpan, work)
	if err != nil {
		return nil, err
	}
	initLines, initLineName, err := enumArrayCodeLines(initRaw.data, object, initCode, initOps, work)
	if err != nil {
		return nil, err
	}
	helperLines, helperLineName, err := enumArrayCodeLines(helperRaw.data, object, arrayCode, arrayOps, work)
	if err != nil {
		return nil, err
	}
	lines := map[int]uint16{}
	for pc, line := range initLines {
		if pc > callPC {
			pc += arrayEnd - 3
		}
		lines[pc] = line
	}
	for pc, line := range helperLines {
		if pc >= arrayEnd {
			return fail("line on removed helper return")
		}
		lines[callPC+pc] = line
	}
	lineName := initLineName
	if lineName == 0 {
		lineName = helperLineName
	}
	newInfo := make([]byte, 8)
	stack := initCode.MaxStack
	if stack < arrayCode.MaxStack {
		stack = arrayCode.MaxStack
	}
	binary.BigEndian.PutUint16(newInfo, stack)
	binary.BigEndian.PutUint16(newInfo[2:], initCode.MaxLocals)
	binary.BigEndian.PutUint32(newInfo[4:], uint32(len(newCode)))
	newInfo = append(newInfo, newCode...)
	newInfo = append(newInfo, 0, 0, 0, 0) // no handlers; no PC-bearing tables yet
	if lineName != 0 {
		if len(lines) > 65535 {
			return fail("line table size")
		}
		newInfo[len(newInfo)-1] = 1
		body := make([]byte, 2+4*len(lines))
		binary.BigEndian.PutUint16(body, uint16(len(lines)))
		pcs := make([]int, 0, len(lines))
		for pc := range lines {
			pcs = append(pcs, pc)
		}
		sort.Ints(pcs)
		for i, pc := range pcs {
			binary.BigEndian.PutUint16(body[2+4*i:], uint16(pc))
			binary.BigEndian.PutUint16(body[4+4*i:], lines[pc])
		}
		header := make([]byte, 6)
		binary.BigEndian.PutUint16(header, lineName)
		binary.BigEndian.PutUint32(header[2:], uint32(len(body)))
		newInfo = append(append(newInfo, header...), body...)
	}
	attribute := make([]byte, 6)
	binary.BigEndian.PutUint16(attribute, initRaw.name)
	binary.BigEndian.PutUint32(attribute[2:], uint32(len(newInfo)))
	attribute = append(attribute, newInfo...)
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(object.Methods)-1))
	return applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{{compiledMetadataSpan{first - 2, first}, count}, {helperSpan, nil}, {initRaw.span, attribute}}, work)
}

type enumArrayCodeSpan struct {
	span compiledMetadataSpan
	name uint16
	data []byte
}

func enumArrayCodePayload(compiled []byte, object *ClassObject, span compiledMetadataSpan, work *workbudget.Budget) (enumArrayCodeSpan, error) {
	fail := func() (enumArrayCodeSpan, error) {
		return enumArrayCodeSpan{}, sourceLambdaMetadataFailure(work, "enum lowering Code byte span")
	}
	if span.start < 0 || span.end > len(compiled) || span.end-span.start < 8 {
		return fail()
	}
	raw := compiled[span.start:span.end]
	var found enumArrayCodeSpan
	for pos := 8; pos < len(raw); {
		if len(raw)-pos < 6 {
			return fail()
		}
		length := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if length > int64(len(raw)-pos-6) || !nativeProofWork(work, length+1) {
			return fail()
		}
		end := pos + 6 + int(length)
		index := binary.BigEndian.Uint16(raw[pos:])
		name, known := sourceBridgeUTF8(object, index)
		if !known {
			return fail()
		}
		if name == "Code" {
			if found.data != nil {
				return fail()
			}
			found = enumArrayCodeSpan{compiledMetadataSpan{span.start + pos, span.start + end}, index, raw[pos+6 : end]}
		}
		pos = end
	}
	if found.data == nil {
		return fail()
	}
	return found, nil
}

// Code length changes here. Only instruction-anchored lines and empty tables
// are admitted. Refusing handlers/frames/annotations is essential: merely
// shifting their numeric offsets would not prove their verifier/debug meaning.
func enumArrayCodeLines(raw []byte, object *ClassObject, code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) (map[int]uint16, uint16, error) {
	fail := func() (map[int]uint16, uint16, error) {
		return nil, 0, sourceLambdaMetadataFailure(work, "enum lowering unproved Code attributes")
	}
	if len(raw) < 12 || int(binary.BigEndian.Uint32(raw[4:])) != len(code.Code) || len(raw) < 12+len(code.Code) || !bytes.Equal(raw[8:8+len(code.Code)], code.Code) {
		return fail()
	}
	end := 8 + len(code.Code)
	if binary.BigEndian.Uint16(raw[end:]) != 0 {
		return fail()
	}
	positions := map[int]bool{}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = true
	}
	lines := map[int]uint16{}
	seen := map[string]bool{}
	var lineName uint16
	pos := end + 4
	for i := 0; i < int(binary.BigEndian.Uint16(raw[end+2:])); i++ {
		if len(raw)-pos < 6 {
			return fail()
		}
		size := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if size > int64(len(raw)-pos-6) || size < 2 || !nativeProofWork(work, size+1) {
			return fail()
		}
		index := binary.BigEndian.Uint16(raw[pos:])
		name, known := sourceBridgeUTF8(object, index)
		if !known || seen[name] {
			return fail()
		}
		seen[name] = true
		body := raw[pos+6 : pos+6+int(size)]
		count := int(binary.BigEndian.Uint16(body))
		if name == "LineNumberTable" {
			if len(body) != 2+4*count {
				return fail()
			}
			lineName = index
			for j := 0; j < count; j++ {
				pc, line := int(binary.BigEndian.Uint16(body[2+4*j:])), binary.BigEndian.Uint16(body[4+4*j:])
				if !positions[pc] || pc >= len(code.Code) {
					return fail()
				}
				if previous, exists := lines[pc]; exists && previous != line {
					return fail()
				}
				lines[pc] = line
			}
		} else if (name != "StackMapTable" && name != "LocalVariableTable" && name != "LocalVariableTypeTable") || len(body) != 2 || count != 0 {
			return fail()
		}
		pos += 6 + int(size)
	}
	if pos != len(raw) {
		return fail()
	}
	return lines, lineName, nil
}
