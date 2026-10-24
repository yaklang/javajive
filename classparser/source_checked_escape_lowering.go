package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

type checkedEscapeSourceOrigin struct {
	constructorNativeSourceOrigin
	helper string
}

// This receipt comes from the concrete temporary-declaration writer, not a
// generated-looking spelling. Only a complete physical source return activates
// it. Native inner/interface helpers have different invocation protocols and
// do not borrow this private-static proof.
func (c *ClassObjectDumper) publishCheckedEscapeLowering(source string, methods []*dumpedMethods) bool {
	a := c.options.sourceConstructorLowering
	if a == nil || !c.reconstructionPhysicalHeader() {
		return true
	}
	var helper *dumpedMethods
	for _, m := range methods {
		if m != nil && m.checkedEscapeTemporary {
			if helper != nil {
				return false
			}
			helper = m
		}
	}
	if helper == nil {
		return true
	}
	origin := checkedEscapeSourceOrigin{}
	if !c.sourceDegraded && !legacySourceHasFailureMarker(source) {
		if helper.member != nil || helper.isStub || strings.Count(source, helper.code) != 1 || !nativeProofWork(c.Work, int64(len(source))+1) {
			return false
		}
		raw, err := a.jar.readJarEntry(c.obj.GetClassName() + ".class")
		if err != nil || !bytes.Equal(raw, c.obj.Bytes()) {
			return false
		}
		origin = checkedEscapeSourceOrigin{constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}, helper.methodName}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.escapePending == nil {
		a.escapePending = map[string]checkedEscapeSourceOrigin{}
	}
	a.escapePending[c.obj.GetClassName()] = origin
	return true
}

func (a *sourceConstructorLoweringAuthority) checkedEscapeSourceReturned(owner string, original []byte, work *workbudget.Budget) (checkedEscapeSourceOrigin, bool) {
	if a == nil || !nativeProofWork(work, 1) {
		return checkedEscapeSourceOrigin{}, false
	}
	a.mu.Lock()
	origin, active := a.escapeActive[owner]
	_, pending := a.escapePending[owner]
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.escapeActive[owner]
		a.parent.mu.Unlock()
	}
	return origin, active && origin.helper != "" && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

// Java's throws-only generic adapter has fresh Code `aload_0; athrow`.
// Its only admitted consumer is `invokestatic adapter; athrow`. Removing the
// invocation leaves precisely the same Throwable for the terminal ATHROW.
// The private method is then removed, recovering the original method universe.
// The original Code is never imported. Three NOPs retain all instruction PCs;
// both throw sites must have identical ordered exception-handler coverage and
// no branch/frame may enter the former return continuation. This intentionally
// does not infer equivalence of other freshly compiled code.
func (a *sourceConstructorLoweringAuthority) lowerCheckedEscapeHelper(owner string, original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	decline := func() ([]byte, error) {
		if err := work.Check(); err != nil {
			return nil, err
		}
		return compiled, nil
	}
	origin, closed := a.checkedEscapeSourceReturned(owner, original, work)
	if !closed {
		return decline()
	}
	if len(original) > 32<<20 || len(compiled) > 32<<20 || !nativeProofWork(work, int64(len(original)+len(compiled))+1) || work.CheckAlloc(int64(len(original)+len(compiled))*5) != nil {
		return nil, constructorLoweringFailure(work, "checked escape input budget")
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
	if want.GetClassName() != owner || !sourceParameterClassIdentityClosed(want, got) || got.AccessFlags&0x200 != 0 || got.MinorVersion != 0 || got.MajorVersion < 52 || got.MajorVersion > 65 || len(got.Methods) > 4096 {
		return decline()
	}
	const descriptor = "(Ljava/lang/Throwable;)Ljava/lang/RuntimeException;"
	identity := func(o *ClassObject, m *MemberInfo) (sourceLambdaMethodIdentity, bool) {
		if m == nil {
			return sourceLambdaMethodIdentity{}, false
		}
		n, nk := sourceParameterUTF8Key(o, m.NameIndex)
		d, dk := sourceParameterUTF8Key(o, m.DescriptorIndex)
		return sourceLambdaMethodIdentity{n, d}, nk && dk
	}
	expected := map[sourceLambdaMethodIdentity]bool{}
	for _, m := range want.Methods {
		k, ok := identity(want, m)
		n, _ := sourceBridgeUTF8(want, m.NameIndex)
		if !ok || expected[k] || n == origin.helper {
			return decline()
		}
		expected[k] = true
	}
	seen := map[sourceLambdaMethodIdentity]bool{}
	var helper *MemberInfo
	for _, m := range got.Methods {
		k, ok := identity(got, m)
		if !ok || seen[k] {
			return decline()
		}
		seen[k] = true
		n, _ := sourceBridgeUTF8(got, m.NameIndex)
		d, _ := sourceBridgeUTF8(got, m.DescriptorIndex)
		if n == origin.helper && d == descriptor && !expected[k] {
			helper = m
			continue
		}
		if !expected[k] {
			return decline()
		}
	}
	if helper == nil {
		return decline()
	} // idempotent completion; the metadata gate still checks the universe
	if len(seen) != len(expected)+1 || helper.AccessFlags != 0x000a {
		return decline()
	}
	body := constructorLoweringCode(helper)
	if body == nil || body.MaxStack != 1 || body.MaxLocals != 1 || len(body.ExceptionTable) != 0 || !bytes.Equal(body.Code, []byte{core.OP_ALOAD_0, core.OP_ATHROW}) {
		return decline()
	}
	// Reflection-visible annotations or opaque execution protocols are not a
	// compiler adapter. Debug tables are inert, but its entire body is exact.
	for _, attribute := range helper.Attributes {
		switch attr := attribute.(type) {
		case *CodeAttribute, *SignatureAttribute, *ExceptionsAttribute:
		case *UnparsedAttribute:
			if attr == nil || attr.Name != "MethodParameters" {
				return decline()
			}
		default:
			_ = attr
			return decline()
		}
	}
	references := map[uint16]bool{}
	for i, constant := range got.ConstantPool {
		if !nativeProofWork(work, 1) {
			return decline()
		}
		r := nativeConstantMember(constant)
		if r == nil {
			continue
		}
		name, nk := sourceBridgeClassName(got, r.ClassIndex)
		if !nk {
			return decline()
		}
		if name != owner {
			continue
		}
		if r.NameAndTypeIndex == 0 || int(r.NameAndTypeIndex) > len(got.ConstantPool) {
			return decline()
		}
		nt, ok := got.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return decline()
		}
		n, nk := sourceBridgeUTF8(got, nt.NameIndex)
		d, dk := sourceBridgeUTF8(got, nt.DescriptorIndex)
		if !nk || !dk {
			return decline()
		}
		if n != origin.helper {
			continue
		}
		if _, ok := constant.(*ConstantMethodrefInfo); !ok || d != descriptor {
			return decline()
		}
		references[uint16(i+1)] = true
	}
	for _, constant := range got.ConstantPool {
		if h, ok := constant.(*ConstantMethodHandleInfo); ok && h != nil && references[h.ReferenceIndex] {
			return decline()
		}
	}
	edits := []compiledMetadataEdit{}
	uses := 0
	for _, m := range got.Methods {
		code := constructorLoweringCode(m)
		if code == nil || m == helper {
			continue
		}
		ops, ok := checkedEscapeMethodOps(code, work)
		if !ok || !enumArrayInstructionSpans(code.Code, ops) {
			return decline()
		}
		calls := []int{}
		forbidden := map[int]bool{}
		handlerEntries := map[int]bool{}
		for i, op := range ops {
			index, known := checkedEscapeInstructionReference(op)
			if !known || !references[index] {
				continue
			}
			pc := int(op.CurrentOffset)
			if op.Instr.OpCode != core.OP_INVOKESTATIC || op.IsWide || i+1 == len(ops) || !nativeEnumOpcode(ops[i+1], core.OP_ATHROW) || int(ops[i+1].CurrentOffset) != pc+3 {
				return decline()
			}
			entry, caught := checkedEscapeCaughtOperand(code, ops, i)
			if !caught {
				return decline()
			}
			handlerEntries[entry] = true
			forbidden[int(ops[i-1].CurrentOffset)] = true
			forbidden[pc] = true
			for _, handler := range code.ExceptionTable {
				if handler == nil || handler.StartPc >= handler.EndPc || int(handler.EndPc) > len(code.Code) || (int(handler.StartPc) <= pc && pc < int(handler.EndPc)) != (int(handler.StartPc) <= pc+3 && pc+3 < int(handler.EndPc)) {
					return decline()
				}
			}
			calls = append(calls, pc)
			forbidden[pc+3] = true
		}
		if len(calls) == 0 {
			continue
		}
		if !checkedEscapeContinuationClosed(got, code, ops, forbidden, handlerEntries, work) {
			return decline()
		}
		raw, err := enumArrayCodePayload(compiled, got, layout.methods[m], work)
		if err != nil {
			return nil, err
		}
		if len(raw.data) < 12+len(code.Code) || int(binary.BigEndian.Uint32(raw.data[4:])) != len(code.Code) || !bytes.Equal(raw.data[8:8+len(code.Code)], code.Code) {
			return decline()
		}
		for _, pc := range calls {
			at := raw.span.start + 6 + 8 + pc
			edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{at, at + 3}, []byte{0, 0, 0}})
			uses++
		}
	}
	if uses == 0 {
		return decline()
	}
	span, ok := layout.methods[helper]
	first := len(compiled)
	for _, s := range layout.methods {
		if s.start < first {
			first = s.start
		}
	}
	if !ok || first < layout.poolEnd+2 || first > len(compiled) || binary.BigEndian.Uint16(compiled[first-2:]) != uint16(len(got.Methods)) {
		return nil, constructorLoweringFailure(work, "checked escape method spans")
	}
	count := []byte{0, 0}
	binary.BigEndian.PutUint16(count, uint16(len(got.Methods)-1))
	edits = append(edits, compiledMetadataEdit{compiledMetadataSpan{first - 2, first}, count}, compiledMetadataEdit{span, nil})
	return applyCompiledMetadataEdits(compiled, edits, work)
}

// Retained frames need the pre-call Throwable, not the old nominal return
// type. Excluding every alternate entry at the continuation makes its only
// predecessor the proved adapter invocation. All other PCs remain fixed.
func checkedEscapeContinuationClosed(object *ClassObject, code *CodeAttribute, ops []*core.OpCode, forbidden, handlerEntries map[int]bool, work *workbudget.Budget) bool {
	positions := map[int]int{}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = op.Instr.OpCode
	}
	target := func(pc int) bool { _, ok := positions[pc]; return ok && !forbidden[pc] && !handlerEntries[pc] }
	for _, h := range code.ExceptionTable {
		if h == nil || positions[int(h.HandlerPc)] == 0 || forbidden[int(h.HandlerPc)] {
			return false
		}
	}
	for _, op := range ops {
		pc, opcode := int(op.CurrentOffset), op.Instr.OpCode
		if !nativeProofWork(work, 1) {
			return false
		}
		switch {
		case opcode == core.OP_JSR || opcode == core.OP_JSR_W || opcode == core.OP_RET || opcode == core.OP_WIDE:
			return false
		case constructorMotionPacketBranch(opcode):
			if opcode == core.OP_GOTO_W {
				if len(op.Data) != 4 || !target(pc+int(int32(binary.BigEndian.Uint32(op.Data)))) {
					return false
				}
			} else if len(op.Data) != 2 || !target(pc+int(int16(binary.BigEndian.Uint16(op.Data)))) {
				return false
			}
		case opcode == core.OP_TABLESWITCH || opcode == core.OP_LOOKUPSWITCH:
			_, data, ok := enumArraySwitchSpan(code.Code, op)
			if !ok || !target(pc+int(int32(binary.BigEndian.Uint32(data)))) {
				return false
			}
			start, step := 12, 4
			if opcode == core.OP_LOOKUPSWITCH {
				start, step = 12, 8
			}
			for i := start; i < len(data); i += step {
				if !target(pc + int(int32(binary.BigEndian.Uint32(data[i:])))) {
					return false
				}
			}
		}
	}
	for pc := range forbidden {
		delete(positions, pc)
	}
	seen := map[string]bool{}
	for _, attribute := range code.Attributes {
		switch a := attribute.(type) {
		case *LineNumberTableAttribute: // no offset or local value changes
			if a == nil || seen["LineNumberTable"] {
				return false
			}
			seen["LineNumberTable"] = true
		case *UnparsedAttribute:
			if a == nil || seen[a.Name] {
				return false
			}
			seen[a.Name] = true
			switch a.Name {
			case "StackMapTable":
				if _, ok := enumArrayRelocateFrames(object, a.Info, positions, 0, 0, work); !ok {
					return false
				}
			case "LocalVariableTable", "LocalVariableTypeTable": // unchanged slots and ranges
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

// An exception-table entry supplies a non-null Throwable. Require its store,
// immediate same-slot reload and adapter call to form one closed packet. In
// particular helper(null) is NOT equivalent: its new NPE originates elsewhere.
func checkedEscapeCaughtOperand(code *CodeAttribute, ops []*core.OpCode, call int) (int, bool) {
	if call < 3 {
		return 0, false
	}
	local := func(op *core.OpCode, store bool) (int, bool) {
		base, compact := core.OP_ALOAD, core.OP_ALOAD_0
		if store {
			base, compact = core.OP_ASTORE, core.OP_ASTORE_0
		}
		if op.IsWide {
			return 0, false
		}
		if op.Instr.OpCode >= compact && op.Instr.OpCode < compact+4 && len(op.Data) == 0 {
			return op.Instr.OpCode - compact, true
		}
		if op.Instr.OpCode == base && len(op.Data) == 1 {
			return int(op.Data[0]), true
		}
		return 0, false
	}
	store, sk := local(ops[call-2], true)
	load, lk := local(ops[call-1], false)
	if !sk || !lk || store != load || store >= int(code.MaxLocals) {
		return 0, false
	}
	before := ops[call-3].Instr.OpCode
	if before != core.OP_ATHROW && before != core.OP_GOTO && before != core.OP_GOTO_W && (before < core.OP_IRETURN || before > core.OP_RETURN) {
		return 0, false
	}
	entry := int(ops[call-2].CurrentOffset)
	for _, h := range code.ExceptionTable {
		if h != nil && int(h.HandlerPc) == entry {
			return entry, true
		}
	}
	return 0, false
}

func checkedEscapeInstructionReference(op *core.OpCode) (uint16, bool) {
	switch op.Instr.OpCode {
	case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W, core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_NEW, core.OP_ANEWARRAY, core.OP_CHECKCAST, core.OP_INSTANCEOF:
		return nativeEnumCPIndex(op)
	case core.OP_INVOKEINTERFACE, core.OP_INVOKEDYNAMIC, core.OP_MULTIANEWARRAY:
		if len(op.Data) >= 2 {
			return binary.BigEndian.Uint16(op.Data), true
		}
	}
	return 0, false
}

// Decode instructions without borrowing the enum protocol's no-handler
// precondition: exception-table edges are precisely what this proof needs.
func checkedEscapeMethodOps(code *CodeAttribute, work *workbudget.Budget) ([]*core.OpCode, bool) {
	if code == nil || !nativeProofWork(work, int64(len(code.Code))+1) {
		return nil, false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	ops := constructorMotionOps(decoder)
	return ops, enumArrayInstructionSpans(code.Code, ops)
}
