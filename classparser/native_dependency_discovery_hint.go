package javaclassparser

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Members of one SCC have the same reachable closure. After a complete
// whole-owner discovery, that component can schedule any of its original
// top-level owners. Independent static boundaries use a different graph and
// cannot publish these hints. This immutable fact owns no parsed model, source
// body, declaration spelling, private scope or constructor permission.
type nativeMemberDependencyFact struct {
	participants []string
	cyclic       bool
}

func (z *JarFS) nativeMemberDependencyHint(entry *nativeMemberCacheEntry) *nativeMemberDependencyFact {
	if z == nil || entry == nil {
		return nil
	}
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	return entry.dependencyHint
}

// A peer need not have had a cache entry when discovery completed. Find the
// existing immutable fact without eagerly allocating entries or policy keys
// for every participant. This is one query of the fixed-capacity metadata
// cache, not an original graph discovery. Charge that query before inspecting
// it; adopting a result separately charges its complete participant metadata.
func (z *JarFS) findNativeMemberDependencyHint(owner, policy string, work *workbudget.Budget) *nativeMemberDependencyFact {
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	if !nativeProofWork(work, 1) {
		return nil
	}
	for key, entry := range z.nativeMembersCache {
		_, sourcePolicy, bound := strings.Cut(key, "\x00")
		if !bound || sourcePolicy != policy || entry == nil || entry.dependencyHint == nil {
			continue
		}
		fact := entry.dependencyHint
		i := sort.SearchStrings(fact.participants, owner)
		if i < len(fact.participants) && fact.participants[i] == owner {
			return fact
		}
	}
	return nil
}

func nativeMemberDependencyFactMatches(fact *nativeMemberDependencyFact, participants []string, cyclic bool) bool {
	if fact == nil || fact.cyclic != cyclic || len(fact.participants) != len(participants) {
		return false
	}
	for i, name := range participants {
		if fact.participants[i] != name {
			return false
		}
	}
	return true
}

func nativeMemberAdoptDependencyFact(owner string, entry *nativeMemberCacheEntry, fact *nativeMemberDependencyFact, work *workbudget.Budget) bool {
	if entry == nil || fact == nil || len(fact.participants) == 0 || len(fact.participants) > nativeMemberDependencyComponentLimit {
		return false
	}
	bytes := nativeMemberDependencyRetainedBytes(fact.participants)
	if !nativeProofWork(work, bytes) || work != nil && work.CheckAlloc(bytes) != nil {
		return false
	}
	found := false
	for i, name := range fact.participants {
		if name == "" || i > 0 && name <= fact.participants[i-1] {
			return false
		}
		found = found || name == owner
	}
	if !found || len(fact.participants) > 1 && !fact.cyclic {
		return false
	}
	entry.dependencyParticipants = fact.participants
	entry.dependenciesCyclic = fact.cyclic
	entry.dependenciesKnown = true
	return true
}

func (z *JarFS) hintNativeMemberDependencyComponent(fact *nativeMemberDependencyFact, policy string) {
	// The normal admission path already charged the one immutable participant
	// slice to the existing shared metadata allowance. Attach only to entries
	// that already exist: no second cache, eager policy-key storage or larger
	// limit. A future peer can find this same fact when its own entry is needed.
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	for key, entry := range z.nativeMembersCache {
		owner, sourcePolicy, bound := strings.Cut(key, "\x00")
		if !bound || sourcePolicy != policy || entry == nil || entry.dependencyHint != nil {
			continue
		}
		i := sort.SearchStrings(fact.participants, owner)
		if i < len(fact.participants) && fact.participants[i] == owner {
			entry.dependencyHint = fact
		}
	}
}
