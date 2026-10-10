package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousSuperWideningNeedsOriginalLoadAndClosedHierarchy(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousGenericSuperFixture, "class WideningParent<T>", "class WideningToken{} class NarrowToken extends WideningToken{} class WideningParent<T extends WideningToken>", 1)
	fixture = strings.ReplaceAll(fixture, "String value", "NarrowToken value")
	fixture = strings.ReplaceAll(fixture, "WideningParent<String>", "WideningParent<NarrowToken>")
	fixture = strings.Replace(fixture, `String v:new String[]{null,"",new String("token")}`, `NarrowToken v:new NarrowToken[]{null,new NarrowToken(),new NarrowToken()}`, 1)
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "missing", "wrong identity", "incomplete", "cycle", "too many edges", "narrowing", "primitive", "unknown parameter", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["WideningOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			root, e := Parse(files["WideningOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			provider := archive.nativeMemberReader(root).buildInvocationMetadata()
			var work *workbudget.Budget
			switch variant {
			case "missing":
				provider = nil
			case "wrong identity", "incomplete", "cycle", "too many edges":
				provider = func(name string) (callbinding.Class, bool) {
					c := callbinding.Class{Name: name, ParentsComplete: true, Parents: []string{"WideningToken"}}
					switch variant {
					case "wrong identity":
						c.Name = "DifferentToken"
					case "incomplete":
						c.ParentsComplete = false
					case "cycle":
						c.Parents = []string{"NarrowToken"}
					case "too many edges":
						c.Parents = make([]string, 65)
						for i := range c.Parents {
							c.Parents[i] = "WideningToken"
						}
					}
					return c, true
				}
			case "narrowing", "primitive", "unknown parameter":
				actual := "Ljava/lang/Object;"
				if variant == "primitive" {
					actual = "I"
				}
				if variant == "unknown parameter" {
					actual = "LUnresolvedToken;"
				}
				for _, m := range obj.Methods {
					if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "<init>" {
						m.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(LWideningOwner;" + actual + ")V"))
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			owner, method, known := originalAnonymousOwner(obj)
			if !known {
				t.Fatal("original anonymous ownership")
			}
			got := nativeAnonymousConstructorWithDeclarations(obj, owner, method, "", work, nil, nil, provider)
			if (got != nil) != (variant == "original") {
				t.Fatalf("admitted=%v", got != nil)
			}
			if got != nil && (got.descriptor != "(LWideningOwner;LNarrowToken;)V" || got.superDescriptor != "(LWideningToken;)V" || len(got.superParams) != 1 || got.superParams[0] != 1) {
				t.Fatal("widening replaced an original physical descriptor or operand")
			}
		})
	}
}
