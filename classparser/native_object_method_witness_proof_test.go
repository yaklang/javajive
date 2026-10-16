package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeObjectMethodWitnessRequiresOriginalOwnedPrivatePacket(t *testing.T) {
	const fixture = `class WitnessRoot<B>{private static <B,T extends B>T project(Class<T> type,B input){return type.cast(input);}static class Caller{Object apply(Class type,Object input){return project(type,input);}}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, fixture, debug)
		for _, variant := range []string{"original", "transitive", "forward", "class formal only", "external bound", "cycle", "intersection", "number", "wrong erased result", "malformed", "duplicate signature", "nil signature", "bridge signature", "wrong physical slot", "missing target", "public target", "throws mismatch", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(bytes.Clone(files["WitnessRoot.class"]))
				if e != nil {
					t.Fatal(e)
				}
				var target, bridge *MemberInfo
				var code *CodeAttribute
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "project" {
						target = m
					}
					if n == "access$000" {
						bridge = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				if target == nil || bridge == nil || code == nil {
					t.Fatal("missing physical private forwarding packet")
				}
				var work *workbudget.Budget
				switch variant {
				case "transitive":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Object;U:TB;T:TU;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "forward":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<T:TB;B:Ljava/lang/Object;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "class formal only":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Class<TB;>;TB;)TB;")
				case "external bound":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "cycle":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:TT;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "intersection":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Object;:Ljava/io/Serializable;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "number":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Number;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;")
				case "wrong erased result":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Object;T:TB;>(Ljava/lang/Class<TT;>;TB;)Ljava/lang/Class<TT;>;")
				case "malformed":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Object;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;x")
				case "duplicate signature":
					for _, a := range target.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							target.Attributes = append(target.Attributes, a)
							break
						}
					}
				case "nil signature":
					target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
				case "bridge signature":
					for _, a := range target.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							bridge.Attributes = append(bridge.Attributes, a)
							break
						}
					}
				case "wrong physical slot":
					code.Code[1] = 0x2a
				case "missing target":
					for i, m := range obj.Methods {
						if m == target {
							obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
							break
						}
					}
				case "public target":
					target.AccessFlags = target.AccessFlags&^uint16(2) | 1
				case "throws mismatch":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<B:Ljava/lang/Object;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;^Ljava/io/IOException;")
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				p := nativeMemberPrivateCallProof(obj, bridge, work)
				want := variant == "original" || variant == "transitive" || variant == "forward"
				if (p != nil) != want {
					t.Fatalf("original packet=%v want=%v", p != nil, want)
				}
				count := 2
				if variant == "transitive" {
					count = 3
				}
				if p != nil && (p.call == nil || p.call.methodFormalCount != count || !p.call.static || !p.call.rawGeneric || p.call.inherited) {
					t.Fatal("lost method-owned Object witness certificate")
				}
			})
		}
	}
}
