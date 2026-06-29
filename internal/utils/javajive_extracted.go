package utils

// This file collects the small set of helper functions that the ported Java
// tooling needs from the original (very large) yaklang utils package. They are
// reproduced here verbatim or with minimal, behavior-preserving simplification so
// that javajive stays self-contained and dependency-light.

import (
	"bufio"
	"bytes"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/javajive/internal/log"
)

// Character classes used by the random-string helpers.
const (
	LittleChar = "abcdefghijklmnopqrstuvwxyz"
	BigChar    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	NumberChar = "1234567890"
	LetterChar = LittleChar + BigChar
)

// RandStringBytes returns a random alphabetic string of length n.
func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = LetterChar[rand.Intn(len(LetterChar))]
	}
	return string(b)
}

// InterfaceToString converts an arbitrary value to its string form, preferring a
// Stringer implementation when present.
func InterfaceToString(i interface{}) string {
	if a, ok := i.(interface{ String() string }); ok {
		return a.String()
	}
	switch v := i.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(i)
	}
}

// StringSliceContain reports whether raw is contained in the string collection s.
// Only the slice forms actually used by the ported code are supported.
func StringSliceContain(s interface{}, raw string) bool {
	switch ret := s.(type) {
	case []string:
		for _, i := range ret {
			if i == raw {
				return true
			}
		}
	case []interface{}:
		for _, i := range ret {
			if InterfaceToString(i) == raw {
				return true
			}
		}
	}
	return false
}

// StringArrayFilterEmpty returns a copy of array with blank entries removed
// (each remaining entry trimmed of surrounding whitespace).
func StringArrayFilterEmpty(array []string) []string {
	var ret []string
	for _, a := range array {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		ret = append(ret, a)
	}
	return ret
}

// CopyMapShallow returns a shallow copy of the given map.
func CopyMapShallow[K comparable, V any](originalMap map[K]V) map[K]V {
	newMap := make(map[K]V, len(originalMap))
	for k, v := range originalMap {
		newMap[k] = v
	}
	return newMap
}

func removeBOMForString(s string) string {
	return strings.TrimPrefix(s, "\xef\xbb\xbf")
}

// ParseStringToLines splits raw into trimmed, non-empty lines (BOM stripped).
func ParseStringToLines(raw string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 1024*64), 16*1024*1024)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, removeBOMForString(line))
		}
	}
	return lines
}

// EscapeInvalidUTF8Byte renders s, escaping invalid UTF-8 bytes and non-space
// control characters as "\xHH". The result is NOT byte-equivalent to the input.
func EscapeInvalidUTF8Byte(s []byte) string {
	var builder strings.Builder
	builder.Grow(len(s) + 20)
	start := 0
	for {
		r, size := utf8.DecodeRune(s[start:])
		if r == utf8.RuneError {
			if size == 0 {
				break
			}
			builder.WriteString("\\x")
			builder.WriteString(hex.EncodeToString([]byte{s[start]}))
		} else {
			if unicode.IsControl(r) && !unicode.IsSpace(r) {
				builder.WriteString("\\x")
				builder.WriteString(hex.EncodeToString([]byte{byte(r)}))
			} else {
				builder.WriteRune(r)
			}
		}
		start += size
	}
	return builder.String()
}

// SimplifyUtf8 normalizes (re-encodes) utf8 bytes.
func SimplifyUtf8(raw []byte) ([]byte, error) {
	res, err := utf8ToUnicode(raw)
	if err != nil {
		return nil, err
	}
	return unicodeToUtf8(res), nil
}

func utf8ToUnicode(raw []byte) ([]uint32, error) {
	reader := bytes.NewReader(raw)
	var res []uint32
	addBinaryBits := func(res *uint32, b byte, l byte) {
		mask := uint32(1<<l - 1)
		*res = *res<<l | uint32(b)&mask
	}
	for i := 0; i < len(raw); {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		switch {
		case b>>7 == 0b0:
			res = append(res, uint32(b))
			i += 1
		case b>>5 == 0b110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 5)
			addBinaryBits(&ch, b1, 6)
			res = append(res, ch)
			i += 2
		case b>>4 == 0b1110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 4)
			addBinaryBits(&ch, b1, 6)
			addBinaryBits(&ch, b2, 6)
			res = append(res, ch)
			i += 3
		case b>>3 == 0b11110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b3, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 4)
			addBinaryBits(&ch, b1, 6)
			addBinaryBits(&ch, b2, 6)
			addBinaryBits(&ch, b3, 6)
			res = append(res, ch)
			i += 4
		default:
			return nil, stderrors.New("utf data format is invalid")
		}
	}
	return res, nil
}

func unicodeToUtf8(str []uint32) []byte {
	var res []byte
	for _, ch := range str {
		if ch < 0x80 {
			res = append(res, byte(ch))
		} else if ch < 0x800 {
			res = append(res, byte(0xc0|ch>>6), byte(0x80|ch&0x3f))
		} else if ch < 0x10000 {
			res = append(res, byte(0xe0|ch>>12), byte(0x80|ch>>6&0x3f), byte(0x80|ch&0x3f))
		} else {
			res = append(res, byte(0xf0|ch>>18), byte(0x80|ch>>12&0x3f), byte(0x80|ch>>6&0x3f), byte(0x80|ch&0x3f))
		}
	}
	return res
}

// Utf8EncodeBySpecificLength force-encodes unicode bytes to utf8 by a specific
// per-character encode length.
func Utf8EncodeBySpecificLength(str []byte, l int) []byte {
	unicodeList, err := utf8ToUnicode(str)
	if err != nil {
		log.Errorf("utf8ToUnicode failed: %s", err)
		return str
	}
	if l == 0 {
		return str
	}
	getChByteLength := func(ch uint32) int {
		if ch < 0x80 {
			return 1
		} else if ch < 0x800 {
			return 2
		} else if ch < 0x10000 {
			return 3
		} else {
			return 4
		}
	}
	encodeBySpecificLength := func(ch uint32, l int) []byte {
		switch l {
		case 1:
			return []byte{byte(ch)}
		case 2:
			buf := bytes.Buffer{}
			buf.WriteByte(0xc0 | byte(ch>>6))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		case 3:
			buf := bytes.Buffer{}
			buf.WriteByte(0xe0 | byte(ch>>12))
			buf.WriteByte(0x80 | byte((ch>>6)&0x3f))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		case 4:
			buf := bytes.Buffer{}
			buf.WriteByte(0xf0 | byte(ch>>18))
			buf.WriteByte(0x80 | byte((ch>>12)&0x3f))
			buf.WriteByte(0x80 | byte((ch>>6)&0x3f))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		default:
			return str
		}
	}
	var res []byte
	for _, u := range unicodeList {
		var maxL = getChByteLength(u)
		for maxL < l {
			maxL = l
		}
		res = append(res, encodeBySpecificLength(u, maxL)...)
	}
	return res
}

// GetLastElement returns the last element of list, or the zero value if empty.
func GetLastElement[T any](list []T) T {
	l := len(list)
	if l == 0 {
		var zero T
		return zero
	}
	return list[l-1]
}

// PathExists reports whether the given path exists.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// IsDir reports whether path exists and is a directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// IsFile reports whether path exists and is a regular file.
func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// GetFirstExistedFileE returns the first path that exists and is a file.
func GetFirstExistedFileE(paths ...string) (string, error) {
	for _, t := range paths {
		r, err := PathExists(t)
		if err != nil {
			continue
		}
		if IsDir(t) {
			continue
		}
		if !r {
			continue
		}
		return t, nil
	}
	return "", Errorf("any path is not existed")
}

// GetFirstExistedFile returns the first path that exists and is a file, or "".
func GetFirstExistedFile(paths ...string) string {
	res, _ := GetFirstExistedFileE(paths...)
	return res
}

// GetFirstExistedPathE returns the first path that exists (file or dir).
func GetFirstExistedPathE(paths ...string) (string, error) {
	for _, t := range paths {
		r, err := PathExists(t)
		if err != nil {
			continue
		}
		if !r {
			continue
		}
		return t, nil
	}
	return "", Errorf("any path is not existed")
}

// GetFirstExistedPath returns the first path that exists (file or dir), or "".
func GetFirstExistedPath(paths ...string) string {
	r, _ := GetFirstExistedPathE(paths...)
	return r
}
