package javaclassparser

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// completedFlatStaticDeclaration supplies a declaration spelling only. A
// physically accessible static type need not reconstruct its outer's family
// to remain nameable in a different source unit. No foreign body, constructor,
// private access, capture or registration becomes owned by the receiving plan.
func (z *JarFS) completedFlatStaticDeclaration(prepared *nativeMemberPrepared, other *ClassObject, peers ...map[string]*nativeMemberPrepared) string {
	return z.nativeMemberFlatStaticDeclaration(prepared, other, false, peers...)
}

// A failed pure local admission is final for this immutable archive/policy.
// Consult that fact without entering a possibly active source completion: a
// type dependency cycle need not become a source publication transaction when
// the foreign family can never participate in one.
func (z *JarFS) terminalFlatStaticDeclaration(prepared *nativeMemberPrepared, other *ClassObject, peers ...map[string]*nativeMemberPrepared) string {
	return z.nativeMemberFlatStaticDeclaration(prepared, other, true, peers...)
}

func (z *JarFS) nativeMemberFlatStaticDeclaration(prepared *nativeMemberPrepared, other *ClassObject, terminalOnly bool, peers ...map[string]*nativeMemberPrepared) string {
	if z == nil || prepared == nil || prepared.reader == nil || prepared.root == nil || prepared.family == nil || prepared.family.failed || prepared.snapshot == nil || prepared.snapshot["JDEC_NATIVE_MEMBER_OFF"] != "" || other == nil {
		return ""
	}
	d := prepared.reader
	if !nativeMemberDeclarationMetadataBounded(other, d.Work) || !nativeProofWork(d.Work, int64(len(other.GetClassName()))+1) {
		return ""
	}
	owner, name, flags, known := originalMemberOwner(other)
	if !known || flags&8 == 0 || flags&7 != 0 && flags&7 != 1 && flags&7 != 4 || owner == prepared.root.GetClassName() || !nativeSourceBinaryName(other.GetClassName()) {
		return ""
	}
	for _, component := range peers {
		if !nativeProofWork(d.Work, 1) || component[owner] != nil {
			// An unpublished peer may still choose lexical source. Its temporary
			// lookup failure cannot establish a stable flat declaration.
			return ""
		}
	}
	resolve := d.nativeAnnotationDeclarationResolver()
	original, found := resolve(other.GetClassName())
	if !found || original == nil || original.GetClassName() != other.GetClassName() || original.AccessFlags != other.AccessFlags {
		return ""
	}
	// Lexical protected access and physical ClassFile visibility are distinct:
	// javac emits protected member types as public ClassFiles. Once this
	// declaration is independently flat, naming it uses that public header.
	// Its private fields, constructors and lexical scope remain unowned.
	physicalOwner, physicalName, physicalFlags, named := originalMemberOwner(original)
	if !named || physicalOwner != owner || physicalName != name || physicalFlags != flags {
		return ""
	}
	outer, found := resolve(owner)
	if !found || outer == nil || outer.GetClassName() != owner || !nativeMemberTopLevelEvidence(outer, d.Work) || !nativeOriginalDeclarationEdge(outer, original, d.Work) {
		return ""
	}
	// A flat type is nameable by its physical public header or from the same
	// original package. A widened flat header grants no new cross-package
	// permission for an originally nonpublic class.
	if original.AccessFlags&1 == 0 && sourceBinaryPackage(original.GetClassName()) != sourceBinaryPackage(prepared.root.GetClassName()) {
		return ""
	}
	// A named package cannot refer to an unnamed-package source declaration.
	if !strings.Contains(original.GetClassName(), "/") && strings.Contains(prepared.root.GetClassName(), "/") {
		return ""
	}
	// Static/access flags live in InnerClasses; ACC_SUPER and the physical
	// public header live in ClassFile. All declaration-kind flags must agree.
	physicalModifiers := original.AccessFlags &^ uint16(0x21)
	if flags&^uint16(7) != physicalModifiers|8 {
		return ""
	}
	var entry *nativeMemberCacheEntry
	jdecenv.Run(prepared.snapshot, func() error {
		if terminalOnly {
			policy, err := json.Marshal(prepared.snapshot)
			if err != nil {
				return nil
			}
			candidate := z.nativeMemberPolicyEntry(owner + "\x00" + string(policy))
			if candidate == nil {
				return nil
			}
			z.admitNativeMemberLocal(candidate, outer, owner, prepared.snapshot)
			if !candidate.planKnown && candidate.originalPlanRefusal {
				entry = candidate
			}
		} else {
			entry = z.nativeMemberEntry(outer)
		}
		return nil
	})
	// A local-plan refusal is terminal. A completed ordinary finish refusal
	// can also commit a flat layout when its cached dependency graph is acyclic.
	// Missing entries and potentially recoverable transactions remain unproved.
	// Transactions require that same local plan to succeed. Any subsequent
	// independent direct static root still uses its physical binary spelling;
	// nested independent cuts are excluded by the top-level evidence above.
	if entry == nil || entry.family != nil {
		return ""
	}
	pinFinishedRefusal := entry.planKnown
	if pinFinishedRefusal {
		// A completed ordinary finish is terminal when the original graph is
		// acyclic, or its exact source component has also completed unsuccessfully.
		// An active/unattempted SCC still owns a possible lexical source layout.
		var cyclic, known bool
		jdecenv.Run(prepared.snapshot, func() error {
			_, cyclic, known = z.nativeMemberDependencyAdmission(owner, d.Work, prepared.snapshot)
			return nil
		})
		if !known {
			return ""
		}
		if cyclic {
			z.nativeMembersMu.Lock()
			refused := nativeMemberCompletedComponentRefusal(entry, owner, prepared.snapshot, d.Work)
			z.nativeMembersMu.Unlock()
			if !refused {
				return ""
			}
		}
	}
	// No foreign family completed, so its indexed constructor-access proof
	// cannot be assumed. A declaration spelling is not a private-access grant.
	if !z.flatStaticOriginalConstructorAccessClosed(prepared, original) {
		return ""
	}
	if !nativeMemberFlatAncestorAccessorNamesClosed(prepared, outer) {
		return ""
	}
	if d.Work != nil && d.Work.CheckAlloc(int64(len(prepared.family.sourceDependencies)+1)*128+int64(len(original.GetClassName()))*2) != nil {
		return ""
	}
	if pinFinishedRefusal {
		z.nativeMembersMu.Lock()
		if entry.transaction != nil && !nativeMemberCompletedComponentRefusal(entry, owner, prepared.snapshot, d.Work) {
			z.nativeMembersMu.Unlock()
			return ""
		}
		entry.flatSourceCommitted = true
		z.nativeMembersMu.Unlock()
	}
	return strings.ReplaceAll(original.GetClassName(), "/", ".")
}

// Caller holds nativeMembersMu. Only the original whole-owner protocol may
// make a top-level owner's failed finish terminal. A provisional, successful,
// partial, foreign-policy or different-component transaction grants no name.
// No source/private ownership is borrowed from this completed failure.
func nativeMemberCompletedComponentRefusal(entry *nativeMemberCacheEntry, owner string, snapshot map[string]string, work *workbudget.Budget) bool {
	if entry == nil || !entry.dependenciesKnown || !entry.dependenciesCyclic || snapshot == nil || !nativeProofWork(work, 1) {
		return false
	}
	tx := entry.transaction
	if tx == nil || tx.snapshot == nil || tx.independent || !tx.completed || tx.entries != nil || len(tx.owners) == 0 || len(tx.owners) > 64 || len(tx.owners) != len(entry.dependencyParticipants) || len(tx.snapshot) != len(snapshot) {
		return false
	}
	found := false
	for i, name := range tx.owners {
		if name != entry.dependencyParticipants[i] || !nativeProofWork(work, int64(len(name))+1) {
			return false
		}
		found = found || name == owner
	}
	for name, value := range snapshot {
		if !nativeProofWork(work, int64(len(name)+len(value))+1) {
			return false
		}
		if original, present := tx.snapshot[name]; !present || original != value {
			return false
		}
	}
	return found
}
