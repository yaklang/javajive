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

// exceptionRankSpan is an inclusive range in may-throw instruction rank order.
// Protected PC sets are contiguous in that order, so comparing a few spans is
// equivalent to comparing every protected PC.
type exceptionRankSpan struct {
	start int
	end   int
}

type handlerCoverage struct {
	order int
	to    *OpCode
	ranks map[int]struct{}
	spans []exceptionRankSpan
}

type handlerCoverageKey struct {
	order int
	to    *OpCode
}

// ValidateExceptionRegions rejects protected ranges that overlap without being
// nested (the structurer cannot prove a Java try nest). Nested, identical, and
// disjoint ranges are accepted. Shared handlers with a normal tail are not
// overlapping ranges.
func (g *SemanticCFG) ValidateExceptionRegions() error {
	if g == nil || len(g.Edges) == 0 {
		return nil
	}
	// Map protected PCs to their order among may-throw instructions. Within a
	// protected range those instructions are consecutive, which lets each
	// handler coverage collapse to a handful of rank spans.
	pcSet := map[uint16]struct{}{}
	for _, e := range g.Edges {
		if e.Kind == EdgeException && e.From != nil {
			pcSet[e.From.CurrentOffset] = struct{}{}
		}
	}
	pcs := make([]int, 0, len(pcSet))
	for pc := range pcSet {
		pcs = append(pcs, int(pc))
	}
	sort.Ints(pcs)
	rankOf := make(map[uint16]int, len(pcs))
	for rank, pc := range pcs {
		rankOf[uint16(pc)] = rank
	}

	byKey := map[handlerCoverageKey]*handlerCoverage{}
	keys := make([]handlerCoverageKey, 0)
	for _, e := range g.Edges {
		if e.Kind != EdgeException || e.From == nil || e.To == nil {
			continue
		}
		key := handlerCoverageKey{order: e.HandlerOrder, to: e.To}
		cov, ok := byKey[key]
		if !ok {
			cov = &handlerCoverage{order: e.HandlerOrder, to: e.To, ranks: map[int]struct{}{}}
			byKey[key] = cov
			keys = append(keys, key)
		}
		rank, ok := rankOf[e.From.CurrentOffset]
		if !ok {
			continue
		}
		cov.ranks[rank] = struct{}{}
	}
	targetIndex := func(to *OpCode) int {
		if index, ok := g.order[to]; ok {
			return index
		}
		return len(g.Nodes)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].order != keys[j].order {
			return keys[i].order < keys[j].order
		}
		return targetIndex(keys[i].to) < targetIndex(keys[j].to)
	})
	for _, key := range keys {
		cov := byKey[key]
		ranks := make([]int, 0, len(cov.ranks))
		for rank := range cov.ranks {
			ranks = append(ranks, rank)
		}
		sort.Ints(ranks)
		for _, rank := range ranks {
			if n := len(cov.spans); n > 0 && rank == cov.spans[n-1].end+1 {
				cov.spans[n-1].end = rank
				continue
			}
			cov.spans = append(cov.spans, exceptionRankSpan{start: rank, end: rank})
		}
	}
	// Most handlers cover exactly one contiguous range. Detect crossings among
	// those with a single endpoint sweep instead of comparing every handler
	// pair. Handlers with several disjoint ranges fall back to exact pairwise
	// span comparison, which is bounded by their (small) range count.
	allSingle := true
	for _, key := range keys {
		if len(byKey[key].spans) != 1 {
			allSingle = false
			break
		}
	}
	if allSingle {
		if a, b, ok := crossingSingleSpanCoverage(keys, byKey); ok {
			return fmt.Errorf("unsupported_overlapping_protected_ranges: region near PC %d (handlers %d and %d)", pcs[byKey[a].spans[0].start], byKey[a].order, byKey[b].order)
		}
		return nil
	}
	for i := 0; i < len(keys); i++ {
		a := byKey[keys[i]]
		for j := i + 1; j < len(keys); j++ {
			b := byKey[keys[j]]
			if !exceptionCoveragesCross(a, b) {
				continue
			}
			pc := uint16(pcs[a.spans[0].start])
			if bStart := pcs[b.spans[0].start]; bStart < int(pc) {
				pc = uint16(bStart)
			}
			return fmt.Errorf("unsupported_overlapping_protected_ranges: region near PC %d (handlers %d and %d)", pc, a.order, b.order)
		}
	}
	return nil
}

type maxEndHeap []int

func (h *maxEndHeap) push(value int) {
	*h = append(*h, value)
	index := len(*h) - 1
	for index > 0 {
		parent := (index - 1) / 2
		if (*h)[parent] >= (*h)[index] {
			break
		}
		(*h)[parent], (*h)[index] = (*h)[index], (*h)[parent]
		index = parent
	}
}

func (h *maxEndHeap) pop() int {
	top := (*h)[0]
	last := len(*h) - 1
	(*h)[0] = (*h)[last]
	*h = (*h)[:last]
	index := 0
	for {
		left, right := index*2+1, index*2+2
		largest := index
		if left < len(*h) && (*h)[left] > (*h)[largest] {
			largest = left
		}
		if right < len(*h) && (*h)[right] > (*h)[largest] {
			largest = right
		}
		if largest == index {
			break
		}
		(*h)[index], (*h)[largest] = (*h)[largest], (*h)[index]
		index = largest
	}
	return top
}

// crossingSingleSpanCoverage reports the first crossing pair when every
// handler has one span. Intervals that start at the same rank are nested with
// respect to each other, so only an earlier-starting live interval whose end
// falls short of the group's maximum end can cross.
func crossingSingleSpanCoverage(keys []handlerCoverageKey, byKey map[handlerCoverageKey]*handlerCoverage) (handlerCoverageKey, handlerCoverageKey, bool) {
	type entry struct {
		key   handlerCoverageKey
		start int
		end   int
	}
	entries := make([]entry, 0, len(keys))
	for _, key := range keys {
		span := byKey[key].spans[0]
		entries = append(entries, entry{key: key, start: span.start, end: span.end})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].start != entries[j].start {
			return entries[i].start < entries[j].start
		}
		return entries[i].end < entries[j].end
	})
	var active maxEndHeap
	var activeKey handlerCoverageKey
	haveActiveKey := false
	for index := 0; index < len(entries); {
		start := entries[index].start
		groupEnd := index
		groupMax := -1
		var groupMaxKey handlerCoverageKey
		for groupEnd < len(entries) && entries[groupEnd].start == start {
			if entries[groupEnd].end > groupMax {
				groupMax = entries[groupEnd].end
				groupMaxKey = entries[groupEnd].key
			}
			groupEnd++
		}
		if len(active) > 0 && active[0] < start {
			// Every remaining active interval ends before this group starts.
			active = active[:0]
			haveActiveKey = false
		}
		if len(active) > 0 && active[0] < groupMax {
			// The longest live interval starts earlier but ends inside this
			// group while the group extends beyond it: neither contains the
			// other.
			if !haveActiveKey {
				return groupMaxKey, groupMaxKey, true
			}
			return activeKey, groupMaxKey, true
		}
		prevTop := -1
		if len(active) > 0 {
			prevTop = active[0]
		}
		for _, item := range entries[index:groupEnd] {
			active.push(item.end)
		}
		if !haveActiveKey || groupMax > prevTop {
			activeKey = groupMaxKey
			haveActiveKey = true
		}
		index = groupEnd
	}
	return handlerCoverageKey{}, handlerCoverageKey{}, false
}

func exceptionCoveragesCross(a, b *handlerCoverage) bool {
	if a == nil || b == nil || len(a.spans) == 0 || len(b.spans) == 0 {
		return false
	}
	if !spansOverlap(a.spans, b.spans) {
		return false
	}
	// Crossing means the coverages intersect but neither contains the other.
	return !spansCover(b.spans, a.spans) && !spansCover(a.spans, b.spans)
}

func spansOverlap(a, b []exceptionRankSpan) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].end < b[j].start {
			i++
			continue
		}
		if b[j].end < a[i].start {
			j++
			continue
		}
		return true
	}
	return false
}

// spansCover reports whether outer covers every rank in inner. Both inputs are
// sorted, disjoint, and merged.
func spansCover(outer, inner []exceptionRankSpan) bool {
	j := 0
	for _, span := range inner {
		covered := span.start
		for covered <= span.end {
			for j < len(outer) && outer[j].end < covered {
				j++
			}
			if j >= len(outer) || outer[j].start > covered {
				return false
			}
			if outer[j].end >= span.end {
				covered = span.end + 1
				break
			}
			covered = outer[j].end + 1
			j++
		}
	}
	return true
}
