package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// statementGraphSnapshot is an exact, order-preserving snapshot of the statement
// CFG reachable from a root node. Nodes are numbered in the same DFS order as
// core.WalkGraph (and therefore GenerateDominatorTree), and every Next edge is
// recorded by index, including order and duplicates. Two snapshots are equal
// only when they describe the same node pointers with the same ordered edge
// lists; no hashing is involved, so a collision cannot make a stale dominator
// tree look current.
//
// Mutation audit: every edge write in the rewriter uses a core.Node method
// (AddNext, RemoveNext, ReplaceNext, ReplaceNextSliceKeepOrder, InsertNextAt,
// DuplicateNext, RemoveAllNext, Replace) or the direct Next assignments that
// still exist in core graph construction outside the rewrite loop. Because a
// refresh re-reads the live Next lists instead of trusting a version counter,
// even an un-instrumented direct write is detected before the next reuse.
type statementGraphSnapshot struct {
	nodes   []*core.Node
	succs   [][]int
	index   map[*core.Node]int
	stack   []*core.Node
	visited map[*core.Node]struct{}
}

func (s *statementGraphSnapshot) build(root *core.Node) {
	s.nodes = s.nodes[:0]
	if s.index == nil {
		s.index = make(map[*core.Node]int)
	} else {
		clear(s.index)
	}
	if root == nil {
		s.succs = s.succs[:0]
		return
	}
	if s.visited == nil {
		s.visited = make(map[*core.Node]struct{})
	} else {
		clear(s.visited)
	}
	s.stack = append(s.stack[:0], root)
	for len(s.stack) > 0 {
		node := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if _, ok := s.visited[node]; ok {
			continue
		}
		s.visited[node] = struct{}{}
		s.index[node] = len(s.nodes)
		s.nodes = append(s.nodes, node)
		s.stack = append(s.stack, node.Next...)
	}
	if cap(s.succs) < len(s.nodes) {
		s.succs = make([][]int, len(s.nodes))
	} else {
		s.succs = s.succs[:len(s.nodes)]
		for i := range s.succs {
			s.succs[i] = s.succs[i][:0]
		}
	}
	for i, node := range s.nodes {
		for _, next := range node.Next {
			if index, ok := s.index[next]; ok {
				s.succs[i] = append(s.succs[i], index)
			}
		}
	}
}

func (s *statementGraphSnapshot) equal(other *statementGraphSnapshot) bool {
	if s == nil || other == nil {
		return s == other
	}
	if len(s.nodes) != len(other.nodes) || len(s.succs) != len(other.succs) {
		return false
	}
	for i := range s.nodes {
		if s.nodes[i] != other.nodes[i] {
			return false
		}
		left, right := s.succs[i], other.succs[i]
		if len(left) != len(right) {
			return false
		}
		for j := range left {
			if left[j] != right[j] {
				return false
			}
		}
	}
	return true
}

// statementDominanceState owns the memoized analyses for one RewriteManager.
// A topology change swaps in a new snapshot and drops every derived result;
// non-topology Node fields (Id, Statement, flags, Source-only edits) cannot
// change dominance and therefore do not invalidate the cache.
type statementDominanceState struct {
	root     *core.Node
	snapshot *statementGraphSnapshot
	scratch  *statementGraphSnapshot
	graph    *core.MutableAnalysisGraph

	domAnalysis  *core.GraphAnalysis
	domTree      map[*core.Node][]*core.Node
	domReady     bool
	domComputes  int
	postMap      map[*core.Node]*core.Node
	postIndex    []int
	postCanExit  []bool
	postReady    bool
	postComputes int
}

func (s *statementDominanceState) refresh(root *core.Node) {
	if s.scratch == nil {
		s.scratch = &statementGraphSnapshot{}
	}
	s.scratch.build(root)
	if s.snapshot != nil && s.snapshot.equal(s.scratch) {
		s.root = root
		return
	}
	s.snapshot, s.scratch = s.scratch, s.snapshot
	s.root = root
	s.domAnalysis = nil
	s.domTree = nil
	s.domReady = false
	s.postMap = nil
	s.postIndex = nil
	s.postCanExit = nil
	s.postReady = false
	s.graph = newStatementAnalysisGraph(s.snapshot)
}

func newStatementAnalysisGraph(snapshot *statementGraphSnapshot) *core.MutableAnalysisGraph {
	nodes := 0
	if snapshot != nil {
		nodes = len(snapshot.nodes)
	}
	graph := core.NewMutableAnalysisGraph(nodes, core.NewGraphAnalysisCache(2))
	if snapshot == nil {
		return graph
	}
	for from, succs := range snapshot.succs {
		for _, to := range succs {
			graph.AddEdge(from, to, false)
		}
	}
	return graph
}

// dominatorTree computes the canonical child-list tree once per topology. The
// node numbering is the snapshot's WalkGraph order, so iterating node indexes in
// order appends children exactly like GenerateDominatorTree.
func (s *statementDominanceState) dominatorTree() map[*core.Node][]*core.Node {
	if s.domReady {
		return s.domTree
	}
	s.domReady = true
	s.domComputes++
	if s.snapshot == nil || len(s.snapshot.nodes) == 0 {
		s.domTree = map[*core.Node][]*core.Node{}
		return s.domTree
	}
	if s.graph == nil {
		s.graph = newStatementAnalysisGraph(s.snapshot)
	}
	analysis := s.graph.GetOrCompute(core.AnalysisDominators, core.GraphAnalysisDomain{Roots: []int{0}})
	s.domAnalysis = analysis
	tree := make(map[*core.Node][]*core.Node)
	if analysis == nil {
		s.domTree = tree
		return tree
	}
	for i, node := range s.snapshot.nodes {
		if i >= len(analysis.IDom) {
			break
		}
		idom := analysis.IDom[i]
		if idom < 0 || idom == i || idom >= len(s.snapshot.nodes) {
			continue
		}
		parent := s.snapshot.nodes[idom]
		tree[parent] = append(tree[parent], node)
	}
	s.domTree = tree
	return tree
}

// postDominatorMap computes the canonical immediate-post-dominator map once per
// topology. The map excludes nodes with no real post-dominator (method exits and
// exit-less SCCs), matching GeneratePostDominatorMap exactly.
func (s *statementDominanceState) postDominatorMap() map[*core.Node]*core.Node {
	if s.postReady {
		return s.postMap
	}
	s.postReady = true
	s.postComputes++
	s.postMap = GeneratePostDominatorMap(s.root)
	s.buildPostIndex()
	return s.postMap
}

func (s *statementDominanceState) buildPostIndex() {
	if s.snapshot == nil {
		s.postIndex = nil
		s.postCanExit = nil
		return
	}
	n := len(s.snapshot.nodes)
	s.postIndex = make([]int, n)
	for i := range s.postIndex {
		s.postIndex[i] = -1
	}
	for i, node := range s.snapshot.nodes {
		parent := s.postMap[node]
		if parent == nil {
			continue
		}
		if index, ok := s.snapshot.index[parent]; ok {
			s.postIndex[i] = index
		}
	}
	s.postCanExit = statementCanReachExit(s.snapshot)
}

// statementCanReachExit marks every node that can reach a real method exit.
// Nodes trapped in an exit-less SCC have no post-dominator and are excluded from
// the scalar PostDominates relation as well as from the cached map.
func statementCanReachExit(snapshot *statementGraphSnapshot) []bool {
	n := len(snapshot.nodes)
	can := make([]bool, n)
	reverse := make([][]int, n)
	queue := make([]int, 0, n)
	for from, succs := range snapshot.succs {
		if len(succs) == 0 {
			can[from] = true
			queue = append(queue, from)
		}
		for _, to := range succs {
			reverse[to] = append(reverse[to], from)
		}
	}
	for len(queue) > 0 {
		node := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, pred := range reverse[node] {
			if !can[pred] {
				can[pred] = true
				queue = append(queue, pred)
			}
		}
	}
	return can
}

func (s *statementDominanceState) dominates(dominator, node *core.Node) bool {
	if dominator == nil || node == nil {
		return false
	}
	// utils.IsDominate treats a node as dominating itself even when it is not
	// reachable from the analyzed root; keep that observable behavior.
	if dominator == node {
		return true
	}
	s.dominatorTree()
	if s.domAnalysis == nil || s.snapshot == nil {
		return false
	}
	domIndex, ok := s.snapshot.index[dominator]
	if !ok {
		return false
	}
	nodeIndex, ok := s.snapshot.index[node]
	if !ok {
		return false
	}
	return s.domAnalysis.Dominates(domIndex, nodeIndex)
}

func (s *statementDominanceState) postDominates(post, node *core.Node) bool {
	if post == nil || node == nil || s.snapshot == nil {
		return false
	}
	s.postDominatorMap()
	nodeIndex, ok := s.snapshot.index[node]
	if !ok || nodeIndex >= len(s.postCanExit) || !s.postCanExit[nodeIndex] {
		return false
	}
	if post == node {
		return true
	}
	postIndex, ok := s.snapshot.index[post]
	if !ok {
		return false
	}
	cur := nodeIndex
	for steps := 0; steps < len(s.postIndex) && cur >= 0; steps++ {
		parent := s.postIndex[cur]
		if parent == postIndex {
			return true
		}
		if parent < 0 || parent == cur {
			return false
		}
		cur = parent
	}
	return false
}

func (s *RewriteManager) dominanceState() *statementDominanceState {
	if s.dominance == nil {
		s.dominance = &statementDominanceState{}
	}
	return s.dominance
}

// refreshDominators is the single place the rewriter refreshes DominatorMap.
// It reuses the memoized tree when the statement topology is byte-for-byte
// identical and returns a defensive copy so callers can never poison the cache.
func (s *RewriteManager) refreshDominators() {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	s.DominatorMap = cloneDominatorTree(state.dominatorTree())
}

// refreshDominatorsShared refreshes the production rewriter's map in place.
// Rewriter call sites only read DominatorMap; avoiding a full map copy on every
// refresh keeps small methods from paying the cache's defensive-copy overhead.
// The exported-field clone contract remains on refreshDominators (exercised by
// tests and available to callers that mutate the field).
func (s *RewriteManager) refreshDominatorsShared() {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	s.DominatorMap = state.dominatorTree()
}

// postDominatorMap returns a defensive copy of the memoized immediate
// post-dominator map for the current topology.
func (s *RewriteManager) postDominatorMap() map[*core.Node]*core.Node {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	return clonePostDominatorMap(state.postDominatorMap())
}

// Dominates is the internal scalar dominance query. It reuses the cached
// analysis in place instead of cloning it per call.
func (s *RewriteManager) Dominates(dominator, node *core.Node) bool {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	return state.dominates(dominator, node)
}

// PostDominates is the internal scalar post-dominance query. It reuses the
// cached immediate-post-dominator chain in place. A node trapped in an
// exit-less SCC has no post-dominator, including itself.
func (s *RewriteManager) PostDominates(post, node *core.Node) bool {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	return state.postDominates(post, node)
}

func cloneDominatorTree(tree map[*core.Node][]*core.Node) map[*core.Node][]*core.Node {
	if tree == nil {
		return nil
	}
	out := make(map[*core.Node][]*core.Node, len(tree))
	for parent, children := range tree {
		out[parent] = append([]*core.Node(nil), children...)
	}
	return out
}

func clonePostDominatorMap(tree map[*core.Node]*core.Node) map[*core.Node]*core.Node {
	if tree == nil {
		return nil
	}
	out := make(map[*core.Node]*core.Node, len(tree))
	for node, parent := range tree {
		out[node] = parent
	}
	return out
}
