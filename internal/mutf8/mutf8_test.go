package mutf8

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"unicode/utf16"
)

//go:embed testdata/vectors.json
var embeddedVectorsJSON []byte

// Independent reference encoder for T02-C04. Do not call production Encode as the oracle.
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

func mustDecode(t *testing.T, raw []byte) []uint16 {
	t.Helper()
	got, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode(%x): %v", raw, err)
	}
	return got
}

func asDecodeError(t *testing.T, err error) *DecodeError {
	t.Helper()
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error %v (%T) is not *DecodeError", err, err)
	}
	return de
}

func TestTaskT02C01_SurrogatePairUnits(t *testing.T) {
	t.Run("T02-C01", func(t *testing.T) {
		want := []uint16{65, 55357, 56832, 90}
		raw := refEncodeUnits(want)
		if bytes.Contains(raw, []byte{0xF0}) {
			t.Fatalf("refEncode produced UTF-8 4-byte lead: %x", raw)
		}
		got := mustDecode(t, raw)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("units = %v want %v (raw %x)", got, want, raw)
		}
		for _, u := range got {
			if u == 0xFFFD {
				t.Fatalf("U+FFFD replacement in units: %v", got)
			}
		}
	})
}

func TestTaskT02C02_LoneSurrogateAndNul(t *testing.T) {
	t.Run("T02-C02", func(t *testing.T) {
		lone := mustDecode(t, refEncodeUnit(0xD800))
		if !reflect.DeepEqual(lone, []uint16{0xD800}) {
			t.Fatalf("lone D800 = %v", lone)
		}

		nul := mustDecode(t, []byte{0xC0, 0x80})
		if !reflect.DeepEqual(nul, []uint16{0}) {
			t.Fatalf("NUL C0 80 = %v", nul)
		}

		// UnicodeLiteral: A, NUL, pair, lone D800, Z
		mixedWant := []uint16{65, 0, 55357, 56832, 0xD800, 90}
		mixed := mustDecode(t, refEncodeUnits(mixedWant))
		if !reflect.DeepEqual(mixed, mixedWant) {
			t.Fatalf("mixed = %v want %v", mixed, mixedWant)
		}
		for _, u := range mixed {
			if u == 0xFFFD {
				t.Fatalf("U+FFFD replacement in mixed units: %v", mixed)
			}
		}
		// NFC must not fold anything; units are already the stored form.
		if utf16.Decode(mixed)[0] == 0xFFFD && mixed[0] != 0xFFFD {
			t.Fatalf("Go rune decode replaced a non-FFFD unit")
		}

		// Historical NUL control: "A\u0000Z" is 41 C0 80 5A.
		hist := mustDecode(t, []byte{0x41, 0xC0, 0x80, 0x5A})
		if !reflect.DeepEqual(hist, []uint16{65, 0, 90}) {
			t.Fatalf("historical A NUL Z = %v", hist)
		}
	})
}

func TestTaskT02C03_RejectInvalid(t *testing.T) {
	t.Run("T02-C03", func(t *testing.T) {
		cases := []struct {
			name   string
			raw    []byte
			reason string
			offset int
		}{
			{name: "raw_00", raw: []byte{0x00}, reason: "nul_byte", offset: 0},
			{name: "lone_continuation", raw: []byte{0x80}, reason: "bad_continuation", offset: 0},
			{name: "truncated_e180", raw: []byte{0xE1, 0x80}, reason: "truncated", offset: 0},
			{name: "four_byte", raw: []byte{0xF0, 0x9F, 0x98, 0x80}, reason: "four_byte", offset: 0},
			{name: "overlong_c181", raw: []byte{0xC1, 0x81}, reason: "overlong", offset: 0},
			{name: "truncated_c0", raw: []byte{0xC0}, reason: "truncated", offset: 0},
			{name: "truncated_eda0", raw: []byte{0xED, 0xA0}, reason: "truncated", offset: 0},
			{name: "overlong_e08080", raw: []byte{0xE0, 0x80, 0x80}, reason: "overlong", offset: 0},
			{name: "truncated_e1", raw: []byte{0xE1}, reason: "truncated", offset: 0},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := Decode(tc.raw)
				if err == nil {
					t.Fatalf("Decode(%x) succeeded with %v", tc.raw, got)
				}
				if got != nil {
					t.Fatalf("Decode(%x) returned units %v with error", tc.raw, got)
				}
				de := asDecodeError(t, err)
				if de.Reason != tc.reason {
					t.Fatalf("reason = %q want %q (err %v)", de.Reason, tc.reason, err)
				}
				if de.Offset != tc.offset {
					t.Fatalf("offset = %d want %d", de.Offset, tc.offset)
				}
			})
		}

		loadMutf8Vectors(t)
	})
}

func TestTaskT02C04_AllSingleUnits(t *testing.T) {
	t.Run("T02-C04", func(t *testing.T) {
		var nerr int
		for u := 0; u <= 0xFFFF; u++ {
			raw := refEncodeUnit(uint16(u))
			got, err := Decode(raw)
			if err != nil {
				nerr++
				t.Errorf("u=%#04x Decode(%x): %v", u, raw, err)
				continue
			}
			if len(got) != 1 || got[0] != uint16(u) {
				nerr++
				t.Errorf("u=%#04x got %v", u, got)
			}
		}
		if nerr != 0 {
			t.Fatalf("T02-C04 round-trip errors = %d want 0", nerr)
		}
	})
}

func TestTaskT02C05_ConcatAndBudget(t *testing.T) {
	t.Run("T02-C05", func(t *testing.T) {
		if _, ok := CheckedAdd(math.MaxInt, 1); ok {
			t.Fatal("CheckedAdd(MaxInt,1) must overflow")
		}
		if EncodeCap(math.MaxInt/3+1) != math.MaxInt {
			t.Fatalf("EncodeCap must saturate, got %d", EncodeCap(math.MaxInt/3+1))
		}
		if EncodeCap(4) != 12 {
			t.Fatalf("EncodeCap(4)=%d", EncodeCap(4))
		}

		a := []uint16{0x0041, 0x0000, 0xD800}
		b := []uint16{0xD83D, 0xDE00}
		c := []uint16{0x005A, 0xFFFD, 0xFFFF}
		want := []uint16{0x0041, 0x0000, 0xD800, 0xD83D, 0xDE00, 0x005A, 0xFFFD, 0xFFFF}
		got := Concat(a, b, c)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Concat = %v want %v", got, want)
		}
		n := len(want)

		ok, err := ConcatBounded(n, a, b, c)
		if err != nil || !reflect.DeepEqual(ok, want) {
			t.Fatalf("ConcatBounded(max=%d) = %v, %v", n, ok, err)
		}
		fail, err := ConcatBounded(n-1, a, b, c)
		if err == nil {
			t.Fatalf("ConcatBounded(max=%d) succeeded with %v", n-1, fail)
		}
		if fail != nil {
			t.Fatalf("ConcatBounded budget failure returned prefix %v", fail)
		}
		de := asDecodeError(t, err)
		if de.Reason != "budget" {
			t.Fatalf("ConcatBounded reason = %q want budget", de.Reason)
		}

		rng := rand.New(rand.NewSource(20260921))
		for trial := 0; trial < 64; trial++ {
			var parts [][]uint16
			var flat []uint16
			nparts := 1 + rng.Intn(5)
			for i := 0; i < nparts; i++ {
				ln := rng.Intn(8)
				p := make([]uint16, ln)
				for j := range p {
					p[j] = uint16(rng.Intn(0x10000))
				}
				parts = append(parts, p)
				flat = append(flat, p...)
			}
			L := len(flat)
			for _, max := range []int{L - 1, L, L + 1} {
				if max < 0 {
					continue
				}
				out, err := ConcatBounded(max, parts...)
				if max < L {
					if err == nil || out != nil {
						t.Fatalf("trial %d max=%d L=%d got %v err=%v", trial, max, L, out, err)
					}
					continue
				}
				if err != nil || len(out) != L {
					t.Fatalf("trial %d max=%d L=%d got %v err=%v want %v", trial, max, L, out, err, flat)
				}
				if L > 0 && !reflect.DeepEqual(out, flat) {
					t.Fatalf("trial %d max=%d mismatch %v vs %v", trial, max, out, flat)
				}
			}
		}

		raw := refEncodeUnits(want)
		du, err := DecodeBounded(raw, n)
		if err != nil || !reflect.DeepEqual(du, want) {
			t.Fatalf("DecodeBounded(max=%d) = %v, %v", n, du, err)
		}
		duFail, err := DecodeBounded(raw, n-1)
		if err == nil {
			t.Fatalf("DecodeBounded(max=%d) succeeded with %v", n-1, duFail)
		}
		if duFail != nil {
			t.Fatalf("DecodeBounded budget failure returned prefix %v", duFail)
		}
		de = asDecodeError(t, err)
		if de.Reason != "budget" {
			t.Fatalf("DecodeBounded reason = %q want budget", de.Reason)
		}

		empty, err := DecodeBounded(nil, 0)
		if err != nil || len(empty) != 0 {
			t.Fatalf("DecodeBounded(empty, 0) = %v, %v", empty, err)
		}
	})
}

func TestTaskT02C06_UseKindSeparation(t *testing.T) {
	t.Run("T02-C06", func(t *testing.T) {
		unitsOf := func(s string) []uint16 {
			return utf16.Encode([]rune(s))
		}

		objectName := unitsOf("java/lang/Object")
		if err := ValidateUse(objectName, UseString); err != nil {
			t.Fatalf("java/lang/Object as string: %v", err)
		}
		if err := ValidateUse(objectName, UseInternalName); err != nil {
			t.Fatalf("java/lang/Object as internal name: %v", err)
		}
		if err := ValidateUse(objectName, UseDescriptor); err == nil {
			t.Fatalf("java/lang/Object must not be a field/method descriptor")
		}

		withSemi := unitsOf("foo;bar")
		if err := ValidateUse(withSemi, UseString); err != nil {
			t.Fatalf("foo;bar as string: %v", err)
		}
		if err := ValidateUse(withSemi, UseInternalName); err == nil {
			t.Fatalf("foo;bar must be invalid as internal name")
		}

		methodDesc := unitsOf("(Ljava/lang/String;)V")
		if err := ValidateUse(methodDesc, UseString); err != nil {
			t.Fatalf("method desc as string: %v", err)
		}
		if err := ValidateUse(methodDesc, UseDescriptor); err != nil {
			t.Fatalf("(Ljava/lang/String;)V as descriptor: %v", err)
		}
		if err := ValidateUse(methodDesc, UseInternalName); err == nil {
			t.Fatalf("method descriptor must be invalid as internal name")
		}

		fieldDesc := unitsOf("Ljava/lang/Object;")
		if err := ValidateUse(fieldDesc, UseDescriptor); err != nil {
			t.Fatalf("Ljava/lang/Object; as descriptor: %v", err)
		}
		if err := ValidateUse(fieldDesc, UseInternalName); err == nil {
			t.Fatalf("field descriptor must be invalid as internal name")
		}

		if err := ValidateUse(nil, UseInternalName); err == nil {
			t.Fatalf("empty internal name must fail")
		}
		if err := ValidateUse(nil, UseString); err != nil {
			t.Fatalf("empty string must be valid: %v", err)
		}

		// Decode success does not imply name validity.
		raw := refEncodeUnits(withSemi)
		got := mustDecode(t, raw)
		if err := ValidateUse(got, UseString); err != nil {
			t.Fatalf("decoded ;-string: %v", err)
		}
		if err := ValidateUse(got, UseInternalName); err == nil {
			t.Fatalf("decoded ;-string must fail as internal name")
		}

		nulName := []uint16{'A', 0, 'Z'}
		if err := ValidateUse(nulName, UseInternalName); err != nil {
			t.Fatalf("NUL is JVM-legal in a binary name: %v", err)
		}
		if JavaSourceIdentifierOK(nulName) {
			t.Fatal("NUL must not be a Java source identifier")
		}
		hyphen := unitsOf("weird-name")
		if err := ValidateUnqualifiedName(hyphen); err != nil {
			t.Fatalf("hyphen is JVM-legal: %v", err)
		}
		if JavaSourceIdentifierOK(hyphen) {
			t.Fatal("hyphen must not be a Java source identifier")
		}

		if err := ValidateUse(methodDesc, UseFieldDescriptor); err == nil {
			t.Fatal("method descriptor must not pass as field descriptor")
		}
		if err := ValidateUse(fieldDesc, UseMethodDescriptor); err == nil {
			t.Fatal("field descriptor must not pass as method descriptor")
		}

		dims255 := make([]uint16, 255+2)
		for i := 0; i < 255; i++ {
			dims255[i] = '['
		}
		dims255[255], dims255[256] = 'I', 0
		dims255 = dims255[:256] // 255 '[' + 'I'
		if err := ValidateFieldDescriptor(dims255); err != nil {
			t.Fatalf("255 array dims must be legal: %v", err)
		}
		dims256 := append([]uint16{'['}, dims255...)
		if err := ValidateFieldDescriptor(dims256); err == nil {
			t.Fatal("256 array dims must be rejected")
		}

		// 255 int parameters, static: legal. 128 longs: 256 slots, illegal.
		ints255 := make([]uint16, 1+255+2)
		ints255[0] = '('
		for i := 0; i < 255; i++ {
			ints255[1+i] = 'I'
		}
		ints255[256], ints255[257] = ')', 'V'
		if err := ValidateMethodDescriptor(ints255); err != nil {
			t.Fatalf("255 int params: %v", err)
		}
		if slots, err := InvokeSlots(ints255, true); err == nil {
			t.Fatalf("instance invoke of 255 int params must fail, slots=%d", slots)
		}
		if slots, err := InvokeSlots(ints255, false); err != nil || slots != 255 {
			t.Fatalf("static invoke of 255 int params: %d %v", slots, err)
		}
		longs := []uint16{'('}
		for i := 0; i < 128; i++ {
			longs = append(longs, 'J')
		}
		longs = append(longs, ')', 'V')
		if err := ValidateMethodDescriptor(longs); err == nil {
			t.Fatal("128 longs = 256 slots must be rejected")
		}
	})
}

func loadMutf8Vectors(t *testing.T) {
	t.Helper()
	type vecFile struct {
		Valid []struct {
			Name  string   `json:"name"`
			Hex   string   `json:"hex"`
			Units []uint16 `json:"units"`
		} `json:"valid"`
		Invalid []struct {
			Hex      string `json:"hex"`
			Expected string `json:"expected"`
		} `json:"invalid"`
	}

	hardValid := []struct {
		hex   string
		units []uint16
	}{
		{"415a", []uint16{65, 90}},
		{"c080", []uint16{0}},
		{"eda0bdedb880", []uint16{55357, 56832}},
		{"eda080", []uint16{55296}},
		{"edb080", []uint16{56320}},
		{"efbfbf", []uint16{65535}},
	}
	hardInvalid := []string{"00", "80", "c0", "e1", "e180", "f09f9880", "c181", "e08080", "eda0"}
	for _, v := range hardValid {
		raw, err := hex.DecodeString(v.hex)
		if err != nil {
			t.Fatalf("hex %s: %v", v.hex, err)
		}
		got := mustDecode(t, raw)
		if !reflect.DeepEqual(got, v.units) {
			t.Fatalf("vector %s: got %v want %v", v.hex, got, v.units)
		}
	}
	for _, h := range hardInvalid {
		raw, err := hex.DecodeString(h)
		if err != nil {
			t.Fatalf("hex %s: %v", h, err)
		}
		got, err := Decode(raw)
		if err == nil || got != nil {
			t.Fatalf("invalid vector %s succeeded: %v", h, got)
		}
	}

	if len(embeddedVectorsJSON) == 0 {
		t.Fatal("embedded testdata/vectors.json is empty")
	}
	var vf vecFile
	if err := json.Unmarshal(embeddedVectorsJSON, &vf); err != nil {
		t.Fatalf("parse embedded vectors: %v", err)
	}
	if len(vf.Valid) == 0 || len(vf.Invalid) == 0 {
		t.Fatalf("embedded vectors inventory empty: valid=%d invalid=%d", len(vf.Valid), len(vf.Invalid))
	}
	for _, v := range vf.Valid {
		raw, err := hex.DecodeString(v.Hex)
		if err != nil {
			t.Fatalf("vector %s hex: %v", v.Name, err)
		}
		got := mustDecode(t, raw)
		want := append([]uint16(nil), v.Units...)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("file vector %s: got %v want %v", v.Name, got, want)
		}
	}
	for _, v := range vf.Invalid {
		raw, err := hex.DecodeString(v.Hex)
		if err != nil {
			t.Fatalf("invalid hex %s: %v", v.Hex, err)
		}
		got, err := Decode(raw)
		if err == nil || got != nil {
			t.Fatalf("file invalid %s succeeded: %v", v.Hex, got)
		}
	}
}
