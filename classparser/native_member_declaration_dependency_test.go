package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeIndependentDeclarationDependencyRequiresOriginalAccessibleEdges(t *testing.T) {
	files := nativeCompileClasses(t, foreignMemberDeclarationFixture())
	packaged := nativeCompileSourceReleaseClasses(t, map[string]string{
		"decl/provider/DependencyBase.java":  "package decl.provider;public class DependencyBase{public class Value{}}",
		"decl/client/DependencyDerived.java": "package decl.client;public class DependencyDerived{class Item{}}",
	}, "none", "8")
	variants := []string{"original", "public", "same-package protected", "cross-package public", "nil root", "nil member", "nil resolver", "same owner", "private", "static", "cross-package package edge", "cross-package protected", "cross-package inaccessible root", "cross-package public default owner", "missing owner", "foreign owner object", "missing self", "duplicate self", "wrong self owner", "nil self row", "nil self table", "oversize attributes", "oversize rows", "missing reciprocal", "duplicate reciprocal", "wrong reciprocal owner", "wrong reciprocal name", "wrong reciprocal flags", "root member metadata", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			originals := files
			rootName, ownerName := "DependencyDerived", "DependencyBase"
			if strings.HasPrefix(variant, "cross-package") && variant != "cross-package public default owner" {
				originals = packaged
				rootName, ownerName = "decl/client/DependencyDerived", "decl/provider/DependencyBase"
			}
			for name, raw := range originals {
				o, err := Parse(append([]byte(nil), raw...))
				if err != nil {
					t.Fatal(err)
				}
				objects[strings.TrimSuffix(name, ".class")] = o
			}
			root, member, parent := objects[rootName], objects[ownerName+"$Value"], objects[ownerName]
			resolve := func(name string) (*ClassObject, bool) { o := objects[name]; return o, o != nil }
			var work *workbudget.Budget
			find := func(obj *ClassObject) (*InnerClassesAttribute, *InnerClassInfo) {
				for _, attribute := range obj.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok && table != nil {
						for _, row := range table.Classes {
							if row != nil {
								name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
								if known && name == ownerName+"$Value" {
									return table, row
								}
							}
						}
					}
				}
				t.Fatal("original declaration edge absent")
				return nil, nil
			}
			selfTable, self := find(member)
			parentTable, reciprocal := find(parent)
			if variant == "cross-package public default owner" {
				constant := root.ConstantPool[root.ThisClass-1].(*ConstantClassInfo)
				root.ConstantPool[constant.NameIndex-1].(*ConstantUtf8Info).SetString("external/DependencyDerived")
			}
			switch variant {
			case "public", "cross-package public", "cross-package inaccessible root", "cross-package public default owner":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 1, 1
				if variant == "cross-package public" || variant == "cross-package public default owner" {
					parent.AccessFlags |= 1
				}
				if variant == "cross-package inaccessible root" {
					parent.AccessFlags &^= 1
				}
			case "cross-package package edge":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 0, 0
			case "same-package protected", "cross-package protected":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 4, 4
			case "nil root":
				root = nil
			case "nil member":
				member = nil
			case "nil resolver":
				resolve = nil
			case "same owner":
				root = parent
			case "private":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 2, 2
			case "static":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 8, 8
			case "missing owner":
				delete(objects, ownerName)
			case "foreign owner object":
				objects[ownerName] = root
			case "missing self":
				selfTable.Classes = nil
			case "duplicate self":
				selfTable.Classes = append(selfTable.Classes, self)
			case "wrong self owner":
				self.OuterClassInfoIndex = member.ThisClass
			case "nil self row":
				selfTable.Classes = append(selfTable.Classes, nil)
			case "nil self table":
				member.Attributes = append(member.Attributes, (*InnerClassesAttribute)(nil))
			case "oversize attributes":
				member.Attributes = make([]AttributeInfo, 65536)
			case "oversize rows":
				selfTable.Classes = make([]*InnerClassInfo, 65536)
			case "missing reciprocal":
				parentTable.Classes = nil
			case "duplicate reciprocal":
				parentTable.Classes = append(parentTable.Classes, reciprocal)
			case "wrong reciprocal owner":
				reciprocal.OuterClassInfoIndex = parent.SuperClass
			case "wrong reciprocal name":
				reciprocal.InnerNameIndex = reciprocal.InnerClassInfoIndex
			case "wrong reciprocal flags":
				reciprocal.InnerClassAccessFlags = 1
			case "root member metadata":
				root.Attributes = append(root.Attributes, &InnerClassesAttribute{Classes: []*InnerClassInfo{{InnerClassInfoIndex: root.ThisClass}}})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "public" || variant == "same-package protected" || variant == "cross-package public"
			if got := nativeMemberIndependentDeclarationDependency(root, member, resolve, work); got != want {
				t.Fatalf("independent declaration admitted=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeIndependentDeclarationDependencyRequiresEveryEnclosingEdge(t *testing.T) {
	files := nativeCompileClasses(t, `class ForeignDeclaration { class Context { public class Value {} } } class IndependentDeclaration { class Local {} }`)
	for _, variant := range []string{"original", "private enclosing", "missing enclosing", "duplicate enclosing", "wrong enclosing name", "enclosing cycle"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for name, raw := range files {
				obj, err := Parse(append([]byte(nil), raw...))
				if err != nil {
					t.Fatal(err)
				}
				objects[strings.TrimSuffix(name, ".class")] = obj
			}
			root, parent, context := objects["IndependentDeclaration"], objects["ForeignDeclaration"], objects["ForeignDeclaration$Context"]
			find := func(obj *ClassObject) (*InnerClassesAttribute, *InnerClassInfo) {
				for _, attribute := range obj.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok && table != nil {
						for _, row := range table.Classes {
							if row != nil {
								if name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex); known && name == context.GetClassName() {
									return table, row
								}
							}
						}
					}
				}
				t.Fatal("original intermediate enclosing edge absent")
				return nil, nil
			}
			_, self := find(context)
			table, reciprocal := find(parent)
			switch variant {
			case "private enclosing":
				self.InnerClassAccessFlags, reciprocal.InnerClassAccessFlags = 2, 2
			case "missing enclosing":
				delete(objects, parent.GetClassName())
			case "duplicate enclosing":
				table.Classes = append(table.Classes, reciprocal)
			case "wrong enclosing name":
				reciprocal.InnerNameIndex = reciprocal.InnerClassInfoIndex
			case "enclosing cycle":
				self.OuterClassInfoIndex = context.ThisClass
			}
			resolve := func(name string) (*ClassObject, bool) { obj := objects[name]; return obj, obj != nil }
			if got := nativeMemberIndependentDeclarationDependency(root, objects["ForeignDeclaration$Context$Value"], resolve, nil); got != (variant == "original") {
				t.Fatalf("complete enclosing path admitted=%v", got)
			}
		})
	}
}
