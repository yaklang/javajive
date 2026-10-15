package javaclassparser

import (
	"encoding/json"
	"github.com/yaklang/javajive/internal/jdecenv"
	"strings"
	"sync"
)

type nativeMemberTransaction struct {
	once     sync.Once
	owners   []string
	snapshot map[string]string
	entries  map[string]*nativeMemberCacheEntry
}

// Original self rows select the physical outermost source owner. Declaration
// spelling and cache state never substitute for this archive identity.
func (z *JarFS) nativeMemberTransactionOwner(obj *ClassObject) (string, bool) {
	return z.nativeMemberOriginalSourceOwner(obj)
}

// An SCC is one source publication transaction with separate lexical plans.
// We may resolve names from the unpublished plans inside this transaction,
// but no caller obtains a source/body/type certificate until all members have
// closed their original ownership, allocation and source proofs. Dependencies
// outside the SCC follow the acyclic condensation graph and must complete first.
func (z *JarFS) nativeMemberTransactionEntry(obj *ClassObject) *nativeMemberCacheEntry {
	owner, known := z.nativeMemberTransactionOwner(obj)
	if !known {
		return nil
	}
	snap, bound := jdecenv.Current()
	if !bound || snap == nil {
		snap = snapshotJDECEnv()
	}
	if snap["JDEC_NATIVE_MEMBER_OFF"] != "" {
		return nil
	}
	policy, _ := json.Marshal(snap)
	ownerKey := owner + "\x00" + string(policy)
	z.nativeMembersMu.Lock()
	cached := z.nativeMembersCache[ownerKey]
	if cached != nil && cached.transaction != nil {
		transaction := cached.transaction
		z.nativeMembersMu.Unlock()
		return z.loadNativeMemberTransaction(transaction, owner)
	}
	z.nativeMembersMu.Unlock()
	// A transaction resolves dependency cycles, not a failed original ownership
	// proof. Consult the same completed single-family planning result first. In
	// particular, an unrepresentable child must not trigger a new archive-wide
	// dependency discovery for every source reference to its parent. Keep only
	// the immutable admission result: a failed finish may have changed its local
	// rendering plan, so the transaction prepares fresh plans for its members.
	standalone := z.nativeMemberPolicyEntry(ownerKey)
	if standalone == nil {
		return nil
	}
	standalone.planOnce.Do(func() { standalone.planKnown = z.nativeMemberLocalPlan(obj, owner, snap) != nil })
	if !standalone.planKnown {
		return nil
	}
	work := z.nativeMemberReader(obj).Work
	participants, cyclic, known := z.nativeMemberDependencyAdmission(owner, work, snap)
	if !known || !cyclic {
		return nil
	}
	if len(participants) == 0 || len(participants) > 64 {
		return nil
	}
	identity, _ := json.Marshal(participants)
	key := string(identity) + "\x00" + string(policy)
	z.nativeMembersMu.Lock()
	transaction := z.nativeMemberTransactions[key]
	if transaction == nil {
		missing := 0
		for _, name := range participants {
			if z.nativeMembersCache[name+"\x00"+string(policy)] == nil {
				missing++
			}
		}
		if len(z.nativeMemberTransactions)+len(z.nativeMembersCache)+missing+1 > 512 {
			z.nativeMembersMu.Unlock()
			return nil
		}
		if z.nativeMemberTransactions == nil {
			z.nativeMemberTransactions = map[string]*nativeMemberTransaction{}
		}
		if z.nativeMembersCache == nil {
			z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
		}
		transaction = &nativeMemberTransaction{owners: participants, snapshot: snap}
		z.nativeMemberTransactions[key] = transaction
		for _, name := range participants {
			k := name + "\x00" + string(policy)
			if z.nativeMembersCache[k] == nil {
				z.nativeMembersCache[k] = &nativeMemberCacheEntry{}
			}
			z.nativeMembersCache[k].transaction = transaction
		}
	}
	z.nativeMembersMu.Unlock()
	return z.loadNativeMemberTransaction(transaction, owner)
}

func (z *JarFS) loadNativeMemberTransaction(transaction *nativeMemberTransaction, owner string) *nativeMemberCacheEntry {
	transaction.once.Do(func() { transaction.entries = z.buildNativeMemberTransaction(transaction.owners, transaction.snapshot) })
	return transaction.entries[owner]
}

func (z *JarFS) buildNativeMemberTransaction(participants []string, snap map[string]string) map[string]*nativeMemberCacheEntry {
	work := z.nativeMemberReader(nil).Work
	policy, err := json.Marshal(snap)
	if err != nil {
		return nil
	}

	prepared := map[string]*nativeMemberPrepared{}
	var inputBytes int64
	for _, name := range participants {
		if !nativeProofWork(work, 1) {
			return nil
		}
		raw, found := z.enumSiblingResolver()(name)
		if !found || len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-inputBytes {
			return nil
		}
		inputBytes += int64(len(raw))
		if work != nil && work.CheckAlloc(inputBytes) != nil {
			return nil
		}
		root, err := z.nativeMemberReader(nil).parseResolved(raw)
		if err != nil || root.GetClassName() != name || !nativeMemberTopLevelEvidence(root, work) {
			return nil
		}
		// Every participant shares the same immutable local admission used by
		// ordinary reads. Replanning a previously refused participant here could
		// change a flat declaration into a lexical one after another unit had
		// already bound its source name. Fresh rendering plans remain private;
		// only the admission result is shared across the two publication paths.
		entry := z.nativeMemberPolicyEntry(name + "\x00" + string(policy))
		if entry == nil {
			return nil
		}
		var plan *nativeMemberPrepared
		entry.planOnce.Do(func() {
			plan = z.prepareNativeMemberFamilyUnpublished(root, snap)
			entry.planKnown = plan != nil
		})
		if !entry.planKnown {
			return nil
		}
		if plan == nil {
			plan = z.prepareNativeMemberFamilyUnpublished(root, snap)
		}
		if plan == nil {
			return nil
		}
		prepared[name] = plan
	}
	lookup := func(name string) *nativeMemberClass {
		name = strings.ReplaceAll(name, ".", "/")
		// Each child is exposed as a spelling/constructor view only. The receiving
		// family's children, lexical objects, getters and registration remain local.
		for _, participant := range participants {
			if child := prepared[participant].family.children[name]; child != nil {
				return child
			}
		}
		return z.nativeMemberLookup(name)
	}
	results := map[string]*nativeMemberCacheEntry{}
	var bytes int64
	for _, name := range participants {
		result := z.finishNativeMemberFamily(prepared[name], lookup, true, prepared)
		if result == nil || int64(len(result.source)) > (16<<20)-bytes {
			return nil
		}
		bytes += int64(len(result.source))
		if work != nil && work.CheckAlloc(bytes) != nil {
			return nil
		}
		results[name] = result
	}
	// No allowance is reserved and no partial result is published on failure.
	// The once completion publishes the immutable map to all concurrent readers.
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	if bytes > (16<<20)-z.nativeMembersBytes || !z.reserveOwnershipSource(bytes) {
		return nil
	}
	z.nativeMembersBytes += bytes
	return results
}
