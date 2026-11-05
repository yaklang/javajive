package classfilebuilder

import (
	"bytes"
	"encoding/binary"
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

func TestJavaVerifyAll(t *testing.T) {
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not installed")
	}
	_, raw := buildVerifyTiny(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VerifyTiny.class"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(java, "-Xverify:all", "-cp", dir, "VerifyTiny")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("java verification failed: %v\n%s", err, output)
	}
}
