package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

type nativeEnumConstantAllocation struct {
	constant, allocatedClass, descriptor string
	ordinal, newPC, invokePC             int
}

// Source erases the hidden name/ordinal, so they must be the original field's
// literal name and declaration ordinal. Immutable typed origins bind each
// PUTSTATIC to its unique NEW and constructor invocation. User arguments can
// contain wide values, nested allocations, local copies and branches: neither
// textual argument positions nor a same-owner call identifies that allocation.
func (c *ClassObjectDumper) nativeEnumConstantInitializations() (map[string]nativeEnumConstantAllocation, error) {
	fail := func(reason string) (map[string]nativeEnumConstantAllocation, error) {
		if c.Work != nil && c.Work.Err() != nil {
			return nil, c.Work.Err()
		}
		return nil, fmt.Errorf("enum regeneration: original constant initialization is unproved: %s", reason)
	}
	obj := c.obj
	owner := obj.GetClassName()
	self := "L" + owner + ";"
	if c.Work != nil && c.Work.CheckAlloc(int64(len(obj.Fields)+len(obj.Methods)+1)*128) != nil {
		return fail("declaration allocation budget")
	}
	ordinals := map[string]int{}
	for _, field := range obj.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			return fail("constant table")
		}
		if field.AccessFlags&0x4000 == 0 {
			continue
		}
		name, nk := sourceBridgeUTF8(obj, field.NameIndex)
		descriptor, dk := sourceBridgeUTF8(obj, field.DescriptorIndex)
		if !nk || !dk || field.AccessFlags != 0x4019 || descriptor != self {
			return fail("constant declaration")
		}
		if _, duplicate := ordinals[name]; duplicate {
			return fail("duplicate constant")
		}
		ordinals[name] = len(ordinals)
	}
	var initializer *MemberInfo
	var code *CodeAttribute
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return fail("method table")
		}
		name, known := sourceBridgeUTF8(obj, method.NameIndex)
		if !known {
			return fail("method identity")
		}
		if name != "<clinit>" {
			continue
		}
		descriptor, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if initializer != nil || !known || descriptor != "()V" || method.AccessFlags != 8 {
			return fail("initializer identity")
		}
		initializer = method
		for _, a := range method.Attributes {
			if body, ok := a.(*CodeAttribute); ok {
				if code != nil || body == nil {
					return fail("initializer code")
				}
				code = body
			}
		}
	}
	if initializer == nil || code == nil {
		return fail("missing initializer")
	}
	// Include the live immutable frame snapshot and its additional lookup maps
	// in one construction high-water, rather than checking each map separately.
	allocationBytes := int64(len(code.Code)+1)*int64(int(code.MaxLocals)+int(code.MaxStack)+1)*64 + int64(len(code.Code)+len(obj.Fields)+1)*384
	if allocationBytes > 64<<20 || c.Work != nil && c.Work.CheckAlloc(allocationBytes) != nil {
		return fail("initializer allocation budget")
	}
	ir, fn, known := c.nativeOriginalMethodSnapshot(initializer, code)
	if !known {
		return fail("typed initializer operands")
	}
	// Index original decoded operands; no synthetic opcodes or source names are
	// manufactured to satisfy the proof.
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return fail("initializer opcodes")
	}
	ops := map[uint16]*core.OpCode{}
	for _, op := range decoder.Opcodes() {
		ops[op.CurrentOffset] = op
	}
	calls := map[int]nativeEnumConstantAllocation{}
	for _, record := range fn.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return fail("constructor analysis budget")
		}
		invoke, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
			continue
		}
		params, result, err := callbinding.Descriptor(invoke.Desc)
		if err != nil || result != "V" {
			return fail("constructor descriptor")
		}
		if len(params) < 2 || params[0] != "Ljava/lang/String;" || params[1] != "I" {
			continue
		}
		if len(record.Uses) != len(params)+1 || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return fail("constructor operand identities")
		}
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if index < 0 {
			return fail("constructor receiver")
		}
		receiver := record.Before.Stack[index]
		if receiver.Kind != frametransfer.UninitNew || receiver.Class != invoke.Class || record.Uses[0].Kind != ssabuild.OriginInstr || record.Uses[0].PC != receiver.NewPC {
			continue
		}
		allocation, found := ir.InstrByID(methodir.InstrID(receiver.NewPC))
		if !found || allocation.Opcode != core.OP_NEW || allocation.Class != invoke.Class {
			return fail("allocation identity")
		}
		// Only calls belonging to a constant assignment are validated below. Other
		// source initializer objects may legitimately have (String,int,...) ctors.
		pc := int(receiver.NewPC)
		if _, duplicate := calls[pc]; duplicate {
			return fail("alternative constructor initialization")
		}
		calls[pc] = nativeEnumConstantAllocation{allocatedClass: invoke.Class, descriptor: invoke.Desc, newPC: pc, invokePC: int(record.PC)}
	}
	result := map[string]nativeEnumConstantAllocation{}
	allocationOwners := map[int]string{}
	records := map[uint16]ssabuild.InstructionValues{}
	for _, record := range fn.Instructions {
		records[record.PC] = record
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	for _, record := range fn.Instructions {
		store, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || store.Opcode != core.OP_PUTSTATIC || store.Class != owner {
			continue
		}
		ordinal, constant := ordinals[store.Member]
		if !constant {
			continue
		}
		if _, duplicate := result[store.Member]; duplicate || store.Desc != self || len(record.Uses) != 1 || record.Uses[0].Kind != ssabuild.OriginInstr {
			return fail("constant assignment origin")
		}
		plan, known := calls[int(record.Uses[0].PC)]
		if !known {
			return fail("constant constructor origin")
		}
		if _, duplicate := allocationOwners[plan.newPC]; duplicate {
			return fail("one allocation assigned to multiple constants")
		}
		original := records[uint16(plan.invokePC)]
		if len(original.Uses) < 3 || original.Uses[1].Kind != ssabuild.OriginInstr || original.Uses[2].Kind != ssabuild.OriginInstr {
			return fail("hidden argument origins")
		}
		nameOp := ops[original.Uses[1].PC]
		ordinalOp := ops[original.Uses[2].PC]
		nameIndex, nk := nativeEnumCPIndex(nameOp)
		literal := ""
		if nk && nameIndex > 0 && int(nameIndex) <= len(obj.ConstantPool) && (nativeEnumOpcode(nameOp, core.OP_LDC) || nativeEnumOpcode(nameOp, core.OP_LDC_W)) {
			if value, ok := obj.ConstantPool[nameIndex-1].(*ConstantStringInfo); ok && value != nil {
				literal, nk = sourceBridgeUTF8(obj, value.StringIndex)
			} else {
				nk = false
			}
		} else {
			nk = false
		}
		originalOrdinal, ok := nativeEnumOriginalInt(obj, ordinalOp)
		if !nk || literal != store.Member || !ok || originalOrdinal != ordinal {
			return fail("hidden name or ordinal differs from its source declaration")
		}
		if !nativeEnumMemberOperand(obj, ops[uint16(plan.invokePC)], core.OP_INVOKESPECIAL, plan.allocatedClass, "<init>", plan.descriptor) || !nativeEnumMemberOperand(obj, ops[record.PC], core.OP_PUTSTATIC, owner, store.Member, self) {
			return fail("constant-pool operand tags")
		}
		if plan.allocatedClass != owner {
			body, known := resolve(plan.allocatedClass)
			if !known || body == nil || !isSyntheticEnumConstantSubclass(body) || body.GetSupperClassName() != owner {
				return fail("constant-specific declaration")
			}
			enclosing, method, known := originalAnonymousOwner(body)
			if !known || enclosing != owner || method != "" {
				return fail("constant-specific enclosing identity")
			}
		}
		plan.constant, plan.ordinal = store.Member, ordinal
		result[store.Member] = plan
		allocationOwners[plan.newPC] = store.Member
	}
	if len(result) != len(ordinals) {
		return fail("missing original constant assignments")
	}
	return result, nil
}
