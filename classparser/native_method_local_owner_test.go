package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Metadata ownership alone must not authorize private/source access decisions.
// Every control starts from freshly parsed authored original class bytes.
func TestNativeMethodLocalOwnerRequiresJointOriginalDeclarations(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalFactOwner {Object first(){class Entry{}return new Entry();}Object second(){class Entry{}return new Entry();}}`, debug)
			for _, variant := range []string{"first", "second", "pre EnclosingMethod version", "foreign owner", "absent declaring method", "wrong descriptor", "duplicate method", "missing EnclosingMethod", "duplicate EnclosingMethod", "bad attribute length", "no method index", "wrong CP kind", "missing self row", "duplicate self row", "unnamed row", "member outer row", "unsafe source name", "static self row", "missing parent corroboration", "duplicate parent corroboration", "wrong parent flags", "overlapping registration", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LocalFactOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					ordinal := "1"
					methodName := "first"
					if variant == "second" {
						ordinal = "2"
						methodName = "second"
					}
					local, e := Parse(append([]byte(nil), files["LocalFactOwner$"+ordinal+"Entry.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					var attr *UnparsedAttribute
					var self *InnerClassInfo
					var parent *InnerClassesAttribute
					var declaration *MemberInfo
					for _, a := range local.Attributes {
						if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
							attr = raw
						}
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								n, _ := sourceBridgeClassName(local, row.InnerClassInfoIndex)
								if n == local.GetClassName() {
									self = row
								}
							}
						}
					}
					for _, a := range root.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							parent = table
						}
					}
					for _, m := range root.Methods {
						n, _ := sourceBridgeUTF8(root, m.NameIndex)
						if n == methodName {
							declaration = m
						}
					}
					if attr == nil || self == nil || parent == nil || declaration == nil {
						t.Fatal("original joint declaration")
					}
					var work *workbudget.Budget
					switch variant {
					case "pre EnclosingMethod version":
						local.MajorVersion = 48
					case "foreign owner":
						root = local
					case "absent declaring method":
						root.Methods = nil
					case "wrong descriptor":
						declaration.DescriptorIndex = root.ThisClass
					case "duplicate method":
						root.Methods = append(root.Methods, declaration)
					case "missing EnclosingMethod":
						attr.Name = "unknown"
					case "duplicate EnclosingMethod":
						local.Attributes = append(local.Attributes, attr)
					case "bad attribute length":
						attr.Length = 3
					case "no method index":
						attr.Info[2], attr.Info[3] = 0, 0
					case "wrong CP kind":
						attr.Info[2], attr.Info[3] = byte(local.ThisClass>>8), byte(local.ThisClass)
					case "missing self row":
						self.InnerClassInfoIndex = local.SuperClass
					case "duplicate self row":
						for _, a := range local.Attributes {
							if table, ok := a.(*InnerClassesAttribute); ok {
								table.Classes = append(table.Classes, self)
							}
						}
					case "unnamed row":
						self.InnerNameIndex = 0
					case "member outer row":
						self.OuterClassInfoIndex = local.ThisClass
					case "unsafe source name":
						local.ConstantPool[self.InnerNameIndex-1].(*ConstantUtf8Info).Value = "not valid"
					case "static self row":
						self.InnerClassAccessFlags |= 8
					case "missing parent corroboration":
						parent.Classes = nil
					case "duplicate parent corroboration":
						for _, row := range parent.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								parent.Classes = append(parent.Classes, row)
								break
							}
						}
					case "wrong parent flags":
						for _, row := range parent.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == local.GetClassName() {
								row.InnerClassAccessFlags ^= 16
							}
						}
					case "overlapping registration":
						originalName := local.GetClassName()
						for _, constant := range local.ConstantPool {
							if text, ok := constant.(*ConstantUtf8Info); ok {
								if text.Value == originalName || text.Value == "Entry" {
									text.Value = "LocalFactOwner$"
								}
							}
						}
						for _, constant := range root.ConstantPool {
							if text, ok := constant.(*ConstantUtf8Info); ok {
								if text.Value == originalName || text.Value == "Entry" {
									text.Value = "LocalFactOwner$"
								}
							}
						}
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					proof, known := originalMethodLocalOwner(local, root, work)
					if known != (variant == "first" || variant == "second") {
						t.Fatalf("ownership known=%v proof=%+v", known, proof)
					}
					if known && (proof.owner != "LocalFactOwner" || proof.method != methodName || proof.descriptor != "()Ljava/lang/Object;" || proof.name != "Entry" || proof.ordinal != int(ordinal[0]-'0') || proof.declaration != declaration) {
						t.Fatalf("wrong original method-local identity %+v", proof)
					}
				})
			}
		})
	}
}
