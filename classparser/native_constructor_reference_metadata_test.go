package javaclassparser

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorReferenceCapabilityCannotInventDeclarations(t *testing.T) {
	for _, variant := range []string{"ancestry", "complete declaration", "incomplete declaration", "wrong declaration owner", "absent parent table", "unknown ancestry", "detached result", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			original := []string{"ReferenceContract"}
			parentsRead := false
			parents := func(name string) ([]string, bool) {
				parentsRead = true
				return original, name == "ReferenceOperand" && variant != "unknown ancestry"
			}
			declaration := callbinding.Class{Name: "ReferenceOperand", Public: true, Final: true, MembersComplete: true, ParentsComplete: true, Parents: original, Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;", Methods: []callbinding.Method{{Name: "known", Desc: "()V", Public: true, Static: true}}}
			var primary callbinding.Provider
			if variant == "complete declaration" || variant == "incomplete declaration" || variant == "wrong declaration owner" {
				if variant == "incomplete declaration" {
					declaration.ParentsComplete = false
				}
				if variant == "wrong declaration owner" {
					declaration.Name = "UnrelatedDeclaration"
				}
				primary = func(string) (callbinding.Class, bool) { return declaration, true }
			}
			if variant == "absent parent table" {
				parents = nil
			}
			var work *workbudget.Budget
			switch variant {
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			provider := callbinding.WithReferenceAncestry(primary, parents, work)
			got, known := provider("ReferenceOperand")
			want := variant == "ancestry" || variant == "complete declaration" || variant == "incomplete declaration" || variant == "detached result"
			if known != want {
				t.Fatalf("known=%v want=%v", known, want)
			}
			if primary != nil && parentsRead {
				t.Fatal("ancestry fallback overrode an original declaration")
			}
			if !known {
				return
			}
			if primary != nil {
				if !reflect.DeepEqual(got, declaration) {
					t.Fatal("declaration capabilities were changed")
				}
				return
			}
			if !got.ParentsComplete || !reflect.DeepEqual(got.Parents, original) || got.MembersComplete || got.Public || got.Final || got.IsInterface || got.Signature != "" || len(got.Methods) != 0 {
				t.Fatal("parent evidence invented declaration/member facts")
			}
			q := newConstructorWideningQuery(provider)
			if !q.assignable("LReferenceOperand;", "LReferenceContract;") {
				t.Fatal("bounded original parent edge unavailable to assignment")
			}
			if q.invocation(&values.JavaClassMember{Name: "ReferenceOperand", Member: "factory", Description: "()Ljava/lang/Object;"}, core.OP_INVOKESTATIC) || q.constructor(&values.JavaClassMember{Name: "ReferenceOperand", Member: "<init>", Description: "()V"}) {
				t.Fatal("assignment ancestry licensed an unknown invocation")
			}
			if variant == "detached result" {
				got.Parents[0] = "MutatedResult"
				again, known := provider("ReferenceOperand")
				if !known || original[0] != "ReferenceContract" || again.Parents[0] != "ReferenceContract" {
					t.Fatal("a result mutated original ancestry")
				}
			}
		})
	}
}

func TestNativeConstructorReferenceCapabilityRetainsExactPlatformAndDeclarationOwner(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ReferenceBytes.java": "class ReferenceBytes implements java.lang.Runnable {public void run(){}}"}, "none", "8")
	for _, variant := range []string{"original", "different owner", "malformed", "unknown profile"} {
		t.Run(variant, func(t *testing.T) {
			root := NewClassObject()
			root.ThisClass = uint16(root.ConstantPoolManager.AddNewClassInfo("ProbeRoot"))
			root.MajorVersion = 52
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			if variant == "unknown profile" {
				d.options.TargetSourceVersion = 10
			}
			d.declarationResolver = func(name string) ([]byte, bool) {
				if name == "java/nio/file/StandardOpenOption" && (variant == "different owner" || variant == "malformed") {
					if variant == "malformed" {
						return []byte{1}, true
					}
					return files["ReferenceBytes.class"], true
				}
				return nil, false
			}
			declarations := d.buildInvocationMetadata()
			if _, known := declarations("java/nio/file/StandardOpenOption"); known {
				t.Fatal("fixture requires absent complete method metadata")
			}
			q := newConstructorWideningQuery(d.nativeConstructorOperandMetadata(declarations))
			got := q.assignable("Ljava/nio/file/StandardOpenOption;", "Ljava/nio/file/OpenOption;")
			if got != (variant == "original") {
				t.Fatalf("exact platform/declaration ancestry=%v", got)
			}
		})
	}
}
