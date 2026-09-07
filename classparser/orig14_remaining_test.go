package javaclassparser

import (
	"os"
	"strings"
	"testing"
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
	assertKillSwitchDecompile(t, "testdata/regression/JDK14Util$RecordAccessor.class", "JDEC_JACKSON_REMAINING_OFF",
		"catch(ClassNotFoundException | NoSuchMethodException var1)",
		"Class var1 = Class.forName(\"java.lang.reflect.RecordComponent\");")
}

func TestLog4jCompositeMergeStrategyNSMEIsLoadBearing(t *testing.T) {
	assertOrig14Decompile(t, "testdata/regression/NsmeCatchAdv.class",
		"ClassNotFoundException | NoSuchMethodException var2",
		"catch(ClassNotFoundException var2){")
}

func TestNettyWildcardAddressHolderIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PcapWriteHandler$WildcardAddressHolder.class", "JDEC_NETTY_REMAINING_OFF",
		"try{\n\t\t\twildcard4 = InetAddress.getByAddress(new byte[4]);",
		"static final InetAddress wildcard4 = InetAddress.getByAddress(new byte[4]);")
}

func TestJSONReaderUTF16BoolExprNeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/JSONReaderUTF16.class", "JDEC_BOOL_EXPR_CMP_ZERO_OFF",
		") != (false)){",
		") != (0)){")
}
