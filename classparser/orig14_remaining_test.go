package javaclassparser

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestOrig14BloomFilterBoolOrSnippet(t *testing.T) {
	in := "\tboolean put() {\n" +
		"\t\tint var2 = 0;\n\t\tvar2 = (var2) | (var1.flag());\n\t\treturn var2;\n\t}\n"
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on := fixOrig14RemainderReconstructs(in)
	if !strings.Contains(on, "boolean var2 = false;") {
		t.Fatalf("ON expected boolean var2 (renamed local), got:\n%s", on)
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	if fixOrig14RemainderReconstructs(in) != in {
		t.Fatal("OFF expected identity")
	}
}

func TestOrig14FieldWriterBareIfSnippet(t *testing.T) {
	in := "\tvoid writeValue() {\n\t\tint var3 = 0;\n\t\tif (var3){\n\t\t\tvar1.writeComma();\n"
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on := fixOrig14RemainderReconstructs(in)
	if !strings.Contains(on, "if ((var3) != (0)){") {
		t.Fatalf("ON expected int bare-if on renamed local, got:\n%s", on)
	}
}

func TestGuavaBloomFilterPutIsLoadBearing(t *testing.T) {
	outer, err := os.ReadFile("testdata/regression/BloomFilterStrategies.class")
	if err != nil {
		t.Fatal(err)
	}
	inner1, err := os.ReadFile("testdata/regression/BloomFilterStrategies$1.class")
	if err != nil {
		t.Fatal(err)
	}
	inner2, err := os.ReadFile("testdata/regression/BloomFilterStrategies$2.class")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name string) ([]byte, bool) {
		switch {
		case strings.Contains(name, "BloomFilterStrategies$1"):
			return inner1, true
		case strings.Contains(name, "BloomFilterStrategies$2"):
			return inner2, true
		default:
			return nil, false
		}
	}
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on, err := DecompileWithResolver(outer, resolve)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "boolean var9 = false;") {
		t.Fatalf("ON missing boolean var9:\n%s", clipForTest(on, "var9"))
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := DecompileWithResolver(outer, resolve)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "int var9 = 0;") {
		t.Fatalf("OFF missing int var9:\n%s", clipForTest(off, "var9"))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}

func TestSpringReflectUtilsNSMEIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/ReflectUtils.class")
	if err != nil {
		t.Fatal(err)
	}
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, "getConstructor(parseTypes") {
		t.Fatalf("missing getConstructor:\n%s", clipForTest(on, "getConstructor"))
	}
	if !strings.Contains(on, "NoSuchMethodException") {
		t.Fatalf("missing NSME on ReflectUtils dump:\n%s", clipForTest(on, "getConstructor"))
	}
}

func TestOkhttpCloseResourceNonPrivateIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ResponseBody.class", "JDEC_CLOSE_RESOURCE_THROWS_OFF",
		"$closeResource(Throwable var0, AutoCloseable var1) throws java.io.IOException",
		"$closeResource(Throwable var0, AutoCloseable var1) {")
}

func TestJacksonRecordAccessorNSMEIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/JDK14Util$RecordAccessor.class")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the original broad Exception handlers, including the initialization
	// failure channel. A guessed CNFE/NSME union excludes other reflective errors.
	assertOriginalCatchContract(t, raw, "JDEC_JACKSON_REMAINING_OFF")
}

func TestLog4jCompositeMergeStrategyNSMEIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/NsmeCatchAdv.class")
	if err != nil {
		t.Fatal(err)
	}
	// This fixture wraps only CNFE. NSME propagates as declared by make(); widening
	// its handler would incorrectly wrap a missing constructor.
	assertOriginalCatchContract(t, raw, "JDEC_ORIG14_REMAINING_OFF")
}

func TestNettyWildcardAddressHolderIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PcapWriteHandler$WildcardAddressHolder.class", "JDEC_NETTY_REMAINING_OFF",
		"try{\n\t\t\twildcard4 = InetAddress.getByAddress(new byte[4]);",
		"static final InetAddress wildcard4 = InetAddress.getByAddress(new byte[4]);")
}

func TestJSONReaderUTF16BoolExprNeIsLoadBearing(t *testing.T) {
	const path = "testdata/regression/JSONReaderUTF16.class"
	raw, code, object := reviewedFixtureMethod(t, path, "skipValue", "()V")
	for _, op := range []struct {
		pc     uint16
		opcode int
		data   []byte
	}{
		{491, core.OP_GETFIELD, nil}, {494, core.OP_BIPUSH, []byte{45}}, {496, core.OP_IF_ICMPEQ, []byte{0, 12}},
		{500, core.OP_GETFIELD, nil}, {503, core.OP_BIPUSH, []byte{43}}, {505, core.OP_IF_ICMPNE, []byte{0, 7}},
		{508, core.OP_ICONST_1, nil}, {512, core.OP_ICONST_0, nil}, {513, core.OP_ISTORE_1, nil}, {514, core.OP_ILOAD_1, nil}, {515, core.OP_IFEQ, []byte{0, 68}},
	} {
		assertReviewedOpcode(t, code, op.pc, op.opcode, op.data...)
	}
	decoder := core.NewDecompiler(code.Code, nil)
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	for _, pc := range []uint16{491, 500} {
		op := decoder.OpcodeByPC(pc)
		index := int(op.Data[0])<<8 | int(op.Data[1])
		field, ok := object.ConstantPoolManager.IndexInfo(index).(*ConstantFieldrefInfo)
		if !ok {
			t.Fatal("original character member is not a field")
		}
		name, descriptor := getNameAndType(object.ConstantPool, field.NameAndTypeIndex)
		if object.ConstantPoolManager.GetClassName(int(field.ClassIndex)) != "com/alibaba/fastjson2/JSONReaderUTF16" || name != "ch" || descriptor != "C" {
			t.Fatal("original sign predicate field tuple changed")
		}
	}
	// The original char comparisons produce a canonical word; the consumer is
	// IFEQ, not a boolean-return narrowing. Independent six-mode word/effect
	// oracles above cover both representations without this historical JVM.
	assertReviewedSources(t, raw, "JDEC_BOOL_EXPR_CMP_ZERO_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `public final void skipValue\(`)
		_, end, ok := reviewedCanonicalPredicateIf(body, "this.ch==45||this.ch==43", true)
		if !ok {
			t.Fatalf("original sign predicate lost a valid canonical-word consumer:\n%s", body)
		}
		if !strings.Contains(body[end:], "this.ch = this.chars[this.offset++];") {
			t.Fatal("sign branch lost original single character advance")
		}
	})
}

func TestOrig14KeepsWrapperExceptionAlternatives(t *testing.T) {
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "")
	in := "try{\nClass[] params=new Class[1];\nparams[0]=String.class;\nthis.getMethod(owner,\"read\",params,false).invoke(value,new Object[0]);\n}catch(IllegalAccessException var7){\nthrow new RuntimeException(var7);\n}"
	if got := fixOrig14RemainderReconstructs(in); got != in {
		t.Fatalf("invented checked exception: %s", got)
	}
}
