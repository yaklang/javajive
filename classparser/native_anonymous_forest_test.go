package javaclassparser

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousForestRequiresCompleteOriginalDeclarations(t *testing.T) {
	base := nativeCompileClasses(t, nativeAnonymousNestedFixture)
	for _, variant := range []string{"original", "missing first child", "missing leaf", "wrong identity", "owner cycle", "missing enclosing", "duplicate enclosing", "duplicate self", "named self", "capture writable", "capture descriptor", "capture metadata", "capture store computed", "missing parent row", "modern root", "modern child", "minor root", "minor child", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range base {
				files[n] = append([]byte(nil), b...)
			}
			path := "NestedOwner$1$1.class"
			if strings.Contains(variant, "root") {
				path = "NestedOwner.class"
			}
			if variant == "missing parent row" {
				path = "NestedOwner$1.class"
			}
			object, e := Parse(files[path])
			if e != nil {
				t.Fatal(e)
			}
			cp := NewConstantPoolWithConstant(&object.ConstantPool)
			switch variant {
			case "missing first child":
				delete(files, "NestedOwner$1.class")
			case "missing leaf":
				delete(files, "NestedOwner$1$1.class")
			case "wrong identity":
				object.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "owner cycle", "missing enclosing", "duplicate enclosing":
				for i, a := range object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						if variant == "owner cycle" {
							binary.BigEndian.PutUint16(raw.Info, object.ThisClass)
						}
						if variant == "missing enclosing" {
							object.Attributes = append(object.Attributes[:i], object.Attributes[i+1:]...)
						}
						if variant == "duplicate enclosing" {
							copy := *raw
							copy.Info = append([]byte(nil), raw.Info...)
							object.Attributes = append(object.Attributes, &copy)
						}
						break
					}
				}
			case "duplicate self", "named self", "missing parent row":
				for _, a := range object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							name, _ := sourceBridgeClassName(object, row.InnerClassInfoIndex)
							if variant == "missing parent row" && name == "NestedOwner$1$1" {
								table.Classes = append(table.Classes[:i], table.Classes[i+1:]...)
								break
							}
							if name != object.GetClassName() {
								continue
							}
							if variant == "named self" {
								row.InnerNameIndex = uint16(cp.AddUtf8Info("Named"))
							}
							if variant == "duplicate self" {
								copy := *row
								table.Classes = append(table.Classes, &copy)
								break
							}
						}
						table.NumberOfClasses = uint16(len(table.Classes))
						table.AttrLen = uint32(2 + 8*len(table.Classes))
					}
				}
			case "capture writable", "capture descriptor", "capture metadata":
				for _, field := range object.Fields {
					name, _ := sourceBridgeUTF8(object, field.NameIndex)
					if !strings.HasPrefix(name, "this$") {
						continue
					}
					if variant == "capture writable" {
						field.AccessFlags &^= 0x10
					}
					if variant == "capture descriptor" {
						field.DescriptorIndex = uint16(cp.AddUtf8Info("Ljava/lang/Object;"))
					}
					if variant == "capture metadata" {
						cp.AddUtf8Info("Signature")
						field.Attributes = append(field.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info("LNestedOwner$1;"))})
					}
				}
			case "capture store computed":
				for _, m := range object.Methods {
					name, _ := sourceBridgeUTF8(object, m.NameIndex)
					if name != "<init>" {
						continue
					}
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							if code.Code[1] != byte(core.OP_ALOAD_1) {
								t.Fatal("original capture input")
							}
							code.Code[1] = byte(core.OP_ACONST_NULL)
						}
					}
				}
			case "modern root", "modern child":
				object.MajorVersion = 55
			case "minor root", "minor child":
				object.MinorVersion = 1
			}
			if variant != "missing first child" && variant != "missing leaf" {
				files[path] = object.Bytes()
				if _, e := Parse(files[path]); e != nil {
					var parseError *ClassParseError
					if variant == "duplicate enclosing" && errors.As(e, &parseError) && parseError.Code == ParseCodeAttrDuplicate {
						if _, _, owned := originalAnonymousOwner(object); owned {
							t.Fatal("duplicate original owner accepted")
						}
						return
					}
					t.Fatalf("mutation parse %v", e)
				}
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			d := z.nativeMemberReader(root)
			if variant == "budget" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			p := d.planNativeAnonymousFamily()
			if (p != nil) != (variant == "original") {
				t.Fatalf("whole-family verdict %v", p != nil)
			}
		})
	}
}

func TestNativeAnonymousForestRequiresClosedArchiveUsers(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousNestedFixture)
	for _, variant := range []string{"original", "foreign type user", "foreign capture user", "method handle", "invalid index", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			p := z.nativeMemberReader(root).planNativeAnonymousForest()
			if p == nil {
				t.Fatal("original forest")
			}
			original := z.originalMemberIndex()
			index := &nativeMemberIndex{valid: original.valid, typeUsers: map[string]map[string]bool{}, captureUsers: map[string]map[string]bool{}, handles: map[string]bool{}}
			for key, users := range original.typeUsers {
				index.typeUsers[key] = map[string]bool{}
				for user := range users {
					index.typeUsers[key][user] = true
				}
			}
			for key, users := range original.captureUsers {
				index.captureUsers[key] = map[string]bool{}
				for user := range users {
					index.captureUsers[key][user] = true
				}
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign type user":
				index.typeUsers["NestedOwner$1"]["Foreign"] = true
			case "foreign capture user":
				index.captureUsers[nativeMemberCaptureIndexKey("NestedOwner$1$1", "this$0")]["Foreign"] = true
			case "method handle":
				index.handles["NestedOwner$1$1"] = true
			case "invalid index":
				index.valid = false
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := nativeAnonymousForestArchiveClosed(p.forest, index, work); known != (variant == "original") {
				t.Fatalf("archive closure %v", known)
			}
		})
	}
}

func TestNativeAnonymousForestRequiresEveryFinalSourceScope(t *testing.T) {
	root := &nativeAnonymousFamily{owner: "Root", children: map[string]*nativeAnonymousClass{"Root$1": {}}}
	inner := &nativeAnonymousFamily{owner: "Root$1", children: map[string]*nativeAnonymousClass{"Root$1$1": {}, "Root$1$2": {}}}
	forest := &nativeAnonymousForest{groups: map[string]*nativeAnonymousFamily{"Root": root, "Root$1": inner}}
	root.forest = forest
	valid := `/*jdec-owned-anonymous-ordinal:1:Root*/new X(){/*jdec-owned-anonymous-ordinal:1:Root$1*/new X(){};/*jdec-owned-anonymous-ordinal:2:Root$1*/new X(){};}`
	for _, variant := range []string{"original", "fake literal", "missing root", "missing leaf", "wrong order", "foreign scope", "unscoped", "child failed"} {
		t.Run(variant, func(t *testing.T) {
			source := valid
			inner.failed = false
			switch variant {
			case "fake literal":
				source = `String s="/*jdec-owned-anonymous-ordinal:9:Foreign*/";` + source
			case "missing root":
				source = strings.Replace(source, "ordinal:1:Root*/", "ordinal:1:Root$1*/", 1)
			case "missing leaf":
				source = strings.Replace(source, "ordinal:2:Root$1*/", "other:2:Root$1*/", 1)
			case "wrong order":
				source = strings.Replace(source, "ordinal:1:Root$1*/", "ordinal:2:Root$1*/", 1)
			case "foreign scope":
				source += "/*jdec-owned-anonymous-ordinal:1:Foreign*/"
			case "unscoped":
				source += "/*jdec-owned-anonymous-ordinal:1*/"
			case "child failed":
				inner.failed = true
			}
			want := variant == "original" || variant == "fake literal"
			if got := root.completeSource(source); got != want {
				t.Fatalf("source closure %v", got)
			}
		})
	}
}
