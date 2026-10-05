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
		graph, known := z.nativeMemberOriginalDependencyGraph(owner, work)
		if !known {
			return
		}
		components, known := nativeMemberSourceComponents(graph, work)
		if !known {
			return
		}
		participants := components[owner]
		if len(participants) == 0 || len(participants) > 64 {
			return
		}
		bytes := nativeMemberDependencyRetainedBytes(participants)
		if work != nil && work.CheckAlloc(bytes) != nil || !z.reserveOwnershipDependencyMetadata(bytes) {
			return
		}
		entry.dependencyParticipants = append([]string(nil), participants...)
		for name, component := range components {
			entry.dependenciesCyclic = entry.dependenciesCyclic || len(component) > 1 || graph[name][name]
		}
		entry.dependenciesKnown = true
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
