package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/javajive/internal/mutf8"
)

/*
*

	CONSTANT_UTF8_INFO {
		u1 tag;
		u2 Length;
		u1 bytes[Length];
	}
*/
// Dual-source policy:
//
//   Units is the lossless UTF-16 payload produced by parse or SetUnits.
//   Value is the compatibility string used by legacy editors (SetClassName,
//   ClassObjectBuilder.SetValue, AddUtf8Info). It is also a display form.
//   Raw is the original on-disk MUTF-8 from parse, forensics only.
//
// Serialization uses semanticMUTF8():
//   - if Value equals unitsToDisplay(Units), Units is the source of truth
//     (preserves unpaired surrogates that Value cannot hold as Go runes);
//   - if Value has been edited away from that display, Value is encoded
//     (legacy rename/edit path);
//   - if Units is nil, Value is encoded.
type ConstantUtf8Info struct {
	Type      string
	Value     string
	Units     []uint16
	Raw       []byte
	DecodeErr error
}

func (self *ConstantUtf8Info) readInfo(cp *ClassParser) {
	length := uint32(cp.reader.readUint16())
	raw := cp.reader.readBytes(length)
	if cp.reader != nil && cp.reader.Err() != nil {
		return
	}
	self.Raw = bytes.Clone(raw)
	units, err := mutf8.Decode(self.Raw)
	if err != nil {
		// Sticky parse errors must not overwrite lossless units with error text.
		self.DecodeErr = err
		self.Units = nil
		self.Value = ""
		if cp.reader != nil {
			cp.reader.fail(ParseCodeInvalidInput, err.Error())
			return
		}
		panic(err)
	}
	self.Units = units
	self.Value = unitsToDisplay(units)
}

// CodeUnits returns a copy of the lossless UTF-16 units, or nil if decode failed.
func (u *ConstantUtf8Info) CodeUnits() []uint16 {
	if u == nil || u.Units == nil {
		return nil
	}
	out := make([]uint16, len(u.Units))
	copy(out, u.Units)
	return out
}

// SetString is the compatibility write path for Value edits. It refreshes Units
// from UTF-16 of s so Bytes()/reparse observe the new string. Lone surrogates
// cannot be expressed this way; use SetUnits.
func (u *ConstantUtf8Info) SetString(s string) {
	if u == nil {
		return
	}
	u.Value = s
	u.Units = utf16.Encode([]rune(s))
	u.Raw = mutf8.Encode(u.Units)
	u.DecodeErr = nil
}

// SetUnits is the lossless write path. Value is updated to the display form
// only; serialization still uses Units.
func (u *ConstantUtf8Info) SetUnits(units []uint16) {
	if u == nil {
		return
	}
	u.Units = append([]uint16(nil), units...)
	u.Raw = mutf8.Encode(u.Units)
	u.Value = unitsToDisplay(u.Units)
	u.DecodeErr = nil
}

// NewUtf8FromString constructs a CONSTANT_Utf8 for AddUtf8Info and tests.
func NewUtf8FromString(s string) *ConstantUtf8Info {
	u := &ConstantUtf8Info{}
	u.SetString(s)
	return u
}

func (u *ConstantUtf8Info) semanticUnits() []uint16 {
	if u == nil {
		return nil
	}
	if u.Units != nil && u.Value == unitsToDisplay(u.Units) {
		return u.Units
	}
	return utf16.Encode([]rune(u.Value))
}

func (u *ConstantUtf8Info) mutf8Bytes() []byte {
	if u == nil {
		return nil
	}
	return mutf8.Encode(u.semanticUnits())
}

func (u *ConstantUtf8Info) mutf8BytesChecked() ([]byte, error) {
	payload := u.mutf8Bytes()
	if len(payload) > 65535 {
		return nil, fmt.Errorf("CONSTANT_Utf8 payload length %d exceeds u2 (65535)", len(payload))
	}
	return payload, nil
}

func unitsToDisplay(units []uint16) string {
	var b strings.Builder
	b.Grow(len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case isHighSurrogate(u) && i+1 < len(units) && isLowSurrogate(units[i+1]):
			b.WriteRune(utf16.DecodeRune(rune(u), rune(units[i+1])))
			i++
		case isHighSurrogate(u) || isLowSurrogate(u):
			fmt.Fprintf(&b, "\\u%04X", u)
		default:
			b.WriteRune(rune(u))
		}
	}
	return b.String()
}

func isHighSurrogate(u uint16) bool { return u >= 0xD800 && u <= 0xDBFF }
func isLowSurrogate(u uint16) bool  { return u >= 0xDC00 && u <= 0xDFFF }
