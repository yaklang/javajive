package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func nativeReplaceOriginalSignature(t *testing.T, obj *ClassObject, attrs []AttributeInfo, signature string) {
	t.Helper()
	for _, a := range attrs {
		if s, ok := a.(*SignatureAttribute); ok {
			s.SignatureIndex = sourceBridgePoolString(t, obj, signature)
			return
		}
	}
	t.Fatal("missing original signature")
}

func TestNativeProtectedGenericCallRequiresClosedRawBinding(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, nativeProtectedGenericSources("Number", "BoundedProtectedOwner", "E"), debug, "8")
		for _, variant := range []string{"original", "method formal", "foreign class formal", "wrong erasure", "dependent bound", "missing declaration signature", "duplicate declaration signature", "missing root signature", "fixed nongeneric receiver", "unknown root bound", "wrong nominal superclass", "malformed method signature", "mismatched generic throws", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				root, e := Parse(files["probe/access/BoundedProtectedOwner.class"])
				if e != nil {
					t.Fatal(e)
				}
				parent, e := Parse(files["probe/base/DispatchParent.class"])
				if e != nil {
					t.Fatal(e)
				}
				var bridge, target *MemberInfo
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "access$000" {
						bridge = m
					}
				}
				for _, m := range parent.Methods {
					n, _ := sourceBridgeUTF8(parent, m.NameIndex)
					d, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
					if n == "prepare" && d == "(Ljava/lang/Number;J)Ljava/lang/Number;" {
						target = m
					}
				}
				if bridge == nil || target == nil {
					t.Fatal("missing original generic virtual packet")
				}
				var work *workbudget.Budget
				switch variant {
				case "method formal":
					nativeReplaceOriginalSignature(t, parent, target.Attributes, "<T:Ljava/lang/Number;>(TT;J)TT;")
				case "foreign class formal":
					nativeReplaceOriginalSignature(t, parent, target.Attributes, "(TE;J)TE;")
				case "wrong erasure":
					nativeReplaceOriginalSignature(t, parent, parent.Attributes, "<T:Ljava/lang/Object;>Ljava/lang/Object;")
				case "dependent bound":
					nativeReplaceOriginalSignature(t, parent, parent.Attributes, "<T:TU;U:Ljava/lang/Number;>Ljava/lang/Object;")
				case "missing declaration signature":
					var kept []AttributeInfo
					for _, a := range parent.Attributes {
						if _, ok := a.(*SignatureAttribute); !ok {
							kept = append(kept, a)
						}
					}
					parent.Attributes = kept
				case "duplicate declaration signature":
					for _, a := range parent.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							parent.Attributes = append(parent.Attributes, a)
							break
						}
					}
				case "missing root signature":
					var kept []AttributeInfo
					for _, a := range root.Attributes {
						if _, ok := a.(*SignatureAttribute); !ok {
							kept = append(kept, a)
						}
					}
					root.Attributes = kept
				case "fixed nongeneric receiver":
					nativeReplaceOriginalSignature(t, root, root.Attributes, "Lprobe/base/DispatchParent<Ljava/lang/Integer;>;")
				case "unknown root bound":
					nativeReplaceOriginalSignature(t, root, root.Attributes, "<E:TU;>Lprobe/base/DispatchParent<TE;>;")
				case "wrong nominal superclass":
					nativeReplaceOriginalSignature(t, root, root.Attributes, "<E:Ljava/lang/Number;>Lprobe/base/OtherParent<TE;>;")
				case "malformed method signature":
					nativeReplaceOriginalSignature(t, parent, target.Attributes, "(TT;J)TT;x")
				case "mismatched generic throws":
					nativeReplaceOriginalSignature(t, parent, target.Attributes, "(TT;J)TT;^TT;")
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				resolve := func(name string) (*ClassObject, bool) { return parent, name == parent.GetClassName() }
				got := nativeMemberProtectedCallProof(root, bridge, resolve, work)
				if (got != nil) != (variant == "original") {
					t.Fatalf("raw generic packet admitted=%v", got != nil)
				}
				if got != nil && (got.call == nil || !got.call.rawGeneric || !got.call.inherited) {
					t.Fatal("missing raw binding certificate")
				}
			})
		}
	}
}
