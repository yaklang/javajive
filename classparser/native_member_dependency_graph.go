package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Family cache entries publish source only after all ownership/binding proofs.
// Before consulting another unfinished-capable entry, discover its original
// source dependencies without consulting the cache. A gray DFS edge means the
// families need a joint transaction; independent source commits cannot prove it.
// Include descriptor, Signature and annotation edges, not just actual invokes.
func (z *JarFS) nativeMemberOriginalDependencyGraph(root string, work *workbudget.Budget) (map[string]map[string]bool, bool) {
	if z == nil || root == "" || !nativeProofWork(work, 1) {
		return nil, false
	}
	graph := map[string]map[string]bool{}
	objects := map[string]*ClassObject{}
	var total int64
	graphEdges := 0
	load := func(name string) (*ClassObject, bool) {
		if o := objects[name]; o != nil {
			return o, true
		}
		if len(objects) >= 4096 || !nativeProofWork(work, 1) {
			return nil, false
		}
		raw, known := z.enumSiblingResolver()(name)
		if !known || len(raw) > 2<<20 || int64(len(raw)) > (128<<20)-total {
			return nil, false
		}
		if work != nil && work.CheckAlloc(total+int64(len(raw))) != nil {
			return nil, false
		}
		o, err := z.nativeMemberReader(nil).parseResolved(raw)
		if err != nil || o.GetClassName() != name {
			return nil, false
		}
		total += int64(len(raw))
		objects[name] = o
		return o, true
	}

	// A named method-local declaration has no OuterClassInfoIndex. Its body is
	// nevertheless emitted in its enclosing source family. Use the same exact
	// EnclosingMethod and reciprocal InnerClasses proof as source planning;
	// neither a binary-name prefix nor an unrelated foreign row creates an edge.
	// Loading stays in this bounded original-byte graph, never a source cache.
	localParent := func(object *ClassObject) (*ClassObject, bool) {
		if object == nil {
			return nil, false
		}
		for _, a := range object.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			raw, ok := a.(*UnparsedAttribute)
			if !ok || raw == nil || raw.Name != "EnclosingMethod" {
				continue
			}
			if len(raw.Info) != 4 {
				return nil, false
			}
			name, known := sourceBridgeClassName(object, binary.BigEndian.Uint16(raw.Info[:2]))
			if !known {
				return nil, false
			}
			parent, known := load(name)
			if !known {
				return nil, false
			}
			if _, known = originalMethodLocalOwner(object, parent, work); !known {
				return nil, false
			}
			return parent, true
		}
		return nil, false
	}

	// An independently certified static source boundary selects its own graph
	// node. Its original children remain in that one lexical family. Ordinary
	// whole-family queries retain their original physical source ownership.
	boundaryProof := map[string]uint8{}
	independentBoundary := func(o *ClassObject) bool {
		if o == nil || !nativeProofWork(work, 1) {
			return false
		}
		name := o.GetClassName()
		if state := boundaryProof[name]; state != 0 {
			return state == 2
		}
		boundaryProof[name] = 1
		parent, _, _, member := originalMemberOwner(o)
		if !member {
			return false
		}
		if _, known := load(parent); !known {
			return false
		}
		d := z.nativeMemberReader(o)
		d.Work = work
		if d.originalNativeMemberIndependentRoot() == nil {
			return false
		}
		// The actual independent entry chooses the outermost static cut. A nearer
		// leaf cannot become a separate graph node while that cut owns it.
		enclosing, known := load(parent)
		if !known {
			return false
		}
		seen := map[string]bool{}
		for {
			outerName := enclosing.GetClassName()
			if len(seen) >= 64 || seen[outerName] || !nativeProofWork(work, 1) {
				return false
			}
			seen[outerName] = true
			next, _, flags, member := originalMemberOwner(enclosing)
			if !member {
				if !nativeMemberTopLevelEvidence(enclosing, work) {
					return false
				}
				break
			}
			if flags&8 != 0 {
				return false
			}
			enclosing, known = load(next)
			if !known {
				return false
			}
		}

		boundaryProof[name] = 2
		return true
	}
	rootObject, rootKnown := load(root)
	if !rootKnown {
		return nil, false
	}
	independentMode := independentBoundary(rootObject)
	// Missing external classes are external edges; missing an owned declaration
	// after original InnerClasses/EnclosingMethod establishes ownership is not.
	outermost := func(o *ClassObject) (string, bool) {
		seen := map[string]bool{}
		for len(seen) < 64 {
			name := o.GetClassName()
			if independentMode && independentBoundary(o) {
				return name, true
			}
			if seen[name] || !nativeProofWork(work, 1) {
				return "", false
			}
			seen[name] = true
			parent, _, _, member := originalMemberOwner(o)
			if !member {
				var anonymous bool
				parent, _, anonymous = originalAnonymousOwner(o)
				if !anonymous {
					enclosing, local := localParent(o)
					if !local {
						return name, nativeMemberTopLevelEvidence(o, work)
					}
					parent = enclosing.GetClassName()
				}
			}
			var known bool
			o, known = load(parent)
			if !known {
				return "", false
			}
		}
		return "", false
	}
	states := map[string]uint8{}
	var visit func(string) bool
	visit = func(owner string) bool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if states[owner] == 1 {
			return true
		}
		if states[owner] == 2 {
			return true
		}
		if len(states) >= nativeMemberDependencyNodeLimit {
			return false
		}
		states[owner] = 1
		object, known := load(owner)
		if !known || !nativeMemberTopLevelEvidence(object, work) && !(independentMode && independentBoundary(object)) {
			return false
		}
		queue := []*ClassObject{object}
		owned := map[string]bool{owner: true}
		for i := 0; i < len(queue); i++ {
			current := queue[i]
			for _, a := range current.Attributes {
				table, ok := a.(*InnerClassesAttribute)
				if !ok {
					continue
				}
				if table == nil {
					return false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(work, 1) {
						return false
					}
					name, known := sourceBridgeClassName(current, row.InnerClassInfoIndex)
					if !known {
						return false
					}
					if row.InnerNameIndex != 0 {
						if row.OuterClassInfoIndex == 0 {
							// Scan every body that source planning may render. In
							// particular, an invocation used only inside a local
							// method need not occur in its enclosing class's CP.
							if _, found := z.enumSiblingResolver()(name); !found {
								continue
							}
							child, known := load(name)
							if !known {
								return false
							}
							if _, local := originalMethodLocalOwner(child, current, work); !local {
								continue
							}
							if owned[name] || len(queue) >= 256 {
								return false
							}
							owned[name] = true
							queue = append(queue, child)
							continue
						}
						parent, known := sourceBridgeClassName(current, row.OuterClassInfoIndex)
						if !known {
							continue
						}
						if parent != current.GetClassName() {
							// A foreign row still identifies an archive-owned member
							// when its declaring class is available. It is not an
							// unknown external edge merely because this reader is
							// outside that family (or is the member itself).
							if _, found := z.enumSiblingResolver()(parent); !found {
								continue
							}
							if _, valid := load(parent); !valid {
								return false
							}
							child, valid := load(name)
							if !valid {
								return false
							}
							actual, _, flags, valid := originalMemberOwner(child)
							if !valid || actual != parent || flags != row.InnerClassAccessFlags {
								return false
							}
							continue
						}
						child, known := load(name)
						if !known {
							return false
						}
						actual, _, flags, valid := originalMemberOwner(child)
						if !valid || actual != parent || flags != row.InnerClassAccessFlags || owned[name] || len(queue) >= 256 {
							return false
						}
						owned[name] = true
						queue = append(queue, child)
					} else {
						// A reference to a different anonymous scope is not an owned child.
						if owned[name] {
							continue
						}
						raw, found := z.enumSiblingResolver()(name)
						if !found {
							continue
						}
						if len(raw) > 2<<20 {
							return false
						}
						child, known := load(name)
						if !known {
							return false
						}
						parent, _, anon := originalAnonymousOwner(child)
						if !anon || parent != current.GetClassName() {
							continue
						}
						if len(queue) >= 256 {
							return false
						}
						owned[name] = true
						queue = append(queue, child)
					}
				}
			}
		}
		dependencies := map[string]bool{}
		for _, current := range queue {
			names, known := nativeMemberSourceBindingNames(current, work)
			if !known {
				return false
			}
			for _, name := range names {
				if !nativeProofWork(work, 1) {
					return false
				}
				if owned[name] {
					continue
				}
				raw, found := z.enumSiblingResolver()(name)
				if !found {
					continue
				}
				if len(raw) > 2<<20 {
					return false
				}
				other, known := load(name)
				if !known {
					return false
				}
				if _, _, _, member := originalMemberOwner(other); !member {
					continue
				}
				target, known := outermost(other)
				if !known {
					return false
				}
				if target != owner {
					dependencies[target] = true
				}
			}
		}

		for _, current := range queue {
			// Only archive declarations define source-cache query targets. Stop at
			// an external boundary and cover every matching archive symbol; do not
			// let an absent or changing external provider invalidate unrelated roots.
			scan, known := nativeMemberOriginalAccessorAncestorScan(current, func(n string) (*ClassObject, bool) {
				if _, present := z.enumSiblingResolver()(n); present {
					// A present but invalid or over-budget archive declaration
					// is not an external boundary. Preserve that hard refusal.
					o, _ := load(n)
					return o, true
				}
				return nil, false
			}, work)
			if !known {
				return false
			}
			addAncestor := func(o *ClassObject) bool {
				target, known := outermost(o)
				if !known {
					return false
				}
				if target != owner {
					dependencies[target] = true
				}
				return true
			}
			for _, m := range scan.matches {
				if !addAncestor(m.object) {
					return false
				}
			}
			if scan.incomplete {
				index := z.originalMemberIndex()
				if index == nil || !index.valid {
					return false
				}
				for symbol := range scan.symbols {
					if !nativeProofWork(work, int64(len(symbol))+1) {
						return false
					}
					for candidate := range index.accessorDeclarations[symbol] {
						if !nativeProofWork(work, 1) {
							return false
						}
						if candidate == current.GetClassName() {
							continue
						}
						o, known := load(candidate)
						if !known {
							return false
						}
						// A valid superclass chain cannot enter a final class or an
						// interface. This excludes impossible candidates without
						// guessing anything about the unavailable external header.
						if o.AccessFlags&(0x0010|0x0200) != 0 {
							continue
						}
						if !addAncestor(o) {
							return false
						}
					}
				}
			}
		}

		// Moving a member declaration also changes the source binding of
		// indexed foreign SUPER callers. This reverse edge joins those callers
		// to the same ownership transaction as the parent; a parent cannot
		// publish a lexical spelling while its child family remains unfinished.
		index := z.originalMemberIndex()
		if !index.valid {
			return false
		}
		for _, current := range queue {
			declaringOwner, _, access, named := originalMemberOwner(current)
			if named && access&4 != 0 {
				// A flattened foreign nested declaration can lose the protected
				// type scope of its enclosing subclass. Restoring this member
				// therefore depends on restoring that caller's lexical family.
				// This edge schedules a joint proof, never grants accessibility:
				// finish still checks every original peer and the complete scope.
				for user := range index.typeUsers[current.GetClassName()] {
					if !nativeProofWork(work, 1) {
						return false
					}
					if owned[user] || nativeBinaryPackage(user) == nativeBinaryPackage(declaringOwner) {
						continue
					}
					caller, known := load(user)
					if !known {
						return false
					}
					used, known := nativeProtectedTypeRequiresSourceAccess(caller, current.GetClassName(), work)
					if !known {
						return false
					}
					if !used || nativeMemberOriginalClassWidening(user, declaringOwner, load, work) {
						continue
					}
					target, known := outermost(caller)
					if !known {
						return false
					}
					if target != owner {
						dependencies[target] = true
					}
				}
			}
			if _, _, flags, member := originalMemberOwner(current); !member || flags&8 != 0 {
				continue
			}
			for user := range index.constructors[current.GetClassName()] {
				caller, known := load(user)
				if !known {
					return false
				}
				if caller.GetSupperClassName() != current.GetClassName() {
					continue
				}
				if _, _, _, member := originalMemberOwner(caller); !member {
					if _, _, anonymous := originalAnonymousOwner(caller); !anonymous {
						if _, local := localParent(caller); !local {
							continue
						}
					}
				}
				target, known := outermost(caller)
				if !known {
					return false
				}
				if target != owner {
					dependencies[target] = true
				}
			}
		}
		graphEdges += len(dependencies)
		if graphEdges > nativeMemberDependencyEdgeLimit || work != nil && work.CheckAlloc(total+int64(len(states))*512+int64(graphEdges)*128) != nil {
			return false
		}
		graph[owner] = dependencies
		for dependency := range dependencies {
			if !visit(dependency) {
				return false
			}
		}
		states[owner] = 2
		return true
	}
	if !visit(root) {
		return nil, false
	}
	return graph, true
}

// The single-family cache still refuses cycles. A source transaction may use
// the same original graph, but must validate and publish every SCC participant.
func (z *JarFS) nativeMemberDependenciesAcyclic(root string, work *workbudget.Budget) bool {
	graph, known := z.nativeMemberOriginalDependencyGraph(root, work)
	if !known {
		return false
	}
	components, known := nativeMemberSourceComponents(graph, work)
	if !known {
		return false
	}
	for owner, component := range components {
		if len(component) != 1 || graph[owner][owner] {
			return false
		}
	}
	return true
}
