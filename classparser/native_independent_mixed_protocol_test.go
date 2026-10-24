package javaclassparser

import (
	"encoding/json"
	"strings"
	"testing"
)

// An independent cut's scheduling graph can contain an ordinary top-level
// family. That is not an independently certified participant. Refusing this
// mixed protocol must not take over the ordinary family's completion cache.
func TestAdversarialIndependentMixedComponentCannotClaimOrdinaryProtocol(t *testing.T) {
	const source = `abstract class MixedParent{MixedParent(){read();}abstract int read();}
class MixedA{static class Capsule{final int value;MixedB.Inner peer;Capsule(int value){this.value=value;}final class Worker extends MixedParent{int read(){return Capsule.this.value;}}}}
class MixedB{MixedA.Capsule peer;class Inner{MixedA.Capsule get(){return peer;}}}`
	files := nativeCompileClasses(t, source)
	for _, independentFirst := range []bool{false, true} {
		name := "ordinary control"
		if independentFirst {
			name = "refused independent component first"
		}
		t.Run(name, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["MixedB.class"])
			if err != nil {
				t.Fatal(err)
			}
			if independentFirst {
				cut, err := Parse(files["MixedA$Capsule.class"])
				if err != nil {
					t.Fatal(err)
				}
				if z.nativeMemberReader(cut).originalNativeMemberIndependentRoot() == nil {
					t.Fatal("independent root positive certificate")
				}
				participants, cyclic, known := z.nativeMemberDependencyAdmission(cut.GetClassName(), nil, snapshotJDECEnv())
				if !known || !cyclic || strings.Join(participants, ",") != "MixedA$Capsule,MixedB" {
					t.Fatal("original mixed component", participants, cyclic, known)
				}
				if result := z.nativeMemberIndependentEntry(cut); result != nil {
					t.Fatal("ordinary peer granted independent source ownership")
				}
				policy, _ := json.Marshal(snapshotJDECEnv())
				entry := z.nativeMembersCache["MixedB\x00"+string(policy)]
				if entry != nil && entry.transaction != nil {
					t.Fatal("refused mixed component took the ordinary completion protocol")
				}
			}
			result := z.nativeMemberTransactionEntry(root)
			if result == nil || result.family == nil || result.family.independentRoot != nil || !strings.Contains(result.source, "class Inner") {
				t.Fatal("independent refusal poisoned ordinary complete source")
			}
		})
	}
}
