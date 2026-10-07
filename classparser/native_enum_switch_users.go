package javaclassparser

import (
	"encoding/base64"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

type nativeEnumSwitchUse struct {
	field                     string
	marker                    string
	getPC, ordinalPC, arrayPC int
	parameterSlot             int
	selector                  *nativeEnumSelectorProducer
	keys                      map[int]bool
	rendered                  bool
}

// A table is a compiler artifact only if all its original archive users are
// table reads consumed by proved switches. Escapes, writes, class literals,
// constructor markers, handles and foreign lexical units are not table uses.
// Computed selectors require an original ordered stack-producer certificate;
// an equal enum result type or source spelling cannot license the rewrite.
func (z *JarFS) nativeEnumSwitchUsersClosed(p *nativeMemberFamily, root *ClassObject, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || root == nil {
		return false
	}
	if len(p.enumSwitchTables) == 0 {
		return true
	}
	if index == nil || !index.valid {
		return false
	}
	objects, known := nativeMemberDependencyObjects(root, p, work)
	if !known {
		return false
	}
	foreignChecked := map[string]bool{}
	own := map[string]*ClassObject{}
	for _, o := range objects {
		own[o.GetClassName()] = o
	}
	for name, table := range p.enumSwitchTables {
		if index.handles[name] {
			return false
		}
		for user := range index.typeUsers[name] {
			if user != name && own[user] == nil {
				return false
			}
		}
		// One enum dependency has one initialization boundary. Multiple tables
		// require a separate source-order certificate for distinct enum initializers.
		if len(table.tables) != 1 {
			return false
		}
		for _, arr := range table.tables {
			// Current javac lowers same-unit enum declarations directly through
			// ordinal(), so a legacy producer's helper would not be regenerated.
			// Only a separately emitted enum declaration licenses this protocol.
			if own[arr.enum] != nil {
				return false
			}
			raw, found := z.enumSiblingResolver()(arr.enum)
			if !found {
				return false
			}
			enum, e := z.nativeMemberReader(root).parseResolved(raw)
			if e != nil || enum.GetClassName() != arr.enum || nativeMemberEnumSynthesisProof(enum, 0x4019, work) == nil {
				return false
			}
			constants := map[string]bool{}
			for _, f := range enum.Fields {
				if f.AccessFlags&0x4000 != 0 {
					n, ok := sourceBridgeUTF8(enum, f.NameIndex)
					if !ok {
						return false
					}
					constants[n] = true
				}
			}
			for _, n := range arr.entries {
				if !constants[n] {
					return false
				}
			}
		}
		table.uses = map[string]map[string]map[int]*nativeEnumSwitchUse{}
		count := 0
		for owner, object := range own {
			for _, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
				if member == nil {
					return false
				}
				desc, ok := sourceBridgeUTF8(object, member.DescriptorIndex)
				if !ok || strings.Contains(desc, "L"+name+";") && !(nativeMemberJointSwitchTableMarker(p, name, work) && nativeMemberJointBridgeDeclaration(p, object, member, name, work)) {
					return false
				}
				for _, a := range member.Attributes {
					if a, ok := a.(*SignatureAttribute); ok {
						if a == nil {
							return false
						}
						sig, known := sourceBridgeUTF8(object, a.SignatureIndex)
						if !known || strings.Contains(sig, "L"+name+";") {
							return false
						}
					}
				}
			}
			bridgeNameTypes := nativeMemberJointBridgeNameTypes(p, object, work)
			if bridgeNameTypes == nil {
				return false
			}
			for constantIndex, constant := range object.ConstantPool {
				if !nativeProofWork(work, 1) {
					return false
				}
				if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
					if nt == nil {
						return false
					}
					desc, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
					if !known || strings.Contains(desc, "L"+name+";") && !(bridgeNameTypes[constantIndex+1] && nativeMemberJointSwitchTableMarker(p, name, work)) {
						return false
					}
				}
				if ref := nativeConstantMember(constant); ref != nil {
					target, ok := sourceBridgeClassName(object, ref.ClassIndex)
					if !ok {
						return false
					}
					if target != name && own[target] == nil && !foreignChecked[target] {
						foreignChecked[target] = true
						if raw, found := z.enumSiblingResolver()(target); found {
							object, err := z.nativeMemberReader(root).parseResolved(raw)
							if err != nil || object.GetClassName() != target || nativeEnumSwitchTableProof(object, work) != nil {
								return false
							}
						}
					}
					if target == name {
						field, ok := constant.(*ConstantFieldrefInfo)
						if !ok || field == nil || field.NameAndTypeIndex == 0 || int(field.NameAndTypeIndex) > len(object.ConstantPool) {
							return false
						}
						nt, ok := object.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						if !ok || nt == nil {
							return false
						}
						fn, nk := sourceBridgeUTF8(object, nt.NameIndex)
						fd, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
						if !nk || !dk || fd != "[I" || table.tables[fn] == nil {
							return false
						}
					}
				}
			}
			for _, method := range object.Methods {
				if method == nil {
					return false
				}
				mn, nk := sourceBridgeUTF8(object, method.NameIndex)
				md, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
				if !nk || !dk {
					return false
				}
				params, _, e := callbinding.Descriptor(md)
				if e != nil {
					return false
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
						return false
					}
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
					d.Work = work
					if d.ParseOpcode() != nil {
						return false
					}
					ops := constructorMotionOps(d)
					entries, closed := nativeMemberLexicalControlEntries(d, code, work)
					if !closed {
						return false
					}
					var parameterFlow *nativeEnumParameterFlow
					for i, op := range ops {
						if (op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W || op.Instr.OpCode == core.OP_NEW || op.Instr.OpCode == core.OP_CHECKCAST || op.Instr.OpCode == core.OP_INSTANCEOF || op.Instr.OpCode == core.OP_ANEWARRAY || op.Instr.OpCode == core.OP_MULTIANEWARRAY) && len(op.Data) > 0 {
							idx := uint16(op.Data[0])
							if len(op.Data) >= 2 {
								idx = core.Convert2bytesToInt(op.Data[:2])
							}
							if n, k := sourceBridgeClassName(object, idx); k && (n == name || strings.Contains(n, "L"+name+";")) {
								return false
							}
						}
						member := constructorMotionMember(object, op, op.Instr.OpCode)
						if member == nil || member.Name != name {
							continue
						}
						arr := table.tables[member.Member]
						if arr == nil || !nativeEnumMemberOperand(object, op, core.OP_GETSTATIC, name, member.Member, "[I") {
							return false
						}
						selector, ordinal, known := nativeEnumSelectorPacket(object, ops, i+1, types, arr.enum, work)
						if !known {
							return false
						}
						slots := map[int]bool{}
						if !nativeEnumSelectorSlots(selector, slots, 0) || !nativeProofWork(work, int64(len(ops))) {
							return false
						}
						// An unchanged method needs no additional graph. Otherwise
						// prove the entry seed at each original leaf read, including
						// all paths that return to it after a later store.
						if !nativeEnumSelectorParametersUnchanged(ops, slots) {
							if parameterFlow == nil {
								parameterFlow = nativeEnumParameterOriginalFlow(d, code, work)
							}
							if !parameterFlow.selector(selector, work, 0) {
								return false
							}
						}
						for _, pc := range entries {
							if pc > int(op.CurrentOffset) && pc <= int(ops[ordinal+2].CurrentOffset) {
								return false
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
							return false
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
		}
		if count == 0 {
			return false
		}
	}
	return true
}
