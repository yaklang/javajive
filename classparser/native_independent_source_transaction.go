package javaclassparser

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A static boundary with children is a complete lexical source family, not a
// leaf declaration. Mutually dependent independent families need one completion
// transaction. Provisional peers grant spelling/constructor views only; each
// family's private, capture, accessor and registration proofs remain separate.
func (z *JarFS) nativeIndependentTransactionEntry(root *ClassObject, participants []string, snap map[string]string) *nativeMemberCacheEntry {
	if z == nil || root == nil || snap == nil || snap["JDEC_NATIVE_MEMBER_OFF"] != "" || len(participants) < 2 || len(participants) > nativeMemberDependencyComponentLimit {
		return nil
	}
	names := append([]string(nil), participants...)
	sort.Strings(names)
	found := false
	for i, name := range names {
		if name == "" || i > 0 && name == names[i-1] {
			return nil
		}
		found = found || name == root.GetClassName()
	}
	if !found {
		return nil
	}
	work := z.nativeMemberReader(root).Work
	if !nativeProofWork(work, int64(len(names))+1) {
		return nil
	}
	actual, cyclic, known := z.nativeMemberDependencyAdmission(root.GetClassName(), work, snap)
	if !known || !cyclic || len(actual) != len(names) {
		return nil
	}
	for i, name := range names {
		if actual[i] != name {
			return nil
		}
	}
	policy, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	if work != nil && work.CheckAlloc(nativeMemberDependencyRetainedBytes(names)*8+int64(len(policy))*2+1024) != nil {
		return nil
	}
	identity, _ := json.Marshal(names)
	key := "independent-static-scc\x00" + string(identity) + "\x00" + string(policy)
	z.nativeMembersMu.Lock()
	cached := z.nativeMemberTransactions[key]
	z.nativeMembersMu.Unlock()
	// A boundary graph can also contain ordinary top-level families. Reject
	// those roles before claiming any participant's cache entry; otherwise a
	// failed independent attempt could block a valid ordinary transaction.
	// Existing transactions already closed this original eligibility query.
	if cached == nil && !z.nativeIndependentParticipantsClosed(names, snap, work) {
		return nil
	}
	z.nativeMembersMu.Lock()
	transaction := z.nativeMemberTransactions[key]
	for _, name := range names {
		for _, prefix := range []string{"", "independent-static\x00"} {
			entry := z.nativeMembersCache[prefix+name+"\x00"+string(policy)]
			if entry != nil && (entry.flatSourceCommitted || entry.transaction != nil && entry.transaction != transaction) {
				// A source namespace decision is monotone. Never replace a flat
				// commitment or steal an entry from another completion protocol.
				z.nativeMembersMu.Unlock()
				return nil
			}
		}
	}
	if transaction == nil {
		missing := 0
		for _, name := range names {
			for _, prefix := range []string{"", "independent-static\x00"} {
				if z.nativeMembersCache[prefix+name+"\x00"+string(policy)] == nil {
					missing++
				}
			}
		}
		if len(z.nativeMemberTransactions)+len(z.nativeMembersCache)+missing+1 > nativeMemberCacheLimit {
			z.nativeMembersMu.Unlock()
			return nil
		}
		if z.nativeMemberTransactions == nil {
			z.nativeMemberTransactions = map[string]*nativeMemberTransaction{}
		}
		if z.nativeMembersCache == nil {
			z.nativeMembersCache = map[string]*nativeMemberCacheEntry{}
		}
		snapshot := map[string]string{}
		for name, value := range snap {
			snapshot[name] = value
		}
		transaction = &nativeMemberTransaction{independent: true, owners: names, snapshot: snapshot}
		z.nativeMemberTransactions[key] = transaction
		for _, name := range names {
			for _, prefix := range []string{"", "independent-static\x00"} {
				k := prefix + name + "\x00" + string(policy)
				if z.nativeMembersCache[k] == nil {
					z.nativeMembersCache[k] = &nativeMemberCacheEntry{}
				}
				z.nativeMembersCache[k].transaction = transaction
			}
		}
	}
	z.nativeMembersMu.Unlock()
	return z.loadNativeMemberTransaction(transaction, root.GetClassName())
}

func (z *JarFS) nativeIndependentParticipantsClosed(names []string, snap map[string]string, work *workbudget.Budget) bool {
	var size int64
	for _, name := range names {
		raw, known := z.enumSiblingResolver()(name)
		if !known || len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-size || !nativeProofWork(work, int64(len(name)+len(raw))+1) {
			return false
		}
		size += int64(len(raw))
		if work != nil && work.CheckAlloc(size) != nil {
			return false
		}
		reader := z.nativeMemberReader(nil)
		reader.Work = work
		object, err := reader.parseResolved(raw)
		if err != nil || object.GetClassName() != name {
			return false
		}
		reader = z.nativeMemberReader(object)
		reader.Work = work
		reader.options.EnvSnapshot = snap
		certificate := reader.originalNativeMemberIndependentRoot()
		if certificate == nil {
			return false
		}
		policy, err := json.Marshal(snap)
		if err != nil {
			return false
		}
		entry := z.nativeMemberPolicyEntry("independent-static\x00" + name + "\x00" + string(policy))
		if entry == nil || z.nativeIndependentStandaloneEntry(entry, object, snap, certificate) != nil {
			// Finish/wait for the smaller existing source path before claiming
			// either role key. Do not steal a completed standalone partition.
			return false
		}
	}
	return true
}

func (z *JarFS) buildNativeIndependentTransaction(participants []string, snap map[string]string) map[string]*nativeMemberCacheEntry {
	work := z.nativeMemberReader(nil).Work
	prepared := map[string]*nativeMemberPrepared{}
	retained := nativeMemberTransactionInputs{}
	owned := map[string]string{}
	var inputBytes int64
	for _, name := range participants {
		if !nativeProofWork(work, int64(len(name))+1) {
			return nil
		}
		raw, known := z.enumSiblingResolver()(name)
		if !known || len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-inputBytes {
			return nil
		}
		inputBytes += int64(len(raw))
		if work != nil && work.CheckAlloc(inputBytes) != nil {
			return nil
		}
		reader := z.nativeMemberReader(nil)
		reader.Work = work
		root, err := reader.parseResolved(raw)
		if err != nil || root.GetClassName() != name {
			return nil
		}
		reader = z.nativeMemberReader(root)
		reader.Work = work
		reader.options.EnvSnapshot = snap
		certificate := reader.originalNativeMemberIndependentRoot()
		if certificate == nil {
			return nil
		}
		plan := z.prepareNativeMemberFamilyFromRoot(root, snap, certificate)
		if plan == nil || plan.family == nil || !certificate.familyClosed(plan.family, work) {
			return nil
		}
		if !retained.add(z, plan, work) {
			return nil
		}
		for symbol := range plan.family.lexicalObjects {
			if prior := owned[symbol]; prior != "" && prior != name {
				return nil
			}
			owned[symbol] = name
		}
		prepared[name] = plan
	}
	lookup := func(name string) *nativeMemberClass {
		name = strings.ReplaceAll(name, ".", "/")
		for _, owner := range participants {
			plan := prepared[owner]
			if child := plan.family.children[name]; child != nil {
				return child
			}
			if owner == name {
				return plan.family.independentRoot.declaration
			}
		}
		return z.nativeMemberLookup(name)
	}
	results := map[string]*nativeMemberCacheEntry{}
	var size int64
	for _, name := range participants {
		result := z.finishNativeMemberFamily(prepared[name], lookup, true, prepared)
		if result == nil || int64(len(result.source)) > (16<<20)-size {
			return nil
		}
		size += int64(len(result.source))
		if work != nil && work.CheckAlloc(size) != nil {
			return nil
		}
		results[name] = result
	}
	// Complete every body before reserving bytes or publishing the shared map.
	z.nativeMembersMu.Lock()
	defer z.nativeMembersMu.Unlock()
	if size > (16<<20)-z.nativeMembersBytes || !z.reserveOwnershipSource(size) {
		return nil
	}
	z.nativeMembersBytes += size
	return results
}
