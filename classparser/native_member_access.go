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
			if user == p.owner || p.children[user] != nil {
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
