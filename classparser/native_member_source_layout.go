package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// A named type declaration has no enclosing-class execution step. It can be
// placed between complete enclosing declarations without moving a field
// initializer or any statement. Registration events constrain those placements
// because legacy javac shares one private-symbol ordinal across the unit.
type nativeMemberLayoutNode struct {
	source string
	events []nativeAccessorSourceEvent
}

// Split only complete top-level declarations, using the existing quote/comment
// lexer. An array/anonymous/lambda initializer ends at its semicolon, not at the
// first closing brace. Annotation-array braces inside parentheses are not bodies.
func nativeMemberLayoutDeclarations(body string) ([]string, bool) {
	var out []string
	state := scanNormal
	depth := 0
	paren := 0
	start := 0
	assignment := false
	blockBody := false
	for i := 0; i < len(body); i++ {
		state = scanAdvance(body, &i, state, &depth)
		if state != scanNormal || i >= len(body) {
			continue
		}
		switch body[i] {
		case '(':
			if depth == 0 {
				paren++
			}
		case ')':
			if depth == 0 {
				paren--
				if paren < 0 {
					return nil, false
				}
			}
		case '=':
			if depth == 0 && paren == 0 {
				assignment = true
			}
		case '{':
			if depth == 0 {
				blockBody = paren == 0 && !assignment
			}
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, false
			}
			if depth == 0 && blockBody {
				out = append(out, body[start:i+1])
				start = i + 1
				assignment = false
				blockBody = false
			}
		case ';':
			if depth == 0 && paren == 0 {
				out = append(out, body[start:i+1])
				start = i + 1
				assignment = false
				blockBody = false
			}
		}
	}
	if depth != 0 || paren != 0 || state != scanNormal && state != scanLineComment {
		return nil, false
	}
	if strings.TrimSpace(body[start:]) != "" {
		return nil, false
	}
	if start < len(body) {
		out = append(out, body[start:])
	}
	return out, true
}

func nativeMemberRegistrationLayout(p *nativeMemberFamily, source string, members []string, work *workbudget.Budget, memberOwners ...[]string) (string, bool) {
	open := javaIndexTopBrace(source)
	close := -1
	if open >= 0 {
		close = javaMatchBrace(source, open)
	}
	if p == nil || p.failed || open < 0 || close < 0 || strings.TrimSpace(source[close+1:]) != "" {
		return "", false
	}
	joined := strings.Join(members, "")
	original := source[:close] + joined + source[close:]
	if len(p.getters) == 0 || nativeMemberPrivateGetterSourceClosed(p, original, work) {
		return original, true
	}
	if len(p.registrationLayouts) != 0 {
		if len(memberOwners) != 1 {
			return "", false
		}
		return nativeMemberRegistrationTreeLayout(p, source, members, work, memberOwners[0])
	}
	declarations, known := nativeMemberLayoutDeclarations(source[open+1 : close])
	// A bounded repair search is optional. The unchanged source certificate is
	// always tried first; oversized or ambiguous layouts retain its refusal.
	if !known || len(declarations) > nativeMemberLayoutNodeLimit || len(members) > nativeMemberLayoutNodeLimit {
		return "", false
	}
	constructors, known := nativeMemberConstructorRegistrations(p, work)
	if !known {
		return "", false
	}
	node := func(text string) (nativeMemberLayoutNode, bool) {
		events, ok := nativeMemberAccessorEvents(p, text, constructors, work)
		return nativeMemberLayoutNode{text, events}, ok
	}
	fixed := make([]nativeMemberLayoutNode, 0, len(declarations))
	for _, s := range declarations {
		n, ok := node(s)
		if !ok {
			return "", false
		}
		fixed = append(fixed, n)
	}
	var movable []nativeMemberLayoutNode
	var empty []string
	for _, s := range members {
		n, ok := node(s)
		if !ok {
			return "", false
		}
		if len(n.events) == 0 {
			empty = append(empty, s)
		} else {
			movable = append(movable, n)
		}
	}
	if len(movable) == 0 || !nativeProofWork(work, int64(len(fixed)+len(movable))) {
		return "", false
	}
	if work != nil && work.CheckAlloc(
		int64(len(original))*3+4096*int64(48+(len(movable)+7)/8)+
			int64(len(fixed)+len(movable)+1)*int64(2*len(p.getters)+len(constructors))*64) != nil {
		return "", false
	}
	type key struct {
		cursor   int
		selected string
	}
	rejected := map[key]bool{}
	var path []nativeMemberLayoutNode
	attempts := 0
	exhausted := false
	var search func(int, string, int, *nativeAccessorOrderState) bool
	search = func(cursor int, selected string, selectedCount int, order *nativeAccessorOrderState) bool {
		// Event-free fixed declarations cannot unlock a different registration state.
		// Consume them without branching and keep every enclosing declaration ordered.
		base := len(path)
		for cursor < len(fixed) && len(fixed[cursor].events) == 0 {
			path = append(path, fixed[cursor])
			cursor++
		}
		restore := func() { path = path[:base] }
		if cursor == len(fixed) && selectedCount == len(movable) {
			if len(order.getters) == len(p.getters) && len(order.constructors) == len(constructors) {
				return true
			}
			restore()
			return false
		}
		k := key{cursor, selected}
		if rejected[k] {
			restore()
			return false
		}
		attempts++
		if attempts > 4096 || !nativeProofWork(work, 1) {
			exhausted = true
			restore()
			return false
		}
		try := func(n nativeMemberLayoutNode, next int, bits string, count int) bool {
			if !nativeProofWork(work, int64(len(n.events)+len(order.getters)+len(order.constructors))) {
				exhausted = true
				return false
			}
			copy := order.clone()
			if !copy.apply(n.events) {
				return false
			}
			at := len(path)
			path = append(path, n)
			if search(next, bits, count, copy) {
				return true
			}
			path = path[:at]
			return false
		}
		// Existing declaration order is preferred. Memoization is valid because the
		// selected nodes determine the symbol set; every admitted field has its fixed
		// original ordinal, independent of the path used to reach that set.
		if cursor < len(fixed) && try(fixed[cursor], cursor+1, selected, selectedCount) {
			return true
		}
		for i, n := range movable {
			if !nativeMemberSelectionContains(selected, i) && try(n, cursor, nativeMemberSelectionAdd(selected, i), selectedCount+1) {
				return true
			}
			if exhausted {
				restore()
				return false
			}
		}
		rejected[k] = true
		restore()
		return false
	}
	if !search(0, nativeMemberSelectionEmpty(len(movable)), 0, newNativeAccessorOrderState()) {
		return "", false
	}
	var out strings.Builder
	out.Grow(len(original))
	out.WriteString(source[:open+1])
	for _, n := range path {
		out.WriteString(n.source)
	}
	for _, s := range empty {
		out.WriteString(s)
	}
	out.WriteString(source[close:])
	result := out.String()
	// Revalidate against original packets after layout; search never grants
	// permission to manufacture a marker or relax the final source certificate.
	return result, nativeMemberPrivateGetterSourceClosed(p, result, work)
}
