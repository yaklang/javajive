package javaclassparser

import (
	"fmt"
	"slices"
	"strings"
)

// An anonymous declaration has its own LambdaToMethod counter. This receipt
// records that class boundary only after its typed allocation renderer has
// completed the child body and its independent lambda/capture checks. It
// grants no access, capture, body or metadata permission to either class.
type lambdaAnonymousSourceScope struct {
	owner       *ClassObject
	group       *nativeAnonymousFamily
	child       *nativeAnonymousClass
	allocations []string
}

func (c *ClassObjectDumper) recordLambdaAnonymousScope(group *nativeAnonymousFamily, child *nativeAnonymousClass, allocation string) bool {
	if c == nil || c.obj == nil || group == nil || child == nil || child.object == nil || group.failed || group.owner != c.obj.GetClassName() || group.children[child.object.GetClassName()] != child || !strings.HasPrefix(allocation, "new ") || len(allocation) <= 4 || len(allocation) > 8192 || !nativeProofWork(c.Work, int64(len(allocation))+1) {
		return false
	}
	if c.lambdaAnonymousScopes == nil {
		c.lambdaAnonymousScopes = map[*nativeAnonymousClass]*lambdaAnonymousSourceScope{}
	}
	prior := c.lambdaAnonymousScopes[child]
	if prior != nil && (prior.owner != c.obj || prior.group != group || prior.child != child) {
		return false
	}
	if prior != nil && slices.Contains(prior.allocations, allocation) {
		return true
	}
	if prior == nil && len(c.lambdaAnonymousScopes) >= 64 || prior != nil && len(prior.allocations) >= 8 || c.Work != nil && c.Work.CheckAlloc(int64(len(allocation))+256) != nil {
		return false
	}
	receipt := &lambdaAnonymousSourceScope{owner: c.obj, group: group, child: child}
	if prior != nil {
		receipt.allocations = slices.Clone(prior.allocations)
	}
	receipt.allocations = append(receipt.allocations, allocation)
	c.lambdaAnonymousScopes[child] = receipt
	return true
}

// Coordinates still refer to the original, unmasked text. Constructor
// arguments remain in the outer class; only the actual allocated class body
// is excluded. Quotes/comments cannot invent a boundary, nor can a matching
// comment authorize an unrecorded allocation or an ordinary executable block.
func (c *ClassObjectDumper) lambdaOwnImplementationTokens(source string, references bool) ([]int, bool) {
	if c == nil {
		return nil, false
	}
	tokens, known := nativeLambdaSourceImplementationTokens(source, c.Work, references)
	if !known || len(c.lambdaAnonymousScopes) == 0 {
		return tokens, known
	}
	if c.obj == nil || len(c.lambdaAnonymousScopes) > 64 || !nativeProofWork(c.Work, int64(len(source))+int64(len(c.lambdaAnonymousScopes))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(c.lambdaAnonymousScopes))*512+int64(len(tokens))*8) != nil {
		return nil, false
	}
	byMarker := map[string]*lambdaAnonymousSourceScope{}
	allocations := map[string]bool{}
	for child, receipt := range c.lambdaAnonymousScopes {
		if receipt == nil || receipt.owner != c.obj || receipt.child != child || child == nil || child.object == nil || receipt.group == nil || receipt.group.failed || receipt.group.owner != c.obj.GetClassName() || receipt.group.children[child.object.GetClassName()] != child || child.ordinal <= 0 || len(receipt.allocations) == 0 || len(receipt.allocations) > 8 || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		owner, method, owned := originalAnonymousOwner(child.object)
		if !owned || owner != receipt.group.owner || method != child.method {
			return nil, false
		}
		marker := fmt.Sprintf("/*jdec-owned-anonymous-ordinal:%d:%s*/", child.ordinal, owner)
		if byMarker[marker] != nil {
			return nil, false
		}
		byMarker[marker] = receipt
		for _, allocation := range receipt.allocations {
			if !strings.HasPrefix(allocation, "new ") || len(allocation) <= 4 || len(allocation) > 8192 || !nativeProofWork(c.Work, int64(len(allocation))+1) {
				return nil, false
			}
			allocations[allocation] = true
		}
	}
	type span struct{ open, end int }
	var bodies []span
	seen := map[*lambdaAnonymousSourceScope]bool{}
	ends := map[int]string{}
	scanned := int64(len(source))
	charge := func(n int) bool {
		if n < 0 || int64(n) > (128<<20)-scanned || !nativeProofWork(c.Work, int64(n)+1) {
			return false
		}
		scanned += int64(n)
		return true
	}
	for at := 0; at < len(source); {
		if strings.HasPrefix(source[at:], "/*jdec-owned-anonymous-ordinal:") {
			close := strings.Index(source[at+2:], "*/")
			if close < 0 {
				return nil, false
			}
			markerEnd := at + 2 + close + 2
			if receipt := byMarker[source[at:markerEnd]]; receipt != nil {
				before := at - 1
				for before >= 0 && strings.ContainsRune(" \t\r\n\f", rune(source[before])) {
					before--
				}
				open := markerEnd
				for open < len(source) && strings.ContainsRune(" \t\r\n\f", rune(source[open])) {
					open++
				}
				if seen[receipt] || !slices.Contains(receipt.allocations, ends[before]) || open >= len(source) || source[open] != '{' || !charge(len(source)-open) {
					return nil, false
				}
				end := javaMatchBrace(source, open)
				if end < 0 {
					return nil, false
				}
				seen[receipt] = true
				bodies = append(bodies, span{open, end})
			} else {
				// A same-class marker cannot borrow a neighboring declaration.
				text := strings.TrimSuffix(strings.TrimPrefix(source[at:markerEnd], "/*jdec-owned-anonymous-ordinal:"), "*/")
				_, owner, scoped := strings.Cut(text, ":")
				if scoped && owner == c.obj.GetClassName() {
					return nil, false
				}
			}
			at = markerEnd
			continue
		}
		if next, skipped := skipJavaNonCode(source, at); skipped {
			at = next
			continue
		}
		if strings.HasPrefix(source[at:], "new ") && (at == 0 || strings.ContainsRune(" \t\r\n\f=([{,:;!+-*/%&|^?<>.", rune(source[at-1]))) {
			for allocation := range allocations {
				if !charge(len(allocation) + 1) {
					return nil, false
				}
				open := at + len(allocation)
				if !strings.HasPrefix(source[at:], allocation) || open >= len(source) || source[open] != '(' {
					continue
				}
				if !charge(len(source) - open) {
					return nil, false
				}
				if end := lambdaAllocationParen(source, open); end >= 0 {
					next := end + 1
					for next < len(source) && strings.ContainsRune(" \t\r\n\f", rune(source[next])) {
						next++
					}
					if strings.HasPrefix(source[next:], "/*jdec-owned-anonymous-ordinal:") {
						ends[end] = allocation
					}
				}
			}
		}
		at++
	}
	filtered := make([]int, 0, len(tokens))
	for _, token := range tokens {
		foreign := false
		for _, body := range bodies {
			foreign = foreign || token > body.open && token < body.end
		}
		if !foreign {
			filtered = append(filtered, token)
		}
	}
	return filtered, true
}

func lambdaAllocationParen(source string, open int) int {
	depth, state, ignored := 0, scanNormal, 0
	for at := open; at < len(source); at++ {
		state = scanAdvance(source, &at, state, &ignored)
		if state != scanNormal || at >= len(source) {
			continue
		}
		switch source[at] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return at
			}
		}
	}
	return -1
}
