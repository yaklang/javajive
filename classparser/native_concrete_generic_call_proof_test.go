package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeConcreteGenericCallRequiresOriginalClosedDescriptor(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, nativeConcreteGenericCallFixture, debug)
		for _, variant := range []string{"original", "method formal", "class formal", "foreign variable", "wrong descriptor", "primitive argument", "void array argument", "malformed", "duplicate signature", "nil signature", "generic throws wrong", "generic throws same", "not private", "virtual packet", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(files["ConcreteOwner.class"])
				if e != nil {
					t.Fatal(e)
				}
				var bridge, target *MemberInfo
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
					if n == "access$000" {
						bridge = m
					}
					if n == "prepare" && d == "(Ljava/util/List;J)Ljava/util/List;" {
						target = m
					}
				}
				if bridge == nil || target == nil {
					t.Fatal("original generic private packet")
				}
				var work *workbudget.Budget
				switch variant {
				case "method formal":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<T:Ljava/lang/Object;>(Ljava/util/List<TT;>;J)Ljava/util/List<TT;>;")
				case "class formal":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<TT;>;J)Ljava/util/List<TT;>;")
				case "foreign variable":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<TUnknown;>;J)Ljava/util/List<Ljava/lang/String;>;")
				case "wrong descriptor":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/ArrayList<Ljava/lang/String;>;J)Ljava/util/List<Ljava/lang/String;>;")
				case "primitive argument":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<I>;J)Ljava/util/List<Ljava/lang/String;>;")
				case "void array argument":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<[V>;J)Ljava/util/List<Ljava/lang/String;>;")
				case "malformed":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<Ljava/lang/String;>;J)Ljava/util/List<Ljava/lang/String;>;x")
				case "duplicate signature":
					for _, a := range target.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							target.Attributes = append(target.Attributes, a)
							break
						}
					}
				case "nil signature":
					target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
				case "generic throws wrong":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<Ljava/lang/String;>;J)Ljava/util/List<Ljava/lang/String;>;^Ljava/lang/Exception;")
				case "generic throws same":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/util/List<Ljava/lang/String;>;J)Ljava/util/List<Ljava/lang/String;>;^Ljava/io/IOException;")
				case "not private":
					target.AccessFlags = (target.AccessFlags &^ 2) | 1
				case "virtual packet":
					for _, a := range bridge.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							c.Code[len(c.Code)-4] = 0xb6
						}
					}
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				got := nativeMemberPrivateCallProof(obj, bridge, work)
				want := variant == "original" || variant == "generic throws same" || variant == "method formal"
				if (got != nil) != want {
					t.Fatalf("packet admitted=%v want=%v", got != nil, want)
				}
				if got != nil && got.call.methodFormalCount != map[bool]int{true: 1, false: 0}[variant == "method formal"] {
					t.Fatal("lost explicit method-owned substitution certificate")
				}
				if got != nil && (got.call == nil || !got.call.rawGeneric || got.call.inherited) {
					t.Fatal("incorrect source binding certificate")
				}
			})
		}
	}
}
