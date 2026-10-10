package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const nativeMemberLambdaFixture = `class LambdaProofOwner {class Child {
 java.util.function.Supplier<Class<?>> type(final java.lang.reflect.Field f,final long n){return ()->n==Long.MIN_VALUE?f.getType():null;}
 java.util.function.Supplier<Object> self(final long n){return ()->n==Long.MIN_VALUE?this:null;}
 java.util.function.Supplier<Integer> zero(){return ()->42;}
}} class LambdaProofForeign {}`

func nativeMemberLambdaTestChild(t *testing.T, files map[string][]byte) (*nativeMemberClass, *MemberInfo, *BootstrapMethodsAttribute, *BootstrapMethod, uint16) {
	t.Helper()
	obj, err := Parse(append([]byte(nil), files["LambdaProofOwner$Child.class"]...))
	if err != nil {
		t.Fatal(err)
	}
	var method *MemberInfo
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if strings.HasPrefix(n, "lambda$type$") {
			method = m
		}
	}
	if method == nil {
		t.Fatal("original static implementation")
	}
	reader := NewClassObjectDumper(obj)
	child := &nativeMemberClass{object: obj, lambdaContext: nativeLambdaImplementationContext{resolve: reader.nativeAnnotationDeclarationResolver(), metadata: reader.buildInvocationMetadata()}}
	var boot *BootstrapMethodsAttribute
	for _, a := range obj.Attributes {
		if b, ok := a.(*BootstrapMethodsAttribute); ok {
			boot = b
		}
	}
	if boot == nil {
		t.Fatal("original bootstrap")
	}
	name, _ := sourceBridgeUTF8(obj, method.NameIndex)
	desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
	for _, site := range boot.BootstrapMethods {
		if len(site.BootstrapArguments) != 3 {
			continue
		}
		idx := site.BootstrapArguments[1]
		h, ok := obj.ConstantPool[idx-1].(*ConstantMethodHandleInfo)
		if !ok || h == nil {
			continue
		}
		ref := nativeConstantMember(obj.ConstantPool[h.ReferenceIndex-1])
		if ref == nil {
			continue
		}
		nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		n, _ := sourceBridgeUTF8(obj, nt.NameIndex)
		d, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
		if n == name && d == desc {
			return child, method, boot, site, idx
		}
	}
	t.Fatal("original metafactory target")
	return nil, nil, nil, nil, 0
}

func TestNativeMemberLambdaImplementationRequiresOriginalExclusiveMetafactoryUse(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	variants := []string{"original", "nil child", "nil method", "foreign method object", "missing synthetic", "public", "bridge", "abstract", "wrong static", "signature", "method annotation", "duplicate method", "missing code", "duplicate code", "missing resolver", "bootstrap count", "bootstrap arity", "bootstrap handle as lookup", "target kind", "interface target", "second handle", "no original site", "data handle", "direct invocation", "wrong captured descriptor", "wrong instantiated type", "primitive SAM mismatch", "condy use", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			child, method, boot, site, handleIndex := nativeMemberLambdaTestChild(t, files)
			obj := child.object
			h := obj.ConstantPool[handleIndex-1].(*ConstantMethodHandleInfo)
			var code *CodeAttribute
			for _, a := range method.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
			var lexicalCode *CodeAttribute
			var dynamic *ConstantInvokeDynamicInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "type" {
					continue
				}
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						lexicalCode = c
					}
				}
			}
			for _, cp := range obj.ConstantPool {
				if d, ok := cp.(*ConstantInvokeDynamicInfo); ok {
					if boot.BootstrapMethods[d.BootstrapMethodAttrIndex] == site {
						dynamic = d
					}
				}
			}
			if code == nil || lexicalCode == nil || dynamic == nil {
				t.Fatal("original code witnesses")
			}
			var work *workbudget.Budget
			switch variant {
			case "nil child":
				child = nil
			case "nil method":
				method = nil
			case "foreign method object":
				copy := *method
				method = &copy
			case "missing synthetic":
				method.AccessFlags &^= 0x1000
			case "public":
				method.AccessFlags = 0x1009
			case "bridge":
				method.AccessFlags |= 0x40
			case "abstract":
				method.AccessFlags |= 0x400
			case "wrong static":
				method.AccessFlags &^= 8
			case "signature":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "method annotation":
				method.Attributes = append(method.Attributes, &RuntimeVisibleAnnotationsAttribute{})
			case "duplicate method":
				obj.Methods = append(obj.Methods, method)
			case "missing code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "missing resolver":
				child.lambdaContext.resolve = nil
			case "bootstrap count":
				boot.NumBootstrapMethods++
			case "bootstrap arity":
				site.NumBootstrapArguments++
			case "bootstrap handle as lookup":
				site.BootstrapMethodRef = handleIndex
			case "target kind":
				h.ReferenceKind = 5
			case "interface target":
				ref := obj.ConstantPool[h.ReferenceIndex-1].(*ConstantMethodrefInfo)
				obj.ConstantPool[h.ReferenceIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "second handle":
				copy := *h
				obj.ConstantPool = append(obj.ConstantPool, &copy)
			case "no original site":
				for i := range lexicalCode.Code {
					lexicalCode.Code[i] = byte(core.OP_NOP)
				}
			case "data handle":
				lexicalCode.Code = append([]byte{byte(core.OP_LDC_W), byte(handleIndex >> 8), byte(handleIndex), byte(core.OP_POP)}, lexicalCode.Code...)
			case "direct invocation":
				lexicalCode.Code = append([]byte{byte(core.OP_INVOKESTATIC), byte(h.ReferenceIndex >> 8), byte(h.ReferenceIndex)}, lexicalCode.Code...)
			case "wrong captured descriptor":
				nt := obj.ConstantPool[dynamic.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = method.DescriptorIndex
			case "wrong instantiated type":
				obj.ConstantPool[site.BootstrapArguments[2]-1].(*ConstantMethodTypeInfo).DescriptorIndex = method.DescriptorIndex
			case "primitive SAM mismatch":
				obj.ConstantPool[site.BootstrapArguments[0]-1].(*ConstantMethodTypeInfo).DescriptorIndex = nativeMemberLambdaTestUTF8(obj, "()I")
			case "condy use":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantDynamicInfo{BootstrapMethodAttrIndex: dynamic.BootstrapMethodAttrIndex, NameAndTypeIndex: dynamic.NameAndTypeIndex})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberLambdaImplementation(child, method, work); got != (variant == "original") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

func nativeMemberLambdaTestUTF8(obj *ClassObject, text string) uint16 {
	obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: text})
	return uint16(len(obj.ConstantPool))
}

func TestNativeMemberLambdaSourceCertificateRequiresConsumedOriginalBody(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	for _, variant := range []string{"original", "missing consumption", "missing body", "foreign body", "stub", "checked escape", "embedded stub", "foreign dumper", "budget"} {
		t.Run(variant, func(t *testing.T) {
			child, method, _, _, _ := nativeMemberLambdaTestChild(t, files)
			if !nativeMemberLambdaImplementation(child, method, nil) {
				t.Fatal("original proof")
			}
			n, _ := sourceBridgeUTF8(child.object, method.NameIndex)
			d, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
			dumper := NewClassObjectDumper(child.object)
			body := &dumpedMethods{member: method, code: "return null;", bodyCode: "return null;"}
			key := fmt.Sprintf("name:%s,desc:%s", n, d)
			dumper.lambdaMethods[n] = []string{d}
			dumper.dumpedMethodsSet[key] = body
			var work *workbudget.Budget
			switch variant {
			case "missing consumption":
				delete(dumper.lambdaMethods, n)
			case "missing body":
				delete(dumper.dumpedMethodsSet, key)
			case "foreign body":
				copy := *method
				body.member = &copy
			case "stub":
				body.bodyCode = "stub"
			case "checked escape":
				body.checkedEscape = true
			case "embedded stub":
				body.code = DecompileStubMarker
			case "foreign dumper":
				dumper.obj = &ClassObject{}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "budget" {
				nativeProofWork(work, 1)
			}
			if got := nativeMemberLambdaSourceClosed(child, dumper, work); got != (variant == "original") {
				t.Fatalf("complete=%v", got)
			}
		})
	}
}

func TestNativeMemberLambdaArchiveClosureRejectsForeignPhysicalReferences(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberLambdaFixture)
	for _, variant := range []string{"original", "foreign method reference", "foreign handle", "unresolved user", "wrong descriptor"} {
		t.Run(variant, func(t *testing.T) {
			child, method, _, _, _ := nativeMemberLambdaTestChild(t, files)
			if !nativeMemberLambdaImplementation(child, method, nil) {
				t.Fatal("original proof")
			}
			input := map[string][]byte{}
			for n, raw := range files {
				input[n] = append([]byte(nil), raw...)
			}
			if variant == "foreign method reference" || variant == "foreign handle" || variant == "wrong descriptor" {
				foreign, err := Parse(input["LambdaProofForeign.class"])
				if err != nil {
					t.Fatal(err)
				}
				owner := nativeMemberLambdaTestUTF8(foreign, child.object.GetClassName())
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantClassInfo{NameIndex: owner})
				classIndex := uint16(len(foreign.ConstantPool))
				n, _ := sourceBridgeUTF8(child.object, method.NameIndex)
				d, _ := sourceBridgeUTF8(child.object, method.DescriptorIndex)
				if variant == "wrong descriptor" {
					d = "()V"
				}
				ni := nativeMemberLambdaTestUTF8(foreign, n)
				di := nativeMemberLambdaTestUTF8(foreign, d)
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: ni, DescriptorIndex: di})
				ntIndex := uint16(len(foreign.ConstantPool))
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: classIndex, NameAndTypeIndex: ntIndex}})
				if variant == "foreign handle" {
					foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(len(foreign.ConstantPool))})
				}
				input["LambdaProofForeign.class"] = foreign.Bytes()
			}
			archive := nativeArchive(t, input)
			defer archive.Close()
			index := archive.originalMemberIndex()
			if !index.valid {
				t.Fatal("original archive index")
			}
			if variant == "unresolved user" {
				if index.typeUsers[child.object.GetClassName()] == nil {
					index.typeUsers[child.object.GetClassName()] = map[string]bool{}
				}
				index.typeUsers[child.object.GetClassName()]["MissingOriginalUser"] = true
			}
			want := variant == "original" || variant == "wrong descriptor"
			if got := archive.nativeMemberLambdaArchiveClosed(child, index, nil); got != want {
				t.Fatalf("archive closed=%v", got)
			}
			if variant == "foreign handle" && nativeMemberOrdinaryHandlesClosed(child.object, index, nil, child) {
				t.Fatal("foreign lookup privilege admitted")
			}
		})
	}
}

func TestNativeMemberLambdaFunctionalTargetUsesMaximallySpecificOriginalDeclarations(t *testing.T) {
	files := nativeCompileClasses(t, `interface LambdaDirect {Object get();}
interface LambdaParent {Object get();}
interface LambdaInherited extends LambdaParent {}
interface LambdaDefault extends LambdaParent {default Object get(){return null;}}
interface LambdaReabstract extends LambdaDefault {Object get();}
interface LambdaOther {Object get();}
interface LambdaDiamond extends LambdaParent,LambdaOther {}
interface LambdaMultiple {Object get();void run();}
interface LambdaObjectMethod {Object get();boolean equals(Object v);}
interface LambdaStatic {static Object get(){return null;}void run();}
class LambdaConcrete {public Object get(){return null;}}`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"LambdaDirect", true}, {"LambdaInherited", true}, {"LambdaDefault", false}, {"LambdaReabstract", true}, {"LambdaDiamond", true}, {"LambdaMultiple", false}, {"LambdaObjectMethod", true}, {"LambdaStatic", false}, {"LambdaConcrete", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := NewClassObjectDumper(&ClassObject{})
			fallback := reader.nativeAnnotationDeclarationResolver()
			resolve := func(n string) (*ClassObject, bool) {
				if raw, ok := files[n+".class"]; ok {
					o, e := Parse(append([]byte(nil), raw...))
					return o, e == nil
				}
				return fallback(n)
			}
			if got := nativeLambdaFunctionalTarget("L"+test.name+";", "get", "()Ljava/lang/Object;", resolve, nil); got != test.want {
				t.Fatalf("functional=%v", got)
			}
		})
	}
}
