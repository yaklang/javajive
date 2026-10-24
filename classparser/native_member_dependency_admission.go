package javaclassparser

import (
	"encoding/json"

	"github.com/yaklang/javajive/internal/workbudget"
)

// The archive and original ownership graph are immutable within a source policy.
// Discover at most once per bounded owner cache entry, including closed negative
// results (missing declarations, unsupported ownership or graph-size limits).
// Retain only admission facts and this owner's SCC, rather than every parsed
// class or a quadratic dependency graph. No source/private scope is published
// by discovery. A failed work budget remains failed and cannot certify source.
func (z *JarFS) nativeMemberDependencyAdmission(owner string, work *workbudget.Budget, snap map[string]string) ([]string, bool, bool) {
	if z == nil || owner == "" || !nativeProofWork(work, 1) {
		return nil, false, false
	}
	policy, err := json.Marshal(snap)
	if err != nil {
		return nil, false, false
	}
	entry := z.nativeMemberPolicyEntry(owner + "\x00" + string(policy))
	if entry == nil {
		return nil, false, false
	}
	entry.dependenciesOnce.Do(func() {
		fact := z.nativeMemberDependencyHint(entry)
		if fact == nil {
			fact = z.findNativeMemberDependencyHint(owner, string(policy), work)
		}
		if fact != nil {
			nativeMemberAdoptDependencyFact(owner, entry, fact, work)
			return
		}
		mode := &nativeMemberDependencyRootMode{}
		graph, known := z.nativeMemberCompletionDependencyGraphWithMode(owner, work, snap, mode)
		if !known {
			return
		}
		components, known := nativeMemberSourceComponents(graph, work)
		if !known {
			return
		}
		participants := components[owner]
		if len(participants) == 0 || len(participants) > nativeMemberDependencyComponentLimit {
			return
		}
		cyclic := len(participants) > 1 || graph[owner][owner]
		// Another owner may have completed the same immutable component while
		// this discovery was running. Reuse its already charged metadata.
		if fact := z.nativeMemberDependencyHint(entry); fact != nil {
			if !nativeMemberDependencyFactMatches(fact, participants, cyclic) {
				return
			}
			nativeMemberAdoptDependencyFact(owner, entry, fact, work)
			return
		}
		bytes := nativeMemberDependencyRetainedBytes(participants)
		if work != nil && work.CheckAlloc(bytes) != nil || !z.reserveOwnershipDependencyMetadata(bytes) {
			return
		}
		fact = &nativeMemberDependencyFact{participants: append([]string(nil), participants...), cyclic: cyclic}
		entry.dependencyParticipants = fact.participants
		// Only this owner's SCC needs a joint publication transaction. A
		// downstream cycle is resolved by its own component when looked up;
		// the condensation graph cannot lead back to this acyclic owner.
		// Treating every reachable cycle as this owner's cycle needlessly
		// blocks ordinary completion and stable flat declaration bindings.
		entry.dependenciesCyclic = fact.cyclic
		entry.dependenciesKnown = true
		if mode.wholeOwner && len(participants) > 1 {
			z.hintNativeMemberDependencyComponent(fact, string(policy))
		}
	})
	if work != nil && (work.Check() != nil || work.CheckAlloc(nativeMemberDependencyRetainedBytes(entry.dependencyParticipants)) != nil) {
		return nil, false, false
	}
	return entry.dependencyParticipants, entry.dependenciesCyclic, entry.dependenciesKnown
}

func nativeMemberDependencyRetainedBytes(participants []string) int64 {
	bytes := int64(128)
	for _, name := range participants {
		bytes += int64(len(name)) + 16
	}
	return bytes
}
