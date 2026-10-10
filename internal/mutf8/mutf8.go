// Package mutf8 implements JVM Modified UTF-8 as lossless UTF-16 code units.
package mutf8

import (
	"fmt"
	"math"
)

// DecodeError is a MUTF-8 structural failure at Offset in the raw input.
type DecodeError struct {
	Offset int    // byte offset in the raw MUTF-8 input
	Reason string // e.g. "nul_byte", "truncated", "bad_continuation", "overlong", "four_byte"
}

func (e *DecodeError) Error() string {
	if e == nil {
		return "mutf8: <nil>"
	}
	return fmt.Sprintf("mutf8: offset %d: %s", e.Offset, e.Reason)
}

func decodeErr(offset int, reason string) error {
	return &DecodeError{Offset: offset, Reason: reason}
}

func isContinuation(b byte) bool { return b&0xC0 == 0x80 }

// Decode converts MUTF-8 bytes to UTF-16 code units (not Go runes).
func Decode(raw []byte) ([]uint16, error) {
	return DecodeBounded(raw, -1)
}

// DecodeBounded is Decode that refuses before allocating more than maxUnits
// code units. maxUnits < 0 means unlimited. Exceeding the budget returns a
// "budget" error and no truncated success slice.
func DecodeBounded(raw []byte, maxUnits int) ([]uint16, error) {
	capHint := len(raw)
	if maxUnits >= 0 && capHint > maxUnits {
		capHint = maxUnits
	}
	out := make([]uint16, 0, capHint)
	appendUnit := func(start int, u uint16) error {
		if maxUnits >= 0 && len(out) >= maxUnits {
			return decodeErr(start, "budget")
		}
		out = append(out, u)
		return nil
	}

	for i := 0; i < len(raw); {
		start := i
		b := raw[i]
		switch {
		case b == 0x00:
			return nil, decodeErr(start, "nul_byte")
		case b < 0x80:
			if err := appendUnit(start, uint16(b)); err != nil {
				return nil, err
			}
			i++
		case b < 0xC0:
			return nil, decodeErr(start, "bad_continuation")
		case b < 0xE0:
			if i+1 >= len(raw) {
				return nil, decodeErr(start, "truncated")
			}
			c := raw[i+1]
			if !isContinuation(c) {
				return nil, decodeErr(start+1, "bad_continuation")
			}
			if b == 0xC0 && c == 0x80 {
				if err := appendUnit(start, 0); err != nil {
					return nil, err
				}
				i += 2
				continue
			}
			if b < 0xC2 {
				return nil, decodeErr(start, "overlong")
			}
			unit := uint16(b&0x1F)<<6 | uint16(c&0x3F)
			if err := appendUnit(start, unit); err != nil {
				return nil, err
			}
			i += 2
		case b < 0xF0:
			if i+2 >= len(raw) {
				return nil, decodeErr(start, "truncated")
			}
			c1 := raw[i+1]
			c2 := raw[i+2]
			if !isContinuation(c1) {
				return nil, decodeErr(start+1, "bad_continuation")
			}
			if !isContinuation(c2) {
				return nil, decodeErr(start+2, "bad_continuation")
			}
			unit := uint16(b&0x0F)<<12 | uint16(c1&0x3F)<<6 | uint16(c2&0x3F)
			if unit < 0x800 {
				return nil, decodeErr(start, "overlong")
			}
			if err := appendUnit(start, unit); err != nil {
				return nil, err
			}
			i += 3
		case b < 0xF8:
			return nil, decodeErr(start, "four_byte")
		default:
			return nil, decodeErr(start, "invalid_lead")
		}
	}
	return out, nil
}

// CheckedAdd returns a+b or false on overflow / negative inputs.
func CheckedAdd(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	if a > math.MaxInt-b {
		return 0, false
	}
	return a + b, true
}

// EncodeCap is a safe 3-bytes-per-unit capacity. Overflow saturates at MaxInt
// rather than wrapping to a tiny allocation.
func EncodeCap(nUnits int) int {
	if nUnits <= 0 {
		return 0
	}
	if nUnits > math.MaxInt/3 {
		return math.MaxInt
	}
	return nUnits * 3
}

// Encode is the inverse of Decode for all 65536 BMP/surrogate units.
func Encode(units []uint16) []byte {
	out := make([]byte, 0, EncodeCap(len(units)))
	for _, u := range units {
		switch {
		case u == 0:
			out = append(out, 0xC0, 0x80)
		case u < 0x80:
			out = append(out, byte(u))
		case u < 0x800:
			out = append(out, 0xC0|byte(u>>6), 0x80|byte(u&0x3F))
		default:
			out = append(out, 0xE0|byte(u>>12), 0x80|byte((u>>6)&0x3F), 0x80|byte(u&0x3F))
		}
	}
	return out
}

// Concat returns a new flattened copy of parts.
func Concat(parts ...[]uint16) []uint16 {
	out, _ := ConcatBounded(-1, parts...)
	return out
}

// ConcatBounded concatenates parts, refusing if the result would exceed maxUnits.
// maxUnits < 0 means unlimited. On budget failure no prefix is returned.
func ConcatBounded(maxUnits int, parts ...[]uint16) ([]uint16, error) {
	n := 0
	for _, p := range parts {
		sum, ok := CheckedAdd(n, len(p))
		if !ok {
			return nil, decodeErr(0, "budget")
		}
		n = sum
		if maxUnits >= 0 && n > maxUnits {
			return nil, decodeErr(0, "budget")
		}
	}
	out := make([]uint16, n)
	off := 0
	for _, p := range parts {
		copy(out[off:], p)
		off += len(p)
	}
	return out, nil
}
