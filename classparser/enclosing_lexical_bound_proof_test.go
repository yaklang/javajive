package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnclosingBoundsRespectDeclarationScopeAndStaticCuts(t *testing.T) {
	files := nativeCompileClasses(t, `class BoundProof<T extends Number,U extends T> {
 class Member {T first;U second;}
 class Shadow<T> {class Leaf {T first;}}
 static class Cut<V> {class Leaf {V first;}}
}
class BoundProof$Pretender {}
class ForeignBound<Q extends CharSequence> {}`)
	for _, variant := range []string{"external", "archive", "shadowed", "static cut", "literal dollar", "missing owner", "foreign owner", "duplicate owner signature", "malformed owner signature", "duplicate membership", "missing membership", "unclosed bound", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			name := "BoundProof$Member"
			switch variant {
			case "shadowed":
				name = "BoundProof$Shadow$Leaf"
			case "static cut":
				name = "BoundProof$Cut$Leaf"
			case "literal dollar":
				name = "BoundProof$Pretender"
			}
			obj := mustParseLowering(t, bytes.Clone(files[name+".class"]))
			parent := mustParseLowering(t, bytes.Clone(files["BoundProof.class"]))
			switch variant {
			case "duplicate owner signature":
				for _, attr := range parent.Attributes {
					if _, ok := attr.(*SignatureAttribute); ok {
						parent.Attributes = append(parent.Attributes, attr)
						break
					}
				}
			case "malformed owner signature":
				nativeReplaceOriginalSignature(t, parent, parent.Attributes, "<T:Ljava/lang/Number;>Ljava/lang/Object;trailing")
			case "duplicate membership", "missing membership":
				for _, attr := range obj.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							if row.InnerClassInfoIndex == obj.ThisClass {
								if variant == "duplicate membership" {
									table.Classes = append(table.Classes, row)
								} else {
									row.InnerClassInfoIndex = obj.SuperClass
								}
								break
							}
						}
					}
				}
			}
			resolve := func(n string) ([]byte, bool) {
				if n == "BoundProof" {
					if variant == "missing owner" {
						return nil, false
					}
					if variant == "foreign owner" {
						return files["ForeignBound.class"], true
					}
					return parent.Bytes(), true
				}
				b, ok := files[n+".class"]
				return b, ok
			}
			d := NewClassObjectDumper(obj)
			if variant == "archive" {
				d.foldSiblingResolver = resolve
			} else {
				d.declarationResolver = resolve
			}
			switch variant {
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			requested := []string{"T", "U"}
			if variant == "shadowed" || variant == "static cut" || variant == "literal dollar" {
				requested = []string{"T"}
			} else if variant == "unclosed bound" {
				requested = []string{"U"}
			}
			got := d.enclosingTypeParamBounds(requested)
			if variant == "external" || variant == "archive" {
				if got["T"] != "Number" || got["U"] != "T" {
					t.Fatal("original dependent declaration bounds", got)
				}
			} else if len(got) != 0 {
				t.Fatal("unproved or out-of-scope bound", got)
			}
		})
	}
}
