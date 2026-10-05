package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeDependencyAllocationNeverBorrowsPrivateOwnership(t *testing.T) {
	files := nativeCompileClasses(t, nativeInheritedDependencyAllocationFixture())
	for _, variant := range []string{"original", "private constructor", "own private constructor", "wrong target identity", "budget", "canceled", "foreign bridge"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			caller, _ := Parse(append([]byte(nil), files["DependencyDerived.class"]...))
			entry := z.nativeMemberEntry(caller)
			if entry == nil || entry.family == nil {
				t.Fatal("independent family did not close")
			}
			p := entry.family
			view := p.allocationDependencies["DependencyBase$Value"]
			if view == nil || p.children["DependencyBase$Value"] != nil || p.lexicalObjects["DependencyBase$Value"] != nil || len(p.constructorBridges("DependencyBase$Value")) != 0 {
				t.Fatal("constructor binding imported private ownership")
			}
			target, _ := Parse(append([]byte(nil), files["DependencyBase$Value.class"]...))
			copy := *view
			copy.object = target
			p.allocationDependencies["DependencyBase$Value"] = &copy
			var work *workbudget.Budget
			switch variant {
			case "private constructor", "own private constructor":
				for _, method := range target.Methods {
					name, _ := sourceBridgeUTF8(target, method.NameIndex)
					if name == "<init>" {
						method.AccessFlags |= 2
					}
				}
				if variant == "own private constructor" {
					caller = target
				}
			case "wrong target identity":
				copy.object = caller
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "foreign bridge":
				copy.accessBridges = map[string]*nativeConstructorAccessBridge{"unproved": {}}
				if _, known := z.nativeMemberReader(caller).nativeMemberAllocations(p); known {
					t.Fatal("foreign bridge imported registration ownership")
				}
				return
			}
			if got := nativeMemberOriginalConstructorAccess(p, caller, "DependencyBase$Value", "(LDependencyBase;Ljava/lang/Object;)V", work); got != (variant == "original" || variant == "own private constructor") {
				t.Fatalf("original constructor access=%v", got)
			}
		})
	}
}
