package javaclassparser

import "strings"

// Type substitution uses the same original named ownership proof as source
// layout. A dollar-spelled name or a Signature alone cannot grant outer scope.
func (c *ClassObjectDumper) nativeMemberTypeOwners(p *nativeMemberFamily) func(string) ([]string, bool) {
	return func(internal string) ([]string, bool) {
		internal = strings.ReplaceAll(internal, ".", "/")
		if p == nil || p.failed || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		child := p.children[internal]
		if child == nil && c.nativeMemberLookup != nil {
			child = c.nativeMemberLookup(internal)
		}
		if child == nil || child.static {
			return nil, false
		}
		path := []string{internal}
		seen := map[string]bool{internal: true}
		for child != nil {
			if child.object == nil || child.object.GetClassName() != path[len(path)-1] {
				return nil, false
			}
			originalOwner, originalName, flags, member := originalMemberOwner(child.object)
			if !member || originalOwner != child.owner || originalName != child.name || (flags&8 != 0) != child.static {
				return nil, false
			}
			if child.static {
				break
			}
			owner := child.owner
			if len(path) >= 64 || owner == "" || seen[owner] || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			seen[owner] = true
			path = append(path, owner)
			child = p.children[owner]
			if child == nil && c.nativeMemberLookup != nil {
				child = c.nativeMemberLookup(owner)
			}
			if child == nil {
				obj := p.lexicalObjects[owner]
				if obj == nil || obj.GetClassName() != owner {
					return nil, false
				}
				if _, _, flags, member := originalMemberOwner(obj); member && flags&8 == 0 {
					return nil, false
				}
			}
		}
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		return path, true
	}
}
