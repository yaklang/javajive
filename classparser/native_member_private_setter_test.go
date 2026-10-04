package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativePrivateSetterRequiresExactOriginalWriteProtocol(t *testing.T) {
	files := nativeCompileClasses(t, nativePrivateSetterFixture)
	for _, owner := range []string{"SetterOwner", "SetterOwner$Layer"} {
		for _, variant := range []string{"original", "public", "non-static", "non-synthetic", "wrong operation", "foreign receiver", "wrong result", "wrong stack", "wrong locals", "extra opcode", "wrong duplicate", "wrong return", "handler", "opaque metadata", "duplicate code", "public field", "static field", "final field", "constant field", "budget", "canceled"} {
			t.Run(owner+"/"+variant, func(t *testing.T) {
				obj, e := Parse(files[owner+".class"])
				if e != nil {
					t.Fatal(e)
				}
				var method *MemberInfo
				var code *CodeAttribute
				var field *MemberInfo
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if strings.HasSuffix(n, "02") {
						method = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == "token" || n == "number" {
						field = f
					}
				}
				if method == nil || code == nil || field == nil {
					t.Fatal("missing original write accessor")
				}
				var work *workbudget.Budget
				switch variant {
				case "public":
					method.AccessFlags |= 1
				case "non-static":
					method.AccessFlags &^= 8
				case "non-synthetic":
					method.AccessFlags &^= 0x1000
				case "wrong operation":
					obj.ConstantPool[method.NameIndex-1].(*ConstantUtf8Info).Value = "access$008"
				case "foreign receiver":
					d, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
					obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = strings.Replace(d, "L"+owner+";", "Ljava/lang/Object;", 1)
				case "wrong result":
					obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = "(L" + owner + ";Ljava/lang/Object;)J"
				case "wrong stack":
					code.MaxStack++
				case "wrong locals":
					code.MaxLocals++
				case "extra opcode":
					code.Code = append([]byte{0}, code.Code...)
				case "wrong duplicate":
					code.Code[2] = 0x59
				case "wrong return":
					code.Code[6] = 0xb1
				case "handler":
					code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
				case "opaque metadata":
					code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque", Info: []byte{0}})
				case "duplicate code":
					method.Attributes = append(method.Attributes, code)
				case "public field":
					field.AccessFlags &^= 2
				case "static field":
					field.AccessFlags |= 8
				case "final field":
					field.AccessFlags |= 16
				case "constant field":
					field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				if got := nativeMemberPrivateAccessProof(obj, method, work); (got != nil) != (variant == "original") {
					t.Fatalf("write accessor accepted=%v", got != nil)
				}
			})
		}
	}
}

// A compound accessor evaluates its field read after the caller's RHS has
// already completed. A source '+=' may read earlier; this proof must refuse it.
func TestNativePrivateSetterRefusesCompoundAccess(t *testing.T) {
	source := strings.Replace(nativePrivateSetterFixture, ".number=SetterEffects.number(n)", ".number+=SetterEffects.number(n)", 1)
	files := nativeCompileClasses(t, source)
	obj, e := Parse(files["SetterOwner$Layer.class"])
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n == "access$114" {
			found = true
			if nativeMemberPrivateAccessProof(obj, m, nil) != nil {
				t.Fatal("compound accessor accepted")
			}
		}
	}
	if !found {
		t.Fatal("independent compiler did not produce compound accessor")
	}
}
