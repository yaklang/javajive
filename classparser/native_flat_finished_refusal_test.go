package javaclassparser

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeFlatFinishedRefusalCommitsOnlyStableDeclarationLayout(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"Consumer.java":     `class Consumer{static class Child{}Producer.BuilderBase ref;}class Producer extends p.HiddenBase{private static Object value;static class BuilderBase{Object read(){return Producer.value;}}}`,
		"p/HiddenBase.java": `package p;public class HiddenBase{}`,
	}, "none", "8")
	for _, variant := range []string{"completed refusal", "transaction owns layout", "cyclic admission", "unclosed admission", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for n, b := range files {
				if n != "p/HiddenBase.class" {
					input[n] = b
				}
			}
			z := nativeArchive(t, input)
			defer z.Close()
			var available atomic.Bool
			z.declarationResolver = func(n string) ([]byte, bool) {
				if n == "p/HiddenBase" && available.Load() {
					return files[n+".class"], true
				}
				return nil, false
			}
			parse := func(n string) *ClassObject {
				t.Helper()
				o, e := Parse(files[n+".class"])
				if e != nil {
					t.Fatal(e)
				}
				return o
			}
			snap := snapshotJDECEnv()
			prepared := z.prepareNativeMemberFamilyUnpublished(parse("Consumer"), snap)
			producer := parse("Producer")
			if prepared == nil {
				t.Fatal("consumer local plan missing")
			}
			entry := z.nativeMemberEntry(producer)
			if entry == nil || !entry.planKnown || entry.family != nil || entry.source != "" {
				t.Fatal("fixture must have a completed finish refusal, not a local refusal")
			}
			_, cyclic, known := z.nativeMemberDependencyAdmission("Producer", nil, snap)
			if !known || cyclic {
				t.Fatal("original scheduling must be closed and acyclic")
			}
			// Negative cache states model unfinished ownership or a dependency
			// receipt that cannot certify a permanent flat source namespace.
			switch variant {
			case "transaction owns layout":
				entry.transaction = &nativeMemberTransaction{}
			case "cyclic admission":
				entry.dependenciesCyclic = true
			case "unclosed admission":
				entry.dependenciesKnown = false
			case "memory":
				prepared.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				prepared.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			member := parse("Producer$BuilderBase")
			var readers sync.WaitGroup
			results := make([]string, 16)
			for i := range results {
				readers.Add(1)
				go func(i int) { defer readers.Done(); results[i] = z.completedFlatStaticDeclaration(prepared, member) }(i)
			}
			readers.Wait()
			want := ""
			if variant == "completed refusal" {
				want = "Producer$BuilderBase"
			}
			for _, result := range results {
				if result != want {
					t.Fatalf("declaration=%q want=%q", result, want)
				}
			}
			z.nativeMembersMu.Lock()
			pinned := entry.flatSourceCommitted
			z.nativeMembersMu.Unlock()
			if pinned != (want != "") {
				t.Fatal("refused or unfinished declaration committed a namespace")
			}
			if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 || prepared.family.lexicalObjects[member.GetClassName()] != nil || prepared.family.children[member.GetClassName()] != nil {
				t.Fatal("declaration binding borrowed source or private lexical ownership")
			}
			if want == "" {
				return
			}
			// A later successful external header lookup cannot retry the completed
			// ordinary source decision or replace a committed physical spelling.
			available.Store(true)
			for i := 0; i < 8; i++ {
				if z.nativeMemberTransactionEntry(producer) != nil || z.nativeMemberEntry(producer) != entry || z.completedFlatStaticDeclaration(prepared, member) != want {
					t.Fatal("late external metadata changed an already bound source layout")
				}
			}
		})
	}
}

func TestNativeSourceTransactionCannotEnrollCommittedFlatParticipant(t *testing.T) {
	files := nativeCompileClasses(t, nativeCyclicDependencyFixture)
	for _, pinned := range []bool{false, true} {
		name := "uncommitted"
		if pinned {
			name = "committed flat participant"
		}
		t.Run(name, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			policy, _ := json.Marshal(snapshotJDECEnv())
			entry := z.nativeMemberPolicyEntry("CycleScopeB\x00" + string(policy))
			z.nativeMembersMu.Lock()
			entry.flatSourceCommitted = pinned
			z.nativeMembersMu.Unlock()
			var readers sync.WaitGroup
			results := make([]*nativeMemberCacheEntry, 16)
			for i := range results {
				o, e := Parse(files[[]string{"CycleScopeA.class", "CycleScopeB.class"}[i%2]])
				if e != nil {
					t.Fatal(e)
				}
				readers.Add(1)
				go func(i int, o *ClassObject) { defer readers.Done(); results[i] = z.nativeMemberTransactionEntry(o) }(i, o)
			}
			readers.Wait()
			for _, r := range results {
				if pinned && (r != nil) || !pinned && (r == nil || r.family == nil) {
					t.Fatal("transaction violated participant namespace commitment")
				}
			}
			if pinned && (len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0) {
				t.Fatal("refused participant allocated transaction ownership")
			}
		})
	}
}
