package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

type sourceEnumArrayOrigin struct {
	owner        string
	originalHash [32]byte
}

// The original backing-array protocol is proved before enum source is
// published. A speculative native reader cannot enroll this flat-source
// transaction. As with constructor lowering, permission becomes active only
// on the exact public source return, including its physical release namespace.
func (c *ClassObjectDumper) publishEnumArrayLowering(source string) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || c.nativeEnumBacking == nil {
		return true
	}
	if c.sourceDegraded || legacySourceHasFailureMarker(source) || c.nativeEnumBacking.object != c.obj {
		// Optional executable authority must not erase an otherwise useful
		// diagnostic source unit. An explicitly uncompleted local receipt also
		// prevents this physical namespace from borrowing a parent's permission.
		a.mu.Lock()
		if a.enumPending == nil {
			a.enumPending = map[string]constructorNativeSourceOrigin{}
		}
		a.enumPending[c.obj.GetClassName()] = constructorNativeSourceOrigin{}
		a.mu.Unlock()
		return true
	}
	if !nativeProofWork(c.Work, int64(len(source))+1) {
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

// A different compiler can add a private array helper to an enum whose
// original initializer constructed that same array inline. Declaration-ordered
// constants and the unique backing producer precede an isolated user suffix.
// User operands need typed allocation
// origins and an empty stack at the array cut. The fresh helper is inlined
// from its OWN bytes and removed only after proving its only consumer. All
// remaining methods/fields keep their complete identities and other flags.
// No original executable byte is copied, nor is an extra method ignored by the
// metadata gate. Suffix branches retain their bytes under an aligned affine PC
// map; verifier/debug locations are relocated together. Handlers and opaque PC
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
	// Inline array construction remains valid in newer class versions. The
	// compiler's helper strategy is not determined by the target version:
	// qualify every supported non-preview input by its actual producer,
	// fresh helper, complete consumers and suffix PCs below.
	if want.GetClassName() != owner || want.MajorVersion < 49 || want.MajorVersion > 65 || want.MinorVersion != 0 || got.MinorVersion != 0 || got.MajorVersion < 52 || got.MajorVersion > 65 || want.AccessFlags&0x4020 != 0x4020 || want.GetSupperClassName() != "java/lang/Enum" || !sourceParameterClassIdentityClosed(want, got) || len(got.Methods) != len(want.Methods)+1 || len(got.Methods) > 4096 {
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
	// Constant ordinals are int operands, not byte-sized indexes. The parsed
	// classfile field tables bound this inventory; account for its maps and
	// ordered views through the request budget. Complete packet lengths and
	// the JVM Code size limit below still bound the executable transformation.
	if work.CheckAlloc(int64(len(want.Fields)+len(got.Fields)+1)*192) != nil {
		return nil, work.Err()
	}
	wantMembers, freshMembers := map[sourceFieldIdentity]*MemberInfo{}, map[sourceFieldIdentity]*MemberInfo{}
	for _, pair := range []struct {
		object *ClassObject
		index  map[sourceFieldIdentity]*MemberInfo
	}{{want, wantMembers}, {got, freshMembers}} {
		for _, field := range pair.object.Fields {
			name, _ := sourceBridgeUTF8(pair.object, field.NameIndex)
			desc, _ := sourceBridgeUTF8(pair.object, field.DescriptorIndex)
			if !nativeProofWork(work, int64(len(name)+len(desc))+1) {
				return decline()
			}
			pair.index[sourceFieldIdentity{name, desc}] = field
		}
	}
	for key, flags := range fields {
		actual, found := freshFields[key]
		if !found || actual != flags || !enumArrayUTF8Equal(want, wantMembers[key].NameIndex, got, freshMembers[key].NameIndex) || !enumArrayUTF8Equal(want, wantMembers[key].DescriptorIndex, got, freshMembers[key].DescriptorIndex) {
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
			if field.AccessFlags != 0x4019 || desc != "L"+owner+";" {
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
		if !known || expected[key] != nil || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return decline()
		}
		expected[key] = method
	}
	var helper, initializer, originalInit *MemberInfo
	var helperKey sourceLambdaMethodIdentity
	seen := map[sourceLambdaMethodIdentity]bool{}
	for _, method := range got.Methods {
		key, known := identity(got, method)
		if !known || seen[key] || !nativeProofWork(work, int64(len(key.name)+len(key.descriptor))+1) {
			return decline()
		}
		seen[key] = true
		if old := expected[key]; old != nil {
			// Source declarations cannot spell ACC_SYNTHETIC. That metadata
			// obligation is discharged after removing the independently proved
			// array helper; its bijection cannot run while that helper exists.
			// Preserve the fresh flags here. No execution/access flag may differ,
			// and no newly synthetic or constructor declaration is accepted.
			flagsMatch := old.AccessFlags == method.AccessFlags || key.name != "<init>" && key.name != "<clinit>" && old.AccessFlags == method.AccessFlags|syntheticMethodFlag
			if !flagsMatch || !enumArrayUTF8Equal(want, old.NameIndex, got, method.NameIndex) || !enumArrayUTF8Equal(want, old.DescriptorIndex, got, method.DescriptorIndex) {
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
	if !oldOK || !initOK || !arrayOK || oldCode == nil || initCode == nil || arrayCode == nil || arrayCode.MaxLocals != 0 || len(arrayOps) == 0 || !nativeEnumOpcode(arrayOps[len(arrayOps)-1], core.OP_ARETURN) || !nativeEnumValuesArrayPacket(got, arrayOps[:len(arrayOps)-1], constants, work) {
		return decline()
	}
	// The source-regeneration decoder can omit harmless NOPs. Executable
	// splicing additionally needs every raw instruction boundary accounted for;
	// a gap cannot be borrowed as a proof for its debug PCs or byte span.
	if !enumArrayInstructionSpans(oldCode.Code, oldOps) || !enumArrayInstructionSpans(initCode.Code, initOps) || !enumArrayInstructionSpans(arrayCode.Code, arrayOps) {
		return decline()
	}
	if nativeEnumOpcode(oldOps[0], core.OP_GOTO) || nativeEnumOpcode(oldOps[0], core.OP_GOTO_W) {
		var known bool
		oldOps, known = enumArrayOriginalEntryPath(oldCode, oldOps, work)
		if !known {
			return decline()
		}
	}
	peak := uint16(1)
	if len(constants) != 0 {
		peak = 4
	}
	if oldCode.MaxStack < peak || initCode.MaxStack < peak || arrayCode.MaxStack < peak {
		return decline()
	}
	// Constant-specific allocations retain their exact class and constructor.
	// Original sibling declarations establish those identities; no sibling Code
	// is copied or edited. Each actual fresh sibling still owes FinishClass.
	declarations := map[string]*ClassObject{}
	declarationBytes := int64(0)
	resolve := func(name string) (*ClassObject, bool) {
		if object, found := declarations[name]; found {
			return object, object != nil
		}
		if a.jar == nil || len(declarations) >= 256 || !nativeProofWork(work, 1) {
			return nil, false
		}
		raw, found := a.jar.enumSiblingResolver()(name)
		if !found || int64(len(raw)) > (32<<20)-declarationBytes {
			return nil, false
		}
		declarationBytes += int64(len(raw))
		if work.CheckAlloc(declarationBytes*8) != nil {
			return nil, false
		}
		object, err := ParseWithBudget(raw, work)
		if err != nil || object.GetClassName() != name {
			return nil, false
		}
		declarations[name] = object
		return object, true
	}
	oldPrefix, oldPrefixOK := enumArrayConstantBoundary(want, originalInit, oldOps, constants, wantMembers, resolve, work)
	prefix, prefixOK := enumArrayConstantBoundary(got, initializer, initOps, constants, freshMembers, resolve, work)
	oldStoreAt := oldPrefix + 2 + 4*len(constants)
	if !oldPrefixOK || !prefixOK || len(initOps) < prefix+3 || len(oldOps) < oldStoreAt+2 || !nativeEnumValuesArrayPacket(want, oldOps[oldPrefix:oldStoreAt], constants, work) || !nativeEnumMemberOperand(got, initOps[prefix], core.OP_INVOKESTATIC, owner, helperKey.name, helperKey.descriptor) || !enumArrayOwnMember(got, initOps[prefix], helper.NameIndex, helper.DescriptorIndex) || !nativeEnumOpcode(initOps[len(initOps)-1], core.OP_RETURN) || !nativeEnumOpcode(oldOps[len(oldOps)-1], core.OP_RETURN) {
		return decline()
	}
	oldStore := constructorMotionMember(want, oldOps[oldStoreAt], core.OP_PUTSTATIC)
	if oldStore == nil || !nativeEnumMemberOperand(want, oldOps[oldStoreAt], core.OP_PUTSTATIC, owner, oldStore.Member, "[L"+owner+";") || !nativeEnumMemberOperand(got, initOps[prefix+1], core.OP_PUTSTATIC, owner, oldStore.Member, "[L"+owner+";") || fields[sourceFieldIdentity{oldStore.Member, "[L" + owner + ";"}] != 0x101a {
		return decline()
	}
	backingKey := sourceFieldIdentity{oldStore.Member, "[L" + owner + ";"}
	if !enumArrayOwnMember(want, oldOps[oldStoreAt], wantMembers[backingKey].NameIndex, wantMembers[backingKey].DescriptorIndex) || !enumArrayOwnMember(got, initOps[prefix+1], freshMembers[backingKey].NameIndex, freshMembers[backingKey].DescriptorIndex) || !enumArrayExactElements(want, oldOps[oldPrefix:oldStoreAt], constants, wantMembers) || !enumArrayExactElements(got, arrayOps[:len(arrayOps)-1], constants, freshMembers) {
		return decline()
	}
	// User initialization is an unchanged suffix. Every branch must stay in
	// that suffix, so no edge can re-enter an allocation or the array producer.
	if !enumArraySuffixAfterAssertionPrefixClosed(want, originalInit, oldCode, oldOps, int(oldOps[oldStoreAt+1].CurrentOffset), work) || !enumArraySuffixAfterAssertionPrefixClosed(got, initializer, initCode, initOps, int(initOps[prefix+2].CurrentOffset), work) {
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
			if !nativeProofWork(work, 1) {
				return decline()
			}
			if !nativeEnumOpcode(op, core.OP_INVOKESTATIC) && !nativeEnumOpcode(op, core.OP_INVOKEVIRTUAL) && !nativeEnumOpcode(op, core.OP_INVOKESPECIAL) && !nativeEnumOpcode(op, core.OP_INVOKEINTERFACE) {
				continue
			}
			member := constructorMotionMember(got, op, op.Instr.OpCode)
			if member == nil {
				return decline()
			}
			if member.Name == owner && member.Member == helperKey.name && member.Description == helperKey.descriptor {
				if !nativeEnumMemberOperand(got, op, core.OP_INVOKESTATIC, owner, helperKey.name, helperKey.descriptor) || method != initializer || int(op.CurrentOffset) != int(initOps[prefix].CurrentOffset) {
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

func enumArrayInstructionSpans(code []byte, ops []*core.OpCode) bool {
	pc := 0
	for _, op := range ops {
		if op == nil || op.Instr == nil || int(op.CurrentOffset) != pc || pc >= len(code) || code[pc] != byte(op.Instr.OpCode) {
			return false
		}
		if op.Instr.OpCode == core.OP_TABLESWITCH || op.Instr.OpCode == core.OP_LOOKUPSWITCH {
			end, _, ok := enumArraySwitchSpan(code, op)
			if !ok {
				return false
			}
			pc = end
		} else {
			if len(op.Data) > len(code)-pc-1 || !bytes.Equal(code[pc+1:pc+1+len(op.Data)], op.Data) {
				return false
			}
			pc += 1 + len(op.Data)
		}
	}
	return pc == len(code)
}

// User constructor arguments have variable spans and JVM category widths. Reuse
// the immutable NEW/invoke/store origin proof, rather than counting expressions
// or instructions. This only locates the empty-stack cut: every argument,
// allocation and constructor instruction remains in its fresh original order.
// The first domain still excludes branches, handlers and user initializer tails.
func enumArrayConstantBoundary(object *ClassObject, initializer *MemberInfo, ops []*core.OpCode, constants []string, fields map[sourceFieldIdentity]*MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (int, bool) {
	if enumArrayConstantPrefix(object, ops, constants, fields, work) {
		return len(constants) * 6, true
	}
	code := constructorLoweringCode(initializer)
	if code == nil || len(constants) == 0 || len(code.ExceptionTable) != 0 {
		return 0, false
	}
	reader := NewClassObjectDumper(object)
	reader.Work = work
	allocations, err := reader.nativeEnumConstantInitializationsWithDeclarations(resolve)
	if err != nil || len(allocations) != len(constants) {
		return 0, false
	}
	_, frames, known := reader.nativeOriginalMethodSnapshot(initializer, code)
	if !known {
		return 0, false
	}
	if work.CheckAlloc(int64(len(ops)+len(frames.Instructions)+1)*192+int64(len(object.Methods)+1)*96) != nil {
		return 0, false
	}
	positions := map[int]int{}
	for i, op := range ops {
		positions[int(op.CurrentOffset)] = i
	}
	records := map[int]ssabuild.InstructionValues{}
	for _, record := range frames.Instructions {
		records[int(record.PC)] = record
	}
	constructors := map[string]*MemberInfo{}
	for _, method := range object.Methods {
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(work, 1) {
			return 0, false
		}
		if name == "<init>" {
			constructors[desc] = method
		}
	}
	end := 0
	// A compiler assertion cache may precede the first allocation. Its exact
	// isolated, empty-stack prefix stays in place; this query only locates the
	// subsequent constant/array cut and grants no status-owner projection.
	if len(ops) >= 8 {
		index, literal := nativeEnumCPIndex(ops[0])
		status, named := sourceBridgeClassName(object, index)
		if literal && named {
			if assertion, valid := nativeMemberAssertionProofMode(object, status, work, true); valid && assertion != nil && assertion.initializer == initializer {
				at, found := positions[assertion.initializerEndPC]
				if !found || at != 7 {
					return 0, false
				}
				end = at
			}
		}
	}
	for ordinal, name := range constants {
		plan, found := allocations[name]
		first, newKnown := positions[plan.newPC]
		last, storeKnown := positions[plan.storePC]
		invoke, invokeKnown := positions[plan.invokePC]
		field := fields[sourceFieldIdentity{name, "L" + object.GetClassName() + ";"}]
		constructor := constructors[plan.descriptor]
		constructorOwner := object
		if found && plan.allocatedClass != object.GetClassName() {
			var known bool
			if resolve == nil {
				return 0, false
			}
			constructorOwner, known = resolve(plan.allocatedClass)
			if !known || constructorOwner == nil {
				return 0, false
			}
			constructor = nil
			for _, method := range constructorOwner.Methods {
				name, nk := sourceBridgeUTF8(constructorOwner, method.NameIndex)
				descriptor, dk := sourceBridgeUTF8(constructorOwner, method.DescriptorIndex)
				if !nk || !dk || !nativeProofWork(work, 1) {
					return 0, false
				}
				if name == "<init>" && descriptor == plan.descriptor {
					if constructor != nil {
						return 0, false
					}
					constructor = method
				}
			}
		}
		if !found || plan.ordinal != ordinal || !newKnown || first != end || !storeKnown || last < first || !invokeKnown || invoke <= first || invoke >= last || field == nil || constructor == nil || !nativeProofWork(work, int64(last-first+1)) {
			return 0, false
		}
		class, ck := nativeEnumCPIndex(ops[first])
		callIndex, callKnown := nativeEnumCPIndex(ops[invoke])
		if !ck || !callKnown || callIndex == 0 || int(callIndex) > len(object.ConstantPool) {
			return 0, false
		}
		ref, refKnown := object.ConstantPool[callIndex-1].(*ConstantMethodrefInfo)
		if !refKnown || ref == nil || !enumArrayClassEqual(object, class, ref.ClassIndex) || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
			return 0, false
		}
		nt, ntKnown := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		newName, newKnown := sourceParameterClassKey(object, class)
		ownerName, ownerKnown := sourceParameterClassKey(constructorOwner, constructorOwner.ThisClass)
		if !ntKnown || nt == nil || !newKnown || !ownerKnown || newName != ownerName || !enumArrayUTF8Equal(object, nt.NameIndex, constructorOwner, constructor.NameIndex) || !enumArrayUTF8Equal(object, nt.DescriptorIndex, constructorOwner, constructor.DescriptorIndex) || !enumArrayOwnMember(object, ops[last], field.NameIndex, field.DescriptorIndex) {
			return 0, false
		}
		call, callKnown := records[plan.invokePC]
		if !callKnown || len(call.Uses) < 3 || call.Uses[1].Kind != ssabuild.OriginInstr {
			return 0, false
		}
		literalAt, literalKnown := positions[int(call.Uses[1].PC)]
		if !literalKnown {
			return 0, false
		}
		index, indexKnown := nativeEnumCPIndex(ops[literalAt])
		if !indexKnown || index == 0 || int(index) > len(object.ConstantPool) {
			return 0, false
		}
		literal, literalKnown := object.ConstantPool[index-1].(*ConstantStringInfo)
		if !literalKnown || literal == nil || !enumArrayUTF8Equal(object, literal.StringIndex, object, field.NameIndex) {
			return 0, false
		}
		for _, op := range ops[first : last+1] {
			if constructorMotionPacketBranch(op.Instr.OpCode) || op.Instr.OpCode == core.OP_TABLESWITCH || op.Instr.OpCode == core.OP_LOOKUPSWITCH {
				return 0, false
			}
		}
		end = last + 1
	}
	if end >= len(ops) {
		return 0, false
	}
	boundary, known := records[int(ops[end].CurrentOffset)]
	return end, known && len(boundary.Before.Stack) == 0
}

// These prefixes end at an empty stack. Each no-extra-argument allocation
// stores its exact original name/ordinal instance before the array is built.
func enumArrayConstantPrefix(object *ClassObject, ops []*core.OpCode, constants []string, fields map[sourceFieldIdentity]*MemberInfo, work *workbudget.Budget) bool {
	if len(ops) < len(constants)*6 || !nativeProofWork(work, int64(len(constants))*6+1) {
		return false
	}
	owner := object.GetClassName()
	var constructor *MemberInfo
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		desc, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nativeProofWork(work, int64(len(name)+len(desc))+1) {
			return false
		}
		if name == "<init>" && desc == "(Ljava/lang/String;I)V" {
			if constructor != nil {
				return false
			}
			constructor = method
		}
	}
	if len(constants) != 0 && constructor == nil {
		return false
	}
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
		field := fields[sourceFieldIdentity{name, "L" + owner + ";"}]
		classIndex, _ := nativeEnumCPIndex(ops[at])
		if field == nil || !nativeEnumOpcode(ops[at], core.OP_NEW) || !nativeEnumClassOperand(object, ops[at], owner) || !enumArrayClassEqual(object, classIndex, object.ThisClass) || !nativeEnumOpcode(ops[at+1], core.OP_DUP) || !known || literal != name || !ordinalOK || ordinal != i || !nativeEnumMemberOperand(object, ops[at+4], core.OP_INVOKESPECIAL, owner, "<init>", "(Ljava/lang/String;I)V") || !nativeEnumMemberOperand(object, ops[at+5], core.OP_PUTSTATIC, owner, name, "L"+owner+";") || !enumArrayOwnMember(object, ops[at+5], field.NameIndex, field.DescriptorIndex) {
			return false
		}
		stringInfo, ok := object.ConstantPool[index-1].(*ConstantStringInfo)
		if !ok || stringInfo == nil || !enumArrayUTF8Equal(object, stringInfo.StringIndex, object, field.NameIndex) || !enumArrayOwnMember(object, ops[at+4], constructor.NameIndex, constructor.DescriptorIndex) {
			return false
		}
	}
	return true
}

// Display strings are not symbol identities: an unpaired UTF-16 unit and a
// literal backslash-u spelling can share the legacy display representation.
func enumArrayUTF8Equal(left *ClassObject, li uint16, right *ClassObject, ri uint16) bool {
	a, ak := sourceParameterUTF8Key(left, li)
	b, bk := sourceParameterUTF8Key(right, ri)
	return ak && bk && a == b
}

func enumArrayClassEqual(object *ClassObject, left, right uint16) bool {
	a, ak := sourceParameterClassKey(object, left)
	b, bk := sourceParameterClassKey(object, right)
	return ak && bk && a == b
}

func enumArrayOwnMember(object *ClassObject, op *core.OpCode, name, descriptor uint16) bool {
	index, known := nativeEnumCPIndex(op)
	if !known || index == 0 || int(index) > len(object.ConstantPool) {
		return false
	}
	ref := nativeConstantMember(object.ConstantPool[index-1])
	if ref == nil || !enumArrayClassEqual(object, ref.ClassIndex, object.ThisClass) || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
		return false
	}
	nt, known := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	return known && nt != nil && enumArrayUTF8Equal(object, nt.NameIndex, object, name) && enumArrayUTF8Equal(object, nt.DescriptorIndex, object, descriptor)
}

func enumArrayExactElements(object *ClassObject, ops []*core.OpCode, constants []string, fields map[sourceFieldIdentity]*MemberInfo) bool {
	if len(ops) != 2+4*len(constants) {
		return false
	}
	index, known := nativeEnumCPIndex(ops[1])
	if !known || !enumArrayClassEqual(object, index, object.ThisClass) {
		return false
	}
	for i, name := range constants {
		field := fields[sourceFieldIdentity{name, "L" + object.GetClassName() + ";"}]
		if field == nil || !enumArrayOwnMember(object, ops[4+4*i], field.NameIndex, field.DescriptorIndex) {
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
	// Only a suffix switch imposes a four-byte alignment constraint. Ordinary
	// branch differences survive any affine displacement; a straight-line
	// suffix should remain the exact fresh instruction composition without
	// invented NOPs. When needed, padding stays inside the proved replacement.
	padding := 0
	for _, op := range initOps[prefix+1:] {
		if op.Instr.OpCode == core.OP_TABLESWITCH || op.Instr.OpCode == core.OP_LOOKUPSWITCH {
			padding = (4 - (arrayEnd-3)%4) % 4
			break
		}
	}
	newCode := append(bytes.Clone(initCode.Code[:callPC]), arrayCode.Code[:arrayEnd]...)
	newCode = append(newCode, make([]byte, padding)...)
	newCode = append(newCode, initCode.Code[callPC+3:]...)
	if len(newCode) > 65535 {
		return fail("aligned fresh Code size")
	}
	initRaw, err := enumArrayCodePayload(compiled, object, initSpan, work)
	if err != nil {
		return nil, err
	}
	helperRaw, err := enumArrayCodePayload(compiled, object, helperSpan, work)
	if err != nil {
		return nil, err
	}
	helperLines, helperLineName, err := enumArrayCodeLines(helperRaw.data, object, arrayCode, arrayOps, work)
	if err != nil {
		return nil, err
	}
	for pc := range helperLines {
		if pc >= arrayEnd {
			return fail("line on removed helper return")
		}
	}
	newInfo, err := enumArrayRelocateCode(object, initRaw.data, initCode, initOps, newCode, callPC, arrayEnd-3+padding, helperLines, helperLineName, arrayCode.MaxStack, work)
	if err != nil {
		return nil, err
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
