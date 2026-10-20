package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Planning all lexical families is insufficient: a later peer can still fail
// body rendering. No healthy prefix may escape that incomplete transaction.
func TestAdversarialIndependentSourceTransactionRejectsLatePeerFailure(t *testing.T) {
	_, java := t04Tools(t)
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("original native8 compiler")
	}
	source := strings.Replace(independentSourceCycleFixture,
		"int sum(){return Capsule.this.value+peer.get();}}Worker make(CyclicA.Capsule peer)",
		"int sum(){return Capsule.this.value+peer.get();}IntUnaryOperator unproved(final String text){return value->value+text.length();}}Worker make(CyclicA.Capsule peer)", 1)
	if source == independentSourceCycleFixture {
		t.Fatal("late peer fixture did not change")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, "CyclicA", debug, true)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "CycleDriver"); got != "25:independent:mutual:scopes\n" {
				t.Fatal("original unchanged caller", got)
			}
			for _, first := range []string{"CyclicA$Capsule", "CyclicB$Capsule"} {
				t.Run(first, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					snap := snapshotJDECEnv()
					plans := map[string]*nativeMemberPrepared{}
					roots := map[string]*ClassObject{}
					for _, name := range []string{"CyclicA$Capsule", "CyclicB$Capsule"} {
						root, err := Parse(files[name+".class"])
						if err != nil {
							t.Fatal(err)
						}
						roots[name] = root
						certificate := z.nativeMemberReader(root).originalNativeMemberIndependentRoot()
						plans[name] = z.prepareNativeMemberFamilyFromRoot(root, snap, certificate)
						if certificate == nil || plans[name] == nil {
							t.Fatal("fixture failed before final body rendering", name)
						}
					}
					lookup := func(name string) *nativeMemberClass {
						for root, plan := range plans {
							if name == root {
								return plan.family.independentRoot.declaration
							}
							if child := plan.family.children[name]; child != nil {
								return child
							}
						}
						return nil
					}
					if healthy := z.finishNativeMemberFamily(plans["CyclicA$Capsule"], lookup, true, plans); healthy == nil {
						t.Fatal("healthy component prefix did not finish")
					}
					if failed := z.finishNativeMemberFamily(plans["CyclicB$Capsule"], lookup, true, plans); failed != nil {
						t.Fatal("late unsupported peer claimed completion")
					}
					names := []string{"CyclicA$Capsule", "CyclicB$Capsule"}
					if got := z.nativeIndependentTransactionEntry(roots[first], names, snap); got != nil {
						t.Fatal("healthy prefix published after late peer failure")
					}
					if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
						t.Fatal("incomplete component reserved source")
					}
					for _, tx := range z.nativeMemberTransactions {
						if len(tx.entries) != 0 {
							t.Fatal("partial source map published")
						}
					}
				})
			}
		})
	}
}
