package javaclassparser

// 承重测试: protobuf-java 剩余 dump 重构. kill-switch: JDEC_PROTOBUF_REMAINING_OFF.

import (
	"os"
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
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this((ArrayList<Object>)(new ArrayList(var1)))") {
		t.Errorf("ON: expected ArrayList<Object> this() disambiguation, got:\n%s", on)
	}

	t.Setenv("JDEC_PROTOBUF_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this((ArrayList<Object>)(new ArrayList(var1)))") {
		t.Errorf("OFF: did not expect ArrayList<Object> this() wrap, got:\n%s", off)
	}
}
