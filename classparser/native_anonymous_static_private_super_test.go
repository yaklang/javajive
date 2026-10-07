package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Refusal models exercise the same original declaration certificate used by
// constructor recovery and by the later committed SUPER-call seal. They do not
// count as legal Java programs or as behavioral positives.
func TestNativeStaticMemberPrivateSuperBridgeRequiresExactDeclaredTarget(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"StaticSuperScope.java": anonymousStaticPrivateSuperFixture}, "none", "8")
	for _, variant := range []string{"original", "missing bridge map", "wrong physical descriptor", "wrong target", "missing private target", "duplicate private target", "nonprivate target", "missing target Code", "nil target Code", "duplicate target Code", "missing bridge declaration", "copied bridge declaration", "wrong bridge flags", "missing object", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["StaticSuperScope.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("actual original family")
			}
			member := p.children["StaticSuperScope$Parent"]
			if member == nil || !member.static || len(member.constructors) != 0 || len(member.accessBridges) != 1 {
				t.Fatal("original static member has a bridge, not an enclosing packet")
			}
			var bridge *nativeConstructorAccessBridge
			for _, b := range member.accessBridges {
				bridge = b
			}
			var target *MemberInfo
			for _, method := range member.object.Methods {
				name, _ := sourceBridgeUTF8(member.object, method.NameIndex)
				desc, _ := sourceBridgeUTF8(member.object, method.DescriptorIndex)
				if name == "<init>" && desc == bridge.target {
					target = method
				}
			}
			if target == nil {
				t.Fatal("independent original private target")
			}
			remove := func(drop *MemberInfo) {
				var keep []*MemberInfo
				for _, method := range member.object.Methods {
					if method != drop {
						keep = append(keep, method)
					}
				}
				member.object.Methods = keep
			}
			want := bridge.target
			var work *workbudget.Budget
			switch variant {
			case "missing bridge map":
				member.accessBridges = nil
			case "wrong physical descriptor":
				bridge.descriptor = "()V"
			case "wrong target":
				bridge.target = "(Ljava/lang/String;)V"
			case "missing private target":
				remove(target)
			case "duplicate private target":
				member.object.Methods = append(member.object.Methods, target)
			case "nonprivate target":
				target.AccessFlags = 1
			case "missing target Code", "nil target Code", "duplicate target Code":
				var attributes []AttributeInfo
				for _, attribute := range target.Attributes {
					if _, ok := attribute.(*CodeAttribute); ok {
						if variant == "missing target Code" {
							continue
						}
						if variant == "nil target Code" {
							attribute = (*CodeAttribute)(nil)
						}
						if variant == "duplicate target Code" {
							attributes = append(attributes, attribute)
						}
					}
					attributes = append(attributes, attribute)
				}
				target.Attributes = attributes
			case "missing bridge declaration":
				remove(bridge.method)
			case "copied bridge declaration":
				copy := *bridge.method
				bridge.method = &copy
			case "wrong bridge flags":
				bridge.method.AccessFlags = 0
			case "missing object":
				member.object = nil
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			desc, known := nativeMemberBridgeTargetSourceDescriptor(member, bridge, work)
			if known != (variant == "original") || known && desc != want {
				t.Fatalf("private target projection %q known=%v", desc, known)
			}
		})
	}
}
