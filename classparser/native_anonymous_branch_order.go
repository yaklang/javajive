package javaclassparser

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
)

// Java numbers anonymous classes in lexical source order. A structured CFG
// may exchange conditional arms without changing behavior, but that exchange
// changes the binary names. Compare only original owned identities from the
// committed allocation proof, using its lexer that excludes literals/comments.
// Whole-arm swaps are allowed only for disjoint increasing ordinal intervals;
// interleaved or unresolved layouts retain the final family refusal.
func nativeAnonymousBranchSwap(p *nativeAnonymousFamily, left, right string, work *workbudget.Budget) bool {
	if p == nil || p.failed {
		return false
	}
	if len(left) > 16<<20 || len(right) > 16<<20 || !nativeProofWork(work, int64(len(left)+len(right))) {
		p.failed = true
		return false
	}
	interval := func(source string) (first, last int, known bool) {
		ordinals, valid := nativeAnonymousOrdinalsWithinOwner(source, p.owner)
		if !valid {
			p.failed = true
			return 0, 0, false
		}
		for _, n := range ordinals {
			child := p.children[p.owner+"$"+strconv.Itoa(n)]
			if child == nil || child.ordinal != n || n <= 0 {
				p.failed = true
				return 0, 0, false
			}
			if first == 0 {
				first = n
			} else if n <= last {
				return 0, 0, false
			}
			last = n
		}
		return first, last, first > 0
	}
	lf, _, lk := interval(left)
	_, rl, rk := interval(right)
	return lk && rk && rl < lf
}
