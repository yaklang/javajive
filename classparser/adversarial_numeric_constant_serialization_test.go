package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

const numericSerializationFixture = `class NumericSerializationProbe{static float F=1.25f;static double D=1.25;static int I=Integer.MIN_VALUE;static long L=Long.MIN_VALUE;public static void main(String[]args){if(Float.floatToRawIntBits(F)!=0x3fa00000||Double.doubleToRawLongBits(D)!=0x3ff4000000000000L||I!=Integer.MIN_VALUE||L!=Long.MIN_VALUE)throw new AssertionError("numeric constants");System.out.println("numeric:serialization:bits");}}`

func TestAdversarialNumericConstantSerializationPreservesRawBits(t *testing.T) {
	files := nativeCompileClasses(t, numericSerializationFixture)
	cases := []struct {
		name string
		f    uint32
		d    uint64
	}{
		{"finite", 0x3fa00000, 0x3ff4000000000000}, {"positive zero", 0, 0}, {"negative zero", 0x80000000, 0x8000000000000000},
		{"smallest subnormal", 1, 1}, {"largest subnormal", 0x007fffff, 0x000fffffffffffff}, {"smallest normal", 0x00800000, 0x0010000000000000},
		{"largest finite", 0x7f7fffff, 0x7fefffffffffffff}, {"negative finite", 0xbf900000, 0xbff2000000000000},
		{"positive infinity", 0x7f800000, 0x7ff0000000000000}, {"negative infinity", 0xff800000, 0xfff0000000000000},
		{"canonical NaN", 0x7fc00000, 0x7ff8000000000000}, {"quiet payload", 0x7fc01234, 0x7ff8000000001234},
		{"signaling payload", 0x7f801234, 0x7ff0000000001234}, {"negative quiet payload", 0xffc01234, 0xfff8000000001234},
		{"negative signaling payload", 0xff801234, 0xfff0000000001234},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := append([]byte(nil), files["NumericSerializationProbe.class"]...)
			for _, patch := range []struct {
				old   []byte
				value []byte
			}{
				{[]byte{4, 0x3f, 0xa0, 0, 0}, binary.BigEndian.AppendUint32(nil, c.f)},
				{[]byte{6, 0x3f, 0xf4, 0, 0, 0, 0, 0, 0}, binary.BigEndian.AppendUint64(nil, c.d)},
			} {
				if bytes.Count(raw, patch.old) != 1 {
					t.Fatal("unique original CP bits")
				}
				offset := bytes.Index(raw, patch.old) + 1
				copy(raw[offset:offset+len(patch.value)], patch.value)
			}
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			rebuilt := obj.Bytes()
			if !bytes.Equal(rebuilt, raw) {
				t.Fatalf("class serialization changed exact original numeric bytes, original=%d rebuilt=%d", len(raw), len(rebuilt))
			}
			if _, e = Parse(rebuilt); e != nil {
				t.Fatalf("reparse serialized class: %v", e)
			}
		})
	}
}
func TestAdversarialNumericConstantSerializationRemainsJVMValid(t *testing.T) {
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, numericSerializationFixture, debug)
			original := t.TempDir()
			rebuilt := t.TempDir()
			for name, raw := range files {
				if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
					t.Fatal(e)
				}
				obj, e := Parse(raw)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(rebuilt, name), obj.Bytes(), 0600); e != nil {
					t.Fatal(e)
				}
			}
			want := "numeric:serialization:bits\n"
			if got := t04RunJava(t, java, original, "NumericSerializationProbe"); got != want {
				t.Fatalf("original=%q", got)
			}
			if got := t04RunJava(t, java, rebuilt, "NumericSerializationProbe"); got != want {
				t.Fatalf("serialized=%q", got)
			}
		})
	}
}
