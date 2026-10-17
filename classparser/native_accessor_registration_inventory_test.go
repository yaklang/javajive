package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAccessorRegistrationRequiresCompletedOriginalInventory(t *testing.T) {
	files := nativeCompileClasses(t, accessorForestRegistrationFixture)
	for _, variant := range []string{"original", "missing anonymous getter", "out of range", "conflicting ordinal", "nil getter", "nil family", "changed packet", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["RegistrationOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || len(p.getters) != 2 || nativeMemberAccessorRegistrationsClosed(p, nil) {
				t.Fatal("named packet inventory must exist without claiming completeness")
			}
			if !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil || len(p.getters) != 3 {
				t.Fatal("complete original forest")
			}
			var anonymous *nativeMemberPrivateGetter
			key := ""
			for k, g := range p.getters {
				if p.anonymousForest.units[g.owner] != nil {
					anonymous, key = g, k
				}
			}
			if anonymous == nil {
				t.Fatal("anonymous symbol identity")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing anonymous getter":
				delete(p.getters, key)
			case "out of range":
				anonymous.ordinal = 400
			case "conflicting ordinal":
				anonymous.ordinal = 0
			case "nil getter":
				p.getters[key] = nil
			case "nil family":
				p = nil
			case "changed packet":
				for _, a := range anonymous.method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[0] = 0
					}
				}
				if nativeMemberCollectPrivateGettersPhase(p, d.nativeAnnotationDeclarationResolver(), nil, false) {
					t.Fatal("staging skipped original packet proof")
				}
				return
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := map[string]*nativeMemberPrivateGetter{}
			if p != nil {
				for k, g := range p.getters {
					before[k] = g
				}
			}
			if got := nativeMemberAccessorRegistrationsClosed(p, work); got != (variant == "original") {
				t.Fatalf("closed inventory=%v", got)
			}
			if p != nil {
				for k, g := range before {
					if p.getters[k] != g {
						t.Fatal("closure check replaced consumer identity")
					}
				}
			}
		})
	}
}
