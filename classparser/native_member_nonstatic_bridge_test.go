package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberNonstaticBridgeRequiresTargetCaptureWitness(t *testing.T) {
	source := `class MemberBridgeOwner<T>{static Object marker(final Object value){return new Object(){Object get(){return value;}};}class Child<U>{final U value;private Child(U value,long n){this.value=value;}U get(){return value;}}Child<T>make(T value,long n){return new Child<T>(value,n);}}`
	files := nativeCompileClasses(t, source)
	for _, variant := range []string{"original", "missing target witness", "missing bridges", "foreign target descriptor", "private target widened", "bridge body changed", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			obj, _ := Parse(append([]byte(nil), files["MemberBridgeOwner.class"]...))
			d := z.nativeMemberReader(obj)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			p.anonymous = d.planNativeAnonymousFamilyWithinMembers(p)
			child := p.children["MemberBridgeOwner$Child"]
			var bridge *nativeConstructorAccessBridge
			for _, b := range child.accessBridges {
				bridge = b
			}
			if bridge == nil || nativeMemberConstructorForAllocation(child, bridge.descriptor) == nil {
				t.Fatal("original bridge/target capture")
			}
			var work *workbudget.Budget
			known := true
			switch variant {
			case "missing target witness":
				delete(child.constructors, bridge.target)
			case "missing bridges":
				child.accessBridges = nil
			case "foreign target descriptor":
				bridge.target = "(LForeign;)V"
			case "private target widened":
				for _, m := range child.object.Methods {
					desc, _ := child.object.getUtf8(m.DescriptorIndex)
					if desc == bridge.target {
						m.AccessFlags = 1
					}
				}
			case "bridge body changed":
				bridge.method.AccessFlags = 0
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.Work = work
			_, known = d.nativeMemberAllocations(p)
			if variant == "missing bridges" { // Standalone ownership remains unproved for the synthetic declaration.
				reader := z.nativeMemberReader(child.object)
				known = nativeMemberProofWithOwner(child.object, obj, nil, reader.buildInvocationMetadata()) != nil
			}
			if variant == "private target widened" || variant == "bridge body changed" {
				reader := z.nativeMemberReader(child.object)
				desc, _ := child.object.getUtf8(bridge.method.DescriptorIndex)
				known = reader.nativeConstructorAccessBridges()[desc] != nil
			}
			if known != (variant == "original") {
				t.Fatalf("capture/bridge proof=%v", known)
			}
		})
	}
}
