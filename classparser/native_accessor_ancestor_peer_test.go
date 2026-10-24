package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAccessorAncestorPeersRequirePolicyAndScope(t *testing.T) {
	files := nativeAccessorAncestorCycleInput(t, "Lease", "none")
	for _, variant := range []string{"original peer", "ambient policy differs", "disabled policy", "missing transaction context", "missing own participant", "unproved ordinary context", "peer policy differs", "wrong lexical identity", "ordinary peer symbol", "nil projected symbol", "missing root", "missing snapshot", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			obj, e := Parse(files["q/LeaseA.class"])
			if e != nil {
				t.Fatal(e)
			}
			snap := snapshotJDECEnv()
			a := z.prepareNativeMemberFamilyUnpublished(obj, snap)
			obj, e = Parse(files["q/LeaseParent.class"])
			if e != nil {
				t.Fatal(e)
			}
			b := z.prepareNativeMemberFamilyUnpublished(obj, snap)
			if a == nil || b == nil || len(a.family.getters) != 1 || len(b.family.getters) != 1 {
				t.Fatal("original independent accessor plans missing")
			}
			graphClosed := true
			peers := []map[string]*nativeMemberPrepared{{"q/LeaseA": a, "q/LeaseParent": b}}
			switch variant {
			case "ambient policy differs":
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			case "disabled policy":
				snap["JDEC_NATIVE_MEMBER_OFF"] = "1"
			case "missing transaction context":
				peers = nil
			case "missing own participant":
				delete(peers[0], "q/LeaseA")
			case "unproved ordinary context":
				graphClosed = false
			case "peer policy differs":
				b.snapshot = map[string]string{}
				for k, v := range snap {
					b.snapshot[k] = v
				}
				b.snapshot["JDEC_NATIVE_MEMBER_OFF"] = "1"
			case "wrong lexical identity":
				b.family.lexicalObjects["q/LeaseParent"] = a.root
			case "ordinary peer symbol":
				b.family.retainedAccessors = b.family.getters
				b.family.getters = nil
			case "nil projected symbol":
				for k := range a.family.getters {
					a.family.getters[k] = nil
				}
			case "missing root":
				a.root = nil
			case "missing snapshot":
				a.snapshot = nil
			case "memory":
				a.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				a.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := z.nativeMemberAccessorAncestorFlagsClosed(a, graphClosed, peers...)
			if got != (variant == "original peer" || variant == "ambient policy differs") {
				t.Fatalf("conditional peer source flag=%v", got)
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMembersCache) != 0 {
				t.Fatal("unpublished peer view queried or published source")
			}
		})
	}
}
