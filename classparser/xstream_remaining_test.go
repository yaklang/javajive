package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestSerialHookKeepsPrivateReadResolve(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PureJavaReflectionProvider.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_SERIAL_HOOK_PRIVATE_OFF")
	os.Unsetenv("JDEC_NEST_PRIVATE_PACKAGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "private Object readResolve()") && !strings.Contains(on, "private java.lang.Object readResolve()") {
		t.Errorf("ON expected private readResolve, got:\n%s", on)
	}
	t.Setenv("JDEC_SERIAL_HOOK_PRIVATE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if strings.Contains(off, "private Object readResolve()") {
		t.Errorf("OFF expected nest-demoted (non-private) readResolve, got:\n%s", off)
	}
}

func TestThrowObjectAsThrowableIsLoadBearing(t *testing.T) {
	in := "\tObject var4 = null;\n\t\tvar4 = new ConversionException((Throwable)(var4_2));\n\t\tthrow var4;\n"
	os.Unsetenv("JDEC_THROW_OBJECT_OFF")
	on := fixThrowObjectAsThrowable(in)
	if !strings.Contains(on, "ConversionException var4 = null;") {
		t.Errorf("ON expected ConversionException var4, got:\n%s", on)
	}
	t.Setenv("JDEC_THROW_OBJECT_OFF", "1")
	if fixThrowObjectAsThrowable(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestTreeUnmarshallerThrowObjectIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TreeUnmarshaller.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_THROW_OBJECT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "Object var4 = null;") && strings.Contains(on, "throw var4;") {
		t.Errorf("ON still throws Object var4:\n%s", on)
	}
	if !strings.Contains(on, "ConversionException var4") && !strings.Contains(on, "throw var4;") {
		// Either retyped or no longer throws the Object local.
	}
	t.Setenv("JDEC_THROW_OBJECT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "Object var4 = null;") {
		t.Errorf("OFF expected Object var4, got:\n%s", off)
	}
}

func TestClassDollarForwardRefIsLoadBearing(t *testing.T) {
	in := "\tstatic final String DATA_HOLDER_KEY = class$Foo.getName();\n\tstatic Class class$Foo;\n"
	os.Unsetenv("JDEC_CLASSDOLLAR_FORWARD_OFF")
	on := fixClassDollarForwardRef(in)
	if strings.Index(on, "static Class class$Foo;") > strings.Index(on, "DATA_HOLDER_KEY") {
		t.Errorf("ON expected class$ before DATA_HOLDER_KEY, got:\n%s", on)
	}
	t.Setenv("JDEC_CLASSDOLLAR_FORWARD_OFF", "1")
	if fixClassDollarForwardRef(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestCustomObjectInputStreamForwardRefIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CustomObjectInputStream.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_CLASSDOLLAR_FORWARD_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	key := strings.Index(on, "DATA_HOLDER_KEY")
	cls := strings.Index(on, "static Class class$")
	if cls < 0 || key < 0 || cls > key {
		t.Errorf("ON expected class$ field before DATA_HOLDER_KEY, got:\n%s", on)
	}
	t.Setenv("JDEC_CLASSDOLLAR_FORWARD_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	keyOff := strings.Index(off, "DATA_HOLDER_KEY")
	clsOff := strings.Index(off, "static Class class$")
	if clsOff < 0 || keyOff < 0 || clsOff < keyOff {
		t.Errorf("OFF expected class$ after DATA_HOLDER_KEY, got:\n%s", off)
	}
}

func TestThrowInitCauseCastIsLoadBearing(t *testing.T) {
	in := "\t\t\tthrow new NoClassDefFoundError().initCause((Throwable)(var1));\n"
	os.Unsetenv("JDEC_THROW_INITCAUSE_OFF")
	on := fixThrowInitCauseCast(in)
	if !strings.Contains(on, "throw (NoClassDefFoundError) new NoClassDefFoundError().initCause") {
		t.Errorf("ON expected cast, got:\n%s", on)
	}
	t.Setenv("JDEC_THROW_INITCAUSE_OFF", "1")
	if fixThrowInitCauseCast(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestXStreamClassDollarInitCauseIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/XStream.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_THROW_INITCAUSE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "throw new NoClassDefFoundError().initCause") {
		t.Errorf("ON still throws uncast initCause:\n%s", on)
	}
	if !strings.Contains(on, "throw (NoClassDefFoundError) new NoClassDefFoundError().initCause") {
		t.Errorf("ON expected (NoClassDefFoundError) cast, got:\n%s", on)
	}
	t.Setenv("JDEC_THROW_INITCAUSE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "throw new NoClassDefFoundError().initCause") {
		t.Errorf("OFF expected uncast throw, got:\n%s", off)
	}
}

func TestXstreamRemainingStringRewrites(t *testing.T) {
	in := "DefaultMapper var1 = new DefaultMapper(this.classLoaderReference);\n" +
		"var1 = new XStream11XmlFriendlyMapper((Mapper)(var1));\n" +
		"ArrayIterator var14 = (var10.value.getClass().isArray()) ? (new ArrayIterator(var10.value)) : (it);\n"
	os.Unsetenv("JDEC_XSTREAM_REMAINING_OFF")
	on := fixXstreamRemainingReconstructs(in)
	if !strings.Contains(on, "Mapper var1 = new DefaultMapper") {
		t.Errorf("ON expected Mapper var1, got:\n%s", on)
	}
	if !strings.Contains(on, "Iterator var14 = (var10.value.getClass().isArray())") {
		t.Errorf("ON expected Iterator var14, got:\n%s", on)
	}
	in2 := "int handleStateTransition(int var1, int var2, String var3, String var4) {\n" +
		"\t\tdefault:\n\t\t\tthrow new AbstractJsonWriter$IllegalWriterStateException(var1,var2,var3);\n\t\t}\n\t}\n\tprotected AbstractJsonWriter$Type getType"
	on2 := fixXstreamRemainingReconstructs(in2)
	if !strings.Contains(on2, "return var2;") {
		t.Errorf("ON expected return var2 after switch, got:\n%s", on2)
	}
	t.Setenv("JDEC_XSTREAM_REMAINING_OFF", "1")
	if fixXstreamRemainingReconstructs(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestXstreamCGLIBFactoryFlagSnippet(t *testing.T) {
	in := "public class CGLIBEnhancedConverter extends SerializableConverter {\n" +
		"int var5 = (((class$net$sf$cglib$proxy$Factory) == (null)) ? (class$net$sf$cglib$proxy$Factory = class$(\"net.sf.cglib.proxy.Factory\")) : (class$net$sf$cglib$proxy$Factory)).isAssignableFrom(var4);\n" +
		"\t\tConversionException var7 = null;\n\t\tdo{\n\t\t\tif ((var7) < (var6.length)){\n"
	os.Unsetenv("JDEC_XSTREAM_REMAINING_OFF")
	out := fixXstreamRemainingReconstructs(in)
	if !strings.Contains(out, "boolean var5 =") {
		t.Fatalf("boolean var5 not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "int var7 = 0;") {
		t.Fatalf("int var7 not rewritten:\n%s", out)
	}
}

func TestXstreamCGLIBFactoryFlagIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CGLIBEnhancedConverter.class", "JDEC_XSTREAM_REMAINING_OFF",
		"boolean var5 = (((class$net$sf$cglib$proxy$Factory) == (null))",
		"int var5 = (((class$net$sf$cglib$proxy$Factory) == (null))")
}

func TestXstreamCGLIBInterfaceLoopIndexIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CGLIBEnhancedConverter.class", "JDEC_XSTREAM_REMAINING_OFF",
		"int var7 = 0;\n\t\tdo{\n\t\t\tif ((var7) < (var6.length)){",
		"ConversionException var7 = null;\n\t\tdo{\n\t\t\tif ((var7) < (var6.length)){")
}
