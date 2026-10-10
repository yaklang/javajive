package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberOrdinaryHandleProofRequiresUnchangedPublicPhysicalBinding(t *testing.T) {
	files := nativeCompileClasses(t, nativeOrdinaryHandleFixture)
	for _, variant := range []string{"original", "absent handles", "nil object", "nil index", "invalid index", "missing targets", "fieldref", "constructor", "initializer", "wrong kind", "special", "interface", "private", "protected", "mixed flags", "synthetic", "bridge", "static mismatch", "unknown descriptor", "bad descriptor", "unknown member", "empty name", "second bad target", "too many targets", "duplicate declaration", "generic", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["HandleOwner$Child.class"])
			if e != nil {
				t.Fatal(e)
			}
			owner := obj.GetClassName()
			target := nativeMemberHandleTarget{kind: 5, methodRef: true, name: "echo", descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;"}
			index := &nativeMemberIndex{valid: true, handles: map[string]bool{owner: true}, handleTargets: map[string][]nativeMemberHandleTarget{owner: {target}}}
			var method *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "echo" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original declaration")
			}
			var work *workbudget.Budget
			switch variant {
			case "absent handles":
				index.handles[owner] = false
			case "nil object":
				obj = nil
			case "nil index":
				index = nil
			case "invalid index":
				index.valid = false
			case "missing targets":
				delete(index.handleTargets, owner)
			case "fieldref":
				target.methodRef = false
			case "constructor":
				target.kind = 8
				target.name = "<init>"
				target.descriptor = "(LHandleOwner;)V"
			case "initializer":
				target.name = "<clinit>"
				target.descriptor = "()V"
			case "wrong kind":
				target.kind = 0
			case "special":
				target.kind = 7
			case "interface":
				target.kind = 9
			case "private":
				method.AccessFlags = 2
			case "protected":
				method.AccessFlags = 4
			case "mixed flags":
				method.AccessFlags = 3
			case "synthetic":
				method.AccessFlags |= 0x1000
			case "bridge":
				method.AccessFlags |= 0x40
			case "static mismatch":
				method.AccessFlags |= 8
			case "unknown descriptor":
				target.descriptor = "()Ljava/lang/Object;"
			case "bad descriptor":
				target.descriptor = "broken"
			case "unknown member":
				target.name = "absent"
			case "empty name":
				target.name = ""
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, method)
			case "generic":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if index != nil && variant != "missing targets" {
				index.handleTargets[owner] = []nativeMemberHandleTarget{target}
				if variant == "second bad target" {
					bad := target
					bad.kind = 8
					index.handleTargets[owner] = append(index.handleTargets[owner], bad)
				}
				if variant == "too many targets" {
					index.handleTargets[owner] = make([]nativeMemberHandleTarget, 4097)
					for i := range index.handleTargets[owner] {
						index.handleTargets[owner][i] = target
					}
				}
			}
			got := nativeMemberOrdinaryHandlesClosed(obj, index, work)
			if got != (variant == "original" || variant == "absent handles") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}
