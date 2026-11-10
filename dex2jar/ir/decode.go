package ir

import (
	"fmt"
	"math"

	"github.com/yaklang/javajive/dex2jar/model"
)

// Decode decodes code units exactly, retaining DEX code-unit offsets and
// format widths. Semantic subset validation is performed by Build.
func Decode(code *model.Code, ctx Context) ([]Instruction, error) {
	if code == nil {
		return nil, diagnostic(ctx, nil, "code", fmt.Errorf("nil code item"))
	}
	words := code.Instructions
	if uint64(len(words)) > math.MaxUint32 {
		return nil, diagnostic(ctx, nil, "code", fmt.Errorf("instruction stream has %d code units; maximum addressable size is %d", len(words), uint64(math.MaxUint32)))
	}
	instructions := make([]Instruction, 0, len(words))
	for offset := uint32(0); offset < uint32(len(words)); {
		word := words[offset]
		opcode := byte(word)
		if opcode == 0 && word>>8 >= 1 && word>>8 <= 3 {
			// Payloads are data, not executable instructions: skip them and let
			// the switch that references the payload decode its contents.
			width, err := payloadUnits(words, offset, ctx)
			if err != nil {
				return nil, err
			}
			offset += width
			continue
		}
		info, known := opcodeTable[opcode]
		if !known {
			return nil, diagnostic(ctx, &Instruction{Offset: offset, Opcode: opcode, Name: "unknown"}, "", fmt.Errorf("unknown DEX 035 opcode"))
		}
		if uint64(offset)+uint64(info.width) > uint64(len(words)) {
			return nil, diagnostic(ctx, &Instruction{Offset: offset, Opcode: opcode, Name: info.name}, "", fmt.Errorf("truncated %s instruction: need %d code units, have %d", info.format, info.width, uint32(len(words))-offset))
		}
		instruction := Instruction{Offset: offset, Opcode: opcode, Name: info.name, Format: info.format, Width: info.width, Target: -1}
		word1 := func() uint16 { return words[offset+1] }
		word2 := func() uint16 { return words[offset+2] }
		switch info.format {
		case "10x":
			if word>>8 != 0 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("reserved high byte is non-zero"))
			}
		case "12x":
			instruction.Regs = []uint16{uint16((word >> 8) & 0xf), uint16(word >> 12)}
		case "11x":
			instruction.Regs = []uint16{word >> 8}
		case "11n":
			instruction.Regs = []uint16{uint16((word >> 8) & 0xf)}
			instruction.Literal = int64(int8((word>>12)<<4) >> 4)
		case "22x":
			instruction.Regs = []uint16{word >> 8, word1()}
		case "32x":
			if word>>8 != 0 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("reserved high byte is non-zero"))
			}
			instruction.Regs = []uint16{word1(), word2()}
		case "21s":
			instruction.Regs = []uint16{word >> 8}
			instruction.Literal = int64(int16(word1()))
		case "31i":
			instruction.Regs = []uint16{word >> 8}
			instruction.Literal = int64(int32(uint32(word1()) | uint32(word2())<<16))
		case "51l":
			instruction.Regs = []uint16{word >> 8}
			instruction.Literal = int64(uint64(words[offset+1]) | uint64(words[offset+2])<<16 | uint64(words[offset+3])<<32 | uint64(words[offset+4])<<48)
		case "21h":
			instruction.Regs = []uint16{word >> 8}
			if opcode == 0x19 {
				instruction.Literal = int64(int16(word1())) << 48
			} else {
				instruction.Literal = int64(int32(uint32(word1()) << 16))
			}
		case "21c":
			instruction.Regs = []uint16{word >> 8}
			instruction.Index = uint32(word1())
		case "31c":
			instruction.Regs = []uint16{word >> 8}
			instruction.Index = uint32(word1()) | uint32(word2())<<16
		case "22c":
			instruction.Regs = []uint16{uint16((word >> 8) & 0xf), uint16(word >> 12)}
			instruction.Index = uint32(word1())
		case "23x":
			instruction.Regs = []uint16{word >> 8, uint16(byte(word1())), uint16(byte(word1() >> 8))}
		case "10t":
			instruction.Target = int64(offset) + int64(int8(word>>8))
		case "20t":
			if word>>8 != 0 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("reserved high byte is non-zero"))
			}
			instruction.Target = int64(offset) + int64(int16(word1()))
		case "30t":
			if word>>8 != 0 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("reserved high byte is non-zero"))
			}
			instruction.Target = int64(offset) + int64(int32(uint32(word1())|uint32(word2())<<16))
		case "21t":
			instruction.Regs = []uint16{word >> 8}
			instruction.Target = int64(offset) + int64(int16(word1()))
		case "22t":
			instruction.Regs = []uint16{uint16((word >> 8) & 0xf), uint16(word >> 12)}
			instruction.Target = int64(offset) + int64(int16(word1()))
		case "22s":
			instruction.Regs = []uint16{uint16((word >> 8) & 0xf), uint16(word >> 12)}
			instruction.Literal = int64(int16(word1()))
		case "22b":
			instruction.Regs = []uint16{word >> 8, uint16(byte(word1()))}
			instruction.Literal = int64(int8(word1() >> 8))
		case "35c":
			count := int(word >> 12)
			if count > 5 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("register count %d exceeds format limit", count))
			}
			instruction.Index = uint32(word1())
			packed := word2()
			registers := []uint16{uint16(packed & 0xf), uint16((packed >> 4) & 0xf), uint16((packed >> 8) & 0xf), uint16(packed >> 12), uint16((word >> 8) & 0xf)}
			instruction.Regs = append([]uint16(nil), registers[:count]...)
		case "3rc":
			count := uint32(word >> 8)
			first := uint32(word2())
			if first+count > math.MaxUint16+1 {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("register range v%d..v%d overflows v65535", first, first+count-1))
			}
			instruction.Index = uint32(word1())
			instruction.Regs = make([]uint16, count)
			for index := range instruction.Regs {
				instruction.Regs[index] = uint16(first + uint32(index))
			}
		case "31t":
			instruction.Regs = []uint16{word >> 8}
			instruction.Target = int64(offset) + int64(int32(uint32(word1())|uint32(word2())<<16))
			if opcode == 0x2b || opcode == 0x2c {
				targets, keys, firstKey, err := decodeSwitchPayload(words, instruction.Target, offset, opcode == 0x2c)
				if err != nil {
					return nil, diagnostic(ctx, &instruction, "", err)
				}
				instruction.Targets, instruction.Keys, instruction.FirstKey = targets, keys, firstKey
			}
			if opcode == 0x26 {
				data, width, err := decodeFillArrayPayload(words, instruction.Target)
				if err != nil {
					return nil, diagnostic(ctx, &instruction, "", err)
				}
				instruction.Data, instruction.DataWidth = data, width
			}
		case "45cc", "4rcc":
			// These are decoded only far enough to issue a precise unsupported
			// diagnostic; their operands are deliberately not exposed in this IR.
		default:
			return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("decoder has no implementation for format %s", info.format))
		}
		instructions = append(instructions, instruction)
		offset += uint32(info.width)
	}
	return instructions, nil
}
