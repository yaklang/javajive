package javaclassparser

import "testing"

const downstreamComponentFixture = `
class DagEntryOwner {static class Local{}DownstreamA.Item input;}
class DownstreamA {static class Item{DownstreamB.Item peer;}}
class DownstreamB {static class Item{DownstreamA.Item peer;}}
class DagEntryDriver {public static void main(String[]args)throws Exception{DownstreamA.Item a=new DownstreamA.Item();DownstreamB.Item b=new DownstreamB.Item();a.peer=b;b.peer=a;DagEntryOwner owner=new DagEntryOwner();owner.input=a;if(owner.input.peer.peer!=a||new DagEntryOwner.Local().getClass().getDeclaringClass()!=DagEntryOwner.class||a.getClass().getDeclaringClass()!=DownstreamA.class||b.getClass().getDeclaringClass()!=DownstreamB.class)throw new AssertionError("component ownership/identity");System.out.println("dag:downstream-scc:separate-ownership:identity");}}
`

func TestNativeDependencyComponentAdmissionKeepsDownstreamCyclesIndependent(t *testing.T) {
	files := nativeCompileClasses(t, downstreamComponentFixture)
	for _, order := range [][]string{{"DagEntryOwner", "DownstreamA", "DownstreamB"}, {"DownstreamB", "DownstreamA", "DagEntryOwner"}} {
		t.Run(order[0], func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			snap := snapshotJDECEnv()
			for _, owner := range order {
				participants, cyclic, known := z.nativeMemberDependencyAdmission(owner, nil, snap)
				wantCycle := owner != "DagEntryOwner"
				wantSize := 1
				if wantCycle {
					wantSize = 2
				}
				if !known || cyclic != wantCycle || len(participants) != wantSize {
					t.Fatalf("owner=%s cyclic=%v known=%v participants=%v", owner, cyclic, known, participants)
				}
			}
			root, e := Parse(files["DagEntryOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			entry := z.nativeMemberEntry(root)
			if entry == nil || entry.family == nil || entry.source == "" {
				t.Fatal("acyclic owner did not complete through the independently published downstream component")
			}
			if entry.family.children["DownstreamA$Item"] != nil || entry.family.lexicalObjects["DownstreamA$Item"] != nil {
				t.Fatal("a dependency acquired foreign lexical ownership")
			}
			if len(z.nativeMemberTransactions) != 1 {
				t.Fatalf("unnecessary singleton transaction: %d", len(z.nativeMemberTransactions))
			}
			for _, transaction := range z.nativeMemberTransactions {
				if len(transaction.owners) != 2 {
					t.Fatal("downstream cycle was combined with its acyclic consumer")
				}
			}
		})
	}
}

func TestAdversarialAcyclicOwnerOverCyclicDependenciesRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, downstreamComponentFixture, []string{"DagEntryOwner", "DownstreamA", "DownstreamB"}, "DagEntryDriver", "dag:downstream-scc:separate-ownership:identity\n", nativeLexicalExactSignatures)
}
