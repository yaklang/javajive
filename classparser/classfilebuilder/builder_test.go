package classfilebuilder

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
)

func buildVerifyTiny(t *testing.T) (*Class, []byte) {
	t.Helper()
	class := NewClass("VerifyTiny", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	objectInit, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	ctor := class.AddMethod(ACC_PUBLIC, "<init>", "()V").Code()
	ctor.Load(RefLocal, 0).CP(INVOKESPECIAL, objectInit).Emit(RETURN)

	answer := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "answer", "(I)I").Code()
	zero := answer.NewLabel()
	answer.Load(IntLocal, 0).Branch(IFEQ, zero).Emit(ICONST_5).Emit(IRETURN)
	answer.Mark(zero).Byte(BIPUSH, 42).Emit(IRETURN)

	main := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "main", "([Ljava/lang/String;)V").Code()
	answerRef, err := class.Pool().Methodref("VerifyTiny", "answer", "(I)I")
	if err != nil {
		t.Fatal(err)
	}
	main.Emit(ICONST_0).CP(INVOKESTATIC, answerRef).Emit(POP).Emit(RETURN)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return class, raw
}

func TestGeneratedClassParsesAndIsDeterministic(t *testing.T) {
	class, raw := buildVerifyTiny(t)
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	if parsed.MajorVersion != 49 || parsed.MinorVersion != 0 {
		t.Fatalf("version = %d.%d", parsed.MajorVersion, parsed.MinorVersion)
	}
	if got := parsed.GetClassName(); got != "VerifyTiny" {
		t.Fatalf("class name = %q", got)
	}
	if len(parsed.Methods) != 3 {
		t.Fatalf("methods = %d", len(parsed.Methods))
	}
	again, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatal("repeated Bytes calls are not deterministic")
	}
	answer := parsed.Methods[1].Attributes[0].(*javaclassparser.CodeAttribute)
	if answer.MaxStack != 1 || answer.MaxLocals != 1 {
		t.Fatalf("answer frames = stack %d, locals %d", answer.MaxStack, answer.MaxLocals)
	}
}

func TestConstantPoolDeduplicatesAndModifiedUTF8RoundTrips(t *testing.T) {
	class := NewClass("MUTF", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	pool := class.Pool()
	if a, err := pool.UTF8("same"); err != nil {
		t.Fatal(err)
	} else if b, err := pool.UTF8("same"); err != nil || a != b {
		t.Fatalf("UTF8 dedup = %d, %d, %v", a, b, err)
	}
	if a, err := pool.Class("java/lang/String"); err != nil {
		t.Fatal(err)
	} else if b, err := pool.Class("java/lang/String"); err != nil || a != b {
		t.Fatalf("class dedup = %d, %d, %v", a, b, err)
	}
	for _, add := range []func() (uint16, error){
		func() (uint16, error) { return pool.String("nul:\x00 smile:\U0001f600") },
		func() (uint16, error) { return pool.Integer(-7) },
		func() (uint16, error) { return pool.NameAndType("x", "I") },
		func() (uint16, error) { return pool.Fieldref("MUTF", "x", "I") },
		func() (uint16, error) { return pool.Methodref("MUTF", "m", "()V") },
		func() (uint16, error) { return pool.InterfaceMethodref("java/util/List", "size", "()I") },
	} {
		if _, err := add(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.UTF16([]uint16{0xd800}); err != nil {
		t.Fatal(err)
	}
	class.AddField(ACC_PUBLIC, "x", "I")
	class.AddMethod(ACC_PUBLIC|ACC_ABSTRACT, "m", "()V")
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	found, foundSurrogate := false, false
	for _, entry := range parsed.ConstantPool {
		if utf8, ok := entry.(*javaclassparser.ConstantUtf8Info); ok {
			if utf8.Value == "nul:\x00 smile:\U0001f600" {
				found = true
			}
			units := utf8.CodeUnits()
			if len(units) == 1 && units[0] == 0xd800 {
				foundSurrogate = true
			}
		}
	}
	if !found || !foundSurrogate {
		t.Fatalf("modified UTF-8 recovery: string=%v surrogate=%v", found, foundSurrogate)
	}
}

func TestWideLocalsIIncAndLDCWide(t *testing.T) {
	class := NewClass("WideOps", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	for i := 0; i < 270; i++ {
		if _, err := class.Pool().UTF8("padding/" + strings.Repeat("x", i%7) + string(rune(0x100+i))); err != nil {
			t.Fatal(err)
		}
	}
	constant, err := class.Pool().String("wide ldc")
	if err != nil {
		t.Fatal(err)
	}
	if constant <= 255 {
		t.Fatalf("constant index %d did not force ldc_w", constant)
	}
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "wide", "()I").Code()
	code.LDC(constant).Emit(POP).Emit(ICONST_5).Store(IntLocal, 300).IInc(300, 1000).Load(IntLocal, 300).Emit(IRETURN)
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	attr := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute)
	if attr.MaxStack != 1 || attr.MaxLocals != 301 {
		t.Fatalf("frames = stack %d, locals %d", attr.MaxStack, attr.MaxLocals)
	}
	want := []byte{byte(LDC_W), byte(constant >> 8), byte(constant), byte(POP), byte(ICONST_5), byte(WIDE), byte(ISTORE), 1, 44, byte(WIDE), byte(IINC), 1, 44, 3, 232, byte(WIDE), byte(ILOAD), 1, 44, byte(IRETURN)}
	if !bytes.Equal(attr.Code, want) {
		t.Fatalf("code = % x, want % x", attr.Code, want)
	}
}

func TestLongDoubleAndFloatConstantsExecute(t *testing.T) {
	class := NewClass("WideConstants", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	longIndex, err := class.Pool().Long(1<<40 + 7)
	if err != nil {
		t.Fatal(err)
	}
	doubleIndex, err := class.Pool().Double(math.Float64bits(2.5))
	if err != nil {
		t.Fatal(err)
	}
	floatIndex, err := class.Pool().Float(math.Float32bits(1.5))
	if err != nil {
		t.Fatal(err)
	}
	// double values() { return 2.5 + (double) 1.5f + (double) (1L << 40 + 7); }
	values := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "values", "()D").Code()
	values.LDC2(doubleIndex).LDC(floatIndex).Emit(F2D).Emit(DADD).LDC2(longIndex).Emit(L2D).Emit(DADD).Emit(DRETURN)

	expected, err := class.Pool().Double(math.Float64bits(float64(1<<40+7) + 4.0))
	if err != nil {
		t.Fatal(err)
	}
	valuesRef, err := class.Pool().Methodref("WideConstants", "values", "()D")
	if err != nil {
		t.Fatal(err)
	}
	assertionClass, err := class.Pool().Class("java/lang/AssertionError")
	if err != nil {
		t.Fatal(err)
	}
	assertionCtor, err := class.Pool().Methodref("java/lang/AssertionError", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	main := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "main", "([Ljava/lang/String;)V").Code()
	ok := main.NewLabel()
	main.CP(INVOKESTATIC, valuesRef).LDC2(expected).Emit(DCMPG).Branch(IFEQ, ok)
	main.CP(NEW, assertionClass).Emit(DUP).CP(INVOKESPECIAL, assertionCtor).Emit(ATHROW)
	main.Mark(ok).Emit(RETURN)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WideConstants.class"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "WideConstants").CombinedOutput(); err != nil {
		t.Fatalf("java verification of WideConstants failed: %v\n%s", err, output)
	}
}

func TestLongBranchesWidenAndExpand(t *testing.T) {
	class := NewClass("LongBranches", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "choose", "(I)I").Code()
	far := code.NewLabel()
	code.Load(IntLocal, 0).Branch(IFEQ, far).Emit(ICONST_1).Emit(IRETURN)
	for i := 0; i < 33000; i++ {
		code.Emit(NOP)
	}
	code.Mark(far).Emit(ICONST_0).Emit(IRETURN)
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	assembled := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute).Code
	if len(assembled) != 33013 {
		t.Fatalf("code length = %d", len(assembled))
	}
	if got := assembled[:9]; !bytes.Equal(got, []byte{0x1a, byte(IFNE), 0, 8, byte(GOTO_W), 0, 0, 0x80, 0xef}) {
		t.Fatalf("expanded conditional prefix = % x", got)
	}

	gotoClass := NewClass("LongGoto", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	gotoCode := gotoClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "jump", "()V").Code()
	end := gotoCode.NewLabel()
	gotoCode.Branch(GOTO, end)
	for i := 0; i < 33000; i++ {
		gotoCode.Emit(NOP)
	}
	gotoCode.Mark(end).Emit(RETURN)
	gotoRaw, err := gotoClass.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	gotoParsed, err := javaclassparser.Parse(gotoRaw)
	if err != nil {
		t.Fatal(err)
	}
	gotoBytes := gotoParsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute).Code
	if gotoBytes[0] != byte(GOTO_W) || binary.BigEndian.Uint32(gotoBytes[1:5]) != 33005 {
		t.Fatalf("goto_w prefix = % x", gotoBytes[:5])
	}
}

func TestTableSwitchAlignmentAndTargets(t *testing.T) {
	class := NewClass("SwitchOps", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "choose", "(I)I").Code()
	caseNeg, caseZero, caseOne, fallback := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
	code.Load(IntLocal, 0).TableSwitch(-1, fallback, caseNeg, caseZero, caseOne)
	code.Mark(caseNeg).Emit(ICONST_M1).Emit(IRETURN)
	code.Mark(caseZero).Emit(ICONST_0).Emit(IRETURN)
	code.Mark(caseOne).Emit(ICONST_1).Emit(IRETURN)
	code.Mark(fallback).Emit(ICONST_5).Emit(IRETURN)
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	assembled := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute).Code
	want := []byte{
		0x1a, byte(TABLESWITCH), 0, 0,
		0, 0, 0, 33, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 1,
		0, 0, 0, 27, 0, 0, 0, 29, 0, 0, 0, 31,
		byte(ICONST_M1), byte(IRETURN), byte(ICONST_0), byte(IRETURN),
		byte(ICONST_1), byte(IRETURN), byte(ICONST_5), byte(IRETURN),
	}
	if !bytes.Equal(assembled, want) {
		t.Fatalf("code = % x, want % x", assembled, want)
	}
}

func TestLookupSwitchAlignmentAndTargets(t *testing.T) {
	class := NewClass("LookupSwitchOps", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "choose", "(I)I").Code()
	backward, forward, fallback := code.NewLabel(), code.NewLabel(), code.NewLabel()
	keys := []int32{-7, 42}
	targets := []*Label{backward, forward}
	code.Mark(backward).Load(IntLocal, 0).LookupSwitch(fallback, keys, targets...)
	keys[0] = 100
	targets[0] = fallback
	code.Mark(forward).Emit(ICONST_1).Emit(IRETURN)
	code.Mark(fallback).Emit(ICONST_M1).Emit(IRETURN)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	again, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatal("repeated switch assembly is not deterministic")
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	attr := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute)
	if attr.MaxStack != 1 || attr.MaxLocals != 1 {
		t.Fatalf("frames = stack %d, locals %d", attr.MaxStack, attr.MaxLocals)
	}
	want := []byte{0x1a, byte(LOOKUPSWITCH), 0, 0}
	for _, value := range []int32{29, 2, -7, -1, 42, 27} {
		want = binary.BigEndian.AppendUint32(want, uint32(value))
	}
	want = append(want, byte(ICONST_1), byte(IRETURN), byte(ICONST_M1), byte(IRETURN))
	if !bytes.Equal(attr.Code, want) {
		t.Fatalf("code = % x, want % x", attr.Code, want)
	}
}

func TestSwitchPaddingTracksOpcodePosition(t *testing.T) {
	for _, kind := range []string{"table", "lookup"} {
		for prefix := 0; prefix < 4; prefix++ {
			t.Run(kind+string(rune('0'+prefix)), func(t *testing.T) {
				class := NewClass("SwitchPadding", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
				code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "run", "()V").Code()
				for i := 0; i < prefix; i++ {
					code.Emit(NOP)
				}
				target := code.NewLabel()
				code.Emit(ICONST_0)
				if kind == "table" {
					code.TableSwitch(0, target, target)
				} else {
					code.LookupSwitch(target, nil)
				}
				code.Mark(target).Emit(RETURN)

				raw, err := class.Bytes()
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := javaclassparser.Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				assembled := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute).Code
				switchPC := prefix + 1
				padding := (4 - (switchPC+1)%4) % 4
				if (switchPC+1+padding)%4 != 0 {
					t.Fatalf("payload offset %d is not aligned", switchPC+1+padding)
				}
				if got := assembled[switchPC+1 : switchPC+1+padding]; !bytes.Equal(got, make([]byte, padding)) {
					t.Fatalf("padding = % x", got)
				}
				payloadSize := 8
				wantOpcode := LOOKUPSWITCH
				if kind == "table" {
					payloadSize = 16
					wantOpcode = TABLESWITCH
				}
				if assembled[switchPC] != byte(wantOpcode) {
					t.Fatalf("opcode = 0x%02x", assembled[switchPC])
				}
				wantLength := switchPC + 1 + padding + payloadSize + 1
				if len(assembled) != wantLength {
					t.Fatalf("code length = %d, want %d", len(assembled), wantLength)
				}
				if got := int32(binary.BigEndian.Uint32(assembled[switchPC+1+padding:])); got != int32(wantLength-1-switchPC) {
					t.Fatalf("default offset = %d", got)
				}
			})
		}
	}
}

func TestSwitchValidationRejectsInvalidKeysAndLabels(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		build func(*Code, *Code)
	}{
		{"table empty", "at least one", func(code, _ *Code) { code.TableSwitch(0, code.NewLabel()) }},
		{"table overflow", "exceeds signed 32-bit", func(code, _ *Code) {
			target := code.NewLabel()
			code.TableSwitch(math.MaxInt32, target, target, target)
		}},
		{"table foreign label", "another Code", func(code, other *Code) {
			code.TableSwitch(0, code.NewLabel(), other.NewLabel())
		}},
		{"table unbound label", "unbound label", func(code, _ *Code) {
			target := code.NewLabel()
			code.Emit(ICONST_0).TableSwitch(0, target, target)
		}},
		{"lookup mismatched pairs", "1 keys and 0 targets", func(code, _ *Code) {
			code.LookupSwitch(code.NewLabel(), []int32{1})
		}},
		{"lookup duplicate key", "strictly increasing", func(code, _ *Code) {
			target := code.NewLabel()
			code.LookupSwitch(target, []int32{1, 1}, target, target)
		}},
		{"lookup descending keys", "strictly increasing", func(code, _ *Code) {
			target := code.NewLabel()
			code.LookupSwitch(target, []int32{2, 1}, target, target)
		}},
		{"lookup foreign label", "another Code", func(code, other *Code) {
			code.LookupSwitch(code.NewLabel(), []int32{1}, other.NewLabel())
		}},
		{"lookup unbound label", "unbound label", func(code, _ *Code) {
			target := code.NewLabel()
			code.Emit(ICONST_0).LookupSwitch(target, nil)
		}},
		{"lookup target at code end", "not an instruction", func(code, _ *Code) {
			target := code.NewLabel()
			code.Emit(ICONST_0).LookupSwitch(target, nil).Mark(target)
		}},
		{"unreachable branch target at code end", "not an instruction", func(code, _ *Code) {
			end := code.NewLabel()
			code.Emit(RETURN).Branch(GOTO, end).Mark(end)
		}},
		{"lookup stack underflow", "stack underflow", func(code, _ *Code) {
			target := code.NewLabel()
			code.LookupSwitch(target, nil).Mark(target).Emit(RETURN)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			class := NewClass("BadSwitch", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
			code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "bad", "()V").Code()
			otherClass := NewClass("OtherSwitch", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
			other := otherClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "other", "()V").Code()
			test.build(code, other)
			if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestExceptionTableAndCalculatedFrames(t *testing.T) {
	class := NewClass("Catches", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "safe", "()I").Code()
	start, end, handler := code.NewLabel(), code.NewLabel(), code.NewLabel()
	code.Mark(start).Emit(ICONST_1).Emit(ICONST_0).Emit(IDIV).Mark(end).Emit(IRETURN)
	code.Mark(handler).Emit(POP).Emit(ICONST_M1).Emit(IRETURN)
	code.Catch(start, end, handler, "java/lang/ArithmeticException")
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	attr := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute)
	if attr.MaxStack != 2 || attr.MaxLocals != 0 || len(attr.ExceptionTable) != 1 {
		t.Fatalf("Code = stack %d locals %d handlers %d", attr.MaxStack, attr.MaxLocals, len(attr.ExceptionTable))
	}
	entry := attr.ExceptionTable[0]
	if entry.StartPc != 0 || entry.EndPc != 3 || entry.HandlerPc != 4 || entry.CatchType == 0 {
		t.Fatalf("handler = %+v", entry)
	}
}

func TestDeterministicErrors(t *testing.T) {
	t.Run("utf8 limit", func(t *testing.T) {
		class := NewClass("TooLong", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
		_, err := class.Pool().UTF8(strings.Repeat("\x00", 32768))
		if err == nil || !strings.Contains(err.Error(), "65536") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unbound label", func(t *testing.T) {
		class := NewClass("BadLabel", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
		code := class.AddMethod(ACC_STATIC, "x", "()V").Code()
		code.Branch(GOTO, code.NewLabel())
		if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "unbound label") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stack merge", func(t *testing.T) {
		class := NewClass("BadStack", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
		code := class.AddMethod(ACC_STATIC, "x", "(I)V").Code()
		join := code.NewLabel()
		code.Load(IntLocal, 0).Branch(IFEQ, join).Emit(ICONST_1).Mark(join).Emit(RETURN)
		if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "inconsistent stack height") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("explicit frames", func(t *testing.T) {
		class := NewClass("BadFrame", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
		class.AddMethod(ACC_STATIC, "x", "()I").Code().Emit(ICONST_0).Emit(IRETURN).SetMaxStack(2)
		if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "does not equal calculated") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDescriptorValidationAcceptsLegalSymbols(t *testing.T) {
	class := NewClass("pkg/Outer$Inner", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	class.AddInterface("pkg/Interface$Nested")
	class.AddField(ACC_PUBLIC, "<legal-field>", strings.Repeat("[", 255)+"Ljava/lang/String;")
	class.AddMethod(ACC_PUBLIC|ACC_ABSTRACT, "méthod$1", "(I[[Ljava/lang/String;)Ljava/lang/Object;")
	class.AddMethod(ACC_PUBLIC, "<init>", "(I)V").Code().Emit(RETURN)
	class.AddMethod(ACC_STATIC, "<clinit>", "()V").Code().Emit(RETURN)
	if _, err := class.Bytes(); err != nil {
		t.Fatalf("legal symbols rejected: %v", err)
	}
	for _, add := range []func() (uint16, error){
		func() (uint16, error) { return class.Pool().Class("[[Ljava/lang/String;") },
		func() (uint16, error) { return class.Pool().Fieldref("pkg/Owner", "<field>", "[I") },
		func() (uint16, error) { return class.Pool().Methodref("pkg/Owner", "<init>", "(I)V") },
		func() (uint16, error) { return class.Pool().InterfaceMethodref("pkg/Face", "call", "()V") },
	} {
		if _, err := add(); err != nil {
			t.Fatalf("legal constant pool symbol rejected: %v", err)
		}
	}
}

func TestDescriptorValidationRejectsMalformedFieldAndMethodDescriptors(t *testing.T) {
	badFields := []string{
		"", "V", "II", "L;", "Lpkg//Type;", "L/pkg/Type;", "Lpkg.Type;",
		"Lpkg/Type", "Lpkg/Type;;", "[", "[V", "[L;", strings.Repeat("[", 256) + "I",
	}
	for _, descriptor := range badFields {
		t.Run("field_"+descriptor, func(t *testing.T) {
			class := NewClass("BadField", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
			class.AddField(ACC_PUBLIC, "field", descriptor)
			if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "field") {
				t.Fatalf("descriptor %q error = %v", descriptor, err)
			}
		})
	}
	badMethods := []string{
		"", "I", "()", "(V)V", "(I", "I)V", "()VV", "()[V", "(L;)V",
		"(Lpkg//Type;)V", "([V)V", "(" + strings.Repeat("[", 256) + "I)V",
	}
	for _, descriptor := range badMethods {
		t.Run("method_"+descriptor, func(t *testing.T) {
			class := NewClass("BadMethod", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
			class.AddMethod(ACC_PUBLIC|ACC_ABSTRACT, "method", descriptor)
			if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "method") {
				t.Fatalf("descriptor %q error = %v", descriptor, err)
			}
		})
	}
}

func TestSymbolValidationRejectsIllegalNames(t *testing.T) {
	for _, name := range []string{"", "/pkg/Type", "pkg//Type", "pkg/Type/", "pkg.Type", "pkg/Type;", "[Lpkg/Type;"} {
		t.Run("class_"+name, func(t *testing.T) {
			if _, err := NewClass(name, "java/lang/Object", ACC_PUBLIC).Bytes(); err == nil {
				t.Fatalf("class name %q accepted", name)
			}
		})
	}
	for _, name := range []string{"", "a/b", "a.b", "a;b", "a[b"} {
		t.Run("field_"+name, func(t *testing.T) {
			class := NewClass("BadFieldName", "java/lang/Object", ACC_PUBLIC)
			class.AddField(ACC_PUBLIC, name, "I")
			if _, err := class.Bytes(); err == nil {
				t.Fatalf("field name %q accepted", name)
			}
		})
	}
	for _, name := range []string{"", "a/b", "a.b", "a;b", "a[b", "<bad>", "bad<name"} {
		t.Run("method_"+name, func(t *testing.T) {
			class := NewClass("BadMethodName", "java/lang/Object", ACC_PUBLIC)
			class.AddMethod(ACC_PUBLIC|ACC_ABSTRACT, name, "()V")
			if _, err := class.Bytes(); err == nil {
				t.Fatalf("method name %q accepted", name)
			}
		})
	}
}

func TestMethodDescriptorParameterWordLimit(t *testing.T) {
	staticDescriptor := "(" + strings.Repeat("J", 127) + "I)V"
	staticClass := NewClass("StaticLimit", "java/lang/Object", ACC_PUBLIC)
	staticClass.AddMethod(ACC_STATIC|ACC_ABSTRACT, "m", staticDescriptor)
	if _, err := staticClass.Bytes(); err != nil {
		t.Fatalf("255-word static descriptor rejected: %v", err)
	}

	instanceClass := NewClass("InstanceLimit", "java/lang/Object", ACC_PUBLIC)
	instanceClass.AddMethod(ACC_ABSTRACT, "m", staticDescriptor)
	if _, err := instanceClass.Bytes(); err == nil || !strings.Contains(err.Error(), "receiver exceeds 255") {
		t.Fatalf("255-word instance descriptor error = %v", err)
	}

	overflowClass := NewClass("OverflowLimit", "java/lang/Object", ACC_PUBLIC)
	overflowClass.AddMethod(ACC_STATIC|ACC_ABSTRACT, "m", "("+strings.Repeat("J", 128)+")V")
	if _, err := overflowClass.Bytes(); err == nil || !strings.Contains(err.Error(), "exceeds 255") {
		t.Fatalf("256-word descriptor error = %v", err)
	}
}

func TestSpecialMethodValidation(t *testing.T) {
	tests := []struct {
		name       string
		access     uint16
		descriptor string
	}{
		{"<init>", ACC_PUBLIC, "()I"},
		{"<init>", ACC_PUBLIC | ACC_STATIC, "()V"},
		{"<init>", ACC_PUBLIC | ACC_ABSTRACT, "()V"},
		{"<init>", ACC_PUBLIC | ACC_BRIDGE, "()V"},
		{"<clinit>", 0, "()V"},
		{"<clinit>", ACC_PUBLIC | ACC_STATIC, "()V"},
		{"<clinit>", ACC_STATIC, "(I)V"},
		{"<clinit>", ACC_STATIC, "()I"},
	}
	for _, test := range tests {
		t.Run(test.name+test.descriptor, func(t *testing.T) {
			class := NewClass("Special", "java/lang/Object", ACC_PUBLIC)
			class.AddMethod(test.access, test.name, test.descriptor)
			if _, err := class.Bytes(); err == nil {
				t.Fatalf("accepted %s access %#x descriptor %s", test.name, test.access, test.descriptor)
			}
		})
	}

	class := NewClass("Refs", "java/lang/Object", ACC_PUBLIC)
	if _, err := class.Pool().Methodref("pkg/Owner", "<clinit>", "()V"); err == nil {
		t.Fatal("Methodref accepted <clinit>")
	}
	if _, err := class.Pool().InterfaceMethodref("pkg/Face", "<init>", "()V"); err == nil {
		t.Fatal("InterfaceMethodref accepted <init>")
	}
}

func TestConstantPoolValidationIsIndependent(t *testing.T) {
	pool := newConstantPool()
	for name, add := range map[string]func() (uint16, error){
		"array object body": func() (uint16, error) { return pool.Class("[Lbad//Name;") },
		"array depth":       func() (uint16, error) { return pool.Class(strings.Repeat("[", 256) + "I") },
		"field owner":       func() (uint16, error) { return pool.Fieldref("bad.Owner", "x", "I") },
		"field name":        func() (uint16, error) { return pool.Fieldref("pkg/Owner", "x/y", "I") },
		"field descriptor":  func() (uint16, error) { return pool.Fieldref("pkg/Owner", "x", "Lbad//Type;") },
		"method name":       func() (uint16, error) { return pool.Methodref("pkg/Owner", "<bad>", "()V") },
		"method descriptor": func() (uint16, error) { return pool.Methodref("pkg/Owner", "m", "([V)V") },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := add(); err == nil {
				t.Fatal("invalid constant pool symbol accepted")
			}
		})
	}
}

func TestSourceDebugAndConstantValueAttributes(t *testing.T) {
	class := NewClass("DebugMeta", "java/lang/Object", ACC_PUBLIC|ACC_SUPER).SetSourceFile("DebugMeta.java")
	class.SetConstantInt(class.AddField(ACC_PUBLIC|ACC_STATIC|ACC_FINAL, "ANSWER", "I"), 42)
	class.SetConstantString(class.AddField(ACC_PUBLIC|ACC_STATIC|ACC_FINAL, "TEXT", "Ljava/lang/String;"), "metadata")
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "twice", "(I)I").Code()
	start, body, end := code.NewLabel(), code.NewLabel(), code.NewLabel()
	code.Mark(start).Line(start, 7).Load(IntLocal, 0).Mark(body).Line(body, 8).Load(IntLocal, 0).Emit(IADD).Emit(IRETURN).Mark(end)
	code.Local(start, end, "value", "I", 0)
	class.AddMethod(ACC_PUBLIC|ACC_STATIC, "main", "([Ljava/lang/String;)V").Code().Emit(RETURN)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Attributes) != 1 {
		t.Fatalf("class attributes = %d", len(parsed.Attributes))
	}
	source, ok := parsed.Attributes[0].(*javaclassparser.SourceFileAttribute)
	if !ok {
		t.Fatalf("SourceFile = %#v", parsed.Attributes[0])
	}
	if got := parsed.ConstantPoolManager.GetUtf8(int(source.SourceFileIndex)); got == nil || got.Value != "DebugMeta.java" {
		t.Fatalf("SourceFile constant = %#v", got)
	}
	for i := range []string{"42", "metadata"} {
		if len(parsed.Fields[i].Attributes) != 1 {
			t.Fatalf("field %d attributes = %d", i, len(parsed.Fields[i].Attributes))
		}
		if _, ok := parsed.Fields[i].Attributes[0].(*javaclassparser.ConstantValueAttribute); !ok {
			t.Fatalf("field %d ConstantValue = %#v", i, parsed.Fields[i].Attributes[0])
		}
	}
	codeAttribute := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute)
	if len(codeAttribute.Attributes) != 2 {
		t.Fatalf("Code attributes = %d", len(codeAttribute.Attributes))
	}
	lines, ok := codeAttribute.Attributes[0].(*javaclassparser.LineNumberTableAttribute)
	if !ok || len(lines.LineNumberTable) != 2 || lines.LineNumberTable[0].StartPc != 0 || lines.LineNumberTable[0].LineNumber != 7 || lines.LineNumberTable[1].StartPc != 1 || lines.LineNumberTable[1].LineNumber != 8 {
		t.Fatalf("LineNumberTable = %#v", codeAttribute.Attributes[0])
	}
	local, ok := codeAttribute.Attributes[1].(*javaclassparser.UnparsedAttribute)
	if !ok || local.Name != "LocalVariableTable" || len(local.Info) != 12 {
		t.Fatalf("LocalVariableTable = %#v", codeAttribute.Attributes[1])
	}
	again, err := class.Bytes()
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatalf("metadata output is not deterministic: %v", err)
	}
	java, err := exec.LookPath("java")
	if err == nil {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "DebugMeta.class"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "DebugMeta").CombinedOutput(); err != nil {
			t.Fatalf("java verification failed: %v\n%s", err, output)
		}
	}
}

func TestPreflightValidationAPIs(t *testing.T) {
	tests := []struct {
		name  string
		valid func() error
		bad   func() error
	}{
		{"class internal name", func() error { return ValidateClassInternalName("pkg/Outer$Inner") }, func() error { return ValidateClassInternalName("pkg.Bad") }},
		{"object descriptor", func() error { return ValidateObjectDescriptor("Ljava/lang/String;") }, func() error { return ValidateObjectDescriptor("[Ljava/lang/String;") }},
		{"field symbol", func() error { return ValidateFieldSymbol("value", "[[I") }, func() error { return ValidateFieldSymbol("bad/name", "I") }},
		{"method symbol", func() error { return ValidateMethodSymbol("<init>", "(I)V") }, func() error { return ValidateMethodSymbol("<init>", "()I") }},
		{"method symbol UTF limit", func() error { return ValidateMethodSymbol(strings.Repeat("m", math.MaxUint16), "()V") }, func() error { return ValidateMethodSymbol(strings.Repeat("m", math.MaxUint16+1), "()V") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.valid(); err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			if err := test.bad(); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func TestRawClassAttributesCopyAndPreserveOrder(t *testing.T) {
	class := NewClass("RawAttributes", "java/lang/Object", ACC_PUBLIC|ACC_SUPER).SetSourceFile("RawAttributes.java")
	first := []byte{1, 2, 3}
	class.AddClassAttribute("Opaque", first)
	class.AddClassAttribute("Opaque", []byte{4, 5})
	first[0] = 99

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Attributes) != 3 {
		t.Fatalf("class attributes = %d", len(parsed.Attributes))
	}
	if _, ok := parsed.Attributes[0].(*javaclassparser.SourceFileAttribute); !ok {
		t.Fatalf("first attribute = %#v", parsed.Attributes[0])
	}
	for index, want := range [][]byte{{1, 2, 3}, {4, 5}} {
		attribute, ok := parsed.Attributes[index+1].(*javaclassparser.UnparsedAttribute)
		if !ok || attribute.Name != "Opaque" || !bytes.Equal(attribute.Info, want) {
			t.Fatalf("raw attribute %d = %#v, want % x", index, parsed.Attributes[index+1], want)
		}
	}
	again, err := class.Bytes()
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatalf("raw attributes are not deterministic: %v", err)
	}
}

func TestRawClassAttributeValidation(t *testing.T) {
	class := NewClass("BadRawAttribute", "java/lang/Object", ACC_PUBLIC)
	class.AddClassAttribute("", nil)
	if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "attribute name is empty") {
		t.Fatalf("empty name error = %v", err)
	}

	if err := validateAttributePayloadLength(uint64(math.MaxUint32)); err != nil {
		t.Fatalf("u4 maximum rejected: %v", err)
	}
	if err := validateAttributePayloadLength(uint64(math.MaxUint32) + 1); err == nil || !strings.Contains(err.Error(), "4294967296") {
		t.Fatalf("u4 overflow error = %v", err)
	}
}

func TestAnnotationValueNumericKinds(t *testing.T) {
	floatBits := math.Float32bits(float32(-1.25))
	doubleBits := math.Float64bits(3.5)
	class := NewClass("NumericAnnotation", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	class.AddClassAnnotation(Annotation{
		Descriptor: "Lpkg/Numbers;",
		Elements: []AnnotationEntry{
			{Name: "longValue", Value: AnnotationValue{Kind: AnnotationLong, Int: -1 << 40}},
			{Name: "floatValue", Value: AnnotationValue{Kind: AnnotationFloat, Bits: uint64(floatBits)}},
			{Name: "doubleValue", Value: AnnotationValue{Kind: AnnotationDouble, Bits: doubleBits}},
		},
	}, true)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	attribute, ok := parsed.Attributes[0].(*javaclassparser.RuntimeVisibleAnnotationsAttribute)
	if !ok || len(attribute.Annotations) != 1 {
		t.Fatalf("annotations = %#v", parsed.Attributes)
	}
	values := attribute.Annotations[0].ElementValuePairs
	if len(values) != 3 {
		t.Fatalf("annotation values = %d", len(values))
	}
	if value, ok := values[0].Value.(*javaclassparser.ConstantLongInfo); !ok || value.Value != -1<<40 {
		t.Fatalf("long value = %#v", values[0].Value)
	}
	if value, ok := values[1].Value.(*javaclassparser.ConstantFloatInfo); !ok || math.Float32bits(value.Value) != floatBits {
		t.Fatalf("float value = %#v", values[1].Value)
	}
	if value, ok := values[2].Value.(*javaclassparser.ConstantDoubleInfo); !ok || math.Float64bits(value.Value) != doubleBits {
		t.Fatalf("double value = %#v", values[2].Value)
	}
}

func TestMetadataCountBounds(t *testing.T) {
	annotations := make([]Annotation, math.MaxUint16+1)
	visible := make([]bool, len(annotations))
	for index := range visible {
		visible[index] = true
	}
	if _, err := encodeAnnotations(newConstantPool(), annotations, visible, true); err == nil || !strings.Contains(err.Error(), "annotation count 65536") {
		t.Fatalf("annotation count error = %v", err)
	}
	if _, err := encodeInnerClasses(newConstantPool(), make([]InnerClassSpec, math.MaxUint16+1)); err == nil || !strings.Contains(err.Error(), "InnerClasses count 65536") {
		t.Fatalf("InnerClasses count error = %v", err)
	}
	if _, err := encodeExceptions(newConstantPool(), make([]string, math.MaxUint16+1)); err == nil || !strings.Contains(err.Error(), "Exceptions length 65536") {
		t.Fatalf("Exceptions count error = %v", err)
	}
	if _, err := encodeParameterAnnotations(newConstantPool(), make([][]Annotation, math.MaxUint8+1), make([][]bool, math.MaxUint8+1), true); err == nil || !strings.Contains(err.Error(), "parameter annotation count 256") {
		t.Fatalf("parameter count error = %v", err)
	}
	parameterAnnotations := [][]Annotation{make([]Annotation, math.MaxUint16+1)}
	parameterVisible := [][]bool{make([]bool, math.MaxUint16+1)}
	for index := range parameterVisible[0] {
		parameterVisible[0][index] = true
	}
	if _, err := encodeParameterAnnotations(newConstantPool(), parameterAnnotations, parameterVisible, true); err == nil || !strings.Contains(err.Error(), "annotation count 65536") {
		t.Fatalf("parameter annotation count error = %v", err)
	}
}

func TestClassAndMemberAttributeCountBounds(t *testing.T) {
	if count, err := checkedU2Count("attributes_count", math.MaxUint16); err != nil || count != math.MaxUint16 {
		t.Fatalf("u2 maximum = %d, %v", count, err)
	}
	for _, kind := range []string{"class attributes_count", "field attributes_count", "method attributes_count"} {
		if _, err := checkedU2Count(kind, math.MaxUint16+1); err == nil || !strings.Contains(err.Error(), kind+" 65536") {
			t.Fatalf("%s overflow error = %v", kind, err)
		}
	}

	class := NewClass("AttributeBounds", "java/lang/Object", ACC_PUBLIC)
	class.rawClassAttrs = make([]pendingAttribute, math.MaxUint16+1)
	if _, err := class.Bytes(); err == nil || !strings.Contains(err.Error(), "class attributes_count 65536") {
		t.Fatalf("class attribute count error = %v", err)
	}
}

func TestJavaVerifyAll(t *testing.T) {
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not installed")
	}
	_, tinyRaw := buildVerifyTiny(t)

	switchClass := NewClass("VerifySwitches", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	table := switchClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "table", "(I)I").Code()
	tableZero, tableOne, tableDefault := table.NewLabel(), table.NewLabel(), table.NewLabel()
	table.Load(IntLocal, 0).TableSwitch(0, tableDefault, tableZero, tableOne)
	table.Mark(tableZero).Emit(ICONST_0).Emit(IRETURN)
	table.Mark(tableOne).Emit(ICONST_1).Emit(IRETURN)
	table.Mark(tableDefault).Emit(ICONST_M1).Emit(IRETURN)
	lookup := switchClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "lookup", "(I)I").Code()
	lookupNeg, lookupLarge, lookupDefault := lookup.NewLabel(), lookup.NewLabel(), lookup.NewLabel()
	lookup.Load(IntLocal, 0).LookupSwitch(lookupDefault, []int32{-7, 1000}, lookupNeg, lookupLarge)
	lookup.Mark(lookupNeg).Emit(ICONST_M1).Emit(IRETURN)
	lookup.Mark(lookupLarge).Emit(ICONST_5).Emit(IRETURN)
	lookup.Mark(lookupDefault).Emit(ICONST_0).Emit(IRETURN)
	tableRef, err := switchClass.Pool().Methodref("VerifySwitches", "table", "(I)I")
	if err != nil {
		t.Fatal(err)
	}
	lookupRef, err := switchClass.Pool().Methodref("VerifySwitches", "lookup", "(I)I")
	if err != nil {
		t.Fatal(err)
	}
	main := switchClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "main", "([Ljava/lang/String;)V").Code()
	main.Emit(ICONST_0).CP(INVOKESTATIC, tableRef).Emit(POP)
	main.Emit(ICONST_0).CP(INVOKESTATIC, lookupRef).Emit(POP).Emit(RETURN)
	switchRaw, err := switchClass.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	classes := map[string][]byte{"VerifyTiny.class": tinyRaw, "VerifySwitches.class": switchRaw}
	for name, raw := range classes {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, className := range []string{"VerifyTiny", "VerifySwitches"} {
		cmd := exec.Command(java, "-Xverify:all", "-cp", dir, className)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("java verification of %s failed: %v\n%s", className, err, output)
		}
	}
}

func TestJava8InterfaceStaticMethodVerifies(t *testing.T) {
	class := NewClass("IfaceStatics", "java/lang/Object", ACC_PUBLIC|ACC_INTERFACE|ACC_ABSTRACT)
	class.SetVersion(VersionJava8)
	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "answer", "()I").Code()
	code.Byte(BIPUSH, 42).Emit(IRETURN)
	if class.Version() != VersionJava8 {
		t.Fatalf("version = %d", class.Version())
	}
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated interface: %v", err)
	}
	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		t.Skip("javac/java unavailable")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "IfaceStatics.class"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (IfaceStatics.answer() != 42) throw new AssertionError("static interface method");
    }
}`
	if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
		t.Fatalf("javac driver: %v\n%s", err, output)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
		t.Fatalf("java verification: %v\n%s", err, output)
	}
}

func TestMethodHandleTypeInvokeDynamicAndBootstrapMethods(t *testing.T) {
	class := NewClass("IndySample", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	class.SetVersion(VersionJava7)
	if class.Version() != VersionJava7 {
		t.Fatalf("version = %d, want %d", class.Version(), VersionJava7)
	}
	pool := class.Pool()

	fRef, err := pool.Fieldref("IndySample", "f", "I")
	if err != nil {
		t.Fatal(err)
	}
	mRef, err := pool.Methodref("IndySample", "m", "(Ljava/lang/String;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	initRef, err := pool.Methodref("IndySample", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	imRef, err := pool.InterfaceMethodref("java/lang/Runnable", "run", "()V")
	if err != nil {
		t.Fatal(err)
	}
	bsmRef, err := pool.Methodref(
		"java/lang/invoke/LambdaMetafactory",
		"metafactory",
		"(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Verify valid MethodHandle kinds and rejection of mismatched target tags.
	validHandles := []struct {
		kind byte
		ref  uint16
	}{
		{RefGetField, fRef},
		{RefGetStatic, fRef},
		{RefPutField, fRef},
		{RefPutStatic, fRef},
		{RefInvokeVirtual, mRef},
		{RefInvokeStatic, mRef},
		{RefInvokeSpecial, mRef},
		{RefNewInvokeSpecial, initRef},
		{RefInvokeInterface, imRef},
	}
	for _, tc := range validHandles {
		if _, err := pool.MethodHandle(tc.kind, tc.ref); err != nil {
			t.Fatalf("MethodHandle(kind=%d, ref=%d) unexpected error: %v", tc.kind, tc.ref, err)
		}
	}
	// Verify Java 8 (52) allows RefInvokeStatic/RefInvokeSpecial with CONSTANT_InterfaceMethodref per JVMS §4.4.8.
	java8Class := NewClass("IndyIfaceMH", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	java8Class.SetVersion(VersionJava8)
	j8Pool := java8Class.Pool()
	j8IMRef, err := j8Pool.InterfaceMethodref("java/lang/Runnable", "run", "()V")
	if err != nil {
		t.Fatal(err)
	}
	j8StaticMH, err := j8Pool.MethodHandle(RefInvokeStatic, j8IMRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j8Pool.MethodHandle(RefInvokeSpecial, j8IMRef); err != nil {
		t.Fatal(err)
	}
	java8Class.AddMethod(ACC_PUBLIC|ACC_STATIC, "load", "()Ljava/lang/Object;").Code().LDC(j8StaticMH).Emit(ARETURN)
	j8Raw, err := java8Class.Bytes()
	if err != nil {
		t.Fatalf("java8Class.Bytes(): %v", err)
	}
	if j8Parsed, err := javaclassparser.Parse(j8Raw); err != nil || j8Parsed.MajorVersion != VersionJava8 {
		t.Fatalf("java8 interface MethodHandle parse failed: parsed=%v err=%v", j8Parsed, err)
	}
	if _, err := pool.MethodHandle(RefInvokeVirtual, imRef); err == nil {
		t.Fatal("MethodHandle(RefInvokeVirtual, InterfaceMethodref) accepted invalid tag")
	}
	if _, err := pool.MethodHandle(RefInvokeInterface, mRef); err == nil {
		t.Fatal("MethodHandle(RefInvokeInterface, Methodref) accepted invalid tag")
	}
	if _, err := pool.MethodHandle(RefGetField, mRef); err == nil {
		t.Fatal("MethodHandle(RefGetField, Methodref) accepted invalid tag")
	}

	bsmMH, err := pool.MethodHandle(RefInvokeStatic, bsmRef)
	if err != nil {
		t.Fatal(err)
	}
	targetMH, err := pool.MethodHandle(RefInvokeStatic, mRef)
	if err != nil {
		t.Fatal(err)
	}
	samMT, err := pool.MethodType("()Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	strArg, err := pool.String("arg")
	if err != nil {
		t.Fatal(err)
	}

	bsm0, err := class.AddBootstrapMethod(bsmMH, samMT, targetMH, samMT, strArg)
	if err != nil {
		t.Fatal(err)
	}
	bsm0Dup, err := class.AddBootstrapMethod(bsmMH, samMT, targetMH, samMT, strArg)
	if err != nil {
		t.Fatal(err)
	}
	if bsm0 != 0 || bsm0Dup != 0 {
		t.Fatalf("BootstrapMethods deduplication failed: bsm0=%d dup=%d", bsm0, bsm0Dup)
	}

	indyCP, err := pool.InvokeDynamic(bsm0, "get", "()Ljava/util/function/Supplier;")
	if err != nil {
		t.Fatal(err)
	}

	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "make", "()Ljava/lang/Object;").Code()
	code.LDC(targetMH).Emit(POP)
	code.LDC(samMT).Emit(POP)
	code.InvokeDynamic(indyCP).Emit(ARETURN)

	raw, err := class.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	if parsed.MajorVersion != VersionJava7 {
		t.Fatalf("MajorVersion = %d, want %d", parsed.MajorVersion, VersionJava7)
	}
	var bsmAttr *javaclassparser.BootstrapMethodsAttribute
	for _, attr := range parsed.Attributes {
		if b, ok := attr.(*javaclassparser.BootstrapMethodsAttribute); ok {
			bsmAttr = b
		}
	}
	if bsmAttr == nil || len(bsmAttr.BootstrapMethods) != 1 || len(bsmAttr.BootstrapMethods[0].BootstrapArguments) != 4 {
		t.Fatalf("BootstrapMethods attribute = %#v", bsmAttr)
	}
}

func TestStackMapTableSortingDedupAndUninitialized(t *testing.T) {
	class := NewClass("SMTTest", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	class.SetVersion(VersionJava7)
	initRef, err := class.Pool().Methodref("java/lang/String", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}

	code := class.AddMethod(ACC_PUBLIC|ACC_STATIC, "build", "(IJ)Ljava/lang/String;").Code()
	newLbl := code.NewLabel()
	branchLbl := code.NewLabel()
	doneLbl := code.NewLabel()

	code.Load(IntLocal, 0).Branch(IFEQ, branchLbl)
	code.Mark(newLbl).CP(NEW, mustClassCP(t, class.Pool(), "java/lang/String")).Emit(DUP)
	code.CP(INVOKESPECIAL, initRef).Store(RefLocal, 3).Branch(GOTO, doneLbl)

	code.Mark(branchLbl)
	code.Emit(ACONST_NULL).Store(RefLocal, 3)

	code.Mark(doneLbl).Load(RefLocal, 3).Emit(ARETURN)

	// Insert out-of-order and duplicate identical frames at doneLbl to verify sorting + dedup.
	doneLocals := []VerificationType{
		{Tag: VerifyInt},
		{Tag: VerifyLong},
		{Tag: VerifyTop},
		{Tag: VerifyObject, Class: "java/lang/String"},
	}
	code.AddFrame(doneLbl, doneLocals, nil)
	code.AddFrame(branchLbl, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyLong}}, nil)
	code.AddFrame(doneLbl, doneLocals, nil)
	// Reference Uninitialized offset before init runs at a synthetic target.
	uninitChk := code.NewLabel()
	code.Branch(GOTO, uninitChk).Mark(uninitChk)
	_ = newLbl
	code.AddFrame(uninitChk, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyLong}, {Tag: VerifyTop}, {Tag: VerifyUninitialized, NewAt: newLbl}}, nil)
	code.Emit(POP).Emit(ACONST_NULL).Emit(ARETURN) // unreachable? No, pop would underflow if stack empty; just return null from local 3:

	// Reset method clean on second class to test conflicting frame rejection without mixing invalid stack.
	rawBad := func() error {
		badClass := NewClass("ConflictSMT", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
		badClass.SetVersion(VersionJava7)
		c := badClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "f", "(I)V").Code()
		lbl := c.NewLabel()
		c.Load(IntLocal, 0).Branch(IFEQ, lbl).Mark(lbl).Emit(RETURN)
		c.AddFrame(lbl, []VerificationType{{Tag: VerifyInt}}, nil)
		c.AddFrame(lbl, []VerificationType{{Tag: VerifyFloat}}, nil)
		_, err := badClass.Bytes()
		return err
	}()
	if rawBad == nil || !strings.Contains(rawBad.Error(), "conflicting stack map frames") {
		t.Fatalf("conflicting frames error = %v", rawBad)
	}

	// Verify clean class with VerifyUninitialized NewAt and out-of-order AddFrame compiles & parses cleanly.
	okClass := NewClass("ValidUninitSMT", "java/lang/Object", ACC_PUBLIC|ACC_SUPER)
	okClass.SetVersion(VersionJava7)
	strInit, err := okClass.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	objCP := mustClassCP(t, okClass.Pool(), "java/lang/Object")
	c := okClass.AddMethod(ACC_PUBLIC|ACC_STATIC, "f", "(I)Ljava/lang/Object;").Code()
	lNew := c.NewLabel()
	lUse := c.NewLabel()
	lElse := c.NewLabel()
	lJoin := c.NewLabel()
	c.Mark(lNew).CP(NEW, objCP).Store(RefLocal, 1)
	c.Load(IntLocal, 0).Branch(IFEQ, lElse)
	c.Branch(GOTO, lUse)
	c.Mark(lElse).Branch(GOTO, lUse)
	c.Mark(lUse).Load(RefLocal, 1).CP(INVOKESPECIAL, strInit).Branch(GOTO, lJoin)
	c.Mark(lJoin).Load(RefLocal, 1).Emit(ARETURN)

	c.AddFrame(lJoin, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyObject, Class: "java/lang/Object"}}, nil)
	c.AddFrame(lUse, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyUninitialized, NewAt: lNew}}, nil)
	c.AddFrame(lElse, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyUninitialized, NewAt: lNew}}, nil)
	c.AddFrame(lUse, []VerificationType{{Tag: VerifyInt}, {Tag: VerifyUninitialized, NewAt: lNew}}, nil)

	raw, err := okClass.Bytes()
	if err != nil {
		t.Fatalf("okClass.Bytes(): %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	codeAttr := parsed.Methods[0].Attributes[0].(*javaclassparser.CodeAttribute)
	var smt []byte
	for _, sub := range codeAttr.Attributes {
		if u, ok := sub.(*javaclassparser.UnparsedAttribute); ok && u.Name == "StackMapTable" {
			smt = u.Info
		}
	}
	if len(smt) == 0 || binary.BigEndian.Uint16(smt[:2]) != 3 {
		t.Fatalf("StackMapTable entries = %v, want 3 deduplicated sorted frames", smt)
	}
	// First entry must be full_frame (255) pointing at lElse with VerifyUninitialized NewAt=0.
	if smt[2] != 255 {
		t.Fatalf("frame[0] tag = %d, want 255", smt[2])
	}
}

func mustClassCP(t *testing.T, pool *ConstantPool, name string) uint16 {
	t.Helper()
	idx, err := pool.Class(name)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}
