package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberLexicalGraphRequiresCompleteOriginalOwnership(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberDeepNamedFixture)
	for _, variant := range []string{"original", "missing ancestor", "foreign ancestor", "duplicate self", "empty name", "foreign owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, _ := Parse(files["DeepOwner.class"])
			layer, _ := Parse(files["DeepOwner$Layer.class"])
			objects := map[string]*ClassObject{root.GetClassName(): root, layer.GetClassName(): layer}
			var self *InnerClassInfo
			for _, a := range layer.Attributes {
				if table, ok := a.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						if name, known := sourceBridgeClassName(layer, row.InnerClassInfoIndex); known && name == layer.GetClassName() {
							self = row
						}
					}
				}
			}
			if self == nil {
				t.Fatal("original selfrow")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing ancestor":
				delete(objects, root.GetClassName())
			case "foreign ancestor":
				objects[layer.GetClassName()] = root
			case "duplicate self":
				for _, a := range layer.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						table.Classes = append(table.Classes, self)
					}
				}
			case "empty name":
				self.InnerNameIndex = 0
			case "foreign owner":
				self.OuterClassInfoIndex = layer.ThisClass
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			name, known := nativeMemberLexicalCaptureField(layer, objects, work)
			if known != (variant == "original") {
				t.Fatalf("capture graph %q %v", name, known)
			}
			if known && name != "this$1" {
				t.Fatalf("capture spelling %s", name)
			}
		})
	}
}

func TestNativeMemberDeepForestRequiresEveryOriginalNamedDeclaration(t *testing.T) {
	base := nativeCompileClasses(t, nativeMemberDeepNamedFixture)
	for _, variant := range []string{"original", "missing leaf", "leaf identity", "duplicate leaf row", "mismatched leaf flags", "missing leaf name", "wrong capture spelling", "capture widened", "ancestor signature foreign"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range base {
				files[n] = append([]byte(nil), b...)
			}
			path := "DeepOwner$Layer$Leaf.class"
			if variant == "duplicate leaf row" || variant == "mismatched leaf flags" || variant == "missing leaf name" {
				path = "DeepOwner$Layer.class"
			}
			obj, e := Parse(files[path])
			if e != nil {
				t.Fatal(e)
			}
			switch variant {
			case "missing leaf":
				delete(files, "DeepOwner$Layer$Leaf.class")
			case "leaf identity":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				obj.ThisClass = uint16(cp.AddNewClassInfo("ForeignLeaf"))
			case "duplicate leaf row", "mismatched leaf flags", "missing leaf name":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if name == "DeepOwner$Layer$Leaf" {
								switch variant {
								case "duplicate leaf row":
									copy := *row
									table.Classes = append(table.Classes, &copy)
									table.NumberOfClasses = uint16(len(table.Classes))
									table.AttrLen = uint32(2 + 8*len(table.Classes))
								case "mismatched leaf flags":
									row.InnerClassAccessFlags |= 8
								case "missing leaf name":
									row.InnerNameIndex = 0
								}
								break
							}
						}
					}
				}
			case "wrong capture spelling", "capture widened":
				for _, field := range obj.Fields {
					name, _ := sourceBridgeUTF8(obj, field.NameIndex)
					if name == "this$1" {
						if variant == "capture widened" {
							field.AccessFlags &^= 0x10
						} else {
							obj.ConstantPool[field.NameIndex-1].(*ConstantUtf8Info).Value = "this$0"
						}
					}
				}
			case "ancestor signature foreign":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				cp.AddUtf8Info("Signature")
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info("LDeepParent<TUnknown;>;"))})
			}
			if variant != "missing leaf" {
				files[path] = obj.Bytes()
				if _, e := Parse(files[path]); e != nil {
					t.Fatalf("structural mutation parse %v", e)
				}
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["DeepOwner.class"])
			entry := z.nativeMemberEntry(root)
			known := entry != nil && entry.family != nil
			if known != (variant == "original") {
				t.Fatalf("whole forest closure %v", known)
			}
		})
	}
}
