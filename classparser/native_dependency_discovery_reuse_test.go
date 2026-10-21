package javaclassparser

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A successful complete graph discovery already proved every reachable
// top-level component. A different owner in the same component must not need
// to rediscover and parse the entire archive under its own request budget.
// The reusable fact concerns scheduling only, never a source/private scope.
func TestNativeDependencyAdmissionReusesCompletedTopLevelComponentDiscovery(t *testing.T) {
	files := nativeCompileClasses(t, nativeThreeCyclicDependencyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	snapshot := snapshotJDECEnv()
	first := workbudget.New(nil, workbudget.Limits{})
	participants, cyclic, known := z.nativeMemberDependencyAdmission("StageOne", first, snapshot)
	want := []string{"StageOne", "StageThree", "StageTwo"}
	if !known || !cyclic || !slices.Equal(participants, want) {
		t.Fatal("original complete three-owner component", participants, cyclic, known)
	}
	for _, owner := range []string{"StageTwo", "StageThree"} {
		current := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 256})
		got, cycle, closed := z.nativeMemberDependencyAdmission(owner, current, snapshot)
		if !closed || !cycle || !slices.Equal(got, want) {
			t.Fatalf("a proved component was rediscovered for %s: participants=%v cyclic=%v known=%v current_scans=%d first_scans=%d", owner, got, cycle, closed, current.Used(workbudget.CounterGraphScans), first.Used(workbudget.CounterGraphScans))
		}
	}
	if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
		t.Fatal("discovery reused source/private ownership instead of metadata")
	}
}

func TestNativeDependencyDiscoveryHintKeepsPolicyAndIndependentBoundaries(t *testing.T) {
	files := nativeCompileClasses(t, nativeThreeCyclicDependencyFixture)
	t.Run("policy", func(t *testing.T) {
		z := nativeArchive(t, files)
		defer z.Close()
		snapshot := snapshotJDECEnv()
		if _, _, known := z.nativeMemberDependencyAdmission("StageOne", nil, snapshot); !known {
			t.Fatal("original whole-owner discovery")
		}
		snapshot["JDEC_NO_SOURCE_REWRITES"] = "1"
		current := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 256})
		if _, _, known := z.nativeMemberDependencyAdmission("StageTwo", current, snapshot); known {
			t.Fatal("a different source policy borrowed a completed discovery")
		}
	})
	t.Run("independent static roots", func(t *testing.T) {
		z := nativeArchive(t, files)
		defer z.Close()
		snapshot := snapshotJDECEnv()
		parts, cyclic, known := z.nativeMemberDependencyAdmission("StageOne$Node", nil, snapshot)
		if !known || !cyclic || len(parts) != 3 {
			t.Fatal("original independent-boundary component", parts, cyclic, known)
		}
		policy, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		for _, owner := range []string{"StageOne", "StageTwo", "StageThree", "StageTwo$Node"} {
			entry := z.nativeMemberPolicyEntry(owner + "\x00" + string(policy))
			if z.nativeMemberDependencyHint(entry) != nil {
				t.Fatal("independent discovery published a whole-owner fact", owner)
			}
		}
	})
}

func TestNativeDependencyDiscoveryDoesNotAllocateUnusedPeerEntries(t *testing.T) {
	fixture := strings.ReplaceAll(nativeCyclicDependencyFixture, "static class Value", "class Value")
	files := nativeCompileClasses(t, fixture)
	z := nativeArchive(t, files)
	defer z.Close()
	snapshot := snapshotJDECEnv()
	want, cyclic, known := z.nativeMemberDependencyAdmission("CycleScopeA", nil, snapshot)
	if !known || !cyclic || len(want) != 2 {
		t.Fatal("original nonstatic component", want, cyclic, known)
	}
	policy, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMembersCache["CycleScopeB\x00"+string(policy)] != nil {
		t.Fatal("discovery eagerly allocated an unused owner/policy entry")
	}
	retained := z.sourceOwnership.dependencyBytes
	current := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 256})
	got, cycle, closed := z.nativeMemberDependencyAdmission("CycleScopeB", current, snapshot)
	if !closed || !cycle || !slices.Equal(got, want) {
		t.Fatal("a later peer did not find the existing complete fact", got, cycle, closed)
	}
	if z.sourceOwnership.dependencyBytes != retained || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
		t.Fatal("late scheduling lookup retained duplicate metadata or source")
	}
}

func TestNativeDependencyDiscoveryHintCannotReplaceOriginalLocalAdmission(t *testing.T) {
	files := nativeCompileClasses(t, nativeCompletionCutFixture)
	nativeTestForeignSourceProtocolUnproved(t, files, "ResidualRight")
	z := nativeArchive(t, files)
	defer z.Close()
	snapshot := snapshotJDECEnv()
	parts, cyclic, known := z.nativeMemberDependencyAdmission("ResidualLeft", nil, snapshot)
	if !known || !cyclic || !slices.Contains(parts, "ResidualRight") {
		t.Fatal("real nonstatic dependency retained", parts, cyclic, known)
	}
	policy, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	current := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 256})
	if got, cycle, known := z.nativeMemberDependencyAdmission("ResidualRight", current, snapshot); !known || !cycle || !slices.Equal(got, parts) {
		t.Fatal("a later peer did not reuse the complete scheduling fact", got, cycle, known)
	}
	entry := z.nativeMemberPolicyEntry("ResidualRight\x00" + string(policy))
	root, err := Parse(files["ResidualRight.class"])
	if err != nil {
		t.Fatal(err)
	}
	if z.nativeMemberTransactionEntry(root) != nil || entry.planKnown || !entry.originalPlanRefusal {
		t.Fatal("a graph fact replaced the original source-admission refusal")
	}
	if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
		t.Fatal("a scheduling fact published source/private ownership")
	}
}

func TestNativeDependencyDiscoveryConcurrentFactsStayComplete(t *testing.T) {
	files := nativeCompileClasses(t, nativeThreeCyclicDependencyFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	snapshot := snapshotJDECEnv()
	want := []string{"StageOne", "StageThree", "StageTwo"}
	start := make(chan struct{})
	var wait sync.WaitGroup
	results := make([]bool, 12)
	for i := range results {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			<-start
			parts, cyclic, known := z.nativeMemberDependencyAdmission(want[i%len(want)], nil, snapshot)
			results[i] = known && cyclic && slices.Equal(parts, want)
		}(i)
	}
	close(start)
	wait.Wait()
	for i, ok := range results {
		if !ok {
			t.Fatal("incomplete concurrent scheduling fact", i)
		}
	}
	if len(z.nativeMemberTransactions) != 0 || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
		t.Fatal("concurrent discovery published source/private ownership")
	}
}

// Denial models reopen the private immutable fact's narrow scheduling role.
// They are malformed metadata controls, not executable bytecode counterexamples.
func TestNativeDependencyDiscoveryFactRequiresCurrentOwnerAndResources(t *testing.T) {
	for _, variant := range []string{"original", "nil fact", "nil entry", "empty", "oversize", "missing owner", "duplicate", "unordered", "empty name", "noncyclic multiple owners", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			fact := &nativeMemberDependencyFact{participants: []string{"A", "B"}, cyclic: true}
			entry := &nativeMemberCacheEntry{}
			owner := "A"
			var work *workbudget.Budget
			switch variant {
			case "nil fact":
				fact = nil
			case "nil entry":
				entry = nil
			case "empty":
				fact.participants = nil
			case "oversize":
				fact.participants = make([]string, nativeMemberDependencyComponentLimit+1)
			case "missing owner":
				owner = "C"
			case "duplicate":
				fact.participants[1] = "A"
			case "unordered":
				fact.participants = []string{"B", "A"}
			case "empty name":
				fact.participants[0] = ""
			case "noncyclic multiple owners":
				fact.cyclic = false
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			known := nativeMemberAdoptDependencyFact(owner, entry, fact, work)
			if known != (variant == "original") {
				t.Fatal("invalid current scheduling admission", known)
			}
			if entry != nil && !known && (entry.dependenciesKnown || entry.dependenciesCyclic || len(entry.dependencyParticipants) != 0) {
				t.Fatal("a refused fact published a partial component")
			}
		})
	}
}
