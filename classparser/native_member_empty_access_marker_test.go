package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberEmptyMarkerRequiresOriginalNonExecutableRole(t *testing.T) {
	files := nativeCompileClasses(t, `class EmptyMarkerOwner{static class Child{private Child(long n){}}Child make(long n){return new Child(n);}}`)
	for _, variant := range []string{"original", "wrong major", "minor preview", "not synthetic", "not SUPER", "interface", "field", "method", "missing enclosing", "duplicate enclosing", "wrong owner", "named row", "row public", "duplicate row", "unknown annotation", "foreign ordinal", "method handle constant", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, _ := Parse(append([]byte(nil), files["EmptyMarkerOwner$1.class"]...))
			var inner *InnerClassesAttribute
			var work *workbudget.Budget
			for _, a := range obj.Attributes {
				if x, ok := a.(*InnerClassesAttribute); ok {
					inner = x
				}
			}
			owner := "EmptyMarkerOwner"
			switch variant {
			case "wrong major":
				obj.MajorVersion = 55
			case "minor preview":
				obj.MinorVersion = 65535
			case "not synthetic":
				obj.AccessFlags &^= 0x1000
			case "not SUPER":
				obj.AccessFlags &^= 0x20
			case "interface":
				obj.Interfaces = []uint16{obj.SuperClass}
			case "field":
				obj.Fields = []*MemberInfo{{}}
			case "method":
				obj.Methods = []*MemberInfo{{}}
			case "missing enclosing":
				obj.Attributes = []AttributeInfo{inner}
			case "duplicate enclosing":
				for _, a := range obj.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						obj.Attributes = append(obj.Attributes, a)
						break
					}
				}
			case "wrong owner":
				owner = "Other"
			case "named row":
				inner.Classes[0].InnerNameIndex = inner.Classes[0].InnerClassInfoIndex
			case "row public":
				inner.Classes[0].InnerClassAccessFlags |= 1
			case "duplicate row":
				inner.Classes = append(inner.Classes, inner.Classes[0])
			case "unknown annotation":
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleAnnotations"})
			case "foreign ordinal":
				owner += "$1"
			case "method handle constant":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberEmptyAccessMarker(obj, owner, work); got != (variant == "original") {
				t.Fatalf("empty role=%v", got)
			}
		})
	}
}
func TestNativeMemberEmptyMarkerRefusesForeignArchiveUsers(t *testing.T) {
	source := `class EmptyMarkerOwner{static class Child{private Child(long n){}}Child make(long n){return new Child(n);}}class MarkerForeign{}`
	original := nativeCompileClasses(t, source)
	for _, variant := range []string{"original", "foreign marker field", "foreign marker handle", "foreign signature marker"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range original {
				files[n] = append([]byte(nil), b...)
			}
			if variant != "original" {
				obj, _ := Parse(files["MarkerForeign.class"])
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "LEmptyMarkerOwner$1;"})
				desc := uint16(len(obj.ConstantPool))
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "marker"})
				name := uint16(len(obj.ConstantPool))
				if variant == "foreign marker field" || variant == "foreign signature marker" {
					obj.Fields = append(obj.Fields, &MemberInfo{AccessFlags: 8, NameIndex: name, DescriptorIndex: desc})
				}
				if variant == "foreign signature marker" {
					obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "Ljava/lang/Object;"})
					obj.Fields[0].DescriptorIndex = uint16(len(obj.ConstantPool))
					obj.Fields[0].Attributes = []AttributeInfo{&SignatureAttribute{SignatureIndex: desc}}
				}
				if variant == "foreign marker handle" {
					obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "EmptyMarkerOwner$Child"})
					utf := uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: utf})
					class := uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "<init>"})
					name = uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "(JLEmptyMarkerOwner$1;)V"})
					desc = uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc})
					nt := uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: class, NameAndTypeIndex: nt}})
					ref := uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: ref})
				}
				files["MarkerForeign.class"] = obj.Bytes()
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["EmptyMarkerOwner.class"])
			e := z.nativeMemberEntry(root)
			got := e != nil && e.family != nil
			if got != (variant == "original") {
				t.Fatalf("archive joint proof=%v", got)
			}
		})
	}
}

func TestNativeMemberBridgeMarkerCannotLeakIntoDeclarationAttributes(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateBridgeFixture)
	for _, variant := range []string{"original", "class bound", "field signature", "method signature", "class annotation", "parameter annotation", "code type annotation", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, _ := Parse(append([]byte(nil), files["BridgeOwner.class"]...))
			marker := "BridgeOwner$1"
			var work *workbudget.Budget
			sig := func(s string) AttributeInfo {
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: s})
				return &SignatureAttribute{SignatureIndex: uint16(len(obj.ConstantPool))}
			}
			a := &AnnotationAttribute{TypeName: "LMarkerAnnotation;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "value", Tag: 'c', Value: "L" + marker + ";"}}}
			switch variant {
			case "class bound":
				obj.Attributes = append(obj.Attributes, sig("<T:L"+marker+";>Ljava/lang/Object;"))
			case "field signature":
				obj.Fields = append(obj.Fields, &MemberInfo{Attributes: []AttributeInfo{sig("L" + marker + ";")}})
			case "method signature":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, sig("()L"+marker+";"))
			case "class annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleAnnotationsAttribute{Annotations: []*AnnotationAttribute{a}})
			case "parameter annotation":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, &RuntimeVisibleParameterAnnotationsAttribute{ParameterAnnotations: [][]*AnnotationAttribute{{a}}})
			case "code type annotation":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, &CodeAttribute{Attributes: []AttributeInfo{&TypeAnnotationsAttribute{Annotations: []*TypeAnnotation{{Annotation: a}}}}})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberBridgeMarkerAttributesClosed(obj, map[string]bool{marker: true}, work); got != (variant == "original") {
				t.Fatalf("attribute role=%v", got)
			}
		})
	}
}
