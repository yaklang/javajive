package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const methodSourceNestUseFixture = `class ModernUseOwner{static class Peer{static int call(){return 9;}}static java.util.function.IntUnaryOperator first(int captured){return value->captured+value;}}class ModernUseDriver{public static void main(String[]args)throws Exception{if(ModernUseOwner.first(7).applyAsInt(2)!=9||ModernUseOwner.Peer.call()!=9)throw new AssertionError("original nest use");java.lang.reflect.Method m=ModernUseOwner.class.getDeclaredMethod("lambda$first$0",int.class,int.class);if(ModernUseOwner.class.getDeclaredMethods().length!=2||!m.isSynthetic()||m.getModifiers()!=4106)throw new AssertionError("original nest declarations");System.out.println("nest-use:original");}}`

func methodSourceNestUseObjects(t *testing.T, files map[string][]byte) (*ClassObjectDumper, map[string]*ClassObject, *MemberInfo) {
	t.Helper()
	objects := modernNestTestObjects(t, files)
	d := NewClassObjectDumper(objects["ModernUseOwner"])
	d.options.TargetSourceVersion = 11
	d.foldSiblingResolver = func(name string) ([]byte, bool) {
		object := objects[name]
		if object == nil {
			return nil, false
		}
		return object.Bytes(), true
	}
	for _, method := range d.obj.Methods {
		name, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
		if name == "lambda$first$0" {
			return d, objects, method
		}
	}
	t.Fatal("original nest implementation")
	return nil, nil, nil
}

func methodSourceRunNestOriginal(t *testing.T, files map[string][]byte, objects map[string]*ClassObject) {
	t.Helper()
	_, java := t04Tools(t)
	dir := methodParameterNameDirectory(t, "nest-original")
	for name, raw := range files {
		if object := objects[name[:len(name)-len(".class")]]; object != nil {
			raw = object.Bytes()
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, dir, "ModernUseDriver"); got != "nest-use:original\n" {
		t.Fatal(got)
	}
}

func methodSourceAddPeerImplementationReference(t *testing.T, peer *ClassObject, execute, handleData bool) {
	t.Helper()
	cp := NewConstantPoolWithConstant(&peer.ConstantPool)
	index := cp.AddNewMethodInfo("ModernUseOwner", "lambda$first$0", "(II)I")
	for _, method := range peer.Methods {
		name, _ := sourceBridgeUTF8(peer, method.NameIndex)
		if name != "call" {
			continue
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			previousLength := len(code.Code)
			if execute {
				code.Code = []byte{0x10, 7, 0x05, 0xb8, byte(index >> 8), byte(index), 0xac}
				code.MaxStack = 2
			}
			if handleData {
				peer.ConstantPool = append(peer.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(index)})
				i := len(peer.ConstantPool)
				code.Code = []byte{0x13, byte(i >> 8), byte(i), 0x57, 0x10, 9, 0xac}
				code.MaxStack = 1
			}
			code.AttrLen = uint32(int(code.AttrLen) + len(code.Code) - previousLength)
			return
		}
	}
	t.Fatal("original peer call code")
}

func TestAdversarialMethodSourceModernNestUseRequiresFreshCompleteScope(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ModernUseOwner.java": methodSourceNestUseFixture}, "none", "11")
	methodSourceRunNestOriginal(t, files, nil)
	for _, variant := range []string{"original", "no resolver", "missing peer", "wrong reciprocal", "duplicate nest", "unused peer target ref", "peer direct invocation", "peer handle data", "duplicate peer code", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, objects, method := methodSourceNestUseObjects(t, files)
			peer := objects["ModernUseOwner$Peer"]
			switch variant {
			case "no resolver":
				d.foldSiblingResolver = nil
			case "missing peer":
				delete(objects, "ModernUseOwner$Peer")
			case "wrong reciprocal":
				modernNestTestRemoveAttribute(peer, "NestHost")
			case "duplicate nest":
				peer.Attributes = append(peer.Attributes, modernNestTestAttribute(peer, "NestHost"))
			case "unused peer target ref":
				methodSourceAddPeerImplementationReference(t, peer, false, false)
				methodSourceRunNestOriginal(t, files, objects)
			case "peer direct invocation":
				methodSourceAddPeerImplementationReference(t, peer, true, false)
				methodSourceRunNestOriginal(t, files, objects)
			case "peer handle data":
				methodSourceAddPeerImplementationReference(t, peer, false, true)
				methodSourceRunNestOriginal(t, files, objects)
			case "duplicate peer code":
				peer.Methods[0].Attributes = append(peer.Methods[0].Attributes, peer.Methods[0].Attributes[0])
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			name, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
			desc, _ := sourceBridgeUTF8(d.obj, method.DescriptorIndex)
			got := d.originalLambdaDeclaration(d.obj.GetClassName(), name, desc)
			want := core.LambdaDeclarationUnknown
			if variant == "original" {
				want = core.LambdaDeclarationSyntheticBody
			}
			if got != want {
				t.Fatalf("original body declaration=%v want=%v", got, want)
			}
		})
	}
}

func TestAdversarialMethodSourceModernNestReceiptRejectsLaterPeerUse(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ModernUseOwner.java": methodSourceNestUseFixture}, "none", "11")
	methodSourceRunNestOriginal(t, files, nil)
	d, objects, method := methodSourceNestUseObjects(t, files)
	name, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
	desc, _ := sourceBridgeUTF8(d.obj, method.DescriptorIndex)
	body := &dumpedMethods{member: method, code: "return l0;", bodyCode: "return l0;"}
	key := registerMethodSourceBodyFixture(t, d, name, desc, 1, body)
	if d.lambdaMethodSourceBody(method, nil) != body {
		t.Fatal("original request receipt")
	}
	methodSourceAddPeerImplementationReference(t, objects["ModernUseOwner$Peer"], true, false)
	methodSourceRunNestOriginal(t, files, objects)
	if d.lambdaMethodSourceBody(method, nil) != nil || d.lambdaMethodSourcePacketClosed(method, body, nil) || d.dumpedMethodsSet[key] != body {
		t.Fatal("later peer use reused old consumption authority or removed independent cache")
	}
}
