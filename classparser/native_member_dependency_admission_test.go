package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeDependencyAdmissionWarmCacheStillChecksCurrentResources(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	for _, variant := range []string{"original", "canceled", "memory", "budget", "shared metadata capacity"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			snap := snapshotJDECEnv()
			if variant == "shared metadata capacity" {
				z.sourceOwnership.dependencyBytes = (16 << 20) - 1
			}
			participants, cyclic, known := z.nativeMemberDependencyAdmission("CycleScopeA", nil, snap)
			if variant == "shared metadata capacity" {
				if known || cyclic || len(participants) != 0 {
					t.Fatal("unbounded shared metadata retained")
				}
				return
			}
			if !known || !cyclic || len(participants) != 2 {
				t.Fatal("original component admission")
			}
			retained := z.sourceOwnership.dependencyBytes
			var work *workbudget.Budget
			switch variant {
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("fixture allowance")
				}
			}
			_, _, known = z.nativeMemberDependencyAdmission("CycleScopeA", work, snap)
			if known != (variant == "original") {
				t.Fatalf("warm cache resource admission=%v", known)
			}
			if z.sourceOwnership.dependencyBytes != retained || z.sourceOwnership.bytes != 0 || z.nativeMembersBytes != 0 {
				t.Fatal("metadata query changed source or retained allowance twice")
			}
		})
	}
}
