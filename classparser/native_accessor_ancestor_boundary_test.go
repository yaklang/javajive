package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The graph schedules possible cache queries, whereas the source flag proof
// requires the actual original superclass chain. Neither may borrow certainty
// from the other. An external declaration provider need not be immutable.
func TestNativeAccessorExternalBoundarySchedulesAllPossibleDeclarations(t *testing.T) {
	base := nativeAccessorAncestorCycleInput(t, "Lease", "none")
	for _, variant := range []string{"synthetic", "ordinary", "bridge", "different result", "different name", "final class", "interface"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = append([]byte(nil), raw...)
			}
			delete(files, "q/LeaseMiddle.class")
			parent, e := Parse(files["q/LeaseParent.class"])
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if n != "access$000" {
					continue
				}
				found = true
				switch variant {
				case "ordinary":
					m.AccessFlags &^= 0x1000
				case "bridge":
					m.AccessFlags |= 0x0040
				case "different result":
					m.DescriptorIndex = sourceBridgePoolString(t, parent, "()Ljava/lang/String;")
				case "different name":
					m.NameIndex = sourceBridgePoolString(t, parent, "access$010")
				}
			}
			if !found {
				t.Fatal("original accessor missing")
			}
			switch variant {
			case "final class":
				parent.AccessFlags |= 0x0010
			case "interface":
				parent.AccessFlags |= 0x0200
			}
			files["q/LeaseParent.class"] = parent.Bytes()
			z := nativeArchive(t, files)
			defer z.Close()
			calls := 0
			z.declarationResolver = func(n string) ([]byte, bool) {
				calls++
				// If discovery asked the provider it would see the complete
				// chain and lose the conservative unknown-boundary invariant.
				b, ok := base[n+".class"]
				return b, ok
			}
			g, known := z.nativeMemberOriginalDependencyGraph("q/LeaseA", nil)
			want := variant == "synthetic" || variant == "ordinary"
			if !known || g["q/LeaseA"]["q/LeaseParent"] != want {
				t.Fatalf("potential source ancestor:%v known=%v want=%v", g, known, want)
			}
			if calls != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMembersCache) != 0 {
				t.Fatal("archive scheduling read external declarations or source state")
			}
			idx := z.originalMemberIndex()
			key := "access$000\x00()Ljava/lang/Object;"
			indexed := variant != "bridge" && variant != "different result" && variant != "different name"
			if idx.accessorDeclarations[key]["q/LeaseParent"] != indexed {
				t.Fatal("index lost original name, full descriptor or bridge kind")
			}
		})
	}
}

func TestNativeAccessorDeclarationIndexRejectsInvalidAndUnboundedMetadata(t *testing.T) {
	base := nativeCompileSourceReleaseClasses(t, map[string]string{"Symbols.java": `class Symbols{static Object access$010(){return null;}static Object access$010(int n){return null;}static String access$011(){return null;}}`}, "none", "8")
	for _, variant := range []string{"original", "bad method name index", "bad descriptor index", "field descriptor", "method descriptor without result", "memory", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			o, e := Parse(base["Symbols.class"])
			if e != nil {
				t.Fatal(e)
			}
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				if n != "access$011" {
					continue
				}
				switch variant {
				case "bad method name index":
					m.NameIndex = o.ThisClass
				case "bad descriptor index":
					m.DescriptorIndex = o.ThisClass
				case "field descriptor":
					m.DescriptorIndex = sourceBridgePoolString(t, o, "Ljava/lang/String;")
				case "method descriptor without result":
					m.DescriptorIndex = sourceBridgePoolString(t, o, "()")
				}
			}
			z := nativeArchive(t, map[string][]byte{"Symbols.class": o.Bytes()})
			defer z.Close()
			var work *workbudget.Budget
			switch variant {
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if work != nil {
				z.archive = &archiveFSState{ctx: context.Background(), budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{})}
			}
			idx := z.originalMemberIndex()
			if idx.valid != (variant == "original") {
				t.Fatalf("declaration index valid=%v", idx.valid)
			}
			if idx.valid {
				for _, key := range []string{"access$010\x00()Ljava/lang/Object;", "access$010\x00(I)Ljava/lang/Object;", "access$011\x00()Ljava/lang/String;"} {
					if !idx.accessorDeclarations[key]["Symbols"] {
						t.Fatal("overload/result symbol lost", key)
					}
				}
				if idx.accessorDeclarations["access$011\x00()Ljava/lang/Object;"]["Symbols"] {
					t.Fatal("distinct result erased from source scheduling key")
				}
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMembersCache) != 0 {
				t.Fatal("index published source ownership")
			}
		})
	}
}
