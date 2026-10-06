package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This certificate describes the complete original table initialization packet.
// It does not license suppressing a class or replacing a source switch. Enum
// declaration binding, every original user, source case mapping and the exact
// compiler regeneration protocol are independent obligations of a future caller.
type nativeEnumSwitchTable struct {
	object *ClassObject
	tables map[string]*nativeEnumSwitchArray
	uses   map[string]map[string]map[int]*nativeEnumSwitchUse
}
type nativeEnumSwitchArray struct {
	enum    string
	entries map[int]string
}

func nativeEnumSwitchTableProof(obj *ClassObject, work *workbudget.Budget) *nativeEnumSwitchTable {
	if obj == nil || obj.AccessFlags != 0x1020 || obj.GetSupperClassName() != "java/lang/Object" || len(obj.Interfaces) != 0 || len(obj.Fields) == 0 || len(obj.Fields) > 16 || len(obj.Methods) != 1 || !nativeAccessorVersion(obj, work) {
		return nil
	}
	fields := map[string]bool{}
	for _, f := range obj.Fields {
		if f == nil || f.AccessFlags != 0x1018 || len(f.Attributes) != 0 || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(obj, f.NameIndex)
		d, dk := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nk || !dk || d != "[I" || fields[n] {
			return nil
		}
		fields[n] = true
	}
	m := obj.Methods[0]
	if m == nil || m.AccessFlags != 8 || len(m.Attributes) != 1 {
		return nil
	}
	name, nk := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
	code, ok := m.Attributes[0].(*CodeAttribute)
	if !nk || !dk || name != "<clinit>" || desc != "()V" || !ok || code == nil || code.MaxStack != 3 || code.MaxLocals != 1 || len(code.Code) > 65535 || len(code.Attributes) > 3 || !nativeProofWork(work, int64(len(code.Code)+len(code.ExceptionTable))) || work != nil && work.CheckAlloc(int64(len(code.Code))*256) != nil {
		return nil
	}
	var frames *UnparsedAttribute
	seenLines, seenLocals := false, false
	for _, a := range code.Attributes {
		switch a := a.(type) {
		case *LineNumberTableAttribute:
			if a == nil || seenLines {
				return nil
			}
			seenLines = true
			for _, line := range a.LineNumberTable {
				if line == nil || int(line.StartPc) >= len(code.Code) || !nativeProofWork(work, 1) {
					return nil
				}
			}
		case *UnparsedAttribute:
			if a == nil {
				return nil
			}
			if a.Name == "LocalVariableTable" {
				// The caught compiler temporary has no live source range.
				if seenLocals || a.Length != 2 || len(a.Info) != 2 || a.Info[0] != 0 || a.Info[1] != 0 {
					return nil
				}
				seenLocals = true
			} else if a.Name == "StackMapTable" && frames == nil {
				frames = a
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	dec.Work = work
	if dec.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(dec)
	packet := &nativeEnumSwitchTable{object: obj, tables: map[string]*nativeEnumSwitchArray{}}
	cursor, handler := 0, 0
	for cursor < len(ops)-1 {
		if cursor+4 >= len(ops) || !nativeProofWork(work, 4) {
			return nil
		}
		call := constructorMotionMember(obj, ops[cursor], core.OP_INVOKESTATIC)
		if call == nil || call.Member != "values" || call.Description != "()[L"+call.Name+";" || !nativeEnumMemberOperand(obj, ops[cursor], core.OP_INVOKESTATIC, call.Name, "values", call.Description) || !nativeEnumOpcode(ops[cursor+1], core.OP_ARRAYLENGTH) || !nativeEnumOpcode(ops[cursor+2], core.OP_NEWARRAY) || len(ops[cursor+2].Data) != 1 || ops[cursor+2].Data[0] != 10 {
			return nil
		}
		store := constructorMotionMember(obj, ops[cursor+3], core.OP_PUTSTATIC)
		if store == nil || store.Name != obj.GetClassName() || store.Description != "[I" || !fields[store.Member] || packet.tables[store.Member] != nil || !nativeEnumMemberOperand(obj, ops[cursor+3], core.OP_PUTSTATIC, store.Name, store.Member, "[I") {
			return nil
		}
		table := &nativeEnumSwitchArray{enum: call.Name, entries: map[int]string{}}
		packet.tables[store.Member] = table
		constants := map[string]bool{}
		cursor += 4
		for cursor < len(ops)-1 && nativeEnumOpcode(ops[cursor], core.OP_GETSTATIC) {
			if cursor+7 >= len(ops) || handler >= len(code.ExceptionTable) || len(table.entries) >= 4096 || !nativeProofWork(work, 7) {
				return nil
			}
			value := constructorMotionMember(obj, ops[cursor+1], core.OP_GETSTATIC)
			key, known := nativeEnumOriginalInt(obj, ops[cursor+3])
			if value == nil || value.Name != table.enum || value.Description != "L"+table.enum+";" || constants[value.Member] || !known || key != len(table.entries)+1 || table.entries[key] != "" || !nativeEnumMemberOperand(obj, ops[cursor], core.OP_GETSTATIC, obj.GetClassName(), store.Member, "[I") || !nativeEnumMemberOperand(obj, ops[cursor+1], core.OP_GETSTATIC, value.Name, value.Member, value.Description) || !nativeEnumMemberOperand(obj, ops[cursor+2], core.OP_INVOKEVIRTUAL, table.enum, "ordinal", "()I") || !nativeEnumOpcode(ops[cursor+4], core.OP_IASTORE) || !nativeEnumOpcode(ops[cursor+5], core.OP_GOTO) || len(ops[cursor+5].Data) != 2 || !nativeEnumOpcode(ops[cursor+6], core.OP_ASTORE_0) {
				return nil
			}
			h := code.ExceptionTable[handler]
			if h == nil {
				return nil
			}
			caught, known := sourceBridgeClassName(obj, h.CatchType)
			delta := int(int16(uint16(core.Convert2bytesToInt(ops[cursor+5].Data))))
			if !known || caught != "java/lang/NoSuchFieldError" || h.StartPc != ops[cursor].CurrentOffset || h.EndPc != ops[cursor+5].CurrentOffset || h.HandlerPc != ops[cursor+6].CurrentOffset || int(ops[cursor+5].CurrentOffset)+delta != int(ops[cursor+7].CurrentOffset) {
				return nil
			}
			constants[value.Member] = true
			table.entries[key] = value.Member
			handler++
			cursor += 7
		}
		if len(table.entries) == 0 {
			return nil
		}
	}
	if cursor != len(ops)-1 || !nativeEnumOpcode(ops[cursor], core.OP_RETURN) || handler != len(code.ExceptionTable) || len(packet.tables) != len(fields) || !nativeEnumSwitchFrames(obj, code, frames, work) {
		return nil
	}
	return packet
}
