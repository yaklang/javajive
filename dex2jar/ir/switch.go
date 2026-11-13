package ir

import (
	"fmt"
	"math"
	"sort"
)

// payloadUnits returns how many code units the payload at offset occupies. The
// payload itself is data, so the decoder skips it and the switch instruction
// that references it decodes the contents.
func payloadUnits(words []uint16, offset uint32, ctx Context) (uint32, error) {
	remaining := uint64(len(words)) - uint64(offset)
	if remaining < 2 {
		instruction := Instruction{Offset: offset, Opcode: 0, Name: "payload"}
		return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("truncated payload header"))
	}
	var total uint64
	switch words[offset] {
	case 0x0100:
		// packed-switch-payload: ident, size, first_key, size targets.
		total = 4 + 2*uint64(words[offset+1])
	case 0x0200:
		// sparse-switch-payload: ident, size, size (key, target) pairs.
		total = 2 + 4*uint64(words[offset+1])
	case 0x0300:
		// fill-array-data-payload: ident, element_width, size, data.
		if remaining < 4 {
			instruction := Instruction{Offset: offset, Opcode: 0, Name: "fill-array-data-payload"}
			return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("truncated fill-array-data payload header"))
		}
		width := uint32(words[offset+1])
		if width != 1 && width != 2 && width != 4 && width != 8 {
			instruction := Instruction{Offset: offset, Opcode: 0, Name: "fill-array-data-payload"}
			return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("fill-array-data element width %d is not 1, 2, 4 or 8", width))
		}
		size := uint32(words[offset+2]) | uint32(words[offset+3])<<16
		total = 4 + (uint64(size)*uint64(width)+1)/2
	default:
		instruction := Instruction{Offset: offset, Opcode: 0, Name: "payload"}
		return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("unknown payload identifier %#04x", words[offset]))
	}
	if total > remaining {
		instruction := Instruction{Offset: offset, Opcode: 0, Name: "payload"}
		return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("payload at %d is truncated: need %d code units, have %d", offset, total, remaining))
	}
	return uint32(total), nil
}

// decodeFillArrayPayload returns the raw element bytes of a fill-array-data
// payload. The width and count are kept in the payload header; the caller
// matches them against the array component type.
func decodeFillArrayPayload(words []uint16, base int64) ([]byte, uint16, error) {
	if base < 0 || base >= int64(len(words)) {
		return nil, 0, fmt.Errorf("fill-array-data payload target %d is outside code units [0,%d)", base, len(words))
	}
	start := int(base)
	if words[start] != 0x0300 {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d has the wrong identifier", base)
	}
	if len(words)-start < 4 {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d has a truncated header", base)
	}
	width := uint32(words[start+1])
	if width != 1 && width != 2 && width != 4 && width != 8 {
		return nil, 0, fmt.Errorf("fill-array-data element width %d is not 1, 2, 4 or 8", width)
	}
	size := uint32(words[start+2]) | uint32(words[start+3])<<16
	total := uint64(size) * uint64(width)
	units := (total + 1) / 2
	if 4+units > uint64(len(words)-start) {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d is truncated", base)
	}
	maxInt := uint64(^uint(0) >> 1)
	if total > maxInt {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d has %d bytes, which exceeds the addressable allocation size", base, total)
	}
	data := make([]byte, int(total))
	for index := uint64(0); index < total; index++ {
		unit := words[start+4+int(index/2)]
		if index%2 == 0 {
			data[index] = byte(unit)
		} else {
			data[index] = byte(unit >> 8)
		}
	}
	return data, uint16(width), nil
}

// fillArrayElementWidth returns the byte width of a primitive array component
// and rejects components that fill-array-data cannot encode.
func fillArrayElementWidth(component string) (int, error) {
	switch component {
	case "Z", "B":
		return 1, nil
	case "C", "S":
		return 2, nil
	case "I", "F":
		return 4, nil
	case "J", "D":
		return 8, nil
	default:
		return 0, fmt.Errorf("fill-array-data requires a primitive array, not %q", component)
	}
}

// decodeSwitchPayload resolves the case table a packed or sparse switch
// references. Case addresses are relative to the switch instruction itself.
func decodeSwitchPayload(words []uint16, base int64, switchOffset uint32, sparse bool) ([]int64, []int32, int32, error) {
	ident := uint16(0x0100)
	if sparse {
		ident = 0x0200
	}
	if base < 0 || base >= int64(len(words)) {
		return nil, nil, 0, fmt.Errorf("switch payload target %d is outside code units [0,%d)", base, len(words))
	}
	start := int(base)
	if words[start] != ident {
		return nil, nil, 0, fmt.Errorf("switch payload at %d does not match the opcode", base)
	}
	if len(words)-start < 2 {
		return nil, nil, 0, fmt.Errorf("switch payload at %d has a truncated header", base)
	}
	size := uint64(words[start+1])
	units := uint64(4) + 2*size
	if sparse {
		units = 2 + 4*size
	}
	if units > uint64(len(words)-start) {
		return nil, nil, 0, fmt.Errorf("switch payload at %d is truncated", base)
	}
	read32 := func(at int) int32 {
		return int32(uint32(words[at]) | uint32(words[at+1])<<16)
	}
	if !sparse {
		first := read32(start + 2)
		if size > 0 && int64(first)+int64(size)-1 > math.MaxInt32 {
			return nil, nil, 0, fmt.Errorf("packed switch key range starting at %d overflows", first)
		}
		targets := make([]int64, int(size))
		for index := 0; index < int(size); index++ {
			targets[index] = int64(switchOffset) + int64(read32(start+4+2*index))
		}
		return targets, nil, first, nil
	}
	// A sparse payload stores all keys first and then all targets. The class
	// format requires sorted lookup pairs, so sort them here instead of
	// rejecting an input whose keys are not ordered.
	type casePair struct {
		key    int32
		target int64
	}
	pairs := make([]casePair, int(size))
	targetsBase := start + 2 + 2*int(size)
	for index := 0; index < int(size); index++ {
		pairs[index] = casePair{
			key:    read32(start + 2 + 2*index),
			target: int64(switchOffset) + int64(read32(targetsBase+2*index)),
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	keys := make([]int32, int(size))
	targets := make([]int64, int(size))
	for index, pair := range pairs {
		if index > 0 && pair.key == pairs[index-1].key {
			return nil, nil, 0, fmt.Errorf("sparse switch repeats key %d", pair.key)
		}
		keys[index], targets[index] = pair.key, pair.target
	}
	return targets, keys, 0, nil
}
