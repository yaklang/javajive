package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The empty-stack cut is already proved by the allocation/array protocols.
// Suffix edges may not enter that prefix. Moving the suffix by a multiple of
// four therefore preserves both branch differences and switch alignment.
func enumArraySuffixClosed(code *CodeAttribute, ops []*core.OpCode, start int) bool {
	positions := map[int]bool{}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = true
	}
	if !positions[start] || len(code.ExceptionTable) != 0 {
		return false
	}
	for _, op := range ops {
		pc, opcode := int(op.CurrentOffset), op.Instr.OpCode
		target := func(delta int) bool { at := pc + delta; return pc >= start && at >= start && positions[at] }
		switch {
		case opcode == core.OP_JSR || opcode == core.OP_JSR_W || opcode == core.OP_RET || opcode == core.OP_WIDE:
			return false
		case constructorMotionPacketBranch(opcode):
			if opcode == core.OP_GOTO_W {
				if len(op.Data) != 4 || !target(int(int32(binary.BigEndian.Uint32(op.Data)))) {
					return false
				}
			} else if len(op.Data) != 2 || !target(int(int16(binary.BigEndian.Uint16(op.Data)))) {
				return false
			}
		case opcode == core.OP_TABLESWITCH || opcode == core.OP_LOOKUPSWITCH:
			_, data, known := enumArraySwitchSpan(code.Code, op)
			if !known {
				return false
			}
			if !target(int(int32(binary.BigEndian.Uint32(data)))) {
				return false
			}
			if opcode == core.OP_TABLESWITCH {
				if len(data) < 12 {
					return false
				}
				lo, hi := int64(int32(binary.BigEndian.Uint32(data[4:]))), int64(int32(binary.BigEndian.Uint32(data[8:])))
				if hi < lo || hi-lo+1 > int64((len(data)-12)/4) || int64(len(data)) != 12+4*(hi-lo+1) {
					return false
				}
				for pos := 12; pos < len(data); pos += 4 {
					if !target(int(int32(binary.BigEndian.Uint32(data[pos:])))) {
						return false
					}
				}
			} else {
				count := int64(int32(binary.BigEndian.Uint32(data[4:])))
				if count < 0 || int64(len(data)) != 8+8*count {
					return false
				}
				var last int32
				for pos := 8; pos < len(data); pos += 8 {
					key := int32(binary.BigEndian.Uint32(data[pos:]))
					if pos > 8 && key <= last || !target(int(int32(binary.BigEndian.Uint32(data[pos+4:])))) {
						return false
					}
					last = key
				}
			}
		}
	}
	return true
}

// The core decoder stores resolved switch destinations, not its raw variable
// width operand. Recover that byte span from the actual Code, including padding.
func enumArraySwitchSpan(code []byte, op *core.OpCode) (int, []byte, bool) {
	pc := int(op.CurrentOffset)
	if pc < 0 || pc >= len(code) || code[pc] != byte(op.Instr.OpCode) {
		return 0, nil, false
	}
	pad := (4 - (pc+1)%4) % 4
	start := pc + 1 + pad
	if start+8 > len(code) || !bytes.Equal(code[pc+1:start], make([]byte, pad)) {
		return 0, nil, false
	}
	data := code[start:]
	length := int64(0)
	if op.Instr.OpCode == core.OP_TABLESWITCH {
		if len(data) < 12 {
			return 0, nil, false
		}
		lo, hi := int64(int32(binary.BigEndian.Uint32(data[4:]))), int64(int32(binary.BigEndian.Uint32(data[8:])))
		if hi < lo {
			return 0, nil, false
		}
		length = 12 + 4*(hi-lo+1)
	} else if op.Instr.OpCode == core.OP_LOOKUPSWITCH {
		count := int64(int32(binary.BigEndian.Uint32(data[4:])))
		if count < 0 {
			return 0, nil, false
		}
		length = 8 + 8*count
	} else {
		return 0, nil, false
	}
	if length > int64(len(data)) {
		return 0, nil, false
	}
	return start + int(length), data[:int(length)], true
}

// All retained instructions have one PC map; there is no branch relaxation or
// frame inference. JVMS 4.7.4 frame deltas and uninitialized NEW offsets must
// follow that map, as do debug interval endpoints. Unknown PC tables decline.
func enumArrayRelocateCode(object *ClassObject, raw []byte, code *CodeAttribute, ops []*core.OpCode, replacement []byte, call, delta int, helperLines map[int]uint16, helperLineName, helperStack uint16, work *workbudget.Budget) ([]byte, error) {
	return enumRelocateInstructionSpanCode(object, raw, code, ops, replacement, call, call+3, delta, helperLines, helperLineName, helperStack, work)
}

// Relocate the metadata for a replaced instruction span. Its caller proves
// that all retained branch source/target pairs are within the affine suffix;
// this editor validates frame/debug positions and rejects opaque PC tables.
func enumRelocateInstructionSpanCode(object *ClassObject, raw []byte, code *CodeAttribute, ops []*core.OpCode, replacement []byte, start, endSpan, delta int, helperLines map[int]uint16, helperLineName, helperStack uint16, work *workbudget.Budget) ([]byte, error) {
	fail := func() ([]byte, error) {
		return nil, sourceLambdaMetadataFailure(work, "enum lowering unproved suffix PC map")
	}
	if len(ops) > 65535 || work.CheckAlloc(int64(len(raw)+len(replacement))*4+int64(len(ops))*128) != nil || delta < 0 || len(replacement) != len(code.Code)+delta || len(replacement) > 65535 || len(raw) < 12+len(code.Code) || int(binary.BigEndian.Uint32(raw[4:])) != len(code.Code) || !bytes.Equal(raw[8:8+len(code.Code)], code.Code) || !nativeProofWork(work, int64(len(raw)+len(replacement))) {
		return fail()
	}
	positions := map[int]int{}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = op.Instr.OpCode
	}
	if _, known := positions[start]; !known || start < 0 || endSpan <= start || endSpan > len(code.Code) {
		return fail()
	}
	if _, known := positions[endSpan]; !known && endSpan != len(code.Code) {
		return fail()
	}
	for pc := range positions {
		if pc > start && pc < endSpan {
			return fail()
		}
	}
	if !enumArraySuffixAlignmentPreserved(positions, endSpan, delta) {
		return fail()
	}
	mapPC := func(pc int, end bool) (int, bool) {
		_, ok := positions[pc]
		if end && pc == len(code.Code) {
			ok = true
		}
		if !ok || pc > start && pc < endSpan {
			return 0, false
		}
		if pc >= endSpan {
			pc += delta
		}
		return pc, pc <= 65535
	}
	end := 8 + len(code.Code)
	if binary.BigEndian.Uint16(raw[end:]) != 0 {
		return fail()
	}
	type attribute struct {
		name uint16
		data []byte
	}
	var attrs []attribute
	seen := map[string]bool{}
	lines := map[int]uint16{}
	lineName := helperLineName
	pos := end + 4
	for i := 0; i < int(binary.BigEndian.Uint16(raw[end+2:])); i++ {
		if len(raw)-pos < 6 {
			return fail()
		}
		length := int64(binary.BigEndian.Uint32(raw[pos+2:]))
		if length < 2 || length > int64(len(raw)-pos-6) || !nativeProofWork(work, length+1) {
			return fail()
		}
		index := binary.BigEndian.Uint16(raw[pos:])
		name, ok := sourceBridgeUTF8(object, index)
		if !ok || seen[name] {
			return fail()
		}
		seen[name] = true
		body := bytes.Clone(raw[pos+6 : pos+6+int(length)])
		count := int(binary.BigEndian.Uint16(body))
		switch name {
		case "LineNumberTable":
			if len(body) != 2+4*count {
				return fail()
			}
			lineName = index
			for j := 0; j < count; j++ {
				pc, valid := mapPC(int(binary.BigEndian.Uint16(body[2+4*j:])), false)
				line := binary.BigEndian.Uint16(body[4+4*j:])
				if !valid {
					return fail()
				}
				if prior, exists := lines[pc]; exists && prior != line {
					return fail()
				}
				lines[pc] = line
			}
		case "LocalVariableTable", "LocalVariableTypeTable":
			if len(body) != 2+10*count {
				return fail()
			}
			for j := 0; j < count; j++ {
				at := 2 + 10*j
				start := int(binary.BigEndian.Uint16(body[at:]))
				limit := start + int(binary.BigEndian.Uint16(body[at+2:]))
				a, ak := mapPC(start, false)
				b, bk := mapPC(limit, true)
				if !ak || !bk || b < a || binary.BigEndian.Uint16(body[at+8:]) >= code.MaxLocals {
					return fail()
				}
				if _, ok := sourceBridgeUTF8(object, binary.BigEndian.Uint16(body[at+4:])); !ok {
					return fail()
				}
				if _, ok := sourceBridgeUTF8(object, binary.BigEndian.Uint16(body[at+6:])); !ok {
					return fail()
				}
				binary.BigEndian.PutUint16(body[at:], uint16(a))
				binary.BigEndian.PutUint16(body[at+2:], uint16(b-a))
			}
			attrs = append(attrs, attribute{index, body})
		case "StackMapTable":
			updated, ok := enumArrayRelocateFrames(object, body, positions, endSpan, delta, work)
			if !ok {
				return fail()
			}
			attrs = append(attrs, attribute{index, updated})
		default:
			return fail()
		}
		pos += 6 + int(length)
	}
	if pos != len(raw) {
		return fail()
	}
	for pc, line := range helperLines {
		if pc < 0 || pc >= delta+endSpan-start {
			return fail()
		}
		lines[start+pc] = line
	}
	if lineName != 0 {
		if len(lines) > 65535 {
			return fail()
		}
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
		attrs = append(attrs, attribute{lineName, body})
	}
	out := bytes.Clone(raw[:8])
	stack := code.MaxStack
	if helperStack > stack {
		stack = helperStack
	}
	binary.BigEndian.PutUint16(out, stack)
	binary.BigEndian.PutUint32(out[4:], uint32(len(replacement)))
	out = append(out, replacement...)
	out = append(out, 0, 0, byte(len(attrs)>>8), byte(len(attrs)))
	for _, a := range attrs {
		h := make([]byte, 6)
		binary.BigEndian.PutUint16(h, a.name)
		binary.BigEndian.PutUint32(h[2:], uint32(len(a.data)))
		out = append(append(out, h...), a.data...)
	}
	return out, nil
}

func enumArraySuffixAlignmentPreserved(positions map[int]int, suffix, delta int) bool {
	if delta%4 == 0 {
		return true
	}
	for pc, opcode := range positions {
		if pc >= suffix && (opcode == core.OP_TABLESWITCH || opcode == core.OP_LOOKUPSWITCH) {
			return false
		}
	}
	return true
}

func enumArrayRelocateFrames(object *ClassObject, raw []byte, positions map[int]int, suffix, delta int, work *workbudget.Budget) ([]byte, bool) {
	if len(raw) < 2 || len(positions) > 65535 || work.CheckAlloc(int64(len(raw))*4) != nil || delta < 0 || !nativeProofWork(work, int64(len(raw)+len(positions))) || !enumArraySuffixAlignmentPreserved(positions, suffix, delta) {
		return nil, false
	}
	out := bytes.Clone(raw[:2])
	cursor, previous := 2, -1
	readU2 := func() (int, bool) {
		if cursor+2 > len(raw) {
			return 0, false
		}
		v := int(binary.BigEndian.Uint16(raw[cursor:]))
		cursor += 2
		return v, true
	}
	verification := func() ([]byte, bool) {
		if cursor >= len(raw) {
			return nil, false
		}
		tag := raw[cursor]
		cursor++
		b := []byte{tag}
		switch tag {
		case 0, 1, 2, 3, 4, 5:
			return b, true
		case 7:
			index, ok := readU2()
			if !ok {
				return nil, false
			}
			if _, ok := sourceBridgeClassName(object, uint16(index)); !ok {
				return nil, false
			}
			return append(b, byte(index>>8), byte(index)), true
		case 8:
			pc, ok := readU2()
			if !ok || pc < suffix || positions[pc] != core.OP_NEW || pc+delta > 65535 {
				return nil, false
			}
			pc += delta
			return append(b, byte(pc>>8), byte(pc)), true
		default:
			// uninitializedThis is illegal in a static initializer and after
			// the constructor SUPER cut used by capture-prefix lowering.
			return nil, false
		}
	}
	for i := 0; i < int(binary.BigEndian.Uint16(raw)); i++ {
		if cursor >= len(raw) {
			return nil, false
		}
		tag := raw[cursor]
		cursor++
		offset := 0
		ok := true
		if tag <= 63 {
			offset = int(tag)
		} else if tag <= 127 {
			offset = int(tag - 64)
		} else if tag >= 247 {
			offset, ok = readU2()
		} else {
			return nil, false
		}
		if !ok {
			return nil, false
		}
		pc := previous + 1 + offset
		if _, ok := positions[pc]; !ok || pc < suffix || pc+delta > 65535 {
			return nil, false
		}
		previous = pc
		if i == 0 {
			offset += delta
		}
		if offset > 65535 {
			return nil, false
		}
		var payload []byte
		items := 0
		switch {
		case tag <= 63:
			tag = 251
		case tag <= 127:
			tag = 247
			items = 1
		case tag == 247:
			items = 1
		case tag >= 248 && tag <= 251:
		case tag >= 252 && tag <= 254:
			items = int(tag - 251)
		case tag == 255:
			for groups := 0; groups < 2; groups++ {
				n, ok := readU2()
				if !ok || n > len(raw)-cursor {
					return nil, false
				}
				payload = append(payload, byte(n>>8), byte(n))
				for j := 0; j < n; j++ {
					v, ok := verification()
					if !ok {
						return nil, false
					}
					payload = append(payload, v...)
				}
			}
		default:
			return nil, false
		}
		for j := 0; j < items; j++ {
			v, ok := verification()
			if !ok {
				return nil, false
			}
			payload = append(payload, v...)
		}
		out = append(out, tag, byte(offset>>8), byte(offset))
		out = append(out, payload...)
	}
	return out, cursor == len(raw)
}
