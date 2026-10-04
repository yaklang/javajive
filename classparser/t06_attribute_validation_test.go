package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
)

func t06NoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v\n%s", rec, debug.Stack())
		}
	}()
	fn()
}

func t06ValidClass(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "invisible_anno.class"))
	if err != nil {
		t.Fatalf("infrastructure: %v", err)
	}
	return data
}

func t06CompileBaseline(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("javac"); err != nil {
		return t06ValidClass(t)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "Baseline.java")
	if err := os.WriteFile(src, []byte("public class Baseline { public static void main(String[] a) { System.out.println(7); } }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("javac", "--release", "8", "-d", dir, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("infrastructure javac: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Baseline.class"))
	if err != nil {
		t.Fatalf("read class: %v", err)
	}
	return data
}

func firstCodeAttributeLengthOffset(t *testing.T, data []byte) int {
	t.Helper()
	p := NewClassParser(data)
	if err := p.parseAndCheckMagic(); err != nil {
		t.Fatalf("magic: %v", err)
	}
	if err := p.readAndCheckVersion(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := p.readConstantPool(); err != nil {
		t.Fatalf("cp: %v", err)
	}
	p.reader.readUint16()
	p.reader.readUint16()
	p.reader.readUint16()
	p.reader.readUint16s()
	scanMembers := func() int {
		n := int(p.reader.readUint16())
		for i := 0; i < n; i++ {
			p.reader.readUint16()
			p.reader.readUint16()
			p.reader.readUint16()
			ac := int(p.reader.readUint16())
			for j := 0; j < ac; j++ {
				ni := p.reader.readUint16()
				name, _ := p.classObj.getUtf8(ni)
				off := p.reader.Offset()
				alen := p.reader.readUint32()
				if name == "Code" {
					return off
				}
				_ = p.reader.readBytes(alen)
			}
		}
		return -1
	}
	if off := scanMembers(); off >= 0 {
		return off
	}
	if off := scanMembers(); off >= 0 {
		return off
	}
	t.Fatal("no Code attribute")
	return -1
}

func TestTaskT06C01ZeroCodeAttributeLength(t *testing.T) {
	t.Run("T06-C01", func(t *testing.T) {
		raw := t06CompileBaseline(t)
		off := firstCodeAttributeLengthOffset(t, raw)
		mut := append([]byte{}, raw...)
		binary.BigEndian.PutUint32(mut[off:off+4], 0)
		var err error
		t06NoPanic(t, func() {
			_, err = Parse(mut)
		})
		if err == nil {
			t.Fatal("zero Code attribute_length accepted")
		}
		if classParseCode(err) == "" && !bytes.Contains([]byte(err.Error()), []byte("code=")) {
			t.Fatalf("missing structured code: %v", err)
		}
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			res, derr := DecompileWithOptions(mut, DecompileOptions{Mode: mode})
			if derr == nil || res.Status != "invalid_input" {
				t.Fatalf("mode %s: status=%s err=%v", mode, res.Status, derr)
			}
		}
	})
}

func TestTaskT06C02AttributeRangeCrossing(t *testing.T) {
	t.Run("T06-C02", func(t *testing.T) {
		parent := NewClassReader([]byte{
			0, 1, 2, 3, 4, 5, 6, 7,
			8, 9, 10, 11, 12, 13, 14, 15,
		})
		parent.SetStage("T06-C02", "attr")
		child := parent.Subreader(4)
		var got []byte
		t06NoPanic(t, func() {
			got = child.readBytes(8)
		})
		if child.Err() == nil || got != nil {
			t.Fatalf("child crossed into next attr: got %v err %v", got, child.Err())
		}
		if child.Offset() > 4 {
			t.Fatalf("child offset %d crossed reserved bound 4", child.Offset())
		}
		if parent.Offset() != 4 {
			t.Fatalf("parent offset %d want 4 (next attr start)", parent.Offset())
		}

		raw := t06CompileBaseline(t)
		off := firstCodeAttributeLengthOffset(t, raw)
		mut := append([]byte{}, raw...)
		origLen := binary.BigEndian.Uint32(mut[off : off+4])
		if origLen < 8 {
			t.Fatalf("code attr too small: %d", origLen)
		}
		binary.BigEndian.PutUint32(mut[off:off+4], origLen-4)
		_, err := Parse(mut)
		if err == nil {
			t.Fatal("truncated nested Code attribute accepted (would swallow following attrs)")
		}
	})
}

func TestTaskT06C03CPClassification(t *testing.T) {
	t.Run("T06-C03", func(t *testing.T) {
		obj := NewClassObject()
		longInfo := &ConstantLongInfo{Value: 1}
		utf := &ConstantUtf8Info{Value: "X"}
		cls := &ConstantClassInfo{NameIndex: 1}
		obj.ConstantPool = []ConstantInfo{utf, longInfo, nil, cls}

		err := obj.checkCPIndex(99, false, "f", CONSTANT_Class)
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("99")) {
			t.Fatalf("out-of-range must mention index: %v", err)
		}
		err = obj.checkCPIndex(0, false, "f", CONSTANT_Class)
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("0")) {
			t.Fatalf("forbidden zero: %v", err)
		}
		if err := obj.checkCPIndex(0, true, "super_class", CONSTANT_Class); err != nil {
			t.Fatalf("allowed zero rejected: %v", err)
		}
		err = obj.checkCPIndex(3, false, "f", CONSTANT_Class)
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("3")) {
			t.Fatalf("unusable slot must mention index: %v", err)
		}
		err = obj.checkCPIndex(1, false, "f", CONSTANT_Class)
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("1")) {
			t.Fatalf("tag mismatch must mention index: %v", err)
		}
		if err := obj.checkCPIndex(4, false, "this_class", CONSTANT_Class); err != nil {
			t.Fatalf("valid class index rejected: %v", err)
		}
		_, err = obj.getConstantInfo(0)
		if err == nil {
			t.Fatal("getConstantInfo(0) succeeded")
		}
		_, err = obj.getConstantInfo(3)
		if err == nil {
			t.Fatal("getConstantInfo hole succeeded")
		}
	})
}

func TestTaskT06C04CodeAndHandler(t *testing.T) {
	t.Run("T06-C04", func(t *testing.T) {
		if err := validateCodeLength(0); err == nil {
			t.Fatal("code_length 0 accepted")
		}
		if err := validateCodeLength(4); err != nil {
			t.Fatalf("code_length 4 rejected: %v", err)
		}
		codeLen := 8
		valid := []*ExceptionTableEntry{{StartPc: 0, EndPc: 8, HandlerPc: 4, CatchType: 0}}
		if err := validateExceptionTable(codeLen, valid); err != nil {
			t.Fatalf("valid handler rejected: %v", err)
		}
		if err := validateExceptionTable(codeLen, []*ExceptionTableEntry{{StartPc: 4, EndPc: 4, HandlerPc: 0, CatchType: 0}}); err == nil {
			t.Fatal("start>=end accepted")
		}
		if err := validateExceptionTable(codeLen, []*ExceptionTableEntry{{StartPc: 0, EndPc: 9, HandlerPc: 0, CatchType: 0}}); err == nil {
			t.Fatal("end>code_length accepted")
		}
		if err := validateExceptionTable(codeLen, []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 8, CatchType: 0}}); err == nil {
			t.Fatal("handler_pc==code_length accepted")
		}
	})
}

func TestTaskT06C05UnknownAttributes(t *testing.T) {
	t.Run("T06-C05", func(t *testing.T) {
		body := []byte{9, 8, 7, 6}
		r := NewClassReader(body)
		u := &UnparsedAttribute{Name: "X", Length: 4}
		cp := &ClassParser{reader: r, classObj: NewClassObject()}
		t06NoPanic(t, func() { u.readInfo(cp) })
		if r.Err() != nil || !bytes.Equal(u.Info, body) {
			t.Fatalf("legal unknown attr not preserved: info=%v err=%v", u.Info, r.Err())
		}

		r = NewClassReader([]byte{1, 2})
		r.SetStage("T06-C05", "unknown")
		u = &UnparsedAttribute{Name: "X", Length: 8}
		cp = &ClassParser{reader: r, classObj: NewClassObject()}
		t06NoPanic(t, func() { u.readInfo(cp) })
		if r.Err() == nil {
			t.Fatal("overlong unknown attr did not fail")
		}
		if r.Offset() > r.Bound() {
			t.Fatalf("cursor crossed bound: %d > %d", r.Offset(), r.Bound())
		}
	})
}

func TestTaskT06C06DuplicateAndPlacement(t *testing.T) {
	t.Run("T06-C06", func(t *testing.T) {
		if !attrMustBeUnique("SourceFile") || !attrMustBeUnique("Code") {
			t.Fatal("SourceFile/Code must be unique in their table")
		}
		if attrMustBeUnique("LineNumberTable") || attrMustBeUnique("LocalVariableTable") {
			t.Fatal("blanket duplicate reject of LineNumberTable")
		}
		if attrMustBeUnique("CustomThing") {
			t.Fatal("unknown attributes must be allowed to repeat")
		}
		if !attrAllowedIn("Code", attrCtxMethod) || attrAllowedIn("Code", attrCtxField) {
			t.Fatal("Code placement")
		}
		if !attrAllowedIn("ConstantValue", attrCtxField) || attrAllowedIn("ConstantValue", attrCtxMethod) {
			t.Fatal("ConstantValue placement")
		}
		if !attrAllowedIn("LineNumberTable", attrCtxCode) || attrAllowedIn("LineNumberTable", attrCtxClass) {
			t.Fatal("LineNumberTable placement")
		}
		if !attrAllowedIn("SourceFile", attrCtxClass) || attrAllowedIn("SourceFile", attrCtxMethod) {
			t.Fatal("SourceFile placement")
		}

		raw := t06ValidClass(t)
		if _, err := Parse(raw); err != nil {
			t.Fatalf("valid control rejected: %v", err)
		}
		longTest, err := classes.FS.ReadFile("LongTest.class")
		if err != nil {
			t.Fatalf("infrastructure LongTest: %v", err)
		}
		if _, err := Parse(longTest); err != nil {
			t.Fatalf("LongTest control rejected: %v", err)
		}
	})
}
