package javaclassparser

import (
	"encoding/json"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An original type-reference SCC is an upper bound on source-completion
// dependencies. A family whose original local admission is terminally refused
// cannot later publish a lexical spelling. An independently accessible static
// declaration from that family can therefore supply a physical name without
// joining a source transaction. Reuse the exact declaration/access proof from
// finish; a cached false, a temporary failed finish or a resource failure cannot
// cut an edge. No source, private nest or constructor view is published here.
func (z *JarFS) nativeMemberCompletionDependencyGraph(root string, work *workbudget.Budget, snapshot map[string]string) (map[string]map[string]bool, bool) {
	return z.nativeMemberCompletionDependencyGraphWithMode(root, work, snapshot, nil)
}

func (z *JarFS) nativeMemberCompletionDependencyGraphWithMode(root string, work *workbudget.Budget, snapshot map[string]string, mode *nativeMemberDependencyRootMode) (map[string]map[string]bool, bool) {
	if z == nil || snapshot == nil || !nativeProofWork(work, 1) {
		return nil, false
	}
	policy, err := json.Marshal(snapshot)
	if err != nil {
		return nil, false
	}
	prepared := map[string]*nativeMemberPrepared{}
	retained := nativeMemberTransactionInputs{}
	type edge struct{ owner, declaration string }
	proofs := map[edge]bool{}
	var graph map[string]map[string]bool
	var known bool
	jdecenv.Run(snapshot, func() error {
		graph, known = z.nativeMemberDependencyGraph(root, work, func(owner, declaration *ClassObject) (bool, bool) {
			if owner == nil || declaration == nil || !nativeProofWork(work, 1) {
				return false, false
			}
			foreign, _, flags, member := originalMemberOwner(declaration)
			if !member || flags&8 == 0 || foreign == owner.GetClassName() {
				return false, true
			}
			key := edge{owner.GetClassName(), declaration.GetClassName()}
			if cut, seen := proofs[key]; seen {
				return cut, true
			}
			if len(proofs) >= nativeMemberDependencyEdgeLimit || !nativeProofWork(work, int64(len(key.owner)+len(key.declaration))+1) || work != nil && work.CheckAlloc(int64(len(proofs)+1)*256+retained.bytes) != nil {
				return false, false
			}
			proofs[key] = false
			entry := z.nativeMemberPolicyEntry(foreign + "\x00" + string(policy))
			if entry == nil {
				return false, true
			}
			z.admitNativeMemberLocal(entry, nil, foreign, snapshot)
			if !nativeProofWork(work, 1) {
				return false, false
			}
			if entry.planKnown || !entry.originalPlanRefusal {
				return false, true
			}
			plan, seen := prepared[key.owner]
			if !seen {
				if len(prepared) >= nativeMemberDependencyNodeLimit {
					return false, false
				}
				local := z.nativeMemberPolicyEntry(key.owner + "\x00" + string(policy))
				if local != nil {
					plan = z.admitNativeMemberLocal(local, owner, key.owner, snapshot)
					if local.planKnown && plan == nil {
						plan = z.prepareNativeMemberFamilyUnpublished(owner, snapshot)
					}
				}
				if plan != nil && !retained.add(z, plan, work) {
					return false, false
				}
				prepared[key.owner] = plan
			}
			if plan == nil {
				return false, true
			}
			cut := z.terminalFlatStaticDeclaration(plan, declaration) != ""
			if !nativeProofWork(work, 1) {
				return false, false
			}
			proofs[key] = cut
			return cut, true
		}, mode)
		return nil
	})
	return graph, known
}
