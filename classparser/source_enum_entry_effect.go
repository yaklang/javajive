package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A physical entry trampoline can put a straight, effectful prelude after the
// enum body while executing it BEFORE every allocation. Java enum syntax puts
// user static statements after constants. Treat these as two ordered regions,
// not commuting statements. This function only partitions instruction spans;
// the region proof separately checks effects, handler ownership and exit cuts.
// Every original byte must belong to exactly one region or its routing edge.
func enumOriginalEntryEffect(code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) (body, prelude []*core.OpCode, known bool) {
	if code == nil || len(ops) < 5 || !enumArrayInstructionSpans(code.Code, ops) || !nativeProofWork(work, int64(len(ops))+1) {
		return nil, nil, false
	}
	target := func(op *core.OpCode) (int, bool) {
		if nativeEnumOpcode(op, core.OP_GOTO) && len(op.Data) == 2 {
			return int(op.CurrentOffset) + int(int16(binary.BigEndian.Uint16(op.Data))), true
		}
		if nativeEnumOpcode(op, core.OP_GOTO_W) && len(op.Data) == 4 {
			return int(op.CurrentOffset) + int(int32(binary.BigEndian.Uint32(op.Data))), true
		}
		return 0, false
	}
	entry, first := target(ops[0])
	resume, last := target(ops[len(ops)-1])
	if !first || !last || resume != int(ops[1].CurrentOffset) {
		return nil, nil, false
	}
	for i := 2; i < len(ops)-1; i++ {
		if int(ops[i].CurrentOffset) == entry && nativeEnumOpcode(ops[i-1], core.OP_RETURN) {
			return ops[1:i], ops[i : len(ops)-1], true
		}
	}
	return nil, nil, false
}

// Symbolic operands retain CP tag and exact UTF-16 member identity. Only the
// two encodings of LDC and integer constant widths normalize here. There is no
// purity assumption: calls and static field effects must match in order, even
// when their result is unused. Dynamic constants and unknown effects decline.
func enumEntryEffectProgram(obj *ClassObject, ops []*core.OpCode, work *workbudget.Budget) ([]sourceBridgeInstruction, bool) {
	if obj == nil || len(ops) == 0 || len(ops) > 256 || work.CheckAlloc(int64(len(ops))*128) != nil {
		return nil, false
	}
	result := make([]sourceBridgeInstruction, 0, len(ops))
	for _, op := range ops {
		if op == nil || op.Instr == nil || op.IsWide || !nativeProofWork(work, 1) {
			return nil, false
		}
		i := sourceBridgeInstruction{opcode: op.Instr.OpCode}
		if value, known := nativeEnumOriginalInt(obj, op); known {
			i.opcode, i.operand = core.OP_ICONST_0, strconv.Itoa(value)
		} else {
			switch i.opcode {
			case core.OP_ACONST_NULL, core.OP_POP, core.OP_POP2:
				if len(op.Data) != 0 {
					return nil, false
				}
			case core.OP_LDC, core.OP_LDC_W, core.OP_LDC2_W:
				index, valid := nativeEnumCPIndex(op)
				if !valid || index == 0 || int(index) > len(obj.ConstantPool) {
					return nil, false
				}
				wide := i.opcode == core.OP_LDC2_W
				i.opcode = core.OP_LDC
				switch c := obj.ConstantPool[index-1].(type) {
				case *ConstantStringInfo:
					if c == nil || wide {
						return nil, false
					}
					key, valid := initializerIdentityUTF8Key(obj, c.StringIndex, work)
					if !valid {
						return nil, false
					}
					i.operand = "string\x00" + key
				case *ConstantClassInfo:
					key, valid := initializerIdentityClassKey(obj, index, work)
					if !valid || wide {
						return nil, false
					}
					i.operand = "class\x00" + key
				case *ConstantLongInfo:
					if c == nil || !wide {
						return nil, false
					}
					i.operand = "long\x00" + strconv.FormatInt(c.Value, 10)
				case *ConstantFloatInfo:
					if c == nil || wide {
						return nil, false
					}
					i.operand = "float\x00" + strconv.FormatUint(uint64(math.Float32bits(c.Value)), 16)
				case *ConstantDoubleInfo:
					if c == nil || !wide {
						return nil, false
					}
					i.operand = "double\x00" + strconv.FormatUint(math.Float64bits(c.Value), 16)
				default:
					return nil, false
				}
			case core.OP_GETSTATIC, core.OP_PUTSTATIC, core.OP_INVOKESTATIC:
				index, valid := nativeEnumCPIndex(op)
				if !valid || index == 0 || int(index) > len(obj.ConstantPool) {
					return nil, false
				}
				var ref *ConstantMemberrefInfo
				tag := "field"
				switch value := obj.ConstantPool[index-1].(type) {
				case *ConstantFieldrefInfo:
					if value != nil && i.opcode != core.OP_INVOKESTATIC {
						ref = &value.ConstantMemberrefInfo
					}
				case *ConstantMethodrefInfo:
					if value != nil && i.opcode == core.OP_INVOKESTATIC {
						ref, tag = &value.ConstantMemberrefInfo, "method"
					}
				case *ConstantInterfaceMethodrefInfo:
					if value != nil && i.opcode == core.OP_INVOKESTATIC {
						ref, tag = &value.ConstantMemberrefInfo, "interface"
					}
				}
				if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
					return nil, false
				}
				pair, valid := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !valid || pair == nil {
					return nil, false
				}
				owner, okOwner := initializerIdentityClassKey(obj, ref.ClassIndex, work)
				name, okName := initializerIdentityUTF8Key(obj, pair.NameIndex, work)
				desc, okDesc := initializerIdentityUTF8Key(obj, pair.DescriptorIndex, work)
				if !okOwner || !okName || !okDesc {
					return nil, false
				}
				i.operand = tag + "\x00" + owner + "\x00" + name + "\x00" + desc
			default:
				return nil, false
			}
		}
		if len(i.operand) > 65535 || !nativeProofWork(work, int64(len(i.operand))+1) {
			return nil, false
		}
		result = append(result, i)
	}
	return result, true
}

func enumEntryEffectEmptyCuts(obj *ClassObject, method *MemberInfo, work *workbudget.Budget, pcs ...int) bool {
	code := constructorLoweringCode(method)
	if code == nil || code.MaxLocals != 0 && len(code.ExceptionTable) == 0 {
		return false
	}
	d := NewClassObjectDumper(obj)
	d.Work = work
	_, snapshot, known := d.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return false
	}
	for _, pc := range pcs {
		found := false
		for _, record := range snapshot.Instructions {
			if int(record.PC) == pc && len(record.Before.Stack) == 0 {
				found = true
				for _, local := range record.Before.Locals {
					if local.Kind != frametransfer.Top && len(code.ExceptionTable) == 0 {
						found = false
					}
				}
			}
		}
		if !found || !nativeProofWork(work, int64(len(snapshot.Instructions))+1) {
			return false
		}
	}
	return true
}

// Relocate ONLY the matched fresh region. There are no branch/frame/handler
// crossings in this first domain, no live locals, and empty stacks at both
// cuts. Lines follow their instructions; other methods, CP entries and every
// fresh effect instruction remain byte-identical. Original Code is never used
// to construct output. The subsequent array-helper transaction still has to
// close the complete declaration/consumer and backing-array protocols.
func enumMoveFreshEntryEffect(compiled []byte, obj *ClassObject, layout *compiledMetadataLayout, method *MemberInfo, ops []*core.OpCode, start, end int, work *workbudget.Budget) ([]byte, bool) {
	code := constructorLoweringCode(method)
	if !enumEntryEffectEmptyCuts(obj, method, work, 0, start, end) || start <= 0 || end <= start || end >= len(code.Code) || work.CheckAlloc(int64(len(compiled))*2) != nil {
		return nil, false
	}
	span, found := layout.methods[method]
	if !found {
		return nil, false
	}
	raw, err := enumArrayCodePayload(compiled, obj, span, work)
	if err != nil {
		return nil, false
	}
	if len(code.ExceptionTable) > 0 {
		return enumMoveFreshEntryRegion(compiled, obj, raw, method, ops, start, end, work)
	}
	if _, _, err := enumArrayCodeLines(raw.data, obj, code, ops, work); err != nil {
		return nil, false
	}
	for _, op := range ops {
		x := op.Instr.OpCode
		if constructorMotionPacketBranch(x) || x == core.OP_TABLESWITCH || x == core.OP_LOOKUPSWITCH || x == core.OP_RET || x == core.OP_WIDE || x == core.OP_ATHROW || x >= core.OP_IRETURN && x < core.OP_RETURN {
			return nil, false
		}
		if x == core.OP_RETURN && int(op.CurrentOffset) != len(code.Code)-1 {
			return nil, false
		}
	}
	mapPC := func(pc int) int {
		if pc < start {
			return pc + end - start
		}
		if pc < end {
			return pc - start
		}
		return pc
	}
	info := bytes.Clone(raw.data)
	at := 8
	at += copy(info[at:], code.Code[start:end])
	at += copy(info[at:], code.Code[:start])
	copy(info[at:], code.Code[end:])
	for pos := 12 + len(code.Code); pos < len(info); {
		size := int(binary.BigEndian.Uint32(info[pos+2:]))
		name, _ := sourceBridgeUTF8(obj, binary.BigEndian.Uint16(info[pos:]))
		if name == "LineNumberTable" {
			for row := pos + 8; row < pos+6+size; row += 4 {
				pc := int(binary.BigEndian.Uint16(info[row:]))
				binary.BigEndian.PutUint16(info[row:], uint16(mapPC(pc)))
			}
		}
		pos += 6 + size
	}
	attribute := bytes.Clone(compiled[raw.span.start:raw.span.end])
	copy(attribute[6:], info)
	result, err := applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{{raw.span, attribute}}, work)
	return result, err == nil
}
