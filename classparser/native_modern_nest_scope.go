package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Version 55 adds nest-based private access. A syntax profile is deliberately
// separate from access permission: publication requires the entire reciprocal
// nest and its original lexical ownership in one source transaction.
func nativeModernNestVersion(object *ClassObject, work *workbudget.Budget) bool {
	if object == nil || object.MajorVersion != 55 || object.MinorVersion != 0 {
		return false
	}
	own, known := sourceBridgeClassName(object, object.ThisClass)
	if !known || !nativeSourceBinaryName(own) {
		return false
	}
	host, members := false, false
	for _, attr := range object.Attributes {
		raw, ok := attr.(*UnparsedAttribute)
		if !ok || raw == nil || (raw.Name != "NestHost" && raw.Name != "NestMembers") {
			continue
		}
		if !nativeProofWork(work, int64(len(raw.Info)+1)) || raw.Length != uint32(len(raw.Info)) {
			return false
		}
		if raw.Name == "NestHost" {
			if host || members {
				return false
			}
			host = true
		} else {
			if host || members {
				return false
			}
			members = true
		}
		names, known := originalNestAttribute(object, raw.Name)
		if !known || len(names) == 0 {
			return false
		}
		for _, name := range names {
			if name == own || !nativeSourceBinaryName(name) || nestPackage(name) != nestPackage(own) {
				return false
			}
		}
	}
	return host != members
}

// Read actual input bytes; version edits, dollar spelling, or a one-way host
// assertion do not grant private access. Named, method-local and anonymous
// declarations each need their own original bounded lexical ownership proof.
func (c *ClassObjectDumper) nativeModernNestOriginalScope() (map[string]*ClassObject, bool) {
	if c.obj == nil {
		return nil, false
	}
	if c.obj.MajorVersion < 55 {
		return nil, true
	}
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = int(c.obj.MajorVersion) - 44
	}
	if target < 11 || !nativeAccessorVersion(c.obj, c.Work) || !nativeMemberTopLevelEvidence(c.obj, c.Work) || hasOriginalNestAttribute(c.obj, "NestHost") || c.foldSiblingResolver == nil {
		return nil, false
	}
	host := c.obj.GetClassName()
	names, known := originalNestAttribute(c.obj, "NestMembers")
	if !known || len(names) == 0 || len(names) > 128 {
		return nil, false
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(names)+1)*512) != nil {
		return nil, false
	}
	objects := map[string]*ClassObject{host: c.obj}
	for _, name := range names {
		if objects[name] != nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		raw, known := c.foldSiblingResolver(name)
		if !known {
			return nil, false
		}
		object, err := c.parseResolved(raw)
		if err != nil || object.GetClassName() != name || !nativeAccessorVersion(object, c.Work) || hasOriginalNestAttribute(object, "NestMembers") {
			return nil, false
		}
		enclosing, known := originalNestAttribute(object, "NestHost")
		if !known || len(enclosing) != 1 || enclosing[0] != host {
			return nil, false
		}
		objects[name] = object
	}
	for _, name := range names {
		seen := map[string]bool{}
		for current := name; current != host; {
			if len(seen) >= 64 || seen[current] || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			seen[current] = true
			object := objects[current]
			if object == nil {
				return nil, false
			}
			owner, _, _, known := originalMemberOwner(object)
			if !known {
				owner, _, known = originalAnonymousOwner(object)
			}
			if !known {
				owner, known = nativeModernNestMethodLocalOwner(object, objects, c.Work)
			}
			if !known || owner == current || objects[owner] == nil {
				return nil, false
			}
			current = owner
		}
	}
	return objects, true
}

// EnclosingMethod's class index locates a candidate, not an ownership token.
// The local-role proof must also corroborate the exact original declaration,
// descriptor, self row and the parent's reciprocal registration row.
func nativeModernNestMethodLocalOwner(object *ClassObject, objects map[string]*ClassObject, work *workbudget.Budget) (string, bool) {
	if object == nil {
		return "", false
	}
	for _, attribute := range object.Attributes {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		raw, ok := attribute.(*UnparsedAttribute)
		if !ok || raw == nil || raw.Name != "EnclosingMethod" {
			continue
		}
		if raw.Length != 4 || len(raw.Info) != 4 {
			return "", false
		}
		name, known := sourceBridgeClassName(object, binary.BigEndian.Uint16(raw.Info))
		if !known || objects[name] == nil {
			return "", false
		}
		owner, known := originalMethodLocalOwner(object, objects[name], work)
		if !known || owner.owner != name {
			return "", false
		}
		return name, true
	}
	return "", false
}

// Every original nest member must be present in the committed source scopes.
// A separately retained flat declaration cannot borrow a reconstructed host's
// private permission, even if it has the same package or binary-name prefix.
func nativeModernNestSourceScopeClosed(original, source map[string]*ClassObject, work *workbudget.Budget) bool {
	if original == nil {
		return true
	}
	if len(original) < 2 || len(original) > 129 || source == nil {
		return false
	}
	host := ""
	for name, object := range original {
		if !nativeProofWork(work, 1) || object == nil {
			return false
		}
		if hasOriginalNestAttribute(object, "NestMembers") {
			if host != "" {
				return false
			}
			host = name
		}
	}
	if host == "" {
		return false
	}
	for name := range original {
		object := source[name]
		if object == nil || object.GetClassName() != name || !nativeModernNestVersion(object, work) {
			return false
		}
		if name == host {
			members, known := originalNestAttribute(object, "NestMembers")
			if !known || len(members) != len(original)-1 {
				return false
			}
			for _, member := range members {
				if original[member] == nil {
					return false
				}
			}
		} else {
			names, known := originalNestAttribute(object, "NestHost")
			if !known || len(names) != 1 || names[0] != host {
				return false
			}
		}
	}
	for name, object := range source {
		if !nativeProofWork(work, 1) || object == nil {
			return false
		}
		if names, known := originalNestAttribute(object, "NestHost"); known && names[0] == host && original[name] == nil {
			return false
		}
	}
	return true
}

// An original private constructor is directly source-accessible only inside
// the same completely committed modern nest. Legacy access markers still use
// their independent original descriptor/delegation certificates.
func nativeModernNestPrivateConstructorAccess(p *nativeMemberFamily, caller, target *ClassObject, work *workbudget.Budget) bool {
	if p == nil || p.failed || caller == nil || target == nil || p.modernNestObjects == nil || p.modernNestObjects[p.owner] != p.lexicalObjects[p.owner] || p.modernNestObjects[caller.GetClassName()] == nil || p.modernNestObjects[target.GetClassName()] == nil {
		return false
	}
	if work != nil && work.CheckAlloc(int64(len(p.lexicalObjects)+len(p.anonymousUnits))*32) != nil {
		return false
	}
	source := map[string]*ClassObject{}
	for name, object := range p.lexicalObjects {
		source[name] = object
	}
	for name, group := range p.anonymousUnits {
		if group == nil || group.children[name] == nil || group.children[name].object == nil {
			return false
		}
		source[name] = group.children[name].object
	}
	if source[caller.GetClassName()] != caller || source[target.GetClassName()] != target {
		return false
	}
	return nativeModernNestSourceScopeClosed(p.modernNestObjects, source, work)
}
