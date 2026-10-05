package javaclassparser

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// These boundaries are captured while rendering the original owned class.
// The source omits its appended members. Original class identity keys the
// packet, since identical Reader declarations in different lexical owners
// are distinct classes. Anonymous bodies, executable blocks and fields are atoms.
type nativeMemberRegistrationScope struct {
	owner, source string
	members       []string
	memberOwners  []string
	declaration   string
}

type nativeMemberRegistrationTreeNode struct {
	source string
	events []nativeAccessorSourceEvent
	child  *nativeMemberRegistrationTree
}

type nativeMemberRegistrationTree struct {
	header, footer string
	fixed, members []nativeMemberRegistrationTreeNode
	empty          []string
}

func nativeMemberRegistrationTreeLayout(p *nativeMemberFamily, source string, members []string, work *workbudget.Budget, memberOwners []string) (string, bool) {
	if p == nil || p.failed {
		return "", false
	}
	constructors, known := nativeMemberConstructorRegistrations(p, work)
	if !known || len(p.registrationLayouts) > 64 {
		return "", false
	}
	active := map[string]bool{}
	nodes := 0
	var build func(*nativeMemberRegistrationScope, int) (*nativeMemberRegistrationTree, bool)
	build = func(scope *nativeMemberRegistrationScope, depth int) (*nativeMemberRegistrationTree, bool) {
		if scope == nil || depth > 64 || active[scope.owner] || !nativeProofWork(work, 1) {
			return nil, false
		}
		active[scope.owner] = true
		defer delete(active, scope.owner)
		open := javaIndexTopBrace(scope.source)
		close := -1
		if open >= 0 {
			close = javaMatchBrace(scope.source, open)
		}
		if open < 0 || close < 0 || strings.TrimSpace(scope.source[close+1:]) != "" {
			return nil, false
		}
		declarations, ok := nativeMemberLayoutDeclarations(scope.source[open+1 : close])
		if !ok || len(declarations) > 1024 || len(scope.members) > 64 || len(scope.members) != len(scope.memberOwners) {
			return nil, false
		}
		nodes += len(declarations) + len(scope.members)
		if nodes > 1024 || !nativeProofWork(work, int64(len(scope.source))) {
			return nil, false
		}
		tree := &nativeMemberRegistrationTree{header: scope.source[:open+1], footer: scope.source[close:]}
		atom := func(text string) (nativeMemberRegistrationTreeNode, bool) {
			events, ok := nativeMemberAccessorEvents(p, text, constructors, work)
			return nativeMemberRegistrationTreeNode{source: text, events: events}, ok
		}
		for _, text := range declarations {
			n, ok := atom(text)
			if !ok {
				return nil, false
			}
			tree.fixed = append(tree.fixed, n)
		}
		seen := map[string]bool{}
		for i, text := range scope.members {
			name := scope.memberOwners[i]
			member := p.children[name]
			if seen[name] || member == nil || member.owner != scope.owner || member.object == nil || member.object.GetClassName() != name {
				return nil, false
			}
			seen[name] = true
			owner, _, _, owned := originalMemberOwner(member.object)
			if !owned || owner != scope.owner {
				return nil, false
			}
			n, ok := atom(text)
			if !ok {
				return nil, false
			}
			if child := p.registrationLayouts[name]; child != nil {
				if child.owner != name || child.declaration != text {
					return nil, false
				}
				// The captured packet must still describe the complete declaration
				// being moved. A stale packet cannot grant new source boundaries.
				end := javaMatchBrace(child.source, javaIndexTopBrace(child.source))
				if end < 0 || strings.TrimSpace(text) != strings.TrimSpace(child.source[:end]+strings.Join(child.members, "")+child.source[end:]) {
					return nil, false
				}
				n.child, ok = build(child, depth+1)
				if !ok {
					return nil, false
				}
			}
			if len(n.events) == 0 {
				tree.empty = append(tree.empty, n.source)
			} else {
				tree.members = append(tree.members, n)
			}
		}
		return tree, true
	}
	root, ok := build(&nativeMemberRegistrationScope{owner: p.owner, source: source, members: members, memberOwners: memberOwners}, 0)
	if !ok {
		return "", false
	}
	// Every memo key includes the incoming registered-symbol set: nested
	// declarations can arrive with different earlier sibling registrations.
	// Field ordinals are fixed by the original packets and derived from this
	// set; getters retain identity even when they share one accessed symbol.
	var getters []*nativeMemberPrivateGetter
	for _, getter := range p.getters {
		getters = append(getters, getter)
	}
	var ctorKeys []string
	for key := range constructors {
		ctorKeys = append(ctorKeys, key)
	}
	sort.Strings(ctorKeys)
	stateBytes := (len(getters) + len(ctorKeys) + 7) / 8
	originalSize := len(source) + len(strings.Join(members, ""))
	if work != nil && work.CheckAlloc(int64(originalSize)*4+4096*int64(64+stateBytes)+int64(nodes+1)*int64(2*len(getters)+len(ctorKeys)+1)*64) != nil {
		return "", false
	}
	fingerprint := func(order *nativeAccessorOrderState) string {
		bits := make([]byte, stateBytes)
		for i, getter := range getters {
			if order.getters[getter] {
				bits[i/8] |= 1 << uint(i%8)
			}
		}
		for i, key := range ctorKeys {
			if order.constructors[key] {
				at := len(getters) + i
				bits[at/8] |= 1 << uint(at%8)
			}
		}
		return string(bits)
	}
	type key struct {
		cursor   int
		selected uint64
		state    string
	}
	attempts := 0
	exhausted := false
	type continuation func(string, *nativeAccessorOrderState) bool
	var visit func(*nativeMemberRegistrationTree, *nativeAccessorOrderState, continuation) bool
	visit = func(tree *nativeMemberRegistrationTree, initial *nativeAccessorOrderState, done continuation) bool {
		var path []string
		rejected := map[key]bool{}
		var search func(int, uint64, *nativeAccessorOrderState) bool
		search = func(cursor int, selected uint64, order *nativeAccessorOrderState) bool {
			base := len(path)
			defer func() { path = path[:base] }()
			for cursor < len(tree.fixed) && len(tree.fixed[cursor].events) == 0 {
				path = append(path, tree.fixed[cursor].source)
				cursor++
			}
			if cursor == len(tree.fixed) && selected == (uint64(1)<<uint(len(tree.members)))-1 {
				return done(tree.header+strings.Join(path, "")+strings.Join(tree.empty, "")+tree.footer, order)
			}
			k := key{cursor: cursor, selected: selected, state: fingerprint(order)}
			if rejected[k] || exhausted {
				return false
			}
			attempts++
			if attempts > 4096 || !nativeProofWork(work, int64(1+len(getters)+len(ctorKeys))) {
				exhausted = true
				return false
			}
			try := func(n nativeMemberRegistrationTreeNode, next int, bits uint64) bool {
				resume := func(text string, updated *nativeAccessorOrderState) bool {
					at := len(path)
					path = append(path, text)
					passed := search(next, bits, updated)
					path = path[:at]
					return passed
				}
				// Child layout is solved with the actual incoming state. The
				// parent's continuation participates in backtracking; selecting
				// a locally valid child schedule cannot commit the whole family.
				if n.child != nil && len(n.events) != 0 {
					return visit(n.child, order, resume)
				}
				if !nativeProofWork(work, int64(len(n.events)+len(order.getters)+len(order.constructors))) {
					exhausted = true
					return false
				}
				updated := order.clone()
				return updated.apply(n.events) && resume(n.source, updated)
			}
			if cursor < len(tree.fixed) && try(tree.fixed[cursor], cursor+1, selected) {
				return true
			}
			for i, n := range tree.members {
				bit := uint64(1) << uint(i)
				if selected&bit == 0 && try(n, cursor, selected|bit) {
					return true
				}
				if exhausted {
					return false
				}
			}
			rejected[k] = true
			return false
		}
		return search(0, 0, initial)
	}
	var result string
	if !visit(root, newNativeAccessorOrderState(), func(text string, order *nativeAccessorOrderState) bool {
		if len(order.getters) != len(p.getters) || len(order.constructors) != len(constructors) {
			return false
		}
		result = text
		return true
	}) {
		return "", false
	}
	return result, nativeMemberPrivateGetterSourceClosed(p, result, work)
}
