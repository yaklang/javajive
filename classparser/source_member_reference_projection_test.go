package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMemberReferenceProjectionRequiresCompleteOwnerGraph(t *testing.T) {
	files := nativeCompileClasses(t, memberReferenceSignatureFixture)
	for _, variant := range []string{"direct", "array", "nested", "upper wildcard", "lower wildcard", "method shadow", "same text", "wrong argument", "raw member", "wrong variance", "wrong array rank", "changed formal bound", "changed throws", "unknown binder", "missing owner", "foreign owner", "missing member", "static member", "own member formal", "unknown source declaration", "wrong source formal", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root := mustParseLowering(t, files["SuffixOwner.class"])
			d := NewClassObjectDumper(root)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			originalResolve := d.nativeAnnotationDeclarationResolver()
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "missing owner" && name == "SuffixOwner" || variant == "missing member" && name == "SuffixOwner$Node" {
					return nil, false
				}
				o, ok := originalResolve(name)
				if !ok {
					return nil, false
				}
				o = mustParseLowering(t, o.Bytes())
				if variant == "foreign owner" && name == "SuffixOwner" {
					o.ThisClass = uint16(o.ConstantPoolManager.AddNewClassInfo("OtherOwner"))
				}
				if variant == "static member" && name == "SuffixOwner$Node" {
					for _, attr := range o.Attributes {
						if table, ok := attr.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								n, _ := sourceBridgeClassName(o, row.InnerClassInfoIndex)
								if n == name {
									row.InnerClassAccessFlags |= StaticFlag
								}
							}
						}
					}
				}
				if variant == "own member formal" && name == "SuffixOwner$Node" {
					o.Attributes = append(o.Attributes, &SignatureAttribute{SignatureIndex: uint16(o.ConstantPoolManager.AddUtf8Info("<U:Ljava/lang/Object;>Ljava/lang/Object;"))})
				}
				return o, true
			}
			projected := d.buildSiblingSourceClassFormals()
			if variant == "unknown source declaration" {
				projected = func(string) ([]string, bool) { return nil, false }
			} else if variant == "wrong source formal" {
				projected = func(string) ([]string, bool) { return []string{"U"}, true }
			}
			oldType, newType := "LSuffixOwner<TT;>.Node;", "LSuffixOwner$Node<TT;>;"
			prefix, throws, newThrows := "", "", ""
			want := false
			switch variant {
			case "direct":
				want = true
			case "array":
				oldType, newType, want = "["+oldType, "["+newType, true
			case "nested":
				oldType, newType, want = "Ljava/util/List<"+oldType+">;", "Ljava/util/List<"+newType+">;", true
			case "upper wildcard", "lower wildcard":
				variance := "+"
				if variant == "lower wildcard" {
					variance = "-"
				}
				oldType, newType, want = "Ljava/util/List<"+variance+oldType+">;", "Ljava/util/List<"+variance+newType+">;", true
			case "method shadow":
				prefix, want = "<T:Ljava/lang/Integer;>", true
			case "same text":
				newType = oldType
			case "wrong argument":
				newType = strings.Replace(newType, "TT;", "Ljava/lang/Number;", 1)
			case "raw member":
				newType = "LSuffixOwner$Node;"
			case "wrong variance":
				oldType, newType = "Ljava/util/List<+"+oldType+">;", "Ljava/util/List<-"+newType+">;"
			case "wrong array rank":
				oldType, newType = "["+oldType, "[["+newType
			case "changed formal bound":
				prefix = "<U:Ljava/lang/Number;>"
			case "changed throws":
				throws, newThrows = "^Ljava/io/IOException;", "^Ljava/lang/Exception;"
			case "unknown binder":
				newType = strings.Replace(newType, "TT;", "TMissing;", 1)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			old, fresh := prefix+"("+oldType+")"+oldType+throws, prefix+"("+newType+")"+newType+newThrows
			if variant == "changed formal bound" {
				fresh = strings.Replace(fresh, "U:Ljava/lang/Number;", "U:Ljava/lang/Integer;", 1)
			}
			scopes := []types.LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}
			descriptor, _, valid := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, old)
			if !valid {
				t.Fatal("invalid original fixture signature")
			}
			if got := memberReferenceSignatureProjection(old, fresh, descriptor, scopes, projected, resolve, d.Work); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
		})
	}
}
