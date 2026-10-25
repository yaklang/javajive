package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Move a closed fresh exception region before the allocation region. A branch
// to its former exit now joins the allocation entry, not the old RETURN. This
// is a continuation map, distinct from the map of instruction locations.
// Handler ranges, ordered catch types, verifier joins and debug intervals are
// edited together; no original executable bytes participate in the output.
func enumMoveFreshEntryRegion(compiled []byte, obj *ClassObject, raw enumArrayCodeSpan, method *MemberInfo, ops []*core.OpCode, start, end int, work *workbudget.Budget) ([]byte, bool) {
	code := constructorLoweringCode(method)
	if code == nil || start <= 0 || end <= start || end >= len(code.Code) || len(raw.data) < 12+len(code.Code) || work.CheckAlloc(int64(len(raw.data))*8) != nil {
		return nil, false
	}
	size := end - start
	positions := map[int]int{}
	handlers := map[int]uint16{}
	targets := map[int]bool{}
	for _, h := range code.ExceptionTable {
		if h == nil {
			return nil, false
		}
		if old, found := handlers[int(h.HandlerPc)]; found && old != h.CatchType {
			return nil, false
		}
		handlers[int(h.HandlerPc)] = h.CatchType
		targets[int(h.HandlerPc)] = true
	}
	mapPC := func(pc int) int {
		if pc < start {
			return pc + size
		}
		if pc < end {
			return pc - start
		}
		return pc
	}
	joinPC := func(pc int) int {
		if pc == end {
			return size
		}
		return mapPC(pc)
	}
	replacement := append(bytes.Clone(code.Code[start:end]), code.Code[:start]...)
	replacement = append(replacement, code.Code[end:]...)
	for _, op := range ops {
		pc, x := int(op.CurrentOffset), op.Instr.OpCode
		positions[pc] = x
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if x == core.OP_GOTO || x == core.OP_GOTO_W {
			if pc < start || pc >= end {
				return nil, false
			}
			delta := 0
			if x == core.OP_GOTO && len(op.Data) == 2 {
				delta = int(int16(binary.BigEndian.Uint16(op.Data)))
			} else if x == core.OP_GOTO_W && len(op.Data) == 4 {
				delta = int(int32(binary.BigEndian.Uint32(op.Data)))
			} else {
				return nil, false
			}
			if pc+delta <= pc || pc+delta > end {
				return nil, false
			}
			targets[pc+delta] = true
			mapped := mapPC(pc)
			difference := joinPC(pc+delta) - mapped
			if x == core.OP_GOTO {
				if difference < -32768 || difference > 32767 {
					return nil, false
				}
				binary.BigEndian.PutUint16(replacement[mapped+1:], uint16(int16(difference)))
			} else {
				binary.BigEndian.PutUint32(replacement[mapped+1:], uint32(int32(difference)))
			}
		} else if constructorMotionPacketBranch(x) || x == core.OP_TABLESWITCH || x == core.OP_LOOKUPSWITCH || x == core.OP_RET || x == core.OP_WIDE || x == core.OP_ATHROW || x >= core.OP_IRETURN && x < core.OP_RETURN {
			return nil, false
		} else if x == core.OP_RETURN && pc != len(code.Code)-1 {
			return nil, false
		}
	}
	boundary := func(pc int) bool { _, ok := positions[pc]; return ok || pc == len(code.Code) }
	interval := func(a, b int) (int, int, bool) {
		if !boundary(a) || !boundary(b) || a > b {
			return 0, 0, false
		}
		if a >= start && b <= end {
			return a - start, b - start, true
		}
		if a >= 0 && b <= start {
			return a + size, b + size, true
		}
		if a >= end && b <= len(code.Code) {
			return a, b, true
		}
		return 0, 0, false
	}
	at := 8 + len(code.Code)
	count := int(binary.BigEndian.Uint16(raw.data[at:]))
	at += 2
	if count != len(code.ExceptionTable) || at+8*count+2 > len(raw.data) {
		return nil, false
	}
	eh := bytes.Clone(raw.data[at : at+8*count])
	at += 8 * count
	for i, h := range code.ExceptionTable {
		a, b, ok := interval(int(h.StartPc), int(h.EndPc))
		c := int(h.HandlerPc)
		if !ok || int(h.StartPc) < start || int(h.EndPc) > end || c < start || c >= end || !boundary(c) {
			return nil, false
		}
		row := eh[8*i : 8*i+8]
		if binary.BigEndian.Uint16(row) != h.StartPc || binary.BigEndian.Uint16(row[2:]) != h.EndPc || binary.BigEndian.Uint16(row[4:]) != h.HandlerPc || binary.BigEndian.Uint16(row[6:]) != h.CatchType {
			return nil, false
		}
		binary.BigEndian.PutUint16(row, uint16(a))
		binary.BigEndian.PutUint16(row[2:], uint16(b))
		binary.BigEndian.PutUint16(row[4:], uint16(mapPC(c)))
	}
	nattrs := int(binary.BigEndian.Uint16(raw.data[at:]))
	at += 2
	type attribute struct {
		name uint16
		data []byte
	}
	var attrs []attribute
	seen := map[string]bool{}
	for i := 0; i < nattrs; i++ {
		if at+6 > len(raw.data) {
			return nil, false
		}
		name := binary.BigEndian.Uint16(raw.data[at:])
		length := int(binary.BigEndian.Uint32(raw.data[at+2:]))
		at += 6
		if length < 2 || length > len(raw.data)-at || !nativeProofWork(work, int64(length)+1) {
			return nil, false
		}
		key, known := sourceBridgeUTF8(obj, name)
		if !known || seen[key] {
			return nil, false
		}
		seen[key] = true
		body := bytes.Clone(raw.data[at : at+length])
		at += length
		n := int(binary.BigEndian.Uint16(body))
		switch key {
		case "LineNumberTable":
			if len(body) != 2+4*n {
				return nil, false
			}
			for j := 0; j < n; j++ {
				row := body[2+4*j:]
				pc := int(binary.BigEndian.Uint16(row))
				if _, ok := positions[pc]; !ok {
					return nil, false
				}
				binary.BigEndian.PutUint16(row, uint16(mapPC(pc)))
			}
		case "LocalVariableTable", "LocalVariableTypeTable":
			if len(body) != 2+10*n {
				return nil, false
			}
			for j := 0; j < n; j++ {
				row := body[2+10*j:]
				a := int(binary.BigEndian.Uint16(row))
				b := a + int(binary.BigEndian.Uint16(row[2:]))
				left, right, ok := interval(a, b)
				if !ok || binary.BigEndian.Uint16(row[8:]) >= code.MaxLocals {
					return nil, false
				}
				if _, ok := sourceBridgeUTF8(obj, binary.BigEndian.Uint16(row[4:])); !ok {
					return nil, false
				}
				if _, ok := sourceBridgeUTF8(obj, binary.BigEndian.Uint16(row[6:])); !ok {
					return nil, false
				}
				binary.BigEndian.PutUint16(row, uint16(left))
				binary.BigEndian.PutUint16(row[2:], uint16(right-left))
			}
		case "StackMapTable":
			var ok bool
			body, ok = enumEntryRegionFrames(obj, body, code.MaxLocals, handlers, targets, joinPC, work)
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
		attrs = append(attrs, attribute{name, body})
	}
	if at != len(raw.data) || obj.MajorVersion >= 50 && !seen["StackMapTable"] {
		return nil, false
	}
	out := bytes.Clone(raw.data[:8])
	out = append(out, replacement...)
	out = append(out, byte(count>>8), byte(count))
	out = append(out, eh...)
	out = append(out, byte(nattrs>>8), byte(nattrs))
	for _, a := range attrs {
		head := make([]byte, 6)
		binary.BigEndian.PutUint16(head, a.name)
		binary.BigEndian.PutUint32(head[2:], uint32(len(a.data)))
		out = append(append(out, head...), a.data...)
	}
	attributeBytes := make([]byte, 6)
	binary.BigEndian.PutUint16(attributeBytes, raw.name)
	binary.BigEndian.PutUint32(attributeBytes[2:], uint32(len(out)))
	attributeBytes = append(attributeBytes, out...)
	result, err := applyCompiledMetadataEdits(compiled, []compiledMetadataEdit{{raw.span, attributeBytes}}, work)
	return result, err == nil
}

// Expand compressed frames before relocating them. Their local-state deltas
// refer to the previous physical frame, so simply permuting encoded frames is
// unsound. Scratch locals are int/TOP, handler stacks retain their declared
// catch class, and all branch/handler targets must have exactly one frame.
func enumEntryRegionFrames(obj *ClassObject, raw []byte, maxLocals uint16, handlers map[int]uint16, targets map[int]bool, mapPC func(int) int, work *workbudget.Budget) ([]byte, bool) {
	if len(raw) < 2 || len(raw) > 1<<20 || work.CheckAlloc(int64(len(raw))*4) != nil {
		return nil, false
	}
	type frame struct {
		pc            int
		locals, stack []byte
	}
	var frames []frame
	seen := map[int]bool{}
	cursor, previous := 2, -1
	var locals []byte
	u2 := func() (int, bool) {
		if cursor+2 > len(raw) {
			return 0, false
		}
		n := int(binary.BigEndian.Uint16(raw[cursor:]))
		cursor += 2
		return n, true
	}
	readLocals := func(n int) ([]byte, bool) {
		if n < 0 || n > 32 || cursor+n > len(raw) {
			return nil, false
		}
		v := bytes.Clone(raw[cursor : cursor+n])
		cursor += n
		for _, tag := range v {
			if tag != 0 && tag != 1 {
				return nil, false
			}
		}
		return v, true
	}
	for i := 0; i < int(binary.BigEndian.Uint16(raw)); i++ {
		if cursor >= len(raw) || !nativeProofWork(work, 1) {
			return nil, false
		}
		tag := int(raw[cursor])
		cursor++
		delta, items := 0, 0
		ok := true
		switch {
		case tag <= 63:
			delta = tag
		case tag <= 127:
			delta = tag - 64
			items = 1
		case tag == 247:
			delta, ok = u2()
			items = 1
		case tag >= 248 && tag <= 250:
			delta, ok = u2()
			chop := 251 - tag
			if chop > len(locals) {
				return nil, false
			}
			locals = locals[:len(locals)-chop]
		case tag == 251:
			delta, ok = u2()
		case tag >= 252 && tag <= 254:
			delta, ok = u2()
			if !ok {
				return nil, false
			}
			added, known := readLocals(tag - 251)
			if !known {
				return nil, false
			}
			locals = append(bytes.Clone(locals), added...)
		case tag == 255:
			delta, ok = u2()
			if !ok {
				return nil, false
			}
			n, valid := u2()
			if !valid {
				return nil, false
			}
			locals, ok = readLocals(n)
			if !ok {
				return nil, false
			}
			items, ok = u2()
			if items > 1 {
				return nil, false
			}
		default:
			return nil, false
		}
		if !ok || len(locals) > 32 || len(locals) > int(maxLocals) {
			return nil, false
		}
		pc := previous + delta + 1
		previous = pc
		catch, isHandler := handlers[pc]
		if seen[pc] || !targets[pc] || isHandler != (items == 1) {
			return nil, false
		}
		seen[pc] = true
		var stack []byte
		if items == 1 {
			if cursor >= len(raw) || raw[cursor] != 7 {
				return nil, false
			}
			cursor++
			index, valid := u2()
			if !valid {
				return nil, false
			}
			typ, valid := initializerIdentityClassKey(obj, uint16(index), work)
			if !valid {
				return nil, false
			}
			expected := "java/lang/Throwable"
			if catch != 0 {
				expected, valid = initializerIdentityClassKey(obj, catch, work)
				if !valid {
					return nil, false
				}
			}
			if typ != expected {
				return nil, false
			}
			stack = []byte{7, byte(index >> 8), byte(index)}
		}
		frames = append(frames, frame{mapPC(pc), bytes.Clone(locals), stack})
	}
	if cursor != len(raw) || len(seen) != len(targets) {
		return nil, false
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].pc < frames[j].pc })
	out := []byte{byte(len(frames) >> 8), byte(len(frames))}
	previous = -1
	for _, f := range frames {
		delta := f.pc - previous - 1
		if delta < 0 || delta > 65535 {
			return nil, false
		}
		previous = f.pc
		out = append(out, 255, byte(delta>>8), byte(delta), 0, byte(len(f.locals)))
		out = append(out, f.locals...)
		count := byte(0)
		if len(f.stack) > 0 {
			count = 1
		}
		out = append(out, 0, count)
		out = append(out, f.stack...)
	}
	return out, true
}
