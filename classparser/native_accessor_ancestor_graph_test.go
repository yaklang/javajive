package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAccessorAncestorGraphRequiresOriginalHierarchy(t *testing.T) {
	base := nativeAccessorAncestorCycleInput(t, "Lease", "none")
	for _, variant := range []string{"original", "external binary ancestor", "unprojected symbol", "different inherited descriptor", "missing intermediate", "wrong intermediate identity", "cyclic superclass", "bad symbolic name", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = append([]byte(nil), raw...)
			}
			var work *workbudget.Budget
			switch variant {
			case "external binary ancestor":
				for n := range files {
					if n != "q/LeaseA.class" && n != "q/LeaseA$Builder.class" && n != "q/LeaseA$Marker.class" {
						delete(files, n)
					}
				}
			case "unprojected symbol", "bad symbolic name":
				o, e := Parse(files["q/LeaseA.class"])
				if e != nil {
					t.Fatal(e)
				}
				changed := false
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "access$000" {
						if variant == "unprojected symbol" {
							m.AccessFlags &^= 0x1000
						} else {
							m.NameIndex = o.ThisClass
						}
						changed = true
					}
				}
				if !changed {
					t.Fatal("original projected method missing")
				}
				files["q/LeaseA.class"] = o.Bytes()
			case "different inherited descriptor":
				o, e := Parse(files["q/LeaseParent.class"])
				if e != nil {
					t.Fatal(e)
				}
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "access$000" {
						m.DescriptorIndex = sourceBridgePoolString(t, o, "()Ljava/lang/String;")
					}
				}
				files["q/LeaseParent.class"] = o.Bytes()
			case "missing intermediate":
				delete(files, "q/LeaseMiddle.class")
			case "wrong intermediate identity":
				files["q/LeaseMiddle.class"] = files["q/LeaseParent.class"]
			case "cyclic superclass":
				o, e := Parse(files["q/LeaseMiddle.class"])
				if e != nil {
					t.Fatal(e)
				}
				o.SuperClass = o.ThisClass
				files["q/LeaseMiddle.class"] = o.Bytes()
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			jar := nativeArchive(t, files)
			defer jar.Close()
			if variant == "external binary ancestor" {
				jar.declarationResolver = func(n string) ([]byte, bool) { b, ok := base[n+".class"]; return b, ok }
			}
			g, known := jar.nativeMemberOriginalDependencyGraph("q/LeaseA", work)
			switch variant {
			case "original":
				if !known || len(g) != 2 || !g["q/LeaseA"]["q/LeaseParent"] || !g["q/LeaseParent"]["q/LeaseA"] {
					t.Fatalf("original ancestor source cycle missing:%v %v", g, known)
				}
			case "external binary ancestor", "unprojected symbol", "different inherited descriptor":
				if !known || len(g) != 1 || len(g["q/LeaseA"]) != 0 {
					t.Fatalf("non-source/mismatched symbol manufactured dependency:%v %v", g, known)
				}
			default:
				if known {
					t.Fatalf("incomplete hierarchy or work budget accepted:%v", g)
				}
			}
			if jar.nativeMembersBytes != 0 || jar.sourceOwnership.bytes != 0 || len(jar.nativeMembersCache) != 0 {
				t.Fatal("original hierarchy discovery published or queried source state")
			}
		})
	}
}
