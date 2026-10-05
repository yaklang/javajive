package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberConstructorHandleNeedsOriginalStaticPublicPhysicalBinding(t *testing.T) {
	files := nativeCompileClasses(t, `class StaticHandleScope{static class PublicItem{public PublicItem(Object x){}}}`)
	for _, variant := range []string{"original", "no source member", "wrong object", "nonstatic plan", "different flags", "generic owner", "capture", "enum", "bridge", "private", "protected", "static method", "synthetic", "abstract", "generic constructor", "fieldref", "wrong kind", "nonvoid", "wrong descriptor", "second invalid target", "two source plans", "missing original self row", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(files["StaticHandleScope.class"])
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			family := z.nativeMemberReader(root).planNativeMemberFamily()
			if family == nil {
				t.Fatal("original static member plan")
			}
			member := family.children["StaticHandleScope$PublicItem"]
			if member == nil {
				t.Fatal("original member")
			}
			obj := member.object
			owner := obj.GetClassName()
			var method *MemberInfo
			for _, m := range obj.Methods {
				if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "<init>" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original constructor")
			}
			target := nativeMemberHandleTarget{kind: 8, methodRef: true, name: "<init>", descriptor: "(Ljava/lang/Object;)V"}
			var work *workbudget.Budget
			sources := []*nativeMemberClass{member}
			switch variant {
			case "no source member":
				sources = nil
			case "wrong object":
				member.object = root
			case "nonstatic plan":
				member.static = false
			case "different flags":
				member.flags ^= 1
			case "generic owner":
				member.formalCount = 1
			case "capture":
				member.field = "this$0"
			case "enum":
				member.enumSynthesis = &nativeMemberEnumSynthesis{}
			case "bridge":
				member.accessBridges = map[string]*nativeConstructorAccessBridge{target.descriptor: {}}
			case "private":
				method.AccessFlags = 2
			case "protected":
				method.AccessFlags = 4
			case "static method":
				method.AccessFlags |= 8
			case "synthetic":
				method.AccessFlags |= 0x1000
			case "abstract":
				method.AccessFlags |= 0x400
			case "generic constructor":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "fieldref":
				target.methodRef = false
			case "wrong kind":
				target.kind = 7
			case "nonvoid":
				target.descriptor = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "wrong descriptor":
				target.descriptor = "()V"
			case "two source plans":
				sources = append(sources, member)
			case "missing original self row":
				for _, a := range obj.Attributes {
					if inner, ok := a.(*InnerClassesAttribute); ok {
						inner.Classes = nil
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
			targets := []nativeMemberHandleTarget{target}
			if variant == "second invalid target" {
				bad := target
				bad.methodRef = false
				targets = append(targets, bad)
			}
			index := &nativeMemberIndex{valid: true, handles: map[string]bool{owner: true}, handleTargets: map[string][]nativeMemberHandleTarget{owner: targets}}
			got := nativeMemberOrdinaryHandlesClosed(obj, index, work, sources...)
			if got != (variant == "original") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

func TestNativeMemberConstructorHandleRepeatedBindingsStayBoundedAndDistinct(t *testing.T) {
	files := nativeCompileClasses(t, `class StaticHandleScope{static class PublicItem{public PublicItem(Object x){} private PublicItem(int x){}}}`)
	for _, variant := range []string{"repeated", "traversal budget", "too many", "private overload", "wrong kind", "wrong tag", "missing descriptor"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["StaticHandleScope.class"])
			if err != nil {
				t.Fatal(err)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			family := archive.nativeMemberReader(root).planNativeMemberFamily()
			if family == nil {
				t.Fatal("original static member plan")
			}
			member := family.children["StaticHandleScope$PublicItem"]
			if member == nil {
				t.Fatal("original static member")
			}
			good := nativeMemberHandleTarget{kind: 8, methodRef: true, name: "<init>", descriptor: "(Ljava/lang/Object;)V"}
			targets := make([]nativeMemberHandleTarget, 4096)
			for i := range targets {
				targets[i] = good
			}
			bad := good
			limit := int64(9000)
			switch variant {
			case "traversal budget":
				limit = 8190 // even repeated witnesses must pay for every occurrence
			case "too many":
				targets = append(targets, good)
			case "private overload":
				bad.descriptor = "(I)V"
			case "wrong kind":
				bad.kind = 7
			case "wrong tag":
				bad.methodRef = false
			case "missing descriptor":
				bad.descriptor = "()V"
			}
			if bad != good {
				targets[len(targets)-1] = bad
			}
			owner := member.object.GetClassName()
			index := &nativeMemberIndex{valid: true, handles: map[string]bool{owner: true}, handleTargets: map[string][]nativeMemberHandleTarget{owner: targets}}
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: limit})
			got := nativeMemberOrdinaryHandlesClosed(member.object, index, work, member)
			if got != (variant == "repeated") {
				t.Fatalf("admitted=%v, work=%v", got, work.Snapshot())
			}
			if variant == "repeated" && work.Used(workbudget.CounterGraphScans) < 8192 {
				t.Fatal("repeated physical witnesses escaped traversal accounting")
			}
		})
	}
}
