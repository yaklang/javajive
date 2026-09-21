package core

import (
	"container/list"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// VirtualExitSentinel is the analysis-only postdominator used when a domain has
// no real return/throw exit, or when an infinite component is connected to the
// virtual sink. It is never a real SemanticCFG node and must not be emitted as a
// return.
const VirtualExitSentinel = -2

const defaultGraphAnalysisCacheCapacity = 64

// AnalysisKind selects a cached graph query.
type AnalysisKind uint8

const (
	AnalysisDominators AnalysisKind = iota
	AnalysisPostDominators
	AnalysisLoopForest
)

// GraphAnalysisDomain is part of the cache key: sorted roots plus whether
// exception edges are in the analyzed graph.
type GraphAnalysisDomain struct {
	Roots            []int
	IncludeException bool
}

// CachedNaturalLoop is one natural loop (header + body + back-edges).
type CachedNaturalLoop struct {
	Header    int
	Nodes     []int
	BackEdges [][2]int
}

// GraphAnalysis is an immutable snapshot of a cached query. Callers receive a
// deep copy; mutating the returned slices must not poison the cache.
type GraphAnalysis struct {
	Kind            AnalysisKind
	Domain          GraphAnalysisDomain
	IDom            []int
	IPDom           []int
	VirtualExitUsed bool
	LoopHeaders     []int
	Loops           []CachedNaturalLoop
	NodeCount       int
}

// graphAnalysisState is attached to an immutable SemanticCFG.
type graphAnalysisState struct {
	id    uint64
	cache *GraphAnalysisCache
}

var (
	nextAnalysisGraphID atomic.Uint64
	analysisInitMu      sync.Mutex
)

type cacheKey struct {
	graphID    uint64
	epoch      uint64
	kind       AnalysisKind
	includeExc bool
	roots      string
}

type cacheEntry struct {
	key cacheKey
	val *GraphAnalysis
	el  *list.Element
}

// GraphAnalysisCache is a bounded LRU of dominance / postdominance / loop-forest
// results. Immutable SemanticCFG caches and mutable rewriter-style graphs must
// use distinct GraphAnalysisCache instances (and distinct graph IDs).
type GraphAnalysisCache struct {
	mu           sync.Mutex
	capacity     int
	entries      map[cacheKey]*cacheEntry
	lru          *list.List
	computes     int64
	nodesTouched int64
}

// NewGraphAnalysisCache returns an LRU cache. Capacity < 1 is treated as 1 so
// T26-C05 can exercise the eviction bound.
func NewGraphAnalysisCache(capacity int) *GraphAnalysisCache {
	if capacity < 1 {
		capacity = 1
	}
	return &GraphAnalysisCache{
		capacity: capacity,
		entries:  map[cacheKey]*cacheEntry{},
		lru:      list.New(),
	}
}

// SetCapacity updates the eviction bound. Existing entries above the new
// capacity are dropped from the LRU tail.
func (c *GraphAnalysisCache) SetCapacity(capacity int) {
	if c == nil {
		return
	}
	if capacity < 1 {
		capacity = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capacity = capacity
	for c.lru.Len() > c.capacity {
		c.evictTailLocked()
	}
}

// ComputeCount is the number of actual constructions (cache misses).
func (c *GraphAnalysisCache) ComputeCount() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.computes
}

// NodesTouched is the sum of node counts processed during constructions.
func (c *GraphAnalysisCache) NodesTouched() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nodesTouched
}

func (c *GraphAnalysisCache) evictTailLocked() {
	back := c.lru.Back()
	if back == nil {
		return
	}
	ent := back.Value.(*cacheEntry)
	c.lru.Remove(back)
	delete(c.entries, ent.key)
}

func domainRootsKey(roots []int) string {
	if len(roots) == 0 {
		return ""
	}
	cp := append([]int(nil), roots...)
	sort.Ints(cp)
	var b strings.Builder
	for i, r := range cp {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(r))
	}
	return b.String()
}

func cloneAnalysis(a *GraphAnalysis) *GraphAnalysis {
	if a == nil {
		return nil
	}
	out := *a
	out.IDom = append([]int(nil), a.IDom...)
	out.IPDom = append([]int(nil), a.IPDom...)
	out.LoopHeaders = append([]int(nil), a.LoopHeaders...)
	out.Domain.Roots = append([]int(nil), a.Domain.Roots...)
	if len(a.Loops) > 0 {
		out.Loops = make([]CachedNaturalLoop, len(a.Loops))
		for i, lp := range a.Loops {
			out.Loops[i] = CachedNaturalLoop{
				Header:    lp.Header,
				Nodes:     append([]int(nil), lp.Nodes...),
				BackEdges: append([][2]int(nil), lp.BackEdges...),
			}
		}
	}
	return &out
}

func (c *GraphAnalysisCache) getOrCompute(graphID, epoch uint64, kind AnalysisKind, domain GraphAnalysisDomain, succs [][]int) *GraphAnalysis {
	key := cacheKey{
		graphID:    graphID,
		epoch:      epoch,
		kind:       kind,
		includeExc: domain.IncludeException,
		roots:      domainRootsKey(domain.Roots),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent, ok := c.entries[key]; ok {
		c.lru.MoveToFront(ent.el)
		return cloneAnalysis(ent.val)
	}
	n := len(succs)
	var computed *GraphAnalysis
	switch kind {
	case AnalysisDominators:
		computed = &GraphAnalysis{
			Kind:      AnalysisDominators,
			Domain:    GraphAnalysisDomain{Roots: append([]int(nil), domain.Roots...), IncludeException: domain.IncludeException},
			IDom:      computeImmediateDominators(n, succs, domain.Roots),
			NodeCount: n,
		}
	case AnalysisPostDominators:
		ipdom, used := computeImmediatePostDominators(n, succs, domain.Roots)
		computed = &GraphAnalysis{
			Kind:            AnalysisPostDominators,
			Domain:          GraphAnalysisDomain{Roots: append([]int(nil), domain.Roots...), IncludeException: domain.IncludeException},
			IPDom:           ipdom,
			VirtualExitUsed: used,
			NodeCount:       n,
		}
	case AnalysisLoopForest:
		idom := computeImmediateDominators(n, succs, domain.Roots)
		headers, loops := computeLoopForest(n, succs, idom)
		computed = &GraphAnalysis{
			Kind:        AnalysisLoopForest,
			Domain:      GraphAnalysisDomain{Roots: append([]int(nil), domain.Roots...), IncludeException: domain.IncludeException},
			IDom:        idom,
			LoopHeaders: headers,
			Loops:       loops,
			NodeCount:   n,
		}
	default:
		computed = &GraphAnalysis{Kind: kind, NodeCount: n}
	}
	c.computes++
	c.nodesTouched += int64(n)
	ent := &cacheEntry{key: key, val: computed}
	ent.el = c.lru.PushFront(ent)
	c.entries[key] = ent
	if c.lru.Len() > c.capacity {
		c.evictTailLocked()
	}
	return cloneAnalysis(computed)
}

func (g *SemanticCFG) analysisState() *graphAnalysisState {
	analysisInitMu.Lock()
	defer analysisInitMu.Unlock()
	if g.analysis == nil {
		g.analysis = &graphAnalysisState{
			id:    nextAnalysisGraphID.Add(1),
			cache: NewGraphAnalysisCache(defaultGraphAnalysisCacheCapacity),
		}
	}
	return g.analysis
}

// SetAnalysisCacheCapacity bounds the SemanticCFG analysis LRU (tests use 1).
func (g *SemanticCFG) SetAnalysisCacheCapacity(capacity int) {
	g.analysisState().cache.SetCapacity(capacity)
}

// AnalysisComputeCount reports cache misses on this CFG.
func (g *SemanticCFG) AnalysisComputeCount() int64 {
	if g.analysis == nil || g.analysis.cache == nil {
		return 0
	}
	return g.analysis.cache.ComputeCount()
}

// AnalysisNodesTouched reports construction work on this CFG.
func (g *SemanticCFG) AnalysisNodesTouched() int64 {
	if g.analysis == nil || g.analysis.cache == nil {
		return 0
	}
	return g.analysis.cache.NodesTouched()
}

func (g *SemanticCFG) nodeIndexMap() map[*OpCode]int {
	m := make(map[*OpCode]int, len(g.Nodes))
	if g.order != nil && len(g.order) == len(g.Nodes) {
		for n, i := range g.order {
			m[n] = i
		}
		return m
	}
	for i, n := range g.Nodes {
		m[n] = i
	}
	return m
}

func (g *SemanticCFG) successorIndexLists(includeException bool) [][]int {
	n := len(g.Nodes)
	succs := make([][]int, n)
	if n == 0 {
		return succs
	}
	idx := g.nodeIndexMap()
	seen := make([]map[int]struct{}, n)
	for _, e := range g.Edges {
		if !includeException && e.Kind == EdgeException {
			continue
		}
		u, ok1 := idx[e.From]
		v, ok2 := idx[e.To]
		if !ok1 || !ok2 {
			continue
		}
		if seen[u] == nil {
			seen[u] = map[int]struct{}{}
		}
		if _, dup := seen[u][v]; dup {
			continue
		}
		seen[u][v] = struct{}{}
		succs[u] = append(succs[u], v)
	}
	return succs
}

// GetOrCompute returns a copied analysis snapshot for this immutable CFG.
func (g *SemanticCFG) GetOrCompute(kind AnalysisKind, domain GraphAnalysisDomain) *GraphAnalysis {
	st := g.analysisState()
	return st.cache.getOrCompute(st.id, 0, kind, domain, g.successorIndexLists(domain.IncludeException))
}

// Dominates reports whether dominator dominates node in this snapshot.
func (a *GraphAnalysis) Dominates(dominator, node int) bool {
	if a == nil || node < 0 || node >= len(a.IDom) || dominator < 0 {
		return false
	}
	if a.IDom[node] < 0 {
		return false
	}
	if dominator == node {
		return true
	}
	seen := map[int]struct{}{}
	for cur := node; cur >= 0 && cur < len(a.IDom); {
		if _, loop := seen[cur]; loop {
			return false
		}
		seen[cur] = struct{}{}
		d := a.IDom[cur]
		if d == dominator {
			return true
		}
		if d < 0 || d == cur {
			return d == dominator
		}
		cur = d
	}
	return false
}

// PostDominates reports whether p postdominates n. The virtual exit is never a
// real return, so PostDominates(VirtualExitSentinel, n) is always false.
func (a *GraphAnalysis) PostDominates(p, n int) bool {
	if a == nil || n < 0 || n >= len(a.IPDom) {
		return false
	}
	if p == VirtualExitSentinel {
		return false
	}
	if a.IPDom[n] == -1 && p != n {
		return false
	}
	if p == n {
		return a.IPDom[n] != -1 || p >= 0
	}
	seen := map[int]struct{}{}
	for cur := n; cur >= 0 && cur < len(a.IPDom); {
		if _, loop := seen[cur]; loop {
			return false
		}
		seen[cur] = struct{}{}
		d := a.IPDom[cur]
		if d == p {
			return true
		}
		if d == VirtualExitSentinel || d < 0 {
			return false
		}
		if d == cur {
			return cur == p
		}
		cur = d
	}
	return false
}

func uniqueInts(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func reachableFrom(n int, succs [][]int, roots []int) []bool {
	mark := make([]bool, n)
	stack := make([]int, 0, n)
	for _, r := range roots {
		if r >= 0 && r < n && !mark[r] {
			mark[r] = true
			stack = append(stack, r)
		}
	}
	for len(stack) > 0 {
		u := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, v := range succs[u] {
			if v >= 0 && v < n && !mark[v] {
				mark[v] = true
				stack = append(stack, v)
			}
		}
	}
	return mark
}

func reversePostorder(n int, succs [][]int, starts []int) []int {
	color := make([]uint8, n)
	post := make([]int, 0, n)
	type frame struct{ u, i int }
	for _, s := range starts {
		if s < 0 || s >= n || color[s] != 0 {
			continue
		}
		stack := []frame{{s, 0}}
		color[s] = 1
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.i < len(succs[top.u]) {
				v := succs[top.u][top.i]
				top.i++
				if v >= 0 && v < n && color[v] == 0 {
					color[v] = 1
					stack = append(stack, frame{v, 0})
				}
				continue
			}
			color[top.u] = 2
			post = append(post, top.u)
			stack = stack[:len(stack)-1]
		}
	}
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

// computeImmediateDominators is the cached Lengauer-style iterative IDom
// (Cooper/Harvey/Kennedy). A virtual source is used only when the domain has
// multiple roots.
func computeImmediateDominators(n int, succs [][]int, roots []int) []int {
	idom := make([]int, n)
	for i := range idom {
		idom[i] = -1
	}
	if n == 0 {
		return idom
	}
	roots = uniqueInts(roots)
	reach := reachableFrom(n, succs, roots)
	filtered := make([]int, 0, len(roots))
	for _, r := range roots {
		if r >= 0 && r < n && reach[r] {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return idom
	}
	vs := n
	total := n + 1
	succs2 := make([][]int, total)
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		for _, v := range succs[i] {
			if v >= 0 && v < n && reach[v] {
				succs2[i] = append(succs2[i], v)
			}
		}
	}
	succs2[vs] = append([]int(nil), filtered...)
	preds := make([][]int, total)
	for u := 0; u < total; u++ {
		for _, v := range succs2[u] {
			preds[v] = append(preds[v], u)
		}
	}
	rpo := reversePostorder(total, succs2, []int{vs})
	rpoIndex := make([]int, total)
	for i := range rpoIndex {
		rpoIndex[i] = -1
	}
	for i, u := range rpo {
		rpoIndex[u] = i
	}
	idom2 := make([]int, total)
	for i := range idom2 {
		idom2[i] = -1
	}
	idom2[vs] = vs
	intersect := func(b1, b2 int) int {
		for b1 != b2 {
			for rpoIndex[b1] > rpoIndex[b2] {
				b1 = idom2[b1]
				if b1 < 0 {
					return b2
				}
			}
			for rpoIndex[b2] > rpoIndex[b1] {
				b2 = idom2[b2]
				if b2 < 0 {
					return b1
				}
			}
		}
		return b1
	}
	changed := true
	for changed {
		changed = false
		for _, b := range rpo {
			if b == vs {
				continue
			}
			newIdom := -1
			for _, p := range preds[b] {
				if idom2[p] != -1 {
					newIdom = p
					break
				}
			}
			if newIdom == -1 {
				continue
			}
			for _, p := range preds[b] {
				if p == newIdom {
					continue
				}
				if idom2[p] != -1 {
					newIdom = intersect(p, newIdom)
				}
			}
			if idom2[b] != newIdom {
				idom2[b] = newIdom
				changed = true
			}
		}
	}
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		d := idom2[i]
		if d == vs || d == i {
			idom[i] = i
		} else if d >= 0 && d < n {
			idom[i] = d
		} else {
			idom[i] = i
		}
	}
	return idom
}

func canReachExitMarks(n int, succs [][]int, reach []bool, exits []bool) []bool {
	rev := make([][]int, n)
	for u := 0; u < n; u++ {
		if !reach[u] {
			continue
		}
		for _, v := range succs[u] {
			if v >= 0 && v < n && reach[v] {
				rev[v] = append(rev[v], u)
			}
		}
	}
	mark := make([]bool, n)
	stack := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if exits[i] {
			mark[i] = true
			stack = append(stack, i)
		}
	}
	for len(stack) > 0 {
		u := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, p := range rev[u] {
			if !mark[p] {
				mark[p] = true
				stack = append(stack, p)
			}
		}
	}
	return mark
}

// computeImmediatePostDominators attaches a virtual exit to every real exit and
// to every reachable node that cannot reach a real exit (infinite components).
// The virtual node is analysis-only.
func computeImmediatePostDominators(n int, succs [][]int, roots []int) ([]int, bool) {
	ipdom := make([]int, n)
	for i := range ipdom {
		ipdom[i] = -1
	}
	if n == 0 {
		return ipdom, false
	}
	reach := reachableFrom(n, succs, roots)
	exits := make([]bool, n)
	hasExit := false
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		alive := false
		for _, v := range succs[i] {
			if v >= 0 && v < n && reach[v] {
				alive = true
				break
			}
		}
		if !alive {
			exits[i] = true
			hasExit = true
		}
	}
	canExit := canReachExitMarks(n, succs, reach, exits)
	ve := n
	total := n + 1
	fwd := make([][]int, total)
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		for _, v := range succs[i] {
			if v >= 0 && v < n && reach[v] {
				fwd[i] = append(fwd[i], v)
			}
		}
		if exits[i] || !canExit[i] {
			fwd[i] = append(fwd[i], ve)
		}
	}
	rev := make([][]int, total)
	for u := 0; u < total; u++ {
		for _, v := range fwd[u] {
			rev[v] = append(rev[v], u)
		}
	}
	idomRev := computeImmediateDominators(total, rev, []int{ve})
	used := false
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		d := idomRev[i]
		if d == ve {
			ipdom[i] = VirtualExitSentinel
			used = true
		} else if d >= 0 && d < n && d != i {
			ipdom[i] = d
		} else if d == i {
			ipdom[i] = i
		}
	}
	if !hasExit {
		used = true
	}
	return ipdom, used
}

func idomDominates(idom []int, dominator, node int) bool {
	if node < 0 || node >= len(idom) || dominator < 0 {
		return false
	}
	if idom[node] < 0 {
		return false
	}
	if dominator == node {
		return true
	}
	seen := map[int]struct{}{}
	for cur := node; cur >= 0 && cur < len(idom); {
		if _, loop := seen[cur]; loop {
			return false
		}
		seen[cur] = struct{}{}
		d := idom[cur]
		if d == dominator {
			return true
		}
		if d < 0 || d == cur {
			return d == dominator
		}
		cur = d
	}
	return false
}

func computeLoopForest(n int, succs [][]int, idom []int) ([]int, []CachedNaturalLoop) {
	preds := make([][]int, n)
	for u := 0; u < n; u++ {
		for _, v := range succs[u] {
			if v >= 0 && v < n {
				preds[v] = append(preds[v], u)
			}
		}
	}
	type loopAcc struct {
		header    int
		nodes     map[int]struct{}
		backEdges [][2]int
	}
	byHeader := map[int]*loopAcc{}
	for u := 0; u < n; u++ {
		if idom[u] < 0 {
			continue
		}
		for _, v := range succs[u] {
			if v < 0 || v >= n || idom[v] < 0 {
				continue
			}
			if !idomDominates(idom, v, u) {
				continue
			}
			acc := byHeader[v]
			if acc == nil {
				acc = &loopAcc{header: v, nodes: map[int]struct{}{v: {}}}
				byHeader[v] = acc
			}
			acc.backEdges = append(acc.backEdges, [2]int{u, v})
			acc.nodes[v] = struct{}{}
			stack := []int{u}
			if _, ok := acc.nodes[u]; !ok {
				acc.nodes[u] = struct{}{}
			} else {
				stack = stack[:0]
			}
			for len(stack) > 0 {
				x := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, p := range preds[x] {
					if p == v {
						continue
					}
					if _, seen := acc.nodes[p]; seen {
						continue
					}
					if idom[p] < 0 {
						continue
					}
					acc.nodes[p] = struct{}{}
					stack = append(stack, p)
				}
			}
		}
	}
	headers := make([]int, 0, len(byHeader))
	for h := range byHeader {
		headers = append(headers, h)
	}
	sort.Ints(headers)
	loops := make([]CachedNaturalLoop, 0, len(headers))
	for _, h := range headers {
		acc := byHeader[h]
		nodes := make([]int, 0, len(acc.nodes))
		for x := range acc.nodes {
			nodes = append(nodes, x)
		}
		sort.Ints(nodes)
		sort.Slice(acc.backEdges, func(i, j int) bool {
			if acc.backEdges[i][0] != acc.backEdges[j][0] {
				return acc.backEdges[i][0] < acc.backEdges[j][0]
			}
			return acc.backEdges[i][1] < acc.backEdges[j][1]
		})
		loops = append(loops, CachedNaturalLoop{Header: h, Nodes: nodes, BackEdges: acc.backEdges})
	}
	return headers, loops
}

// MutableAnalysisGraph is an independent mutation-epoch graph for rewriter-style
// Node CFGs. It must not share a cache with an immutable SemanticCFG.
type MutableAnalysisGraph struct {
	id    uint64
	epoch uint64
	n     int
	edges []mutableEdge
	cache *GraphAnalysisCache
}

type mutableEdge struct {
	from, to  int
	exception bool
}

// NewMutableAnalysisGraph constructs a mutable analysis graph with its own cache.
func NewMutableAnalysisGraph(nodes int, cache *GraphAnalysisCache) *MutableAnalysisGraph {
	if cache == nil {
		cache = NewGraphAnalysisCache(defaultGraphAnalysisCacheCapacity)
	}
	return &MutableAnalysisGraph{
		id:    nextAnalysisGraphID.Add(1),
		n:     nodes,
		cache: cache,
	}
}

func (m *MutableAnalysisGraph) bump() { m.epoch++ }

// Epoch is the mutation counter; any add/delete/replace of a node or edge misses
// previously cached results.
func (m *MutableAnalysisGraph) Epoch() uint64 { return m.epoch }

func (m *MutableAnalysisGraph) AddNode() int {
	id := m.n
	m.n++
	m.bump()
	return id
}

func (m *MutableAnalysisGraph) AddEdge(from, to int, exception bool) {
	m.edges = append(m.edges, mutableEdge{from, to, exception})
	m.bump()
}

func (m *MutableAnalysisGraph) DeleteEdge(from, to int, exception bool) {
	out := m.edges[:0]
	for _, e := range m.edges {
		if e.from == from && e.to == to && e.exception == exception {
			continue
		}
		out = append(out, e)
	}
	m.edges = out
	m.bump()
}

// ReplaceNode retires node id and rewires its incident edges onto a fresh node.
func (m *MutableAnalysisGraph) ReplaceNode(id int) int {
	fresh := m.n
	m.n++
	for i := range m.edges {
		if m.edges[i].from == id {
			m.edges[i].from = fresh
		}
		if m.edges[i].to == id {
			m.edges[i].to = fresh
		}
	}
	m.bump()
	return fresh
}

func (m *MutableAnalysisGraph) successorIndexLists(includeException bool) [][]int {
	succs := make([][]int, m.n)
	seen := make([]map[int]struct{}, m.n)
	for _, e := range m.edges {
		if !includeException && e.exception {
			continue
		}
		if e.from < 0 || e.from >= m.n || e.to < 0 || e.to >= m.n {
			continue
		}
		if seen[e.from] == nil {
			seen[e.from] = map[int]struct{}{}
		}
		if _, dup := seen[e.from][e.to]; dup {
			continue
		}
		seen[e.from][e.to] = struct{}{}
		succs[e.from] = append(succs[e.from], e.to)
	}
	return succs
}

// GetOrCompute is the mutable-graph counterpart of SemanticCFG.GetOrCompute.
func (m *MutableAnalysisGraph) GetOrCompute(kind AnalysisKind, domain GraphAnalysisDomain) *GraphAnalysis {
	return m.cache.getOrCompute(m.id, m.epoch, kind, domain, m.successorIndexLists(domain.IncludeException))
}

func (m *MutableAnalysisGraph) ComputeCount() int64 { return m.cache.ComputeCount() }

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func analysesEqual(a, b *GraphAnalysis) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind || a.VirtualExitUsed != b.VirtualExitUsed || a.NodeCount != b.NodeCount {
		return false
	}
	if a.Domain.IncludeException != b.Domain.IncludeException || domainRootsKey(a.Domain.Roots) != domainRootsKey(b.Domain.Roots) {
		return false
	}
	if !equalIntSlices(a.IDom, b.IDom) || !equalIntSlices(a.IPDom, b.IPDom) || !equalIntSlices(a.LoopHeaders, b.LoopHeaders) {
		return false
	}
	if len(a.Loops) != len(b.Loops) {
		return false
	}
	for i := range a.Loops {
		if a.Loops[i].Header != b.Loops[i].Header {
			return false
		}
		if !equalIntSlices(a.Loops[i].Nodes, b.Loops[i].Nodes) {
			return false
		}
		if len(a.Loops[i].BackEdges) != len(b.Loops[i].BackEdges) {
			return false
		}
		for j := range a.Loops[i].BackEdges {
			if a.Loops[i].BackEdges[j] != b.Loops[i].BackEdges[j] {
				return false
			}
		}
	}
	return true
}

func formatIDom(idoms []int) string {
	return fmt.Sprintf("%v", idoms)
}
