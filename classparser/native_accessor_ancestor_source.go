package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeAccessorAncestorMethod struct {
	object           *ClassObject
	method           *MemberInfo
	name, descriptor string
}

// The same synthetic access$ domain used by getter planning bounds regenerated accessor
// symbols. The complete superclass chain influences javac source conflicts even
// when its declaring class is absent from the child's direct constant pool.
type nativeAccessorAncestorScan struct {
	matches    []nativeAccessorAncestorMethod
	symbols    map[string]bool
	incomplete bool
}

// An incomplete scan can schedule potential cache dependencies, but cannot
// certify a source declaration's regenerated flags. Keep that boundary here
// so no source publication caller accidentally accepts a partial hierarchy.
func nativeMemberOriginalAccessorAncestors(object *ClassObject, load func(string) (*ClassObject, bool), work *workbudget.Budget) ([]nativeAccessorAncestorMethod, bool) {
	s, known := nativeMemberOriginalAccessorAncestorScan(object, load, work)
	if !known || s.incomplete {
		return nil, false
	}
	return s.matches, true
}
func nativeMemberOriginalAccessorAncestorScan(object *ClassObject, load func(string) (*ClassObject, bool), work *workbudget.Budget) (nativeAccessorAncestorScan, bool) {
	if object == nil || load == nil || !nativeProofWork(work, 1) {
		return nativeAccessorAncestorScan{}, false
	}
	if work != nil && work.CheckAlloc(64*128) != nil {
		return nativeAccessorAncestorScan{}, false
	}
	keys := map[string]bool{}
	var bytes int64
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nativeAccessorAncestorScan{}, false
		}
		if m.AccessFlags&0x1008 != 0x1008 {
			continue
		}
		name, nk := sourceBridgeUTF8(object, m.NameIndex)
		if !nk {
			return nativeAccessorAncestorScan{}, false
		}
		if !strings.HasPrefix(name, "access$") {
			continue
		}
		desc, dk := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nk || !dk || len(name) > 65535 || len(desc) > 65535 || !nativeProofWork(work, int64(len(name)+len(desc))+1) {
			return nativeAccessorAncestorScan{}, false
		}
		if work != nil && work.CheckAlloc(bytes+int64(len(desc))*32+128) != nil {
			return nativeAccessorAncestorScan{}, false
		}
		if _, _, e := callbinding.Descriptor(desc); e != nil {
			return nativeAccessorAncestorScan{}, false
		}
		bytes += int64(len(name)+len(desc)) + 64
		if work != nil && work.CheckAlloc(bytes) != nil {
			return nativeAccessorAncestorScan{}, false
		}
		keys[name+"\x00"+desc] = true
	}
	result := nativeAccessorAncestorScan{symbols: keys}
	if len(keys) == 0 {
		return result, true
	}
	current := object
	seen := map[string]bool{}
	var matches []nativeAccessorAncestorMethod
	for depth := 0; depth < 64; depth++ {
		name := current.GetClassName()
		if name == "" || seen[name] || !nativeProofWork(work, 1) {
			return nativeAccessorAncestorScan{}, false
		}
		seen[name] = true
		parent := current.GetSupperClassName()
		if parent == "java/lang/Object" {
			result.matches = matches
			return result, true
		}
		if parent == "" {
			return nativeAccessorAncestorScan{}, false
		}
		next, known := load(parent)
		if !known {
			result.matches = matches
			result.incomplete = true
			return result, true
		}
		if next == nil || next.GetClassName() != parent {
			return nativeAccessorAncestorScan{}, false
		}
		current = next
		for _, m := range current.Methods {
			if m == nil || !nativeProofWork(work, 1) {
				return nativeAccessorAncestorScan{}, false
			}
			n, nk := sourceBridgeUTF8(current, m.NameIndex)
			d, dk := sourceBridgeUTF8(current, m.DescriptorIndex)
			if !nk || !dk || len(n) > 65535 || len(d) > 65535 || !nativeProofWork(work, int64(len(n)+len(d))+1) {
				return nativeAccessorAncestorScan{}, false
			}
			if work != nil && work.CheckAlloc(bytes+int64(len(n)+len(d))*8+128) != nil {
				return nativeAccessorAncestorScan{}, false
			}
			if !keys[n+"\x00"+d] {
				continue
			}
			if work != nil && work.CheckAlloc(bytes+int64(len(d))*32+128) != nil {
				return nativeAccessorAncestorScan{}, false
			}
			if _, _, e := callbinding.Descriptor(d); e != nil {
				return nativeAccessorAncestorScan{}, false
			}
			// javac excludes bridges from synthetic-name conflicts. Source
			// emission omits their original declarations: a regenerated bridge
			// keeps ACC_BRIDGE, and an omitted bridge contributes no symbol.
			// This is flag-conflict evidence, not a bridge-body equivalence proof.
			if m.AccessFlags&0x0040 != 0 {
				continue
			}
			bytes += 128
			if work != nil && work.CheckAlloc(bytes) != nil {
				return nativeAccessorAncestorScan{}, false
			}
			matches = append(matches, nativeAccessorAncestorMethod{current, m, n, d})
		}
	}
	return nativeAccessorAncestorScan{}, false
}

// Source flags are publication facts. A peer plan contributes its conditional
// regenerated symbols inside one transaction; a completed foreign family
// contributes its actual getter choices. A flat archive class retains ordinary
// methods. External binary declarations retain their original access flags.
func (z *JarFS) nativeMemberAccessorAncestorFlagsClosed(prepared *nativeMemberPrepared, dependencyGraphClosed bool, peers ...map[string]*nativeMemberPrepared) bool {
	if z == nil || prepared == nil || prepared.reader == nil || prepared.family == nil || prepared.family.failed || prepared.root == nil || prepared.snapshot == nil || prepared.family.owner != prepared.root.GetClassName() || prepared.snapshot["JDEC_NATIVE_MEMBER_OFF"] != "" {
		return false
	}
	p, d := prepared.family, prepared.reader
	if len(p.getters) == 0 {
		return true
	}
	if dependencyGraphClosed {
		if len(peers) != 1 || peers[0][p.owner] != prepared {
			return false
		}
	} else {
		if len(peers) != 0 {
			return false
		}
		_, cyclic, known := z.nativeMemberDependencyAdmission(p.owner, d.Work, prepared.snapshot)
		if !known || cyclic {
			return false
		}
	}
	for _, component := range peers {
		for _, peer := range component {
			if peer == nil || peer.family == nil || peer.family.failed || peer.snapshot == nil || len(peer.snapshot) != len(prepared.snapshot) || !nativeProofWork(d.Work, int64(len(prepared.snapshot))+1) {
				return false
			}
			for key, value := range prepared.snapshot {
				other, known := peer.snapshot[key]
				if !known || other != value {
					return false
				}
			}
		}
	}
	checked := map[string]bool{}
	resolve := d.nativeAnnotationDeclarationResolver()
	familyFlag := func(f *nativeMemberFamily, name, method, desc string) (bool, bool) {
		if f == nil || f.failed || f.lexicalObjects[name] == nil || f.lexicalObjects[name].GetClassName() != name {
			return false, false
		}
		key := nativeMemberGetterKey(name, method, desc)
		g := f.getters[key]
		if g != nil && (g.owner != name || g.name != method || g.descriptor != desc || f.retainedAccessors[key] != nil) {
			return false, false
		}
		return g != nil, true
	}
	for _, getter := range p.getters {
		if getter == nil || !nativeProofWork(d.Work, 1) {
			return false
		}
		if checked[getter.owner] {
			continue
		}
		if d.Work != nil && d.Work.CheckAlloc(int64(len(checked)+1)*128+int64(len(getter.owner))) != nil {
			return false
		}
		checked[getter.owner] = true
		object := p.lexicalObjects[getter.owner]
		if object == nil || object.GetClassName() != getter.owner {
			return false
		}
		matches, known := nativeMemberOriginalAccessorAncestors(object, resolve, d.Work)
		if !known {
			return false
		}
		for _, match := range matches {
			if p.getters[nativeMemberGetterKey(getter.owner, match.name, match.descriptor)] == nil {
				continue
			}
			if synthetic, known := familyFlag(p, match.object.GetClassName(), match.name, match.descriptor); known {
				if !synthetic {
					return false
				}
				continue
			}
			handled := false
			for _, component := range peers {
				for _, peer := range component {
					if synthetic, known := familyFlag(peer.family, match.object.GetClassName(), match.name, match.descriptor); known {
						if !synthetic {
							return false
						}
						handled = true
						break
					}
				}
				if handled {
					break
				}
			}
			if handled {
				continue
			}
			if _, archive := z.enumSiblingResolver()(match.object.GetClassName()); !archive {
				if match.method.AccessFlags&0x1000 == 0 {
					return false
				}
				continue
			}
			sourceOwner, known := z.nativeMemberOriginalSourceOwner(match.object)
			if !known {
				return false
			}
			if sourceOwner == p.owner {
				return false
			}
			for _, component := range peers {
				if component[sourceOwner] != nil {
					return false
				}
			}
			var entry *nativeMemberCacheEntry
			jdecenv.Run(prepared.snapshot, func() error {
				entry = z.nativeMemberEntry(match.object)
				if entry == nil || entry.family == nil {
					entry = z.nativeMemberTransactionEntry(match.object)
				}
				if entry == nil || entry.family == nil {
					entry = z.nativeMemberIndependentEntry(match.object)
				}
				return nil
			})
			if entry == nil || entry.family == nil || entry.source == "" {
				return false
			}
			synthetic, known := familyFlag(entry.family, match.object.GetClassName(), match.name, match.descriptor)
			if !known || !synthetic {
				return false
			}
		}
	}
	return true
}
