package values

import (
	"fmt"
	"math"
	"unicode/utf16"
	"unicode/utf8"
)

// Length helpers compatible with the T03 canonical printer
// (classparser/decompiler/core/values/java_literal_print.go on main):
// JavaUnitsToStringLiteral / JavaUnitToCharLiteral. This file does not
// reimplement that printer; it only counts the bytes that printer emits so
// MaxOutputBytes can be charged without undercounting escapes or Units.

// JavaUnitsStringLiteralCap is the allocation cap used by the T03 printer
// (2 + 6*nUnits). Use this with Budget.CheckAlloc before make([]byte, 0, cap).
func JavaUnitsStringLiteralCap(nUnits int) int {
	if nUnits <= 0 {
		return 2
	}
	if nUnits > (math.MaxInt-2)/6 {
		return math.MaxInt
	}
	return 2 + nUnits*6
}

// JavaUnitsStringLiteralLen is the exact UTF-8 byte length of
// JavaUnitsToStringLiteral(units), including the surrounding quotes.
func JavaUnitsStringLiteralLen(units []uint16) int {
	n := 2
	for i := 0; i < len(units); i++ {
		unit := units[i]
		if unit == '\\' {
			n += 2
			if i+1 < len(units) && units[i+1] == 'u' && sizeUnicodeEscapeTail(units, i+1) {
				n += 6 // \u0075
				i++
			}
			continue
		}
		add := javaUnitEmittedLen(unit, '"')
		if add < 1 {
			add = 6
		}
		if n > math.MaxInt-add {
			return math.MaxInt
		}
		n += add
	}
	return n
}

// JavaUnitCharLiteralLen is the exact UTF-8 byte length of JavaUnitToCharLiteral(unit).
func JavaUnitCharLiteralLen(unit uint16) int {
	add := javaUnitEmittedLen(unit, '\'')
	if add < 1 {
		add = 6
	}
	if add > math.MaxInt-2 {
		return math.MaxInt
	}
	return 2 + add
}

// JavaStringLiteralOutputBytes is the T03 emitted length of a STRING literal.
// Units, when non-nil, win (including empty). []uint16 Data is treated as units.
// A Go string is converted with utf16.Encode([]rune(s)) (same as the compatibility
// printer path). This is not len(Data)+2: control chars, quotes, backslashes,
// surrogates, and C1 controls expand.
func JavaStringLiteralOutputBytes(data any, units []uint16) int {
	if units != nil {
		return JavaUnitsStringLiteralLen(units)
	}
	switch v := data.(type) {
	case []uint16:
		return JavaUnitsStringLiteralLen(v)
	case string:
		return JavaUnitsStringLiteralLen(utf16.Encode([]rune(v)))
	default:
		return JavaUnitsStringLiteralLen(utf16.Encode([]rune(fmt.Sprint(v))))
	}
}

func literalUnitCount(data any, units []uint16) int {
	if units != nil {
		return len(units)
	}
	switch v := data.(type) {
	case []uint16:
		return len(v)
	case string:
		return len(utf16.Encode([]rune(v)))
	default:
		return len(utf16.Encode([]rune(fmt.Sprint(v))))
	}
}

func javaUnitEmittedLen(unit uint16, quote byte) int {
	switch unit {
	case uint16(quote), '\\', '\n', '\r', '\t', '\b', '\f':
		return 2
	}
	if unit < 0x20 || unit == 0x7F || (unit >= 0x80 && unit <= 0x9F) {
		return 4
	}
	if unit >= 0xD800 && unit <= 0xDFFF {
		return 6
	}
	n := utf8.RuneLen(rune(unit))
	if n < 1 {
		return 6
	}
	return n
}

func sizeUnicodeEscapeTail(units []uint16, i int) bool {
	if i >= len(units) || units[i] != 'u' {
		return false
	}
	j := i + 1
	for j < len(units) && units[j] == 'u' {
		j++
	}
	if len(units)-j < 4 {
		return false
	}
	for k := 0; k < 4; k++ {
		if !sizeIsJavaHexDigit(units[j+k]) {
			return false
		}
	}
	return true
}

func sizeIsJavaHexDigit(u uint16) bool {
	return u >= '0' && u <= '9' || u >= 'a' && u <= 'f' || u >= 'A' && u <= 'F'
}
