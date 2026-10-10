package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeJointProtectedTypeAccessRequiresEveryOriginalLexicalPeer(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, jointProtectedTypeScopeSources(), "none", "8")
	for _, scenario := range []string{"original", "absent peers", "absent subclass family", "failed family", "foreign root identity", "missing lexical root", "missing source root", "missing child", "missing source child", "foreign source child", "nil child object", "foreign child identity", "foreign child lexical object", "wrong planned owner", "wrong planned name", "wrong planned flags", "wrong source spelling", "missing original self row", "foreign original self row", "private target", "uncommitted foreign user", "oversized transaction", "budget", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			snap := snapshotJDECEnv()
			peers := map[string]*nativeMemberPrepared{}
			for _, owner := range []string{"access/base/ScopeBase", "access/use/ScopeDerived"} {
				o, e := Parse(append([]byte(nil), files[owner+".class"]...))
				if e != nil {
					t.Fatal(e)
				}
				p := z.prepareNativeMemberFamilyUnpublished(o, snap)
				if p == nil {
					t.Fatal("original unpublished plan absent")
				}
				peers[owner] = p
			}
			target := peers["access/base/ScopeBase"].family
			peer := peers["access/use/ScopeDerived"]
			child := peer.family.children["access/use/ScopeDerived$Child"]
			user := "access/use/ScopeDerived$Child"
			var work *workbudget.Budget
			switch scenario {
			case "absent peers":
				peers = nil
			case "absent subclass family":
				delete(peers, "access/use/ScopeDerived")
			case "failed family":
				peer.family.failed = true
			case "foreign root identity":
				peer.root = peers["access/base/ScopeBase"].root
			case "missing lexical root":
				delete(peer.family.lexicalObjects, peer.family.owner)
			case "missing source root":
				delete(peer.objects, peer.family.owner)
			case "missing source child":
				delete(peer.objects, user)
			case "foreign source child":
				peer.objects[user] = peer.root
			case "missing child":
				delete(peer.family.children, user)
			case "nil child object":
				child.object = nil
			case "foreign child identity":
				child.object = peer.root
			case "foreign child lexical object":
				peer.family.lexicalObjects[user] = peer.root
			case "wrong planned owner":
				child.owner = target.owner
			case "wrong planned name":
				child.name = "Other"
			case "wrong planned flags":
				child.flags ^= 4
			case "wrong source spelling":
				child.sourceName = "Other.Child"
			case "missing original self row", "foreign original self row":
				for _, a := range child.object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						kept := table.Classes[:0]
						for _, row := range table.Classes {
							if row.InnerClassInfoIndex == child.object.ThisClass {
								if scenario == "missing original self row" {
									continue
								}
								row.OuterClassInfoIndex = child.object.ThisClass
							}
							kept = append(kept, row)
						}
						table.Classes = kept
					}
				}
			case "private target":
				target.children["access/base/ScopeBase$Item"].flags = 2
			case "uncommitted foreign user":
				user = "access/use/ScopeDerived$Other"
			case "oversized transaction":
				for i := 0; i < 65; i++ {
					peers[string(rune(0x100+i))] = peer
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("setup")
				}
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if scenario == "original" {
				if z.prepareNativeMemberFamily(peers["access/base/ScopeBase"].root, snap) != nil {
					t.Fatal("strict standalone prepare accepted uncommitted foreign lexical access")
				}
				if z.finishNativeMemberFamily(peers["access/base/ScopeBase"], z.nativeMemberLookup, false, peers) != nil {
					t.Fatal("peer map without a closed atomic dependency graph accepted")
				}
			}
			index := &nativeMemberIndex{typeUsers: map[string]map[string]bool{"access/base/ScopeBase$Item": {user: true}}}
			if z.nativeMemberAccessRepresentable(target, index, nil) {
				t.Fatal("independent flattened user granted lexical access")
			}
			if got := z.nativeMemberAccessRepresentable(target, index, work, peers); got != (scenario == "original") {
				t.Fatalf("joint lexical protected access=%v", got)
			}
		})
	}
}
