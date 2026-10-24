package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestMemberGenericSupertypeProjectionKeepsTypedArgumentsAndOwners(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"Outer.java": `class Outer<T> {class Child<U> {}}`}, "none", "8")
	resolve := func(name string) (*ClassObject, bool) {
		b, k := files[name+".class"]
		if !k {
			return nil, false
		}
		return mustParseLowering(t, b), true
	}
	original := `LOuter<TT;>.Child<Ljava/util/List<+[TT;>;>;`
	projected := `LOuter$Child<Ljava/util/List<+[TT;>;>;`
	for _, variant := range []string{"control", "unchanged", "changed variable", "class instead of variable", "variance", "dimensions", "raw child", "raw argument", "wrong child", "missing owner", "wrong original owner", "static cut", "wrong arity", "duplicate ownership", "trailing junk", "method signature", "primitive superclass", "array superclass", "variable superclass", "variable interface", "array interface", "declaration formals", "budget"} {
		t.Run(variant, func(t *testing.T) {
			left, right := original, projected
			r := resolve
			var work *workbudget.Budget
			switch variant {
			case "unchanged":
				right = left
			case "changed variable":
				right = strings.ReplaceAll(right, "TT;", "TU;")
			case "class instead of variable":
				right = strings.ReplaceAll(right, "TT;", "LT;")
			case "variance":
				right = strings.ReplaceAll(right, "+", "-")
			case "dimensions":
				right = strings.ReplaceAll(right, "[", "[[")
			case "raw child":
				right = `LOuter$Child;`
			case "raw argument":
				right = `LOuter$Child<Ljava/util/List;>;`
			case "wrong child":
				right = strings.ReplaceAll(right, "Child", "Other")
			case "missing owner":
				r = func(string) (*ClassObject, bool) { return nil, false }
			case "wrong original owner", "static cut", "wrong arity", "duplicate ownership":
				r = func(name string) (*ClassObject, bool) {
					o, k := resolve(name)
					if !k {
						return nil, false
					}
					if name != "Outer$Child" {
						return o, k
					}
					for _, a := range o.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								if row.InnerClassInfoIndex == o.ThisClass {
									switch variant {
									case "wrong original owner":
										row.OuterClassInfoIndex = o.SuperClass
									case "static cut":
										row.InnerClassAccessFlags |= StaticFlag
									case "duplicate ownership":
										copy := *row
										copy.OuterClassInfoIndex = o.SuperClass
										table.Classes = append(table.Classes, &copy)
									}
								}
							}
						}
						if sig, ok := a.(*SignatureAttribute); ok && variant == "wrong arity" {
							sig.SignatureIndex = uint16(o.ConstantPoolManager.AddUtf8Info("<U:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;"))
						}
					}
					return o, true
				}
			case "trailing junk":
				right += "!"
			case "primitive superclass":
				left, right = "I", "I"
			case "array superclass":
				left, right = "[Ljava/lang/Object;", "[Ljava/lang/Object;"
			case "variable superclass":
				left, right = "TT;", "TT;"
			case "variable interface":
				left, right = "Ljava/lang/Object;TT;", "Ljava/lang/Object;TT;"
			case "array interface":
				left, right = "Ljava/lang/Object;[Ljava/util/List;", "Ljava/lang/Object;[Ljava/util/List;"
			case "method signature":
				left, right = "()V", "()V"
			case "declaration formals":
				left, right = "<T:Ljava/lang/Object;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if got, want := memberGenericSupertypeProjection(left, right, r, work), variant == "control" || variant == "unchanged"; got != want {
				t.Fatalf("projection admitted=%v want=%v", got, want)
			}
		})
	}
}
