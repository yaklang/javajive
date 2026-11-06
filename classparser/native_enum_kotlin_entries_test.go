package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// kotlinEntriesFixture builds the exact erased shape Kotlin emits in <clinit>:
//
//	getstatic $VALUES
//	checkcast [Ljava/lang/Enum;
//	invokestatic EnumEntriesKt.enumEntries
//	putstatic $ENTRIES
func kotlinEntriesFixture(t *testing.T, callOwner, callDescriptor, castName string) (*ClassObject, []*core.OpCode) {
	t.Helper()
	class := classfilebuilder.NewClass("probe/KEnum", "java/lang/Enum", 0x4031)
	pool := class.Pool()
	valuesRef, err := pool.Fieldref("probe/KEnum", "$VALUES", "[Lprobe/KEnum;")
	if err != nil {
		t.Fatal(err)
	}
	entriesRef, err := pool.Fieldref("probe/KEnum", "$ENTRIES", "Lkotlin/enums/EnumEntries;")
	if err != nil {
		t.Fatal(err)
	}
	castRef, err := pool.Class(castName)
	if err != nil {
		t.Fatal(err)
	}
	callRef, err := pool.Methodref(callOwner, "enumEntries", callDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	class.AddField(0x101a, "$VALUES", "[Lprobe/KEnum;")
	class.AddField(0x101a, "$ENTRIES", "Lkotlin/enums/EnumEntries;")
	initializer := class.AddMethod(0x0008, "<clinit>", "()V")
	initializer.Code().
		CP(classfilebuilder.GETSTATIC, valuesRef).
		CP(classfilebuilder.CHECKCAST, castRef).
		CP(classfilebuilder.INVOKESTATIC, callRef).
		CP(classfilebuilder.PUTSTATIC, entriesRef).
		Emit(classfilebuilder.RETURN)
	raw, err := class.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range obj.Methods {
		name, ok := sourceBridgeUTF8(obj, method.NameIndex)
		if !ok || name != "<clinit>" {
			continue
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			return obj, constructorMotionOps(decoder)
		}
	}
	t.Fatal("fixture <clinit> has no Code attribute")
	return nil, nil
}

func TestNativeEnumKotlinEntriesRecognition(t *testing.T) {
	const descriptor = "([Ljava/lang/Enum;)Lkotlin/enums/EnumEntries;"

	t.Run("accepts exact Kotlin shape", func(t *testing.T) {
		obj, ops := kotlinEntriesFixture(t, "kotlin/enums/EnumEntriesKt", descriptor, "[Ljava/lang/Enum;")
		recognized := false
		for i, op := range ops {
			if op.Instr == nil || op.Instr.OpCode != core.OP_GETSTATIC {
				continue
			}
			field, ok := nativeEnumKotlinEntriesRead(obj, ops, i)
			if !ok {
				t.Fatalf("exact Kotlin shape rejected at op %d", i)
			}
			if field != "$ENTRIES" {
				t.Fatalf("entries field = %q, want $ENTRIES", field)
			}
			recognized = true
		}
		if !recognized {
			t.Fatal("fixture has no getstatic")
		}
	})

	t.Run("rejects foreign helper", func(t *testing.T) {
		obj, ops := kotlinEntriesFixture(t, "kotlin/enums/OtherKt", descriptor, "[Ljava/lang/Enum;")
		assertKotlinEntriesRejected(t, obj, ops)
	})

	t.Run("rejects erased checkcast", func(t *testing.T) {
		obj, ops := kotlinEntriesFixture(t, "kotlin/enums/EnumEntriesKt", descriptor, "[Ljava/lang/Object;")
		assertKotlinEntriesRejected(t, obj, ops)
	})

	t.Run("rejects different descriptor", func(t *testing.T) {
		obj, ops := kotlinEntriesFixture(t,
			"kotlin/enums/EnumEntriesKt", "([Ljava/lang/Enum;)Ljava/util/List;", "[Ljava/lang/Enum;")
		assertKotlinEntriesRejected(t, obj, ops)
	})
}

func assertKotlinEntriesRejected(t *testing.T, obj *ClassObject, ops []*core.OpCode) {
	t.Helper()
	for i, op := range ops {
		if op.Instr == nil || op.Instr.OpCode != core.OP_GETSTATIC {
			continue
		}
		if field, ok := nativeEnumKotlinEntriesRead(obj, ops, i); ok {
			t.Fatalf("non-Kotlin shape accepted at op %d as %q", i, field)
		}
	}
}
