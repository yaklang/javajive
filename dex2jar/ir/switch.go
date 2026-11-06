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
	if offset+1 >= uint32(len(words)) {
		instruction := Instruction{Offset: offset, Opcode: 0, Name: "payload"}
		return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("truncated payload header"))
	}
	switch words[offset] {
	case 0x0100:
		// packed-switch-payload: ident, size, first_key, size targets.
		return 4 + 2*uint32(words[offset+1]), nil
	case 0x0200:
		// sparse-switch-payload: ident, size, size (key, target) pairs.
		return 2 + 4*uint32(words[offset+1]), nil
	case 0x0300:
		// fill-array-data-payload: ident, element_width, size, data.
		if offset+3 >= uint32(len(words)) {
			instruction := Instruction{Offset: offset, Opcode: 0, Name: "fill-array-data-payload"}
			return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("truncated fill-array-data payload header"))
		}
		width := uint32(words[offset+1])
		if width != 1 && width != 2 && width != 4 && width != 8 {
			instruction := Instruction{Offset: offset, Opcode: 0, Name: "fill-array-data-payload"}
			return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("fill-array-data element width %d is not 1, 2, 4 or 8", width))
		}
		size := uint32(words[offset+2]) | uint32(words[offset+3])<<16
		total := 4 + (uint64(size)*uint64(width)+1)/2
		if total > uint64(len(words)) {
			instruction := Instruction{Offset: offset, Opcode: 0, Name: "fill-array-data-payload"}
			return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("fill-array-data payload is truncated"))
		}
		return uint32(total), nil
	default:
		instruction := Instruction{Offset: offset, Opcode: 0, Name: "payload"}
		return 0, diagnostic(ctx, &instruction, "payload", fmt.Errorf("unknown payload identifier %#04x", words[offset]))
	}
}

// decodeFillArrayPayload returns the raw element bytes of a fill-array-data
// payload. The width and count are kept in the payload header; the caller
// matches them against the array component type.
func decodeFillArrayPayload(words []uint16, base uint32) ([]byte, uint16, error) {
	if base+3 >= uint32(len(words)) || words[base] != 0x0300 {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d is missing or has the wrong identifier", base)
	}
	width := uint32(words[base+1])
	if width != 1 && width != 2 && width != 4 && width != 8 {
		return nil, 0, fmt.Errorf("fill-array-data element width %d is not 1, 2, 4 or 8", width)
	}
	size := uint32(words[base+2]) | uint32(words[base+3])<<16
	total := uint64(size) * uint64(width)
	units := (total + 1) / 2
	if uint64(base)+4+units > uint64(len(words)) {
		return nil, 0, fmt.Errorf("fill-array-data payload at %d is truncated", base)
	}
	data := make([]byte, total)
	for index := uint64(0); index < total; index++ {
		unit := words[base+4+uint32(index/2)]
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
func decodeSwitchPayload(words []uint16, base, switchOffset uint32, sparse bool) ([]int64, []int32, int32, error) {
	ident := uint16(0x0100)
	units := uint64(0)
	if sparse {
		ident = 0x0200
		units = 2
	}
	if base >= uint32(len(words)) || words[base] != ident {
		return nil, nil, 0, fmt.Errorf("switch payload at %d does not match the opcode", base)
	}
	size := uint32(words[base+1])
	if sparse {
		units += 4 * uint64(size)
	} else {
		units = 4 + 2*uint64(size)
	}
	if uint64(base)+units > uint64(len(words)) {
		return nil, nil, 0, fmt.Errorf("switch payload at %d is truncated", base)
	}
	read32 := func(at uint32) int32 {
		return int32(uint32(words[at]) | uint32(words[at+1])<<16)
	}
	if !sparse {
		first := read32(base + 2)
		if first > math.MaxInt32-int32(size)+1 {
			return nil, nil, 0, fmt.Errorf("packed switch key range starting at %d overflows", first)
		}
		targets := make([]int64, size)
		for index := uint32(0); index < size; index++ {
			targets[index] = int64(switchOffset) + int64(read32(base+4+2*index))
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
	pairs := make([]casePair, size)
	targetsBase := base + 2 + 2*size
	for index := uint32(0); index < size; index++ {
		pairs[index] = casePair{
			key:    read32(base + 2 + 2*index),
			target: int64(switchOffset) + int64(read32(targetsBase+2*index)),
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	keys := make([]int32, size)
	targets := make([]int64, size)
	for index, pair := range pairs {
		if index > 0 && pair.key == pairs[index-1].key {
			return nil, nil, 0, fmt.Errorf("sparse switch repeats key %d", pair.key)
		}
		keys[index], targets[index] = pair.key, pair.target
	}
	return targets, keys, 0, nil
}
