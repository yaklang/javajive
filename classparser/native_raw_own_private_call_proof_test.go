package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeRawOwnPrivateCallRequiresOriginalDeclaringErasure(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, strings.ReplaceAll(rawOwnPrivateCallFixture, "<T extends Number>{", "<T extends Number> implements java.io.Serializable{"), debug)
		for _, variant := range []string{"original", "same throws", "method formal", "unknown formal", "wrong first bound", "dependent bound", "wrong superclass", "wrong interface", "missing interface", "class named T", "primitive argument", "primitive bound argument", "void array", "wrong descriptor", "duplicate class signature", "nil class signature", "duplicate method signature", "nil method signature", "static target", "public target", "wrong throws", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(append([]byte(nil), files["RawOwnCallOwner.class"]...))
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
					if n == "choose" && d == "(Ljava/lang/Object;J)Ljava/lang/Number;" {
						target = m
					}
				}
				if bridge == nil || target == nil {
					t.Fatal("missing original packet")
				}
				var work *workbudget.Budget
				switch variant {
				case "same throws":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object;J)TT;^Ljava/io/IOException;")
				case "method formal":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "<T:Ljava/lang/Number;>(Ljava/lang/Object;J)TT;")
				case "unknown formal":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object;J)TU;")
				case "wrong first bound":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;Ljava/io/Serializable;")
				case "dependent bound":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:TU;U:Ljava/lang/Number;>Ljava/lang/Object;Ljava/io/Serializable;")
				case "wrong superclass":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:Ljava/lang/Number;>Ljava/util/ArrayList<TT;>;Ljava/io/Serializable;")
				case "wrong interface":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:Ljava/lang/Number;>Ljava/lang/Object;Ljava/lang/Runnable;")
				case "missing interface":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:Ljava/lang/Number;>Ljava/lang/Object;")
				case "class named T":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object;J)LT;")
				case "primitive argument":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object<I>;J)TT;")
				case "primitive bound argument":
					nativeReplaceOriginalSignature(t, obj, obj.Attributes, "<T:Ljava/lang/Number<I>;>Ljava/lang/Object;Ljava/io/Serializable;")
				case "void array":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object;J)[V")
				case "wrong descriptor":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/String;J)TT;")
				case "duplicate class signature":
					for _, a := range obj.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							obj.Attributes = append(obj.Attributes, a)
							break
						}
					}
				case "nil class signature":
					obj.Attributes = append(obj.Attributes, (*SignatureAttribute)(nil))
				case "duplicate method signature":
					for _, a := range target.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							target.Attributes = append(target.Attributes, a)
							break
						}
					}
				case "nil method signature":
					target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
				case "static target":
					target.AccessFlags |= 8
				case "public target":
					target.AccessFlags = (target.AccessFlags &^ 2) | 1
				case "wrong throws":
					nativeReplaceOriginalSignature(t, obj, target.Attributes, "(Ljava/lang/Object;J)TT;^Ljava/lang/Exception;")
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
				want := variant == "original" || variant == "same throws"
				if (got != nil) != want {
					t.Fatalf("admitted=%v want=%v", got != nil, want)
				}
				if got != nil && (got.call == nil || !got.call.rawGeneric || got.call.inherited || got.call.static || got.call.methodFormalCount != 0) {
					t.Fatal("lost raw declaring receiver binding")
				}
			})
		}
	}
}
