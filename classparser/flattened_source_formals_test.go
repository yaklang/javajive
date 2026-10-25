package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"testing"
)

func TestNativeFlattenedSourceFormalsRequireOriginalOwnership(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class ProjectedOwner<K,V>{class Range{K key;V value;boolean inRange(K k){return key==k;}}static class StaticRange{Object key;}class OwnRange<X>{X key;}class Level<U>{class Inner{U key;}}}`, "none")
	for _, row := range []struct {
		name, target string
		want         []string
	}{
		{"ordered enclosing", "ProjectedOwner$Range", []string{"K", "V"}},
		{"dependency ancestor", "ProjectedOwner$Range", []string{"K", "V"}},
		{"nearest declaration", "ProjectedOwner$Level$Inner", []string{"U"}},
		{"static boundary", "ProjectedOwner$StaticRange", nil},
		{"own declaration", "ProjectedOwner$OwnRange", nil},
		{"dependency only", "ProjectedOwner$Range", nil},
		{"wrong resolver identity", "ProjectedOwner$Range", nil},
		{"missing ancestor", "ProjectedOwner$Range", nil},
		{"native source name", "ProjectedOwner$Range", nil},
		{"native family member", "ProjectedOwner$Range", nil},
		{"duplicate signature", "ProjectedOwner$Range", nil},
		{"static original metadata", "ProjectedOwner$Range", nil},
		{"budget", "ProjectedOwner$Range", nil}, {"canceled", "ProjectedOwner$Range", nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			obj, e := Parse(files[row.target+".class"])
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(obj)
			d.FuncCtx = &class_context.ClassContext{}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if row.name == "dependency only" || (row.name == "missing ancestor" || row.name == "dependency ancestor") && name == "ProjectedOwner" {
					return nil, false
				}
				if row.name == "wrong resolver identity" && name == row.target {
					return files["ProjectedOwner.class"], true
				}
				if name == row.target && (row.name == "duplicate signature" || row.name == "static original metadata") {
					o, e := Parse(files[name+".class"])
					if e != nil {
						t.Fatal(e)
					}
					if row.name == "duplicate signature" {
						o.ConstantPool = append(o.ConstantPool, &ConstantUtf8Info{Value: "Ljava/lang/Object;"})
						attr := &SignatureAttribute{SignatureIndex: uint16(len(o.ConstantPool))}
						o.Attributes = append(o.Attributes, attr, attr)
					} else {
						for _, a := range o.Attributes {
							if table, ok := a.(*InnerClassesAttribute); ok {
								for _, r := range table.Classes {
									n, known := sourceBridgeClassName(o, r.InnerClassInfoIndex)
									if known && n == name {
										r.InnerClassAccessFlags |= 8
									}
								}
							}
						}
					}
					return o.Bytes(), true
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			if row.name == "dependency ancestor" {
				d.declarationResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			}
			if row.name == "native source name" {
				d.FuncCtx.DeclarationSourceName = func(string) (string, bool) { return "ProjectedOwner.Range", true }
			}
			if row.name == "native family member" {
				d.nativeMemberLookup = func(string) *nativeMemberClass { return &nativeMemberClass{} }
			}
			if row.name == "budget" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if row.name == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			projection := d.buildSiblingSourceClassFormals()
			if projection == nil {
				t.Fatal("projection provider")
			}
			names, known := projection(row.target)
			if known != (row.want != nil) || !reflect.DeepEqual(names, row.want) {
				t.Fatalf("projection=%v,%v want=%v", names, known, row.want)
			}
			// Cached refusals and accepted declaration identities are stable.
			again, found := projection(row.target)
			if found != known || !reflect.DeepEqual(again, names) {
				t.Fatal("cache changed original identity")
			}
		})
	}
}

func TestNativeFlattenedExpressionBoundsUseOriginalScopesAndStaticCuts(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class BindingRoot<T extends Number>{class Member{T value;}static class Cut<U extends CharSequence>{class Leaf{U value;}}}`, "none")
	for _, variant := range []string{"member", "static cut", "native context", "existing context", "missing parent", "wrong identity", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			target := "BindingRoot$Member"
			want := []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}
			if variant == "static cut" {
				target = "BindingRoot$Cut$Leaf"
				want = []string{"<U::Ljava/lang/CharSequence;>Ljava/lang/Object;"}
			}
			obj := mustParseLowering(t, files[target+".class"])
			d := NewClassObjectDumper(obj)
			d.FuncCtx = &class_context.ClassContext{}
			d.declarationResolver = func(n string) ([]byte, bool) {
				if variant == "missing parent" {
					return nil, false
				}
				if variant == "wrong identity" {
					return files[target+".class"], true
				}
				b, ok := files[n+".class"]
				return b, ok
			}
			switch variant {
			case "native context":
				d.nativeOuterContext = &class_context.ClassContext{}
				want = nil
			case "existing context":
				d.FuncCtx.LexicalTypeParamSignatures = []string{"<X:Ljava/lang/Object;>Ljava/lang/Object;"}
				want = d.FuncCtx.LexicalTypeParamSignatures
			case "missing parent", "wrong identity":
				want = nil
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				want = nil
			case "cancelled":
				base, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(base, workbudget.Limits{})
				want = nil
			}
			d.wireFlattenedLexicalTypeScopes()
			if !reflect.DeepEqual(d.FuncCtx.LexicalTypeParamSignatures, want) {
				t.Fatalf("scopes=%v want=%v", d.FuncCtx.LexicalTypeParamSignatures, want)
			}
		})
	}
}
