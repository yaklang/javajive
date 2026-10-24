package javaclassparser

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAdversarialIndependentPriorityClosesAllRolesAndNamespacesBeforeSource(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("original native8 compiler")
	}
	for _, variant := range []string{"mixed ordinary role", "normal conflict", "independent conflict", "flat namespace"} {
		t.Run(variant, func(t *testing.T) {
			source, first, second := independentSourceCycleFixture, "CyclicA$Capsule", "CyclicB$Capsule"
			if variant == "mixed ordinary role" {
				source = `import java.util.function.IntUnaryOperator;abstract class PriorityParent{final int seen;PriorityParent(){seen=read();}abstract int read();}class PriorityA{static class Capsule{private final int value;Capsule(int value){this.value=value;}final class Worker extends PriorityParent{int read(){return Capsule.this.value;}}Worker make(){return new Worker();}PriorityB.Token token(){return null;}}static IntUnaryOperator unrelated(final String text){return n->n+text.length();}}class PriorityB{PriorityA.Capsule peer;static class Token{private Token(){}}static IntUnaryOperator unrelated(final String text){return n->n+text.length();}}`
				first, second = "PriorityA$Capsule", "PriorityB"
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, strings.Split(first, "$")[0], "none", true)
			root, err := Parse(files[first+".class"])
			if err != nil {
				t.Fatal(err)
			}
			// This control proves the old priority phase could actually publish
			// the first source before encountering the later rejection.
			control := nativeArchive(t, files)
			result := control.nativeMemberIndependentEntry(root)
			if result == nil || result.family == nil || control.nativeMembersBytes == 0 {
				control.Close()
				t.Fatal("first participant was not independently completable")
			}
			control.Close()
			z := nativeArchive(t, files)
			defer z.Close()
			snap := snapshotJDECEnv()
			parts, cyclic, known := z.nativeMemberDependencyAdmission(first, nil, snap)
			if !known || !cyclic || len(parts) != 2 || parts[0] != first || parts[1] != second {
				t.Fatal("original ordered component not established", parts, cyclic, known)
			}
			if variant != "mixed ordinary role" {
				policy, _ := json.Marshal(snap)
				prefix := ""
				if variant != "normal conflict" {
					prefix = "independent-static\x00"
				}
				entry := z.nativeMemberPolicyEntry(prefix + second + "\x00" + string(policy))
				if variant == "flat namespace" {
					entry.flatSourceCommitted = true
				} else {
					entry.transaction = &nativeMemberTransaction{}
				}
			}
			if got := z.nativeIndependentTransactionEntry(root, parts, snap); got != nil || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || len(z.nativeMemberTransactions) != 0 {
				t.Fatal("denied component published source during its eligibility query")
			}
		})
	}
}
