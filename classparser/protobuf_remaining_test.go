package javaclassparser

// 承重测试: protobuf-java 剩余 dump 重构. kill-switch: JDEC_PROTOBUF_REMAINING_OFF.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtobufRemainingReconstructsAreLoadBearing(t *testing.T) {
	in := strings.Join([]string{
		"public final class ArrayDecoders {",
		"		Internal$ProtobufList<?> var6 = var4;",
		"		Internal$ProtobufList<?> var7 = var5;",
		"}",
		"public class BinaryWriter$SafeDirectWriter {",
		"										if (!(Character.isSurrogatePair(var4 = var1.charAt((var2) - (1)),var3))){",
		"}",
		"final class ByteBufferWriter {",
		"		BUFFER.set((Object)(null));",
		"}",
		"final class DiscardUnknownFieldsParser$1<T> extends AbstractParser<T> {",
		"}",
		"public abstract class GeneratedMessageLite {",
		"		return isInitialized(this,Boolean.TRUE.booleanValue());",
		"}",
		"public class LazyStringArrayList {",
		"		this(new ArrayList(var1));",
		"}",
		"public final class RpcUtil {",
		"	public static <Type extends Message> RpcCallback<Type> specializeCallback(RpcCallback<Message> var0) {",
		"		return var0;",
		"	}",
		"}",
		"final class SmallSortedMap$1<FieldDescriptorType> extends SmallSortedMap<FieldDescriptorType, Object> {",
		"}",
		"public class LazyFieldLite {",
		"				LazyFieldLite var1 = this;",
		"				synchronized(this){",
		"",
		"				}",
		"}",
		"public class Struct {",
		"					.setKey(var3.getKey()).setValue(var3.getValue())",
		"}",
		"public final class GeneratedMessageV3$FieldAccessorTable {",
		"			GeneratedMessageV3$FieldAccessorTable var3 = this;",
		"			synchronized(this){",
		"",
		"			}",
		"}",
		"public final class GeneratedMessage$FieldAccessorTable {",
		"			GeneratedMessage$FieldAccessorTable var3 = this;",
		"			synchronized(this){",
		"",
		"			}",
		"}",
		"class DescriptorMessageInfoFactory$IsInitializedCheckAnalyzer {",
		"			DescriptorMessageInfoFactory$IsInitializedCheckAnalyzer var3 = this;",
		"			synchronized(this){",
		"",
		"			}",
		"			return false;",
		"}",
	}, "\n")

	os.Unsetenv("JDEC_PROTOBUF_REMAINING_OFF")
	on := fixProtobufRemainingReconstructs(in)
	checks := []string{
		"Internal$ProtobufList var6 = var4;",
		"Character.isSurrogatePair((char)(var4 = var1.charAt((var2) - (1))),(char)(var3))",
		"BUFFER.set(null);",
		"DiscardUnknownFieldsParser$1<T extends MessageLite>",
		"isInitialized((GeneratedMessageLite)(this),true)",
		"this((ArrayList<Object>)(new ArrayList(var1)));",
		"return (RpcCallback<Type>) (RpcCallback) (var0);",
		"SmallSortedMap$1<FieldDescriptorType extends Comparable<FieldDescriptorType>>",
		"return this.value.toByteString();",
		".setKey((String)(var3.getKey())).setValue((Value)(var3.getValue()))",
		"synchronized(this){\n\t\t\t\treturn this;",
		"synchronized(this){\n\t\t\t\treturn false;",
	}
	for _, want := range checks {
		if !strings.Contains(on, want) {
			t.Errorf("ON: missing %q, got:\n%s", want, on)
		}
	}

	t.Setenv("JDEC_PROTOBUF_REMAINING_OFF", "1")
	off := fixProtobufRemainingReconstructs(in)
	if off != in {
		t.Errorf("OFF: expected identity, got:\n%s", off)
	}
}

func TestProtobufLazyStringArrayListThisIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ProtobufLazyStringArrayList.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_PROTOBUF_REMAINING_OFF")
	os.Unsetenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this((ArrayList<Object>)(new ArrayList((Collection)(var1))))") {
		t.Errorf("ON: expected descriptor-pinned this() overload, got:\n%s", on)
	}
	if !strings.Contains(on, "return new LazyStringArrayList((ArrayList<Object>)(var2));") {
		t.Errorf("ON: expected descriptor-pinned constructor invocation, got:\n%s", on)
	}

	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this((ArrayList<Object>)(new ArrayList((Collection)(var1))))") {
		t.Errorf("OFF: did not expect descriptor-pinned this() overload, got:\n%s", off)
	}
	if strings.Contains(off, "return new LazyStringArrayList((ArrayList<Object>)(var2));") {
		t.Errorf("OFF: did not expect descriptor-pinned constructor invocation, got:\n%s", off)
	}
}

func TestImmediateZeroArgCheckcastInvokeRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("javac"); err != nil {
		t.Skipf("javac unavailable: %v", err)
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skipf("java unavailable: %v", err)
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "ImmediateCheckcast.java")
	source := `import java.lang.reflect.Method;
public final class ImmediateCheckcast {
  static boolean isDefault(Object value) { return Boolean.TRUE.equals(value); }
  public static boolean present(Method method, Object target, Object value) throws Exception {
    return method == null ? !isDefault(value) : ((Boolean) method.invoke(target)).booleanValue();
  }
  public static void main(String[] args) throws Exception {
    Method method = args.length == 0 ? null : Boolean.class.getMethod("booleanValue");
    Object value = args.length == 0 ? Boolean.TRUE : Boolean.FALSE;
    System.out.print(present(method, Boolean.TRUE, value));
  }
}`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command("javac", "--release", "8", "-d", dir, sourcePath)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile fixture: %v\n%s", err, output)
	}
	classBytes, err := os.ReadFile(filepath.Join(dir, "ImmediateCheckcast.class"))
	if err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF")
	on, err := Decompile(classBytes)
	if err != nil {
		t.Fatalf("decompile enabled: %v", err)
	}
	if !strings.Contains(on, "((Boolean)") || !strings.Contains(on, ").booleanValue()") {
		t.Fatalf("expected branch-local checked cast in invocation expression, got:\n%s", on)
	}
	decompiledPath := filepath.Join(dir, "roundtrip", "ImmediateCheckcast.java")
	if err := os.MkdirAll(filepath.Dir(decompiledPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decompiledPath, []byte(on), 0o644); err != nil {
		t.Fatal(err)
	}
	compileDecompiled := exec.Command("javac", "--release", "8", "-d", filepath.Dir(decompiledPath), decompiledPath)
	if output, err := compileDecompiled.CombinedOutput(); err != nil {
		t.Fatalf("compile decompiled fixture: %v\n%s\n%s", err, output, on)
	}

	run := func(classPath string, args ...string) string {
		t.Helper()
		cmdArgs := append([]string{"-Xverify:all", "-cp", classPath, "ImmediateCheckcast"}, args...)
		output, err := exec.Command("java", cmdArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("run fixture %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	for _, args := range [][]string{nil, {"present"}} {
		original := run(dir, args...)
		rebuilt := run(filepath.Dir(decompiledPath), args...)
		if original != rebuilt {
			t.Errorf("args=%v: original %q, rebuilt %q", args, original, rebuilt)
		}
	}

	t.Setenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF", "1")
	off, err := Decompile(classBytes)
	if err != nil {
		t.Fatalf("decompile disabled: %v", err)
	}
	if off == on {
		t.Fatal("kill switch did not disable immediate checkcast inlining")
	}
}

func TestNonConditionalForkDiamondRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("javac"); err != nil {
		t.Skipf("javac unavailable: %v", err)
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skipf("java unavailable: %v", err)
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "WeakPhi.java")
	source := `import java.lang.ref.WeakReference;
public final class WeakPhi {
  static String select(WeakReference<String> ref) {
    String value = ref == null ? null : ref.get();
    return value == null ? "missing" : value;
  }
  public static void main(String[] args) {
    System.out.print(select(null) + "," + select(new WeakReference<String>("alive")));
  }
}`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command("javac", "--release", "8", "-d", dir, sourcePath)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile fixture: %v\n%s", err, output)
	}
	classBytes, err := os.ReadFile(filepath.Join(dir, "WeakPhi.class"))
	if err != nil {
		t.Fatal(err)
	}
	reconstructed, err := Decompile(classBytes)
	if err != nil {
		t.Fatalf("decompile fixture: %v", err)
	}
	rebuiltDir := filepath.Join(dir, "rebuilt")
	if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rebuiltPath := filepath.Join(rebuiltDir, "WeakPhi.java")
	if err := os.WriteFile(rebuiltPath, []byte(reconstructed), 0o644); err != nil {
		t.Fatal(err)
	}
	compileRebuilt := exec.Command("javac", "--release", "8", "-d", rebuiltDir, rebuiltPath)
	if output, err := compileRebuilt.CombinedOutput(); err != nil {
		t.Fatalf("compile reconstructed fixture: %v\n%s\n%s", err, output, reconstructed)
	}
	run := func(classPath, name string) string {
		t.Helper()
		output, err := exec.Command("java", "-Xverify:all", "-cp", classPath, "WeakPhi").CombinedOutput()
		if err != nil {
			t.Fatalf("run %s fixture: %v\n%s\n%s", name, err, output, reconstructed)
		}
		return string(output)
	}
	if original, rebuilt := run(dir, "original"), run(rebuiltDir, "reconstructed"); original != rebuilt {
		t.Fatalf("original output %q differs from reconstructed output %q\n%s", original, rebuilt, reconstructed)
	}
}
