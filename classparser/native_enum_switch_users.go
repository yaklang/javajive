package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeEnumSwitchUse struct {
	field                     string
	marker                    string
	getPC, ordinalPC, arrayPC int
	parameterSlot             int
	selector                  *nativeEnumSelectorProducer
	keys                      map[int]bool
	rendered                  bool
}

// A table is a compiler artifact only if all its original archive users are
// table reads consumed by proved switches. Escapes, writes, class literals,
// constructor markers, handles and foreign lexical units are not table uses.
// Computed selectors require an original ordered stack-producer certificate;
// an equal enum result type or source spelling cannot license the rewrite.
func (z *JarFS) nativeEnumSwitchUsersClosed(p *nativeMemberFamily, root *ClassObject, index *nativeMemberIndex, work *workbudget.Budget) bool {
	return z.nativeEnumSwitchUsers(p, root, index, work, false)
}

// Source admission can use a distinct typed-storage certificate only when the
// complete source transaction will discharge every retained parameter writer.
// The standalone entry-value proof must still reject a clobbered parameter.
func (z *JarFS) nativeEnumSwitchSourceUsersClosed(p *nativeMemberFamily, root *ClassObject, index *nativeMemberIndex, work *workbudget.Budget) bool {
	return z.nativeEnumSwitchUsers(p, root, index, work, true)
}

func (z *JarFS) nativeEnumSwitchUsers(p *nativeMemberFamily, root *ClassObject, index *nativeMemberIndex, work *workbudget.Budget, sourceStorage bool) bool {
	if p != nil {
		for _, table := range p.enumSwitchTables {
			if table != nil {
				table.usersClosedRoot = nil
			}
		}
	}
	if p == nil || root == nil {
		return false
	}
	if len(p.enumSwitchTables) == 0 {
		return true
	}
	if index == nil || !index.valid {
		return false
	}
	objects, known := nativeMemberDependencyObjects(root, p, work)
	if !known {
		return false
	}
	foreignChecked := map[string]bool{}
	own := map[string]*ClassObject{}
	for _, o := range objects {
		own[o.GetClassName()] = o
	}
	for name, table := range p.enumSwitchTables {
		if index.handles[name] {
			return false
		}
		for user := range index.typeUsers[name] {
			if user != name && own[user] == nil {
				return false
			}
		}
		// Every table shares this helper's initialization boundary. Final source
		// admission separately proves their complete physical initialization order.
		if !nativeEnumSwitchInitializationClosed(table, work) {
			return false
		}
		for _, arr := range table.tables {
			// Modern javac bypasses the helper for a same-unit enum. Native
			// javac8 regenerates it, but only the explicitly selected compiler
			// profile can promise that lowering. This does not replace the
			// original enum, complete table, user or source-registration proofs.
			if enum := own[arr.enum]; enum != nil && !z.nativeEnumSwitchOwnUnitCompiler(root, enum, table, work) {
				return false
			}
			raw, found := z.enumSiblingResolver()(arr.enum)
			if !found {
				return false
			}
			enum, e := z.nativeMemberReader(root).parseResolved(raw)
			if e != nil || enum.GetClassName() != arr.enum {
				return false
			}
			if original := own[arr.enum]; original != nil {
				// A same-unit declaration can have constant-specific subclasses
				// and abstract methods. Its original member flags, not a final
				// enum template, determine the compiler's synthesis protocol.
				// Reprove the emitted packet; a cached plan cannot license changes
				// to its factories, declaration or constant-body constructors.
				// A canonical enum may itself be the top-level source owner.
				// Preserve that original protocol; only the distinct member/body
				// protocol needs a proved enclosing declaration and siblings.
				if nativeMemberEnumSynthesisProof(original, 0x4019, work) == nil && !z.nativeEnumSwitchOwnedDeclaration(p, original, own, work) {
					return false
				}
				enum = original
			} else if nativeMemberEnumSynthesisProof(enum, 0x4019, work) == nil {
				return false
			}
			constants := map[string]bool{}
			for _, f := range enum.Fields {
				if f.AccessFlags&0x4000 != 0 {
					n, ok := sourceBridgeUTF8(enum, f.NameIndex)
					if !ok {
						return false
					}
					constants[n] = true
				}
			}
			for _, n := range arr.entries {
				if !constants[n] {
					return false
				}
			}
		}
		table.uses = map[string]map[string]map[int]*nativeEnumSwitchUse{}
		count := 0
		for _, object := range own {
			reader := z.nativeMemberReader(object)
			reader.Work = work
			registered, known := reader.nativeEnumSwitchObjectUses(p, table, name, own, foreignChecked, sourceStorage)
			if !known {
				return false
			}
			count += registered
		}
		if count == 0 {
			return false
		}
	}
	for _, table := range p.enumSwitchTables {
		table.usersClosedRoot = root
	}
	return true
}

func (z *JarFS) nativeEnumSwitchOwnedDeclaration(p *nativeMemberFamily, enum *ClassObject, own map[string]*ClassObject, work *workbudget.Budget) bool {
	if z == nil || p == nil || p.failed || enum == nil || own[p.owner] == nil || !nativeProofWork(work, 1) {
		return false
	}
	member := p.children[enum.GetClassName()]
	owner, name, flags, known := originalMemberOwner(enum)
	if !known || member == nil || member.object != enum || member.enumSynthesis == nil || member.owner != owner || member.name != name || member.flags != flags || own[enum.GetClassName()] != enum || own[owner] == nil || own[owner].GetClassName() != owner {
		return false
	}
	reader := z.nativeMemberReader(enum)
	reader.Work = work
	declarations := reader.nativeAnnotationDeclarationResolver()
	resolve := func(binary string) (*ClassObject, bool) {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if object, present := own[binary]; present {
			return object, object != nil && object.GetClassName() == binary
		}
		return declarations(binary)
	}
	proof := nativeMemberEnumSynthesisWithDeclarations(enum, flags, resolve, work)
	if proof == nil {
		return false
	}
	for binary, body := range proof.bodies {
		original := p.enumConstants[binary]
		if !nativeProofWork(work, 1) || body == nil || original == nil || body.object != original.object || own[binary] != body.object {
			return false
		}
	}
	return true
}

func (z *JarFS) nativeEnumSwitchOwnUnitCompiler(root, enum *ClassObject, table *nativeEnumSwitchTable, work *workbudget.Budget) bool {
	if z == nil || root == nil || enum == nil || table == nil || table.object == nil || !nativeProofWork(work, 1) || z.sourceCompiler != NativeJavac8 || z.targetSourceVersion != 8 {
		return false
	}
	// Independently compiled Java7/8 originals share this native lowering.
	// Mixed versions and preview/unknown variants have no regeneration proof.
	if (root.MajorVersion != 51 && root.MajorVersion != 52) || root.MinorVersion != 0 || enum.MajorVersion != root.MajorVersion || enum.MinorVersion != 0 || table.object.MajorVersion != root.MajorVersion || table.object.MinorVersion != 0 {
		return false
	}
	return enum.AccessFlags&0x4000 != 0
}
