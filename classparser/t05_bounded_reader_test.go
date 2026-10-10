package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func assertNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v\n%s", rec, debug.Stack())
		}
	}()
	fn()
}

func classParseCode(err error) string {
	var pe *ClassParseError
	if errors.As(err, &pe) && pe != nil {
		return pe.Code
	}
	return ""
}

func assertCursor(t *testing.T, r *ClassReader, label string) {
	t.Helper()
	if r.Offset() < 0 || r.Offset() > r.Bound() {
		t.Fatalf("%s cursor %d not in [0,%d]", label, r.Offset(), r.Bound())
	}
	if r.Remaining() < 0 {
		t.Fatalf("%s remaining %d < 0", label, r.Remaining())
	}
	if r.Remaining() != r.Bound()-r.Offset() {
		t.Fatalf("%s remaining %d != bound-offset %d", label, r.Remaining(), r.Bound()-r.Offset())
	}
}

func TestTaskT05C01ReadWidthBounds(t *testing.T) {
	t.Run("T05-C01", func(t *testing.T) {
		type widthCase struct {
			name  string
			width int
			read  func(*ClassReader)
		}
		cases := []widthCase{
			{"u1", 1, func(r *ClassReader) { _ = r.readUint8() }},
			{"u2", 2, func(r *ClassReader) { _ = r.readUint16() }},
			{"u4", 4, func(r *ClassReader) { _ = r.readUint32() }},
			{"u8", 8, func(r *ClassReader) { _ = r.readUint64() }},
		}
		panicCount := 0
		for _, c := range cases {
			for rem := 0; rem <= c.width+1; rem++ {
				buf := bytes.Repeat([]byte{0xff}, rem)
				r := NewClassReader(buf)
				r.SetStage("T05-C01", c.name)
				func() {
					defer func() {
						if rec := recover(); rec != nil {
							panicCount++
							t.Errorf("%s remaining=%d panicked: %v", c.name, rem, rec)
						}
					}()
					c.read(r)
				}()
				assertCursor(t, r, c.name)
				if rem < c.width {
					if r.Err() == nil {
						t.Fatalf("%s remaining=%d: want error", c.name, rem)
					}
					if r.Offset() > rem {
						t.Fatalf("%s remaining=%d advanced past bound to %d", c.name, rem, r.Offset())
					}
				} else {
					if r.Err() != nil {
						t.Fatalf("%s remaining=%d: unexpected err %v", c.name, rem, r.Err())
					}
					if r.Offset() != c.width {
						t.Fatalf("%s remaining=%d: offset=%d want %d", c.name, rem, r.Offset(), c.width)
					}
					if r.Remaining() != rem-c.width {
						t.Fatalf("%s remaining after read=%d want %d", c.name, r.Remaining(), rem-c.width)
					}
				}

				br := NewClassReader(bytes.Repeat([]byte{0xff}, rem))
				br.SetStage("T05-C01", "bytes")
				var got []byte
				func() {
					defer func() {
						if rec := recover(); rec != nil {
							panicCount++
							t.Errorf("readBytes(%d) remaining=%d panicked: %v", c.width, rem, rec)
						}
					}()
					got = br.readBytes(uint32(c.width))
				}()
				assertCursor(t, br, "bytes")
				if rem < c.width {
					if br.Err() == nil {
						t.Fatalf("readBytes(%d) remaining=%d: want error", c.width, rem)
					}
					if got != nil {
						t.Fatalf("readBytes(%d) remaining=%d returned %v", c.width, rem, got)
					}
				} else {
					if br.Err() != nil || len(got) != c.width {
						t.Fatalf("readBytes(%d) remaining=%d: got len=%d err=%v", c.width, rem, len(got), br.Err())
					}
				}
			}
		}
		if panicCount != 0 {
			t.Fatalf("panic count %d want 0", panicCount)
		}
	})
}

func TestTaskT05C02LargeLengthRejectBeforeAlloc(t *testing.T) {
	t.Run("T05-C02", func(t *testing.T) {
		r := NewClassReader([]byte{1, 2, 3, 4})
		r.SetStage("T05-C02", "bytes")
		assertNoPanic(t, func() {
			got := r.readBytes(0)
			if r.Err() != nil || len(got) != 0 {
				t.Fatalf("length 0: got %d err %v", len(got), r.Err())
			}
		})
		r = NewClassReader([]byte{1, 2, 3, 4})
		r.SetStage("T05-C02", "bytes")
		assertNoPanic(t, func() {
			got := r.readBytes(1)
			if r.Err() != nil || len(got) != 1 || got[0] != 1 {
				t.Fatalf("length 1: %v err %v", got, r.Err())
			}
		})
		r = NewClassReader([]byte{1, 2, 3, 4})
		r.SetStage("T05-C02", "bytes")
		var got []byte
		assertNoPanic(t, func() {
			got = r.readBytes(0xffffffff)
		})
		if r.Err() == nil {
			t.Fatal("0xffffffff must be rejected before allocation")
		}
		if got != nil {
			t.Fatalf("0xffffffff returned slice len %d", len(got))
		}
		if r.Offset() != 0 {
			t.Fatalf("failed huge read moved cursor to %d", r.Offset())
		}
		err := r.Err()
		if classParseCode(err) == "" {
			t.Fatalf("error missing code: %v", err)
		}
		text := err.Error()
		if !strings.Contains(text, "offset=") || !strings.Contains(text, "stage=") || !strings.Contains(text, "code=") {
			t.Fatalf("error must contain offset+stage+code: %v", err)
		}
		r = NewClassReader([]byte{1, 2, 3, 4})
		r.SetStage("T05-C02", "subreader")
		assertNoPanic(t, func() {
			sub := r.Subreader(0xffffffff)
			if sub.Err() == nil || r.Err() == nil {
				t.Fatal("huge Subreader must fail")
			}
			if r.Offset() != 0 {
				t.Fatalf("failed Subreader advanced parent to %d", r.Offset())
			}
			if sub.Remaining() != 0 {
				t.Fatalf("failed child remaining=%d want 0", sub.Remaining())
			}
			stext := r.Err().Error()
			if !strings.Contains(stext, "offset=") || !strings.Contains(stext, "stage=") || !strings.Contains(stext, "code=") {
				t.Fatalf("subreader error must contain offset+stage+code: %v", r.Err())
			}
		})
	})
}

func TestTaskT05C03NestedSubreader(t *testing.T) {
	t.Run("T05-C03", func(t *testing.T) {
		buf := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		parent := NewClassReader(buf)
		parent.SetStage("T05-C03", "parent")
		child := parent.Subreader(4)
		if parent.Err() != nil || child.Err() != nil {
			t.Fatalf("Subreader(4) failed: parent=%v child=%v", parent.Err(), child.Err())
		}
		if parent.Offset() != 4 {
			t.Fatalf("parent offset=%d want 4 after reserve", parent.Offset())
		}
		if child.Bound() != 4 || child.Remaining() != 4 {
			t.Fatalf("child bound=%d remaining=%d", child.Bound(), child.Remaining())
		}
		var got []byte
		assertNoPanic(t, func() {
			got = child.readBytes(5)
		})
		if child.Err() == nil {
			t.Fatal("child readBytes(5) must fail")
		}
		if got != nil {
			t.Fatalf("child must not return parent byte 4; got %v", got)
		}
		if child.Offset() > 4 {
			t.Fatalf("child offset %d crossed parent byte 4", child.Offset())
		}
		if parent.Remaining() != 6 {
			t.Fatalf("parent remaining=%d want 6", parent.Remaining())
		}

		parent2 := NewClassReader(buf)
		parent2.SetStage("T05-C03", "parent")
		child2 := parent2.Subreader(4)
		for i := 0; i < 4; i++ {
			b := child2.readUint8()
			if child2.Err() != nil || b != buf[i] {
				t.Fatalf("child2 byte %d: got %d err %v", i, b, child2.Err())
			}
		}
		_ = child2.readUint8()
		var pe *ClassParseError
		if !errors.As(child2.Err(), &pe) || pe.Offset != 4 {
			t.Fatalf("want absolute offset 4, got %v", child2.Err())
		}
	})
}

func t05LoadValidClass(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join("testdata", "invisible_anno.class")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("infrastructure: read %s: %v", p, err)
	}
	return data
}

func TestTaskT05C04StrictTrailingAndControls(t *testing.T) {
	t.Run("T05-C04", func(t *testing.T) {
		valid := t05LoadValidClass(t)
		obj, err := Parse(valid)
		if err != nil || obj == nil {
			t.Fatalf("valid class control rejected: %v", err)
		}
		junked := append(append([]byte{}, valid...), 0x00, 0x01, 0x02, 0x03)
		_, err = Parse(junked)
		if err == nil {
			t.Fatal("trailing junk accepted")
		}
		if classParseCode(err) != ParseCodeTrailingBytes {
			t.Fatalf("trailing code=%s err=%v", classParseCode(err), err)
		}
		if !strings.Contains(err.Error(), "trailing") || !strings.Contains(err.Error(), "offset=") {
			t.Fatalf("trailing error text: %v", err)
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			res, derr := DecompileWithOptions(junked, DecompileOptions{Mode: mode})
			if derr == nil || res.Status != "invalid_input" {
				t.Fatalf("mode %s trailing: status=%s err=%v", mode, res.Status, derr)
			}
			if res.Status == "complete" {
				t.Fatalf("mode %s reported complete on junked class", mode)
			}
		}
		_, err = Parse([]byte{0, 0, 0, 0})
		if err == nil {
			t.Fatal("bad magic accepted")
		}
		if classParseCode(err) != ParseCodeBadMagic {
			if !strings.Contains(err.Error(), "Magic") && classParseCode(err) != ParseCodeTruncated {
				t.Fatalf("bad magic: %v", err)
			}
		}
		if len(valid) < 10 {
			t.Fatal("valid class too small")
		}
		_, err = Parse(valid[:10])
		if err == nil {
			t.Fatal("truncated class accepted")
		}
		assertNoPanic(t, func() { _, _ = Parse(valid[:10]) })

		// Unknown major version is not a corrupt classfile.
		future := append([]byte{}, valid...)
		binary.BigEndian.PutUint16(future[6:8], 61) // Java 17
		obj, err = Parse(future)
		if err != nil || obj == nil {
			t.Fatalf("unknown version treated as corrupt: %v", err)
		}
	})
}

func TestTaskT05C05StickyErrorNoForgedSuccess(t *testing.T) {
	t.Run("T05-C05", func(t *testing.T) {
		r := NewClassReader(nil)
		r.SetStage("T05-C05", "u1")
		_ = r.readUint8()
		if r.Err() == nil {
			t.Fatal("empty read must fail")
		}
		sticky := r.Err()
		_ = r.readUint32()
		got := r.readBytes(2)
		if r.Err() != sticky {
			t.Fatalf("sticky error mutated: %v vs %v", r.Err(), sticky)
		}
		if got != nil {
			t.Fatalf("forged bytes after failure: %v", got)
		}
		if r.Offset() != 0 {
			t.Fatalf("sticky reads moved cursor to %d", r.Offset())
		}

		// cafe babe + version + large cp count, no entries
		buf := []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x34}
		cpCount := make([]byte, 2)
		binary.BigEndian.PutUint16(cpCount, 50)
		buf = append(buf, cpCount...)
		obj, err := Parse(buf)
		if err == nil || obj != nil {
			t.Fatalf("truncated pool returned obj=%v err=%v", obj, err)
		}
		res, derr := DecompileWithOptions(buf, DecompileOptions{Mode: Precision})
		if derr == nil || res.Status == "complete" {
			t.Fatalf("forged complete: %+v %v", res, derr)
		}
		if res.Status != "invalid_input" {
			t.Fatalf("status=%s want invalid_input", res.Status)
		}
	})
}

func TestTaskT05C06FuzzCursorInvariants(t *testing.T) {
	t.Run("T05-C06", func(t *testing.T) {
		rng := rand.New(rand.NewSource(20260921))
		ops := []string{"u1", "u2", "u4", "u8", "bytes", "sub"}
		for iter := 0; iter < 200; iter++ {
			n := rng.Intn(65)
			buf := make([]byte, n)
			rng.Read(buf)
			r := NewClassReader(buf)
			r.SetStage("T05-C06", "fuzz")
			nops := 1 + rng.Intn(20)
			for opi := 0; opi < nops; opi++ {
				op := ops[rng.Intn(len(ops))]
				var panicked any
				func() {
					defer func() { panicked = recover() }()
					switch op {
					case "u1":
						_ = r.readUint8()
					case "u2":
						_ = r.readUint16()
					case "u4":
						_ = r.readUint32()
					case "u8":
						_ = r.readUint64()
					case "bytes":
						var length uint32
						switch rng.Intn(6) {
						case 0:
							length = 0
						case 1:
							length = 1
						case 2:
							length = 0xffffffff
						default:
							length = uint32(rng.Intn(128))
						}
						_ = r.readBytes(length)
					case "sub":
						var length uint32
						if rng.Intn(4) == 0 {
							length = 0xffffffff
						} else {
							length = uint32(rng.Intn(80))
						}
						ch := r.Subreader(length)
						for k := 0; k < 1+rng.Intn(3); k++ {
							switch rng.Intn(4) {
							case 0:
								_ = ch.readUint8()
							case 1:
								_ = ch.readUint16()
							case 2:
								_ = ch.readBytes(uint32(rng.Intn(16)))
							default:
								_ = ch.readUint32()
							}
							if ch.Offset() < 0 || ch.Offset() > ch.Bound() {
								t.Fatalf("iter %d child cursor %d bound %d", iter, ch.Offset(), ch.Bound())
							}
							if ch.Remaining() != ch.Bound()-ch.Offset() {
								t.Fatalf("iter %d child remaining %d != bound-offset %d", iter, ch.Remaining(), ch.Bound()-ch.Offset())
							}
						}
					}
				}()
				if panicked != nil {
					t.Fatalf("iter %d op %s panic: %v", iter, op, panicked)
				}
				if r.Offset() < 0 || r.Offset() > r.Bound() {
					t.Fatalf("iter %d cursor %d not in [0,%d]", iter, r.Offset(), r.Bound())
				}
				if r.Remaining() < 0 {
					t.Fatalf("iter %d remaining %d < 0", iter, r.Remaining())
				}
				if r.Remaining() != r.Bound()-r.Offset() {
					t.Fatalf("iter %d remaining %d != bound-offset %d", iter, r.Remaining(), r.Bound()-r.Offset())
				}
			}
		}

		tiny := NewClassReader(make([]byte, 8))
		tiny.SetStage("T05-C06", "huge")
		assertNoPanic(t, func() {
			_ = tiny.readBytes(0xffffffff)
		})
		if tiny.Err() == nil {
			t.Fatal("readBytes(0xffffffff) on 8-byte buffer must error")
		}
	})
}
