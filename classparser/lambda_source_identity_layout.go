package javaclassparser

import (
	"sort"
	"strings"
)

// Ordinary method declarations have no class-initialization execution step.
// Their complete packets can be scheduled in existing method slots to satisfy
// the compiler's lambda numbering constraints. Fields, initializers, statement
// order and constructor packets stay in place. Other synthesis protocols need
// a joint registration proof; this layout does not borrow their permissions.
func (c *ClassObjectDumper) native8LambdaSourceIdentityLayout(source string, members []string, methods []*dumpedMethods) (string, bool) {
	events, known := c.native8LambdaSourceIdentityEvents(source, members, methods)
	if !known {
		return source, false
	}
	if nativeLambdaSourceIdentityOrderClosed(events) {
		return source, true
	}
	if len(members) != 0 || c.nativeMemberRoot != nil || c.nativeAnonymousRoot != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil || c.nativeCaptureFields != nil || c.ordinaryConstructorEmissionSeen || len(c.constructorBoundaryHelpers) != 0 || !nativeProofWork(c.Work, int64(len(source))+int64(len(c.obj.Methods))) {
		return source, false
	}
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) || isSyntheticMethod(method.AccessFlags) && c.lambdaMethodSourceBodies[method] == nil {
			return source, false
		}
	}
	// Unknown anonymous/compiler-owned rows and owned member declarations may
	// share other javac registration counters. A foreign named type reference,
	// such as MethodHandles.Lookup, does not define a scope in this class.
	for _, attribute := range c.obj.Attributes {
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return source, false
			}
			for _, row := range table.Classes {
				if row == nil || row.OuterClassInfoIndex == 0 || row.InnerNameIndex == 0 || !nativeProofWork(c.Work, 1) {
					return source, false
				}
				owner, known := sourceBridgeClassName(c.obj, row.OuterClassInfoIndex)
				if !known || owner == c.obj.GetClassName() {
					return source, false
				}
			}
		}
	}
	type group struct {
		parent      *dumpedMethods
		declaration string
		first       int
		count       int
		node        int
	}
	groups := map[*dumpedMethods]*group{}
	seen := make([]bool, len(events))
	for _, event := range events {
		if event.ordinal < 0 || event.ordinal >= len(events) || seen[event.ordinal] || event.parent == nil {
			return source, false
		}
		seen[event.ordinal] = true
		name, named := sourceBridgeUTF8(c.obj, event.parent.member.NameIndex)
		if !named || name == "<init>" || name == "<clinit>" || isSyntheticMethod(event.parent.member.AccessFlags) {
			return source, false
		}
		current := groups[event.parent]
		if current == nil {
			declaration, _, known := c.lambdaParentDeclaration(source, event.parent)
			if !known {
				return source, false
			}
			current = &group{parent: event.parent, declaration: declaration, first: event.ordinal, node: -1}
			groups[event.parent] = current
		}
		// Scheduling whole declarations cannot change the order of expressions
		// inside a method. Each method must already own a contiguous interval.
		if event.ordinal != current.first+current.count {
			return source, false
		}
		current.count++
	}
	open := javaIndexTopBrace(source)
	close := -1
	if open >= 0 {
		close = javaMatchBrace(source, open)
	}
	if open < 0 || close < 0 || strings.TrimSpace(source[close+1:]) != "" || c.Work != nil && c.Work.CheckAlloc(int64(len(source))*3+int64(len(events))*256) != nil {
		return source, false
	}
	nodes, known := nativeMemberLayoutDeclarations(source[open+1 : close])
	if !known || len(nodes) > 4096 {
		return source, false
	}
	var slots []int
	var ordered []*group
	var searched int64
	for _, current := range groups {
		for i, node := range nodes {
			cost := int64(len(node)) + 1
			if cost > (128<<20)-searched || !nativeProofWork(c.Work, cost) {
				return source, false
			}
			searched += cost
			if strings.TrimSpace(node) == strings.TrimSpace(current.declaration) {
				if current.node >= 0 {
					return source, false
				}
				current.node = i
			}
		}
		if current.node < 0 {
			return source, false
		}
		slots = append(slots, current.node)
		ordered = append(ordered, current)
	}
	sort.Ints(slots)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].first < ordered[j].first })
	reordered := append([]string(nil), nodes...)
	for i, slot := range slots {
		reordered[slot] = nodes[ordered[i].node]
	}
	result := source[:open+1] + strings.Join(reordered, "") + source[close:]
	// Reopen every original factory/cache packet against the actual new text.
	// No speculative declaration order is published after a failed check.
	if !c.native8LambdaSourceIdentitiesClosed(result, members, methods) {
		return source, false
	}
	return result, true
}
