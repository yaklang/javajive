package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// Family cache entries publish source only after all ownership/binding proofs.
// Before consulting another unfinished-capable entry, discover its original
// source dependencies without consulting the cache. A gray DFS edge means the
// families need a joint transaction; independent source commits cannot prove it.
// Include descriptor, Signature and annotation edges, not just actual invokes.
func (z *JarFS) nativeMemberDependenciesAcyclic(root string, work *workbudget.Budget) bool {
	if z == nil || root == "" || !nativeProofWork(work, 1) {
		return false
	}
	objects := map[string]*ClassObject{}
	var total int64
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
		if work != nil && work.CheckAlloc(int64(len(raw))) != nil {
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
	// Missing external classes are external edges; missing an owned declaration
	// after original InnerClasses/EnclosingMethod establishes ownership is not.
	outermost := func(o *ClassObject) (string, bool) {
		seen := map[string]bool{}
		for len(seen) < 64 {
			name := o.GetClassName()
			if seen[name] || !nativeProofWork(work, 1) {
				return "", false
			}
			seen[name] = true
			parent, _, _, member := originalMemberOwner(o)
			if !member {
				return name, nativeMemberTopLevelEvidence(o, work)
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
			return false
		}
		if states[owner] == 2 {
			return true
		}
		if len(states) >= 64 {
			return false
		}
		states[owner] = 1
		object, known := load(owner)
		if !known || !nativeMemberTopLevelEvidence(object, work) {
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
			names, known := nativeMemberDependencyNames(current, work)
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
		for dependency := range dependencies {
			if !visit(dependency) {
				return false
			}
		}
		states[owner] = 2
		return true
	}
	return visit(root)
}
