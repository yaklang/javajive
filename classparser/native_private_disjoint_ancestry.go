package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An absent full declaration is not a private-access permission. A complete
// ancestry graph can instead prove that no symbol of that owner could resolve
// to any declaration being joined. This proof grants neither accessibility nor
// overload/constructor/effect facts about the external declaration.
func nativePrivatePermissionDisjointAncestry(owner string, source map[string]*ClassObject, resolve func(string) (*ClassObject, bool), parents func(string) ([]string, bool), work *workbudget.Budget) bool {
	if owner == "" || source == nil || resolve == nil || parents == nil {
		return false
	}
	state := map[string]uint8{}
	nodes := 0
	var visit func(string, int) bool
	visit = func(name string, depth int) bool {
		if name == "" || len(name) > 65535 || depth >= 64 || state[name] == 1 || !nativeProofWork(work, int64(len(name))+1) {
			return false
		}
		if _, owned := source[name]; owned {
			return false
		}
		if state[name] == 2 {
			return true
		}
		nodes++
		if nodes > 256 || work != nil && work.CheckAlloc(int64(nodes)*128) != nil {
			return false
		}
		state[name] = 1
		var edges []string
		if object, known := resolve(name); known {
			if object == nil || object.GetClassName() != name || len(object.Interfaces) > 256 || work != nil && work.CheckAlloc(int64(len(object.Interfaces)+1)*128) != nil {
				return false
			}
			if object.SuperClass != 0 {
				parent, valid := sourceBridgeClassName(object, object.SuperClass)
				if !valid {
					return false
				}
				edges = append(edges, parent)
			}
			for _, index := range object.Interfaces {
				parent, valid := sourceBridgeClassName(object, index)
				if !valid {
					return false
				}
				edges = append(edges, parent)
			}
		} else {
			var known bool
			edges, known = parents(name)
			if !known {
				return false
			}
		}
		if len(edges) > 256 || !nativeProofWork(work, int64(len(edges))) || work != nil && work.CheckAlloc(int64(len(edges))*128) != nil {
			return false
		}
		for _, parent := range edges {
			if !visit(parent, depth+1) {
				return false
			}
		}
		state[name] = 2
		return true
	}
	return visit(owner, 0)
}

// Caller declarations shadow the trusted platform. Even malformed supplied
// bytes must not borrow the catalog's ancestry. Exact release ancestry is the
// fallback only when no caller declaration is available at all.
func (c *ClassObjectDumper) nativePrivatePermissionPlatformParents() func(string) ([]string, bool) {
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	return func(name string) ([]string, bool) {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if c.foldSiblingResolver != nil {
			if _, known := c.foldSiblingResolver(name); known {
				return nil, false
			}
		}
		if c.declarationResolver != nil {
			if _, known := c.declarationResolver(name); known {
				return nil, false
			}
		}
		return jdkReferenceSupertypes(name, target)
	}
}
