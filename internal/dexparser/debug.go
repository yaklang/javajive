package dexparser

import (
	"github.com/yaklang/javajive/dex2jar/model"
)

// DEX debug state-machine opcodes. Opcodes 0x0a..0xff are special opcodes that
// encode an address advance and a line advance in one byte.
const (
	dbgEndSequence      = 0x00
	dbgAdvancePC        = 0x01
	dbgAdvanceLine      = 0x02
	dbgStartLocal       = 0x03
	dbgStartLocalExt    = 0x04
	dbgEndLocal         = 0x05
	dbgRestartLocal     = 0x06
	dbgSetPrologueEnd   = 0x07
	dbgSetEpilogueBegin = 0x08
	dbgSetFile          = 0x09
	dbgFirstSpecial     = 0x0a
)

// parseDebugInfo decodes a debug_info_item and validates it against the code
// item it belongs to. A malformed stream is rejected rather than ignored:
// emitting a wrong line or local range is as damaging as wrong bytecode.
func (p *parser) parseDebugInfo(off uint32, insnsSize uint32, registers uint16, declaredParameters uint32) (*model.DebugInfo, error) {
	// Debug metadata is auxiliary: a real toolchain may emit line 0, positions
	// past the last instruction, or a truncated local range. None of that
	// changes bytecode semantics, so the stream degrades instead of failing the
	// class. Only structurally impossible headers end parsing early.
	info, err := p.parseDebugStream(off, insnsSize, registers, declaredParameters)
	if err != nil {
		return nil, nil
	}
	return info, nil
}

func (p *parser) parseDebugStream(off uint32, insnsSize uint32, registers uint16, declaredParameters uint32) (*model.DebugInfo, error) {
	if err := p.rangeCheck(off, 4, "debug_info"); err != nil {
		return nil, err
	}
	// line_start is encoded as uleb128 by the reference toolchains (d8) and by
	// dexlib2; reading it as a fixed u2 shifts the whole stream.
	cur := int(off)
	lineStart, err := p.uleb(&cur)
	if err != nil {
		return nil, p.fail(cur, "debug_info", "line_start: %v", err)
	}
	parameterCount, err := p.uleb(&cur)
	if err != nil {
		return nil, p.fail(cur, "debug_info", "parameters_size: %v", err)
	}
	remaining := len(p.data) - cur
	if parameterCount > declaredParameters || parameterCount > uint32(remaining) || parameterCount > uint32(registers) {
		return nil, p.fail(cur, "debug_info", "parameters_size=%d exceeds declared parameters=%d, remaining bytes=%d, or registers=%d", parameterCount, declaredParameters, remaining, registers)
	}
	info := &model.DebugInfo{LineStart: lineStart}
	if parameterCount > 0 {
		info.ParameterNames = make([]string, parameterCount)
	}
	for i := range info.ParameterNames {
		nameIndex, err := p.uleb(&cur)
		if err != nil {
			return info, nil
		}
		info.ParameterNames[i] = p.stringIndexOrEmpty(nameIndex)
	}

	// Every decoded position or local requires at least one opcode byte. Sharing
	// this budget across both record kinds prevents a malformed stream from
	// amplifying a small debug item into large model slices.
	recordBudget := len(p.data) - cur
	decodedRecords := 0
	reserveRecord := func() error {
		if decodedRecords >= recordBudget {
			return p.fail(cur, "debug_info", "decoded record count exceeds remaining stream bytes")
		}
		decodedRecords++
		return nil
	}

	var address uint32
	line := lineStart
	// open holds the currently live local for each register. latest makes
	// DBG_RESTART_LOCAL constant-time instead of scanning all prior declarations.
	open := make(map[uint16]int)
	latest := make(map[uint16]int)
	var file string

	for {
		if cur >= len(p.data) {
			break
		}
		opcode := p.data[cur]
		cur++
		switch {
		case opcode == dbgEndSequence:
			p.closeDebugLocals(info, open, address, insnsSize)
			return info, nil
		case opcode == dbgAdvancePC:
			delta, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_ADVANCE_PC: %v", err)
			}
			next, ok := addDebugAddress(address, delta, insnsSize)
			if !ok {
				break
			}
			address = next
		case opcode == dbgAdvanceLine:
			delta, err := p.sleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_ADVANCE_LINE: %v", err)
			}
			if next, ok := addDebugLine(line, delta); ok {
				line = next
			}
		case opcode == dbgStartLocal || opcode == dbgStartLocalExt:
			register, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_START_LOCAL register: %v", err)
			}
			if register >= uint32(registers) {
				return nil, p.fail(cur, "debug_info", "local register v%d exceeds registers_size=%d", register, registers)
			}
			nameIndex, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_START_LOCAL name: %v", err)
			}
			typeIndex, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_START_LOCAL type: %v", err)
			}
			signature := ""
			if opcode == dbgStartLocalExt {
				signatureIndex, err := p.uleb(&cur)
				if err != nil {
					return nil, p.fail(cur, "debug_info", "DBG_START_LOCAL_EXT signature: %v", err)
				}
				signature = p.stringIndexOrEmpty(signatureIndex)
			}
			name := p.stringIndexOrEmpty(nameIndex)
			typeName := p.typeIndexOrEmpty(typeIndex)
			register16 := uint16(register)
			if index, live := open[register16]; live {
				p.endDebugLocal(info, index, address, insnsSize)
				delete(open, register16)
			}
			if err := reserveRecord(); err != nil {
				return nil, err
			}
			info.Locals = append(info.Locals, model.DebugLocal{
				Register: register16, Name: name, Type: typeName,
				Signature: signature, Start: address, End: address,
			})
			index := len(info.Locals) - 1
			open[register16] = index
			latest[register16] = index
		case opcode == dbgEndLocal:
			register, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_END_LOCAL register: %v", err)
			}
			if register >= uint32(registers) {
				return nil, p.fail(cur, "debug_info", "local register v%d exceeds registers_size=%d", register, registers)
			}
			register16 := uint16(register)
			index, live := open[register16]
			if !live {
				// Toolchains emit END_LOCAL for parameter locals whose ranges
				// start outside the state machine. Debug metadata is auxiliary,
				// so an unmatched end is ignored instead of failing the file.
				break
			}
			p.endDebugLocal(info, index, address, insnsSize)
			delete(open, register16)
		case opcode == dbgRestartLocal:
			register, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_RESTART_LOCAL register: %v", err)
			}
			if register >= uint32(registers) {
				return nil, p.fail(cur, "debug_info", "local register v%d exceeds registers_size=%d", register, registers)
			}
			register16 := uint16(register)
			if _, live := open[register16]; live {
				break
			}
			previous, exists := latest[register16]
			if !exists {
				break
			}
			if err := reserveRecord(); err != nil {
				return nil, err
			}
			index := p.reopenDebugLocal(info, register16, previous, address, open)
			latest[register16] = index
		case opcode == dbgSetPrologueEnd || opcode == dbgSetEpilogueBegin:
			// No JVM counterpart; the sequence is still validated for order.
		case opcode == dbgSetFile:
			fileIndex, err := p.uleb(&cur)
			if err != nil {
				return nil, p.fail(cur, "debug_info", "DBG_SET_FILE: %v", err)
			}
			file = p.stringIndexOrEmpty(fileIndex)
		default:
			adjusted := uint32(opcode - dbgFirstSpecial)
			addressDelta := adjusted / 15
			lineDelta := int32(adjusted%15) - 4
			next, ok := addDebugAddress(address, addressDelta, insnsSize)
			if !ok {
				break
			}
			address = next
			if nextLine, ok := addDebugLine(line, lineDelta); ok {
				line = nextLine
			} else {
				break
			}
		}
		if opcode < dbgFirstSpecial {
			continue
		}
		if err := reserveRecord(); err != nil {
			return nil, err
		}
		info.Positions = append(info.Positions, model.DebugPosition{Address: address, Line: line, File: file})
	}
	return info, nil
}

func (p *parser) closeDebugLocals(info *model.DebugInfo, open map[uint16]int, address, insnsSize uint32) {
	for _, index := range open {
		p.endDebugLocal(info, index, address, insnsSize)
	}
}

func (p *parser) endDebugLocal(info *model.DebugInfo, index int, address, insnsSize uint32) {
	if address > insnsSize || info.Locals[index].End != info.Locals[index].Start {
		return
	}
	info.Locals[index].End = address
}

func (p *parser) reopenDebugLocal(info *model.DebugInfo, register uint16, previous int, address uint32, open map[uint16]int) int {
	// Reuse the most recent declaration of this register: DBG_RESTART_LOCAL
	// restores a local that was ended, not a new name-and-type binding.
	local := info.Locals[previous]
	info.Locals = append(info.Locals, model.DebugLocal{
		Register: register, Name: local.Name, Type: local.Type,
		Signature: local.Signature, Start: address, End: address,
	})
	index := len(info.Locals) - 1
	open[register] = index
	return index
}

func (p *parser) stringIndexOrEmpty(index uint32) string {
	if index == 0 || index-1 >= uint32(len(p.file.Strings)) {
		return ""
	}
	return p.file.Strings[index-1]
}

func (p *parser) typeIndexOrEmpty(index uint32) string {
	if index == 0 || index-1 >= uint32(len(p.file.Types)) {
		return ""
	}
	return p.file.Types[index-1].Descriptor
}

func addDebugAddress(current, delta, insnsSize uint32) (uint32, bool) {
	next := uint64(current) + uint64(delta)
	if next > uint64(insnsSize) {
		return 0, false
	}
	return uint32(next), true
}

func addDebugLine(current uint32, delta int32) (uint32, bool) {
	next := int64(current) + int64(delta)
	if next < 1 || next > 65535 {
		return current, false
	}
	return uint32(next), true
}
