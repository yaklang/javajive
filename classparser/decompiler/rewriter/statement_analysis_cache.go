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

// statementTopologyDiff describes a snapshot change without hosing every
// cached loop body. changed lists nodes whose ordered successor list differs;
// neighbors is the union of their old and new successors. addedOrRemoved means
// node identity changed, which invalidates all cached bodies conservatively.
type statementTopologyDiff struct {
	changed        []*core.Node
	neighbors      []*core.Node
	addedOrRemoved bool
}

func (d *statementTopologyDiff) reset() {
	d.changed = d.changed[:0]
	d.neighbors = d.neighbors[:0]
	d.addedOrRemoved = false
}

// diff reports whether the snapshots are identical in node order and ordered
// successor lists. Otherwise it records enough information for a loop-body
// cache to keep entries whose loop region was untouched.
func (s *statementGraphSnapshot) diff(other *statementGraphSnapshot, out *statementTopologyDiff) bool {
	out.reset()
	if s == nil || other == nil {
		return s == other
	}
	if len(s.nodes) != len(other.nodes) {
		out.addedOrRemoved = true
	}
	orderSame := len(s.nodes) == len(other.nodes)
	for i, node := range s.nodes {
		j, ok := other.index[node]
		if !ok {
			out.addedOrRemoved = true
			out.changed = append(out.changed, node)
			continue
		}
		if i >= len(other.nodes) || other.nodes[i] != node {
			orderSame = false
		}
		left, right := s.succs[i], other.succs[j]
		same := len(left) == len(right)
		if same {
			for k := range left {
				if s.nodes[left[k]] != other.nodes[right[k]] {
					same = false
					break
				}
			}
		}
		if !same {
			out.changed = append(out.changed, node)
			for _, index := range left {
				out.neighbors = append(out.neighbors, s.nodes[index])
			}
			for _, index := range right {
				out.neighbors = append(out.neighbors, other.nodes[index])
			}
		}
	}
	for j, node := range other.nodes {
		if _, ok := s.index[node]; !ok {
			out.addedOrRemoved = true
			out.changed = append(out.changed, node)
			for _, index := range other.succs[j] {
				out.neighbors = append(out.neighbors, other.nodes[index])
			}
		}
	}
	return orderSame && !out.addedOrRemoved && len(out.changed) == 0
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
	// diff describes the last topology change. topologyChanged is true when the
	// snapshot was replaced; the loop-body cache consumes diff after refresh.
	diff            statementTopologyDiff
	topologyChanged bool

	domAnalysis      *core.GraphAnalysis
	domTree          map[*core.Node][]*core.Node
	domParents       []*core.Node
	domParentScratch []*core.Node
	domReady         bool
	domComputes      int
	postMap          map[*core.Node]*core.Node
	postIndex        []int
	postCanExit      []bool
	postReady        bool
	postComputes     int
}

func (s *statementDominanceState) refresh(root *core.Node) {
	if s.scratch == nil {
		s.scratch = &statementGraphSnapshot{}
	}
	s.scratch.build(root)
	if s.snapshot != nil {
		if s.snapshot.diff(s.scratch, &s.diff) {
			s.root = root
			s.topologyChanged = false
			return
		}
		s.topologyChanged = true
	} else {
		// No derived analysis or loop-body cache exists yet.
		s.diff.reset()
		s.topologyChanged = false
	}
	s.snapshot, s.scratch = s.scratch, s.snapshot
	s.root = root
	s.domAnalysis = nil
	s.domReady = false
	s.postMap = nil
	s.postIndex = nil
	s.postCanExit = nil
	s.postReady = false
	s.resetGraph()
}

// resetGraph replaces the graph topology in place. The graph identity and its
// edge/scratch storage survive, while the epoch changes so cached analyses from
// the previous topology cannot be mistaken for the new one.
func (s *statementDominanceState) resetGraph() {
	nodes := 0
	if s.snapshot != nil {
		nodes = len(s.snapshot.nodes)
	}
	if s.graph == nil {
		s.graph = core.NewMutableAnalysisGraph(nodes, core.NewGraphAnalysisCache(2))
	} else {
		s.graph.Reset(nodes)
	}
	if s.snapshot == nil {
		return
	}
	for from, succs := range s.snapshot.succs {
		for _, to := range succs {
			s.graph.AddEdge(from, to, false)
		}
	}
}

// dominatorTree computes the canonical child-list tree once per topology. The
// node numbering is the snapshot's WalkGraph order, so iterating node indexes in
// order appends children exactly like GenerateDominatorTree.
//
// The map and its child slices survive topology changes: each rebuild truncates
// the previous parents' children in place and drops parents that lost all
// children. Callers must treat the returned map as read-only, which the rewrite
// loop already does (refreshDominators still clones for the exported field).
func (s *statementDominanceState) dominatorTree() map[*core.Node][]*core.Node {
	if s.domReady {
		return s.domTree
	}
	s.domReady = true
	s.domComputes++
	if s.domTree == nil {
		s.domTree = make(map[*core.Node][]*core.Node)
	}
	if s.snapshot == nil || len(s.snapshot.nodes) == 0 {
		s.clearDominatorTree()
		return s.domTree
	}
	if s.graph == nil {
		s.resetGraph()
	}
	// The state only reads IDom and the graph is replaced on every topology
	// change, so the cache-owned analysis is safe to borrow here; a defensive
	// deep copy per rewrite step would only add allocation.
	analysis := s.graph.GetOrComputeShared(core.AnalysisDominators, core.GraphAnalysisDomain{Roots: []int{0}})
	s.domAnalysis = analysis
	if analysis == nil {
		s.clearDominatorTree()
		return s.domTree
	}
	previous := s.domParents
	for _, parent := range previous {
		children, ok := s.domTree[parent]
		if !ok {
			continue
		}
		s.domTree[parent] = children[:0]
	}
	next := s.domParentScratch[:0]
	for i, node := range s.snapshot.nodes {
		if i >= len(analysis.IDom) {
			break
		}
		idom := analysis.IDom[i]
		if idom < 0 || idom == i || idom >= len(s.snapshot.nodes) {
			continue
		}
		parent := s.snapshot.nodes[idom]
		children, ok := s.domTree[parent]
		// Truncated parents have len 0 and must be re-registered on their first
		// child; parents with no remaining children are dropped below.
		if !ok || len(children) == 0 {
			next = append(next, parent)
		}
		s.domTree[parent] = append(children, node)
	}
	for _, parent := range previous {
		if children, ok := s.domTree[parent]; ok && len(children) == 0 {
			delete(s.domTree, parent)
		}
	}
	s.domParentScratch = previous[:0]
	s.domParents = next
	return s.domTree
}

// clearDominatorTree empties the reusable tree and its parent index. The map
// allocation survives; only the entries are dropped.
func (s *statementDominanceState) clearDominatorTree() {
	for _, parent := range s.domParents {
		delete(s.domTree, parent)
	}
	s.domParents = s.domParents[:0]
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
	s.domMapVersion++
	s.domMapDirty = false
	s.invalidateLoopBasesAfterTopology(state)
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
	if state.topologyChanged || s.domMapDirty {
		s.domMapVersion++
		s.domMapDirty = false
	}
	s.invalidateLoopBasesAfterTopology(state)
}

// postDominatorMap returns a defensive copy of the memoized immediate
// post-dominator map for the current topology.
func (s *RewriteManager) postDominatorMap() map[*core.Node]*core.Node {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	if state.topologyChanged {
		// These query methods do not publish a fresh DominatorMap; keep the
		// base-body cache and dominance index conservative until the rewrite
		// loop refreshes.
		s.domMapDirty = true
		s.domIndex = nil
		s.clearLoopBases()
	}
	return clonePostDominatorMap(state.postDominatorMap())
}

// Dominates is the internal scalar dominance query. It reuses the cached
// analysis in place instead of cloning it per call.
func (s *RewriteManager) Dominates(dominator, node *core.Node) bool {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	if state.topologyChanged {
		s.domMapDirty = true
		s.domIndex = nil
		s.clearLoopBases()
	}
	return state.dominates(dominator, node)
}

// PostDominates is the internal scalar post-dominance query. It reuses the
// cached immediate-post-dominator chain in place. A node trapped in an
// exit-less SCC has no post-dominator, including itself.
func (s *RewriteManager) PostDominates(post, node *core.Node) bool {
	state := s.dominanceState()
	state.refresh(s.RootNode)
	if state.topologyChanged {
		s.domMapDirty = true
		s.domIndex = nil
		s.clearLoopBases()
	}
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
