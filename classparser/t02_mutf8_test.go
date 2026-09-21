package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/yaklang/javajive/internal/mutf8"
)

// Independent reference encoder (test-only). Do not use production Encode as the oracle.
func refEncodeUnit(u uint16) []byte {
	if u == 0 {
		return []byte{0xC0, 0x80}
	}
	if u < 0x80 {
		return []byte{byte(u)}
	}
	if u < 0x800 {
		return []byte{0xC0 | byte(u>>6), 0x80 | byte(u&0x3F)}
	}
	return []byte{0xE0 | byte(u>>12), 0x80 | byte((u>>6)&0x3F), 0x80 | byte(u&0x3F)}
}

func refEncodeUnits(units []uint16) []byte {
	var out []byte
	for _, u := range units {
		out = append(out, refEncodeUnit(u)...)
	}
	return out
}

func utf8InfoFromPayload(payload []byte) (info *ConstantUtf8Info, panicked error) {
	buf := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(buf, uint16(len(payload)))
	copy(buf[2:], payload)
	info = &ConstantUtf8Info{}
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				panicked = e
			} else {
				panicked = errors.New("panic")
			}
		}
	}()
	info.readInfo(NewClassParser(buf))
	return info, panicked
}

func constantUtf8ASCII(s string) []byte {
	b := []byte(s)
	out := []byte{CONSTANT_Utf8, byte(len(b) >> 8), byte(len(b))}
	return append(out, b...)
}

func constantUtf8Raw(payload []byte) []byte {
	out := []byte{CONSTANT_Utf8, byte(len(payload) >> 8), byte(len(payload))}
	return append(out, payload...)
}

func constantClass(idx uint16) []byte {
	return []byte{CONSTANT_Class, byte(idx >> 8), byte(idx)}
}

func buildClassWithUtf8Payload(payload []byte) []byte {
	var cp []byte
	cp = append(cp, constantUtf8ASCII("java/lang/Object")...)
	cp = append(cp, constantClass(1)...)
	cp = append(cp, constantUtf8ASCII("T02")...)
	cp = append(cp, constantClass(3)...)
	cp = append(cp, constantUtf8Raw(payload)...)
	cpCount := 6

	var b []byte
	b = append(b, 0xCA, 0xFE, 0xBA, 0xBE)
	b = append(b, 0x00, 0x00)
	b = append(b, 0x00, 0x31)
	b = append(b, byte(cpCount>>8), byte(cpCount))
	b = append(b, cp...)
	b = append(b, 0x00, 0x21)
	b = append(b, 0x00, 0x04)
	b = append(b, 0x00, 0x02)
	b = append(b, 0x00, 0x00)
	b = append(b, 0x00, 0x00)
	b = append(b, 0x00, 0x00)
	b = append(b, 0x00, 0x00)
	return b
}

func findPayloadUtf8(t *testing.T, obj *ClassObject, payload []byte) *ConstantUtf8Info {
	t.Helper()
	for _, c := range obj.ConstantPool {
		u, ok := c.(*ConstantUtf8Info)
		if !ok {
			continue
		}
		if bytes.Equal(u.Raw, payload) {
			return u
		}
	}
	t.Fatalf("no CONSTANT_Utf8 with payload %x", payload)
	return nil
}

func assertNoErrorText(t *testing.T, value string) {
	t.Helper()
	low := strings.ToLower(value)
	if strings.Contains(low, "parse utf8") || strings.Contains(low, "error") {
		t.Fatalf("Value looks like error text: %q", value)
	}
}

func TestTaskT02C01_ClassUtf8SurrogatePair(t *testing.T) {
	t.Run("T02-C01", func(t *testing.T) {
		want := []uint16{65, 55357, 56832, 90}
		payload := refEncodeUnits(want)
		if bytes.Contains(payload, []byte{0xF0}) {
			t.Fatalf("payload used UTF-8 4-byte form: %x", payload)
		}

		info, err := utf8InfoFromPayload(payload)
		if err != nil {
			t.Fatalf("readInfo: %v", err)
		}
		if !reflect.DeepEqual(info.Units, want) {
			t.Fatalf("Units = %v want %v", info.Units, want)
		}
		assertNoErrorText(t, info.Value)
		if strings.ContainsRune(info.Value, '\uFFFD') && !containsUnit(want, 0xFFFD) {
			t.Fatalf("Value contains U+FFFD: %q", info.Value)
		}

		obj, err := Parse(buildClassWithUtf8Payload(payload))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		u := findPayloadUtf8(t, obj, payload)
		if !reflect.DeepEqual(u.Units, want) {
			t.Fatalf("Parse Units = %v want %v", u.Units, want)
		}
		gotCopy := u.CodeUnits()
		if !reflect.DeepEqual(gotCopy, want) {
			t.Fatalf("CodeUnits = %v", gotCopy)
		}
		if len(gotCopy) > 0 {
			gotCopy[0] = 0
			if u.Units[0] == 0 {
				t.Fatalf("CodeUnits must return a copy")
			}
		}

		rewritten := obj.Bytes()
		obj2, err := Parse(rewritten)
		if err != nil {
			t.Fatalf("re-Parse marshaled class: %v", err)
		}
		u2 := findPayloadUtf8(t, obj2, payload)
		if !reflect.DeepEqual(u2.Units, want) {
			t.Fatalf("marshaled Units = %v want %v", u2.Units, want)
		}
	})
}

func TestTaskT02C02_ClassUtf8LoneAndNul(t *testing.T) {
	t.Run("T02-C02", func(t *testing.T) {
		lonePayload := refEncodeUnit(0xD800)
		info, err := utf8InfoFromPayload(lonePayload)
		if err != nil {
			t.Fatalf("lone D800 readInfo: %v", err)
		}
		if !reflect.DeepEqual(info.Units, []uint16{0xD800}) {
			t.Fatalf("lone D800 Units = %v", info.Units)
		}
		if strings.ContainsRune(info.Value, '\uFFFD') {
			t.Fatalf("lone D800 Value used replacement: %q", info.Value)
		}
		assertNoErrorText(t, info.Value)

		nulPayload := []byte{0xC0, 0x80}
		info, err = utf8InfoFromPayload(nulPayload)
		if err != nil {
			t.Fatalf("NUL readInfo: %v", err)
		}
		if !reflect.DeepEqual(info.Units, []uint16{0}) {
			t.Fatalf("NUL Units = %v", info.Units)
		}

		// Historical "A\u0000Z" MUTF-8 is 41 C0 80 5A.
		hist := []byte{0x41, 0xC0, 0x80, 0x5A}
		info, err = utf8InfoFromPayload(hist)
		if err != nil {
			t.Fatalf("A NUL Z readInfo: %v", err)
		}
		if !reflect.DeepEqual(info.Units, []uint16{65, 0, 90}) {
			t.Fatalf("A NUL Z Units = %v", info.Units)
		}

		mixed := []uint16{65, 0, 55357, 56832, 0xD800, 90}
		info, err = utf8InfoFromPayload(refEncodeUnits(mixed))
		if err != nil {
			t.Fatalf("mixed readInfo: %v", err)
		}
		if !reflect.DeepEqual(info.Units, mixed) {
			t.Fatalf("mixed Units = %v", info.Units)
		}
		if containsUnit(info.Units, 0xFFFD) {
			t.Fatalf("mixed inserted U+FFFD: %v", info.Units)
		}
	})
}

func TestTaskT02C03_ClassUtf8RejectsInvalid(t *testing.T) {
	t.Run("T02-C03", func(t *testing.T) {
		invalid := [][]byte{
			{0x00},
			{0x80},
			{0xE1, 0x80},
			{0xF0, 0x9F, 0x98, 0x80},
			{0xC1, 0x81},
			{0xC0},
			{0xED, 0xA0},
		}
		for _, raw := range invalid {
			info, err := utf8InfoFromPayload(raw)
			if err == nil {
				t.Fatalf("readInfo(%x) succeeded Value=%q Units=%v", raw, info.Value, info.Units)
			}
			if info.Units != nil {
				t.Fatalf("readInfo(%x) left Units %v", raw, info.Units)
			}
			if info.Value != "" {
				t.Fatalf("readInfo(%x) Value=%q want empty", raw, info.Value)
			}
			assertNoErrorText(t, info.Value)
			if info.DecodeErr == nil {
				t.Fatalf("readInfo(%x) DecodeErr is nil", raw)
			}
			if !bytes.Equal(info.Raw, raw) {
				t.Fatalf("readInfo(%x) Raw = %x", raw, info.Raw)
			}

			obj, perr := Parse(buildClassWithUtf8Payload(raw))
			if perr == nil {
				t.Fatalf("Parse(%x) succeeded", raw)
			}
			if obj != nil {
				for _, c := range obj.ConstantPool {
					if u, ok := c.(*ConstantUtf8Info); ok {
						assertNoErrorText(t, u.Value)
					}
				}
			}
			if !strings.Contains(strings.ToLower(perr.Error()), "mutf8") &&
				!strings.Contains(strings.ToLower(perr.Error()), "parse class") {
				t.Fatalf("Parse error unexpected: %v", perr)
			}
		}
	})
}

func TestTaskT02C04_ClassUtf8SingleUnitsSample(t *testing.T) {
	t.Run("T02-C04", func(t *testing.T) {
		samples := []uint16{0, 1, 0x7F, 0x80, 0x7FF, 0x800, 0xD800, 0xDFFF, 0xFFFF, 65}
		for _, u := range samples {
			info, err := utf8InfoFromPayload(refEncodeUnit(u))
			if err != nil {
				t.Fatalf("u=%#04x: %v", u, err)
			}
			if len(info.Units) != 1 || info.Units[0] != u {
				t.Fatalf("u=%#04x Units=%v", u, info.Units)
			}
		}

		data := []ConstantInfo{}
		pool := NewConstantPoolWithConstant(&data)
		idx := pool.AddUtf8Info("java/lang/Object")
		got := pool.GetUtf8(idx)
		if got == nil || got.Value != "java/lang/Object" {
			t.Fatalf("AddUtf8Info compatibility: %+v", got)
		}
		payload := got.mutf8Bytes()
		if string(payload) != "java/lang/Object" {
			t.Fatalf("AddUtf8Info payload = %q", payload)
		}
	})
}

func TestTaskT02C05_ClassUtf8BudgetAndCopy(t *testing.T) {
	t.Run("T02-C05", func(t *testing.T) {
		a := []uint16{0x0041, 0x0000, 0xD800}
		b := []uint16{0xD83D, 0xDE00}
		c := []uint16{0x005A, 0xFFFD, 0xFFFF}
		want := append(append(append([]uint16{}, a...), b...), c...)
		n := len(want)
		got, err := mutf8.ConcatBounded(n, a, b, c)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("ConcatBounded max=N: %v %v", got, err)
		}
		fail, err := mutf8.ConcatBounded(n-1, a, b, c)
		if err == nil || fail != nil {
			t.Fatalf("ConcatBounded max=N-1 returned %v err=%v", fail, err)
		}
		raw := refEncodeUnits(want)
		du, err := mutf8.DecodeBounded(raw, n)
		if err != nil || !reflect.DeepEqual(du, want) {
			t.Fatalf("DecodeBounded max=N: %v %v", du, err)
		}
		duFail, err := mutf8.DecodeBounded(raw, n-1)
		if err == nil || duFail != nil {
			t.Fatalf("DecodeBounded max=N-1 returned %v err=%v", duFail, err)
		}
	})
}

func TestTaskT02C06_ClassUtf8UseKind(t *testing.T) {
	t.Run("T02-C06", func(t *testing.T) {
		unitsOf := func(s string) []uint16 { return utf16.Encode([]rune(s)) }
		name := unitsOf("java/lang/Object")
		info, err := utf8InfoFromPayload(refEncodeUnits(name))
		if err != nil {
			t.Fatalf("decode name: %v", err)
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseString); err != nil {
			t.Fatalf("string use: %v", err)
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseInternalName); err != nil {
			t.Fatalf("internal name: %v", err)
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseDescriptor); err == nil {
			t.Fatalf("java/lang/Object must not validate as descriptor")
		}

		desc := unitsOf("(Ljava/lang/String;)V")
		info, err = utf8InfoFromPayload(refEncodeUnits(desc))
		if err != nil {
			t.Fatalf("decode desc: %v", err)
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseDescriptor); err != nil {
			t.Fatalf("method descriptor: %v", err)
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseInternalName); err == nil {
			t.Fatalf("method descriptor must not be an internal name")
		}
		if err := mutf8.ValidateUse(info.Units, mutf8.UseString); err != nil {
			t.Fatalf("string use of descriptor: %v", err)
		}
	})
}

func containsUnit(units []uint16, u uint16) bool {
	for _, x := range units {
		if x == u {
			return true
		}
	}
	return false
}
