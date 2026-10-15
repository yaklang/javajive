package javaclassparser

import (
	"encoding/base64"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strconv"
	"strings"
)

// Register exact producers and consumers from one original class. This is
// independent of archive closure: only the archive caller can certify the
// complete user set and grant suppression/access to a separate helper unit.
// The class-source caller can reuse these operand witnesses without claiming
// that it has enumerated an archive or owns any foreign source declaration.
func (c *ClassObjectDumper) nativeEnumSwitchObjectUses(p *nativeMemberFamily, table *nativeEnumSwitchTable, name string, own map[string]*ClassObject, foreignChecked map[string]bool, sourceStorage bool) (int, bool) {
	object := c.obj
	owner := object.GetClassName()
	work := c.Work
	count := 0
	for _, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
		if member == nil {
			return 0, false
		}
		desc, ok := sourceBridgeUTF8(object, member.DescriptorIndex)
		if !ok || strings.Contains(desc, "L"+name+";") && !(nativeMemberJointSwitchTableMarker(p, name, work) && nativeMemberJointBridgeDeclaration(p, object, member, name, work)) {
			return 0, false
		}
		for _, a := range member.Attributes {
			if a, ok := a.(*SignatureAttribute); ok {
				if a == nil {
					return 0, false
				}
				sig, known := sourceBridgeUTF8(object, a.SignatureIndex)
				if !known || strings.Contains(sig, "L"+name+";") {
					return 0, false
				}
			}
		}
	}
	bridgeNameTypes := nativeMemberJointBridgeNameTypes(p, object, work)
	if bridgeNameTypes == nil {
		return 0, false
	}
	for constantIndex, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return 0, false
		}
		if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
			if nt == nil {
				return 0, false
			}
			desc, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
			if !known || strings.Contains(desc, "L"+name+";") && !(bridgeNameTypes[constantIndex+1] && nativeMemberJointSwitchTableMarker(p, name, work)) {
				return 0, false
			}
		}
		if ref := nativeConstantMember(constant); ref != nil {
			target, ok := sourceBridgeClassName(object, ref.ClassIndex)
			if !ok {
				return 0, false
			}
			if target != name && own[target] == nil && !foreignChecked[target] {
				foreignChecked[target] = true
				if raw, found := c.foldSiblingResolver(target); found {
					object, err := c.parseResolved(raw)
					if err != nil || object.GetClassName() != target || nativeEnumSwitchTableProof(object, work) != nil {
						return 0, false
					}
				}
			}
			if target == name {
				field, ok := constant.(*ConstantFieldrefInfo)
				if !ok || field == nil || field.NameAndTypeIndex == 0 || int(field.NameAndTypeIndex) > len(object.ConstantPool) {
					return 0, false
				}
				nt, ok := object.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return 0, false
				}
				fn, nk := sourceBridgeUTF8(object, nt.NameIndex)
				fd, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !nk || !dk || fd != "[I" || table.tables[fn] == nil {
					return 0, false
				}
			}
		}
	}
	for _, method := range object.Methods {
		if method == nil {
			return 0, false
		}
		mn, nk := sourceBridgeUTF8(object, method.NameIndex)
		md, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk {
			return 0, false
		}
		params, _, e := callbinding.Descriptor(md)
		if e != nil {
			return 0, false
		}
		types := map[int]string{}
		slot := 0
		if method.AccessFlags&8 == 0 {
			types[0] = "L" + object.GetClassName() + ";"
			slot = 1
		}
		for _, param := range params {
			types[slot] = param
			slot += 1
			if param == "J" || param == "D" {
				slot++
			}
		}
		for _, a := range method.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(work, int64(len(code.Code))) || work != nil && work.CheckAlloc(int64(len(code.Code))*256) != nil {
				return 0, false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
			d.Work = work
			if d.ParseOpcode() != nil {
				return 0, false
			}
			ops := constructorMotionOps(d)
			entries, closed := nativeMemberLexicalControlEntries(d, code, work)
			if !closed {
				return 0, false
			}
			var parameterFlow *nativeEnumParameterFlow
			var localReads map[int]*nativeEnumLocalRead
			localsChecked := false
			for i, op := range ops {
				if (op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W || op.Instr.OpCode == core.OP_NEW || op.Instr.OpCode == core.OP_CHECKCAST || op.Instr.OpCode == core.OP_INSTANCEOF || op.Instr.OpCode == core.OP_ANEWARRAY || op.Instr.OpCode == core.OP_MULTIANEWARRAY) && len(op.Data) > 0 {
					idx := uint16(op.Data[0])
					if len(op.Data) >= 2 {
						idx = core.Convert2bytesToInt(op.Data[:2])
					}
					if n, k := sourceBridgeClassName(object, idx); k && (n == name || strings.Contains(n, "L"+name+";")) {
						return 0, false
					}
				}
				member := constructorMotionMember(object, op, op.Instr.OpCode)
				if member == nil || member.Name != name {
					continue
				}
				arr := table.tables[member.Member]
				if arr == nil || !nativeEnumMemberOperand(object, op, core.OP_GETSTATIC, name, member.Member, "[I") {
					return 0, false
				}
				selector, ordinal, known := nativeEnumSelectorPacket(object, ops, i+1, types, arr.enum, work)
				if !known {
					if parameterFlow == nil {
						parameterFlow = nativeEnumParameterOriginalFlow(d, code, work)
					}
					if !localsChecked {
						reader := c
						reader.Work = work
						var proved bool
						localReads, proved = reader.nativeEnumLocalSelectorReads(method, code, parameterFlow, types)
						if !proved {
							return 0, false
						}
						localsChecked = true
					}
					selector, ordinal, known = nativeEnumSelectorPacket(object, ops, i+1, types, arr.enum, work, localReads)
				}
				if !known {
					return 0, false
				}
				slots := map[int]bool{}
				if !nativeEnumSelectorSlots(selector, slots, 0) || !nativeProofWork(work, int64(len(ops))) {
					return 0, false
				}
				// An unchanged method needs no additional graph. Otherwise
				// prove the entry seed at each original leaf read, including
				// all paths that return to it after a later store.
				if !nativeEnumSelectorParametersUnchanged(ops, slots) {
					if parameterFlow == nil {
						parameterFlow = nativeEnumParameterOriginalFlow(d, code, work)
					}
					if !parameterFlow.selector(selector, work, 0) {
						reader := c
						reader.Work = work
						if !sourceStorage || !reader.nativeEnumSelectorStorageFlow(method, code, selector, parameterFlow, 0) {
							return 0, false
						}
					}
				}
				for _, pc := range entries {
					if pc > int(op.CurrentOffset) && pc <= int(ops[ordinal+2].CurrentOffset) {
						return 0, false
					}
				}
				keys := map[int]bool{}
				valid := true
				ops[ordinal+2].SwitchJmpCase.ForEach(func(key int, target int32) bool {
					if !nativeProofWork(work, 1) || arr.entries[key] == "" {
						valid = false
						return false
					}
					keys[key] = true
					return true
				})
				if !valid || len(keys) == 0 {
					return 0, false
				}
				if table.uses[owner] == nil {
					table.uses[owner] = map[string]map[int]*nativeEnumSwitchUse{}
				}
				if table.uses[owner][mn+md] == nil {
					table.uses[owner][mn+md] = map[int]*nativeEnumSwitchUse{}
				}
				use := &nativeEnumSwitchUse{field: member.Member, getPC: int(op.CurrentOffset), ordinalPC: int(ops[ordinal].CurrentOffset), arrayPC: int(ops[ordinal+1].CurrentOffset), parameterSlot: selector.slot, selector: selector, keys: keys}
				use.marker = "/*jdec-owned-enum-switch:" + base64.RawURLEncoding.EncodeToString([]byte(owner+"\x00"+mn+md+"\x00"+strconv.Itoa(use.arrayPC))) + "*/"
				table.uses[owner][mn+md][use.arrayPC] = use
				count++
			}
		}
	}
	return count, true
}
