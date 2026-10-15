package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestFlatStaticDeclarationKeepsOriginalConstructorAccessBoundaries(t *testing.T) {
	const source = `class FlatConstructScope{static class Value{Value(){}private Value(int n){}static Value own(){return new Value();}static java.util.function.Supplier<Value> ownHandle(){return Value::new;}}}
 class FlatConstructCaller{Object create(){return new FlatConstructScope.Value();}}
 class FlatConstructOwner{static class Member{}FlatConstructScope.Value pass(FlatConstructScope.Value value){return value;}}`
	original := nativeCompileClasses(t, source)
	factory := nativeCompileClasses(t, strings.Replace(source, "return new FlatConstructScope.Value();", "return FlatConstructScope.Value.own();", 1))
	handles := nativeCompileClasses(t, strings.Replace(source, "return new FlatConstructScope.Value();", "return ((java.util.function.Supplier<FlatConstructScope.Value>)FlatConstructScope.Value::new).get();", 1))
	for _, variant := range []string{"public overload", "own private factory and handle", "unused private constructor CP entry", "foreign private call", "foreign private handle", "missing constructor", "duplicate constructor", "nil method", "bad descriptor", "conflicting access flags", "static constructor", "native constructor", "abstract constructor", "synchronized constructor", "nil prepared", "nil reader", "nil target", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := original
			if variant == "own private factory and handle" || variant == "unused private constructor CP entry" {
				input = factory
			}
			if variant == "foreign private handle" {
				input = handles
			}
			files := map[string][]byte{}
			for n, b := range input {
				files[n] = append([]byte(nil), b...)
			}
			target, err := Parse(files["FlatConstructScope$Value.class"])
			if err != nil {
				t.Fatal(err)
			}
			var constructor *MemberInfo
			for _, m := range target.Methods {
				n, _ := sourceBridgeUTF8(target, m.NameIndex)
				d, _ := sourceBridgeUTF8(target, m.DescriptorIndex)
				if n == "<init>" && d == "()V" {
					constructor = m
				}
			}
			if constructor == nil {
				t.Fatal("original constructor table")
			}
			if variant == "own private factory and handle" || variant == "unused private constructor CP entry" || variant == "foreign private call" || variant == "foreign private handle" {
				constructor.AccessFlags = 2
				files["FlatConstructScope$Value.class"] = target.Bytes()
			}
			if variant == "unused private constructor CP entry" {
				caller, e := Parse(files["FlatConstructCaller.class"])
				if e != nil {
					t.Fatal(e)
				}
				owner := sourceBridgePoolString(t, caller, "FlatConstructScope$Value")
				name := sourceBridgePoolString(t, caller, "<init>")
				desc := sourceBridgePoolString(t, caller, "()V")
				caller.ConstantPool = append(caller.ConstantPool, &ConstantClassInfo{NameIndex: owner})
				classIndex := uint16(len(caller.ConstantPool))
				caller.ConstantPool = append(caller.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc})
				typeIndex := uint16(len(caller.ConstantPool))
				caller.ConstantPool = append(caller.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: classIndex, NameAndTypeIndex: typeIndex}})
				files["FlatConstructCaller.class"] = caller.Bytes()
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, err := Parse(files["FlatConstructOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := &nativeMemberPrepared{reader: archive.nativeMemberReader(root)}
			switch variant {
			case "missing constructor":
				for i, m := range target.Methods {
					if m == constructor {
						target.Methods = append(target.Methods[:i], target.Methods[i+1:]...)
						break
					}
				}
			case "duplicate constructor":
				target.Methods = append(target.Methods, constructor)
			case "nil method":
				target.Methods = append(target.Methods, nil)
			case "bad descriptor":
				constructor.DescriptorIndex = sourceBridgePoolString(t, target, "()I")
			case "conflicting access flags":
				constructor.AccessFlags = 3
			case "static constructor":
				constructor.AccessFlags |= 8
			case "native constructor":
				constructor.AccessFlags |= 0x100
			case "abstract constructor":
				constructor.AccessFlags |= 0x400
			case "synchronized constructor":
				constructor.AccessFlags |= 0x20
			case "nil prepared":
				prepared = nil
			case "nil reader":
				prepared.reader = nil
			case "nil target":
				target = nil
			case "work":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				prepared.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "public overload" || variant == "own private factory and handle" || variant == "unused private constructor CP entry"
			if got := archive.flatStaticOriginalConstructorAccessClosed(prepared, target); got != want {
				t.Fatalf("original construction access=%v want=%v", got, want)
			}
		})
	}
}
