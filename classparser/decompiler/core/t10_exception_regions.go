package core

import (
	"fmt"
	"sort"
)

// ExceptionEdgeIdentity is the production identity of one exceptional CFG edge.
// HandlerOrder is the exception-table index and must not be copied or dropped
// when several protected ranges share a handler.
type ExceptionEdgeIdentity struct {
	FromPC       uint16
	ToPC         uint16
	HandlerOrder int
	FromIndex    int
	ToIndex      int
}

// ExceptionEdgeIdentities lists every EdgeException in edge-table order.
func (g *SemanticCFG) ExceptionEdgeIdentities() []ExceptionEdgeIdentity {
	if g == nil {
		return nil
	}
	idx := g.nodeIndexMap()
	out := make([]ExceptionEdgeIdentity, 0)
	for _, e := range g.Edges {
		if e.Kind != EdgeException || e.From == nil || e.To == nil {
			continue
		}
		out = append(out, ExceptionEdgeIdentity{
			FromPC:       e.From.CurrentOffset,
			ToPC:         e.To.CurrentOffset,
			HandlerOrder: e.HandlerOrder,
			FromIndex:    idx[e.From],
			ToIndex:      idx[e.To],
		})
	}
	return out
}

type handlerCoverage struct {
	order int
	to    *OpCode
	pcs   map[uint16]struct{}
	min   uint16
	max   uint16
}

// ValidateExceptionRegions rejects protected ranges that overlap without being
// nested (the structurer cannot prove a Java try nest). Nested, identical, and
// disjoint ranges are accepted. Shared handlers with a normal tail are not
// overlapping ranges.
func (g *SemanticCFG) ValidateExceptionRegions() error {
	if g == nil || len(g.Edges) == 0 {
		return nil
	}
	byKey := map[string]*handlerCoverage{}
	orderKeys := []string{}
	for _, e := range g.Edges {
		if e.Kind != EdgeException || e.From == nil || e.To == nil {
			continue
		}
		key := fmt.Sprintf("%d:%p", e.HandlerOrder, e.To)
		cov, ok := byKey[key]
		if !ok {
			cov = &handlerCoverage{
				order: e.HandlerOrder,
				to:    e.To,
				pcs:   map[uint16]struct{}{},
				min:   e.From.CurrentOffset,
				max:   e.From.CurrentOffset,
			}
			byKey[key] = cov
			orderKeys = append(orderKeys, key)
		}
		pc := e.From.CurrentOffset
		cov.pcs[pc] = struct{}{}
		if pc < cov.min {
			cov.min = pc
		}
		if pc > cov.max {
			cov.max = pc
		}
	}
	sort.Strings(orderKeys)
	for i := 0; i < len(orderKeys); i++ {
		a := byKey[orderKeys[i]]
		for j := i + 1; j < len(orderKeys); j++ {
			b := byKey[orderKeys[j]]
			if !exceptionCoveragesCross(a, b) {
				continue
			}
			pc := a.min
			if b.min < pc {
				pc = b.min
			}
			return fmt.Errorf("unsupported_overlapping_protected_ranges: region near PC %d (handlers %d and %d)", pc, a.order, b.order)
		}
	}
	return nil
}

func exceptionCoveragesCross(a, b *handlerCoverage) bool {
	if a == nil || b == nil || len(a.pcs) == 0 || len(b.pcs) == 0 {
		return false
	}
	aSub, bSub, overlap := false, false, false
	for pc := range a.pcs {
		if _, ok := b.pcs[pc]; ok {
			overlap = true
		} else {
			bSub = true
		}
	}
	for pc := range b.pcs {
		if _, ok := a.pcs[pc]; !ok {
			aSub = true
		} else {
			overlap = true
		}
	}
	if !overlap {
		return false
	}
	if !aSub || !bSub {
		return false
	}
	return true
}
