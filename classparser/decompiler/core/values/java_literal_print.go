package values

import (
	"fmt"
	"math"
	"unicode/utf16"
	"unicode/utf8"
)

const hexDigits = "0123456789ABCDEF"

// JavaUnitsToStringLiteral prints a Java STRING literal including the surrounding double quotes.
// Each UTF-16 unit is preserved; CR/LF/quote/backslash are never emitted as \u000A / \u000D /
// \u0022 / \u0027 / \u005C (those Unicode escapes are expanded before lexical analysis).
func printerCap(nUnits int) int {
	if nUnits <= 0 {
		return 2
	}
	if nUnits > (math.MaxInt-2)/6 {
		return math.MaxInt
	}
	return 2 + nUnits*6
}

func JavaUnitsToStringLiteral(units []uint16) string {
	buf := make([]byte, 0, printerCap(len(units)))
	buf = append(buf, '"')
	for i := 0; i < len(units); i++ {
		unit := units[i]
		if unit == '\\' {
			buf = append(buf, '\\', '\\')
			// A following "u" plus hex digits would be a Unicode escape after a JLS-naive
			// translator sees the last source backslash. Break it with \u0075 (the produced
			// 'u' does not start another Unicode escape).
			if i+1 < len(units) && units[i+1] == 'u' && unicodeEscapeTail(units, i+1) {
				buf = appendUnicodeEscape(buf, 'u')
				i++
			}
			continue
		}
		buf = appendJavaUnit(buf, unit, '"')
	}
	buf = append(buf, '"')
	return string(buf)
}

// JavaUnitToCharLiteral prints a Java CHAR literal including the surrounding single quotes.
// Always exactly one char literal, never a String.
func JavaUnitToCharLiteral(unit uint16) string {
	buf := make([]byte, 0, 8)
	buf = append(buf, '\'')
	buf = appendJavaUnit(buf, unit, '\'')
	buf = append(buf, '\'')
	return string(buf)
}

// JavaStringToLiteral remains for compatibility: if i is []uint16, print those units;
// if i is string, convert via utf16.Encode([]rune(s)) which loses unpaired surrogates — that
// path is compatibility only. Prefer units.
func JavaStringToLiteral(i any) string {
	switch v := i.(type) {
	case []uint16:
		return JavaUnitsToStringLiteral(v)
	case string:
		return JavaUnitsToStringLiteral(utf16.Encode([]rune(v)))
	default:
		return JavaUnitsToStringLiteral(utf16.Encode([]rune(fmt.Sprint(v))))
	}
}

func appendJavaUnit(buf []byte, unit uint16, quote byte) []byte {
	switch unit {
	case uint16(quote):
		return append(buf, '\\', quote)
	case '\\':
		return append(buf, '\\', '\\')
	case '\n':
		return append(buf, '\\', 'n')
	case '\r':
		return append(buf, '\\', 'r')
	case '\t':
		return append(buf, '\\', 't')
	case '\b':
		return append(buf, '\\', 'b')
	case '\f':
		return append(buf, '\\', 'f')
	}
	if unit < 0x20 || unit == 0x7F || (unit >= 0x80 && unit <= 0x9F) {
		return appendOctal3(buf, byte(unit))
	}
	if unit >= 0xD800 && unit <= 0xDFFF {
		return appendUnicodeEscape(buf, unit)
	}
	return utf8.AppendRune(buf, rune(unit))
}

func appendOctal3(buf []byte, b byte) []byte {
	return append(buf, '\\', '0'+(b>>6), '0'+((b>>3)&7), '0'+(b&7))
}

func appendUnicodeEscape(buf []byte, unit uint16) []byte {
	return append(buf, '\\', 'u',
		hexDigits[unit>>12],
		hexDigits[(unit>>8)&0xF],
		hexDigits[(unit>>4)&0xF],
		hexDigits[unit&0xF])
}

func unicodeEscapeTail(units []uint16, i int) bool {
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
		if !isJavaHexDigit(units[j+k]) {
			return false
		}
	}
	return true
}

func isJavaHexDigit(u uint16) bool {
	return u >= '0' && u <= '9' || u >= 'a' && u <= 'f' || u >= 'A' && u <= 'F'
}

func javaLiteralCharUnit(j *JavaLiteral) (uint16, bool) {
	if j == nil {
		return 0, false
	}
	if len(j.Units) > 0 {
		return j.Units[0], true
	}
	return charUnitFromData(j.Data)
}

func charUnitFromData(data any) (uint16, bool) {
	switch v := data.(type) {
	case uint16:
		return v, true
	case uint8:
		return uint16(v), true
	case uint32:
		return uint16(v), true
	case uint64:
		return uint16(v), true
	case int:
		return uint16(v), true
	case int8:
		return uint16(v), true
	case int16:
		return uint16(v), true
	case int32:
		return uint16(v), true
	case int64:
		return uint16(v), true
	case string:
		if v == "" {
			return 0, false
		}
		enc := utf16.Encode([]rune(v))
		if len(enc) == 0 {
			return 0, false
		}
		return enc[0], true
	default:
		return 0, false
	}
}
