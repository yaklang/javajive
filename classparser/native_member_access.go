package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// Moving a binary member into its original owner restores Java's member
// accessibility. A foreign flattened nested subclass has lost its enclosing
// owner's protected access, even though the original classfile was legal.
// Until those enclosing scopes can be committed together, retain the old
// declaration layout. A direct subclass remains a valid protected caller.
func (z *JarFS) nativeMemberAccessRepresentable(p *nativeMemberFamily, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || index == nil {
		return false
	}
	pkg := func(n string) string {
		if i := strings.LastIndex(n, "/"); i >= 0 {
			return n[:i]
		}
		return ""
	}
	known := map[string]bool{p.owner: true}
	resolved := map[string]bool{p.owner: true}
	subclass := func(user string) bool {
		path := []string{}
		visiting := map[string]bool{}
		node := user
		result := false
		for depth := 0; depth < 128; depth++ {
			if !nativeProofWork(work, 1) {
				break
			}
			if known[node] {
				result = resolved[node]
				break
			}
			if visiting[node] {
				break
			}
			visiting[node] = true
			path = append(path, node)
			raw, found := z.enumSiblingResolver()(node)
			if !found {
				break
			}
			reader := z.nativeMemberReader(nil)
			obj, e := reader.parseResolved(raw)
			if e != nil || obj.GetClassName() != node {
				break
			}
			node = obj.GetSupperClassName()
			if node == "" {
				break
			}
		}
		if work != nil && work.CheckAlloc(int64(len(known)+len(path))*64) != nil {
			return false
		}
		for _, node := range path {
			known[node] = true
			resolved[node] = result
		}
		return result
	}
	for name, child := range p.children {
		if child.flags&1 != 0 {
			continue
		}
		for user := range index.typeUsers[name] {
			if !nativeProofWork(work, 1) {
				return false
			}
			if user == p.owner || p.children[user] != nil || nativeMemberJointAnonymousAccess(p, user, work) {
				continue
			}
			if child.flags&2 != 0 {
				return false
			}
			if pkg(user) == pkg(p.owner) {
				continue
			}
			if child.flags&4 == 0 || !subclass(user) {
				return false
			}
		}
	}
	return true
}

// Access belongs to the lexical declaration being committed, not its old flat
// binary source unit. Only a proved anonymous child of this very owner receives
// that scope. The caller still requires the whole anonymous source/ordinal
// closure before publishing the member family; a foreign or deeper declaration
// has not been moved into this scope and must retain the ordinary access check.
func nativeMemberJointAnonymousAccess(p *nativeMemberFamily, user string, work *workbudget.Budget) bool {
	if p == nil || !nativeProofWork(work, 1) {
		return false
	}
	group := p.anonymousUnits[user]
	if group == nil {
		group = p.anonymous
	}
	if group == nil || group.failed || group.owner != p.owner && p.children[group.owner] == nil {
		return false
	}
	child := group.children[user]
	if child == nil || child.object == nil {
		return false
	}
	identity, known := sourceBridgeClassName(child.object, child.object.ThisClass)
	if !known || identity != user || len(child.object.Attributes) > 65535 || !nativeProofWork(work, int64(len(child.object.Attributes))) {
		return false
	}
	for _, attr := range child.object.Attributes {
		if inner, ok := attr.(*InnerClassesAttribute); ok && inner != nil && (len(inner.Classes) > 65535 || !nativeProofWork(work, int64(len(inner.Classes)))) {
			return false
		}
	}
	owner, method, anonymous := originalAnonymousOwner(child.object)
	return anonymous && owner == group.owner && method == child.method
}
