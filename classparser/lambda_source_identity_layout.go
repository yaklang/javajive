package javaclassparser

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// Ordinary method declarations have no class-initialization execution step.
// Their complete packets can be scheduled around anchored initializers to
// satisfy the compiler's lambda numbering constraints. Fields, initializers,
// statements and constructors retain their relative order. Other protocols need
// a joint registration proof; this layout does not borrow their permissions.
func (c *ClassObjectDumper) native8LambdaSourceIdentityLayout(source string, members []string, methods []*dumpedMethods) (string, bool) {
	events, known := c.native8LambdaSourceIdentityEvents(source, members, methods)
	if !known {
		return source, false
	}
	// An explicit artifact restores proved private helper names after javac.
	// Its mapping uses this actual source-event order, so declaration movement
	// is unnecessary. Plain source still owes exact original compiler names.
	if nativeLambdaSourceIdentityOrderClosed(events) || c.artifactLambdaMetadataAllowed() {
		return source, true
	}
	lookupOnly := c.lambdaRegistrationDeclarationLookupOnly()
	if len(members) != 0 || c.nativeMemberRoot != nil && !lookupOnly || c.nativeAnonymousRoot != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil || c.nativeCaptureFields != nil || c.ordinaryConstructorEmissionSeen || len(c.constructorBoundaryHelpers) != 0 || !nativeProofWork(c.Work, int64(len(source))+int64(len(c.obj.Methods))) {
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
				if !known {
					return source, false
				}
				if owner == c.obj.GetClassName() {
					// A declaration-only lookup can reference named static
					// members without emitting their declarations here. These
					// references allocate no enclosing lambda ordinal. This
					// query grants no member-source ownership/publication role.
					member, memberKnown := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
					name, nameKnown := sourceBridgeUTF8(c.obj, row.InnerNameIndex)
					if !lookupOnly || !memberKnown || !nameKnown || !class_context.PreservesJava8Identifier(name) || member != owner+"$"+name || row.InnerClassAccessFlags&8 == 0 || row.InnerClassAccessFlags & ^uint16(0x061f) != 0 {
						return source, false
					}
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
		anchored    bool
	}
	groups := map[*dumpedMethods]*group{}
	seen := make([]bool, len(events))
	for _, event := range events {
		if event.ordinal < 0 || event.ordinal >= len(events) || seen[event.ordinal] || event.parent == nil {
			return source, false
		}
		seen[event.ordinal] = true
		name, named := sourceBridgeUTF8(c.obj, event.parent.member.NameIndex)
		if !named || name == "<init>" || isSyntheticMethod(event.parent.member.AccessFlags) || name == "<clinit>" && event.parent.member.AccessFlags != 8 {
			return source, false
		}
		current := groups[event.parent]
		if current == nil {
			declaration, _, known := c.lambdaParentDeclaration(source, event.parent)
			if !known {
				return source, false
			}
			current = &group{parent: event.parent, declaration: declaration, first: event.ordinal, node: -1, anchored: name == "<clinit>"}
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
	anchored := false
	for _, current := range ordered {
		anchored = anchored || current.anchored
	}
	if !anchored {
		for i, slot := range slots {
			reordered[slot] = nodes[ordered[i].node]
		}
	} else {
		// Stable merge: only event-bearing ordinary declarations move.
		// Every other node is copied in its original order, byte for byte.
		// A merged initializer whose ordinals straddle an ordinary method
		// cannot pass the contiguous-interval proof above; do not reorder its
		// statements or pretend its original field phases were one interval.
		byNode := map[int]*group{}
		var movable []*group
		for _, current := range ordered {
			byNode[current.node] = current
			if !current.anchored {
				movable = append(movable, current)
			}
		}
		reordered = nil
		cursor, ordinal := 0, 0
		emit := func(current *group) bool {
			if current.first != ordinal {
				return false
			}
			packet := current.declaration
			if current.anchored {
				packet = nodes[current.node]
			}
			reordered = append(reordered, packet)
			ordinal += current.count
			return true
		}
		for i, node := range nodes {
			current := byNode[i]
			if current != nil && !current.anchored {
				// Move only the completed packet. Preserve the node's outer
				// whitespace at its original boundary as part of the fixed
				// sequence. The trimmed equality above excludes detached
				// documentation or any other semantic text in this margin.
				at := strings.Index(node, current.declaration)
				if at < 0 || strings.Index(node[at+len(current.declaration):], current.declaration) >= 0 {
					return source, false
				}
				reordered = append(reordered, node[:at]+node[at+len(current.declaration):])
				continue
			}
			if current == nil {
				reordered = append(reordered, node)
				continue
			}
			for cursor < len(movable) && movable[cursor].first < current.first {
				if !emit(movable[cursor]) {
					return source, false
				}
				cursor++
			}
			if !emit(current) {
				return source, false
			}
		}
		for ; cursor < len(movable); cursor++ {
			if !emit(movable[cursor]) {
				return source, false
			}
		}
	}
	result := source[:open+1] + strings.Join(reordered, "") + source[close:]
	// Reopen every original factory/cache packet against the actual new text.
	// No speculative declaration order is published after a failed check.
	if !c.native8LambdaSourceIdentitiesClosed(result, members, methods) {
		return source, false
	}
	return result, true
}

// The presence of a member-type lookup is not a source registration event.
// An enrolled family, private accessors, constructors, anonymous/local scopes
// and retained helper protocols still need their joint registration proof.
func (c *ClassObjectDumper) lambdaRegistrationDeclarationLookupOnly() bool {
	p := c.nativeMemberRoot
	return p != nil && p.declarationLookupOnly && !p.failed && p.owner == "" && p.independentRoot == nil && p.rootHandleSource == nil && p.anonymous == nil && p.anonymousForest == nil && len(p.getters) == 0 && len(p.retainedAccessors) == 0 && len(p.rootAccessBridges) == 0 && len(p.rootBridgeDelegations) == 0 && len(p.methodLocals) == 0 && len(p.enumConstants) == 0 && len(p.enumSwitchTables) == 0 && len(p.registrationLayouts) == 0 && len(p.anonymousUnits) == 0 && len(p.memberAnonymous) == 0 && len(p.emptyMarkers) == 0
}
