package javaclassparser

import (
	"encoding/json"
	"github.com/yaklang/javajive/internal/jdecenv"
	"testing"
)

// A completed failed local admission is a source-namespace decision. Another
// caller cannot bypass it by preparing that participant again inside an SCC.
func TestNativeSourceTransactionRespectsCompletedParticipantAdmission(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	for _, refused := range []bool{false, true} {
		for _, first := range []string{"CycleScopeA", "CycleScopeB"} {
			name := "complete/" + first
			if refused {
				name = "prior refusal/" + first
			}
			t.Run(name, func(t *testing.T) {
				archive := nativeArchive(t, files)
				defer archive.Close()
				snapshot := snapshotJDECEnv()
				policy, err := json.Marshal(snapshot)
				if err != nil {
					t.Fatal(err)
				}
				if refused {
					entry := archive.nativeMemberPolicyEntry("CycleScopeB\x00" + string(policy))
					if entry == nil {
						t.Fatal("original participant cache")
					}
					entry.planOnce.Do(func() { entry.planKnown = false })
				}
				other := "CycleScopeA"
				if first == other {
					other = "CycleScopeB"
				}
				for _, name := range []string{first, other} {
					root, err := Parse(files[name+".class"])
					if err != nil {
						t.Fatal(err)
					}
					var owned bool
					jdecenv.Run(snapshot, func() error { _, owned = archive.nativeMemberSource(root); return nil })
					if owned == refused {
						t.Fatalf("source ownership %s=%v, completed participant refusal=%v", name, owned, refused)
					}
				}
				if refused && (archive.nativeMembersBytes != 0 || archive.sourceOwnership.bytes != 0) {
					t.Fatal("rejected component reserved source ownership")
				}
			})
		}
	}
}
