package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaImplementationKindKeepsClassInterfaceAndReceiverDomains(t *testing.T) {
	for _, anInterface := range []bool{false, true} {
		for _, static := range []bool{false, true} {
			for kind := uint8(0); kind <= 9; kind++ {
				for _, interfaceTag := range []bool{false, true} {
					t.Run(fmt.Sprintf("interface%v-static%v-kind%d-tag%v", anInterface, static, kind, interfaceTag), func(t *testing.T) {
						object := &ClassObject{}
						if anInterface {
							object.AccessFlags = 0x0601
						}
						method := &MemberInfo{AccessFlags: 0x1002}
						if static {
							method.AccessFlags |= 8
						}
						// Independent protocol matrix: constructor/field kinds
						// cannot implement this body, virtual uses a class tag,
						// and bound interface bodies use special/interface kinds.
						allowed := map[uint8]bool{5: true, 7: true}
						if anInterface {
							allowed = map[uint8]bool{7: true, 9: true}
						}
						if static {
							allowed = map[uint8]bool{6: true}
						}
						want := interfaceTag == anInterface && allowed[kind]
						if got := nativeLambdaImplementationKind(object, method, kind, interfaceTag); got != want {
							t.Fatalf("implementation invocation shape=%v want=%v", got, want)
						}
					})
				}
			}
		}
	}
}

func TestNativeMemberInterfaceLambdaKindRequiresActualFactoryAndExactParameterSchema(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original member-interface metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, ownedInterfaceLambdaFixture(), "UncapturedLambdaNames", "none")
	raw := files["UncapturedLambdaNames$Gate.class"]
	for _, variant := range []string{"original", "public synthetic", "ordinary private", "synchronized", "wrong handle kind", "unreferenced factory", "shared metadata factory", "direct invocation", "duplicate declaration", "duplicate parameter table", "unknown parameter flag", "unknown attribute", "missing Code", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(raw))
			if err != nil {
				t.Fatal(err)
			}
			_, _, flags, known := originalMemberOwner(object)
			if !known || flags != 0x0609 {
				t.Fatal("original static member-interface declaration")
			}
			var method *MemberInfo
			for _, candidate := range object.Methods {
				if candidate.AccessFlags == 0x1002 {
					method = candidate
				}
			}
			p, known := nativeLambdaBodyDeclarationSchema(object, method, nil)
			if !known || p == nil || p.factory == nil || !p.factory.sealed || !nativeMemberDeclarationKindRepresentable(object, flags, nil) {
				t.Fatal("original implementation/factory/schema must be admitted as planning evidence")
			}
			var work *workbudget.Budget
			switch variant {
			case "public synthetic":
				method.AccessFlags = 0x1001
			case "ordinary private":
				method.AccessFlags = 2
			case "synchronized":
				method.AccessFlags |= 0x20
			case "wrong handle kind", "direct invocation":
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				for _, item := range object.ConstantPool {
					if handle, ok := item.(*ConstantMethodHandleInfo); ok {
						if target, known := nativeLambdaOriginalHandleTarget(object, handle); known && target.owner == object.GetClassName() && target.name == name {
							if variant == "wrong handle kind" {
								handle.ReferenceKind = 6
							} else {
								index := handle.ReferenceIndex
								p.factory.code.Code = append(p.factory.code.Code, core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ICONST_0, core.OP_INVOKESPECIAL, byte(index>>8), byte(index), core.OP_POP, core.OP_RETURN)
							}
						}
					}
				}
			case "unreferenced factory":
				for i := 0; i < 5; i++ {
					p.factory.code.Code[p.factory.pc+i] = core.OP_NOP
				}
			case "shared metadata factory":
				p.factory.code.Code = append(p.factory.code.Code, p.factory.code.Code...)
			case "duplicate declaration":
				copy := *method
				object.Methods = append(object.Methods, &copy)
			case "duplicate parameter table":
				method.Attributes = append(method.Attributes, p.table)
			case "unknown parameter flag":
				p.table.Info[3] |= 0x80 // mandated is not javac's captured-name role
			case "unknown attribute":
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "RuntimeInvisibleAnnotations", Length: 2, Info: []byte{0, 0}})
			case "missing Code":
				var kept []AttributeInfo
				for _, attribute := range method.Attributes {
					if _, ok := attribute.(*CodeAttribute); !ok {
						kept = append(kept, attribute)
					}
				}
				method.Attributes = kept
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			if got := nativeMemberDeclarationKindRepresentable(object, flags, work); got != (variant == "original") || !bytes.Equal(before, object.Bytes()) {
				t.Fatalf("member-interface declaration kind=%v", got)
			}
		})
	}
}
