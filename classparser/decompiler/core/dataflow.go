package core

import (
	"os"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// dataflow.go 是 Phase 1 的「可信图」: 把一个方法里每个局部变量的 store / load 字节码, 按到达定义
// (reaching-definition) 关系划分成不相交的「web」。一个 store 与它能到达的每个 load 属于同一 web;
// 两个 store 到达同一个 load 也属于同一 web (phi 合并点)。每个 web = 源码层面的一个变量, 与 DFS 遍历
// 序无关——同一个 JVM 槽被多个不相交活跃区间复用时, 它们落在不同 web (各自独立变量), 而在分支汇合处读
// 同一个值的分支落在同一 web (一个变量)。
//
// 它纯结构化 (只依赖 CFG 的 Source 边 + 槽号), 不依赖前向模拟已经赋好的 ref, 因此可在模拟期间随时查询。
// 当前用途: 给 load 侧到达定义修复 (reachingSlotVersionByWeb) 提供「这个读属于哪个变量」的权威判据,
// 推广原先只在「唯一到达定义」时触发的 reachingSlotVersionGeneral。Phase 2 的 LUB 会接它做 phi 合并定型。

// slotWeb is the precomputed reaching-definition partition for one method.
type slotWeb struct {
	webOf map[*OpCode]int // store/load opcode -> web id
}

// unionFind is a tiny disjoint-set over dense int ids used to coalesce store/load opcodes into webs.
type unionFind struct {
	parent []int
}

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]] // path halving
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// reachingStoresOf walks Source edges backward from `load` and returns the nearest same-slot store on
// each path (the reaching definitions). It stops expanding past any same-slot store because that store
// dominates everything earlier on its path. Pure CFG/slot structural analysis — no refs needed, so it
// is valid before/during the forward simulation. The bool reports whether method entry was reached
// without crossing any same-slot store (the slot is live-in here, e.g. a parameter or an
// uninitialized-on-some-path read), which the web builder links to a per-slot entry pseudo-def.
func reachingStoresOf(load *OpCode, slot int) (stores []*OpCode, reachesEntry bool) {
	if load == nil {
		return nil, false
	}
	visited := map[*OpCode]bool{load: true}
	queue := append([]*OpCode{}, load.Source...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		if cur.Instr != nil && isLocalStoreOpcode(cur.Instr.OpCode) && GetStoreIdx(cur) == slot {
			stores = append(stores, cur)
			continue // dominates this path; do not expand past it
		}
		if len(cur.Source) == 0 || cur.CurrentOffset == 0 {
			reachesEntry = true
		}
		queue = append(queue, cur.Source...)
	}
	return stores, reachesEntry
}

// computeSlotWebs builds the reaching-definition web partition for the whole method. It is
// deterministic (keyed by opcode discovery order over d.opCodes) so repeated decompiles are stable.
func (d *Decompiler) computeSlotWebs() *slotWeb {
	// Assign each local store/load opcode a dense index; also reserve one entry pseudo-def per slot.
	idx := map[*OpCode]int{}
	var nodes []*OpCode
	add := func(op *OpCode) {
		if _, ok := idx[op]; !ok {
			idx[op] = len(nodes)
			nodes = append(nodes, op)
		}
	}
	maxSlot := -1
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if isLocalStoreOpcode(op.Instr.OpCode) {
			add(op)
			if s := GetStoreIdx(op); s > maxSlot {
				maxSlot = s
			}
		} else if isLocalLoadOpcode(op.Instr.OpCode) {
			add(op)
			if s := GetRetrieveIdx(op); s > maxSlot {
				maxSlot = s
			}
		}
	}
	base := len(nodes)
	total := base + (maxSlot + 1) // entry pseudo-def nodes, one per slot, at base+slot
	if maxSlot < 0 {
		total = base
	}
	uf := newUnionFind(total)
	entryNode := func(slot int) int { return base + slot }

	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isLocalLoadOpcode(op.Instr.OpCode) {
			continue
		}
		slot := GetRetrieveIdx(op)
		if slot < 0 {
			continue
		}
		stores, reachesEntry := reachingStoresOf(op, slot)
		li := idx[op]
		for _, st := range stores {
			uf.union(li, idx[st])
		}
		if reachesEntry && slot <= maxSlot {
			uf.union(li, entryNode(slot))
		}
	}

	webOf := make(map[*OpCode]int, base)
	for op, i := range idx {
		webOf[op] = uf.find(i)
	}
	return &slotWeb{webOf: webOf}
}

// slotWebs returns the cached web partition, computing it on first use. Returns nil when the
// kill-switch JDEC_LIVEINTERVAL_OFF is set, so all web-driven repairs fall back to the legacy passes.
func (d *Decompiler) slotWebs() *slotWeb {
	if os.Getenv("JDEC_LIVEINTERVAL_OFF") != "" {
		return nil
	}
	if d.cachedSlotWebs == nil {
		d.cachedSlotWebs = d.computeSlotWebs()
	}
	return d.cachedSlotWebs
}

// reachingSlotVersionByWeb repairs a slot read whose DFS-resolved ref (`current`) belongs to a
// DIFFERENT reaching-definition web than the load — a later/disjoint-branch version that leaked in
// through traversal order. It generalizes reachingSlotVersionGeneral (which only fires on a single
// reaching definition): using the precomputed web it accepts a load with several reaching definitions
// as long as those definitions resolve to ONE variable (same VarUid), and rejects the read only when
// the web genuinely merges distinct variables (a real phi we must not rewrite). Returns the web's
// canonical ref to install, or nil to keep `current`. Gated by JDEC_LIVEINTERVAL_OFF via slotWebs().
func (d *Decompiler) reachingSlotVersionByWeb(load *OpCode, slot int, current *values.JavaRef) *values.JavaRef {
	// Default ON (kill-switch JDEC_LIVEINTERVAL_WEB_OFF). An earlier revision kept this opt-in because
	// the web load/store repairs measured net-neutral on the iso per-file metric and slightly negative
	// under an older tree-masking run. Re-measured against the current 8-jar tree inventory (the real
	// repackage metric), enabling it is a strict improvement: fastjson2 24 -> 22 tree errLines
	// (ObjectReaderCreator 3->2, JSONPathParser 2->1) and every other jar unchanged (delta >= 0 across
	// the board). The repair only redirects a load to a web-proven SAME-variable definition (same
	// VarUid); disjoint live ranges (e.g. try-with-resources `primaryExc`) fall in different webs and
	// are left untouched, so it cannot merge genuinely-distinct variables. Kill-switch
	// JDEC_LIVEINTERVAL_WEB_OFF restores the opt-in-off behaviour for A/B delta regression checks.
	if os.Getenv("JDEC_LIVEINTERVAL_WEB_OFF") != "" {
		return nil
	}
	webs := d.slotWebs()
	if webs == nil || load == nil || current == nil {
		return nil
	}
	loadWeb, ok := webs.webOf[load]
	if !ok {
		return nil
	}
	// Collect the refs of this load's reaching definitions (same web). A store is in opcodeIdToRef
	// only after it has been simulated; that is fine — the dominating definition is simulated first.
	var canon *values.JavaRef
	currentInWeb := false
	for _, st := range d.reachingStoreOpsByWeb(load, slot, loadWeb, webs) {
		refs, ok := d.opcodeIdToRef[st]
		if !ok || len(refs) == 0 {
			continue
		}
		ref, ok := refs[len(refs)-1][0].(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		if ref.VarUid == current.VarUid {
			currentInWeb = true
			break
		}
		if canon == nil {
			canon = ref
		} else if canon.VarUid != ref.VarUid {
			// The reaching definitions disagree on identity: a genuine multi-variable merge that we
			// must not rewrite (it needs phi/LUB handling, not a read redirect).
			return nil
		}
	}
	if currentInWeb || canon == nil {
		return nil
	}
	return canon
}

// reachingStoreOpsByWeb returns the load's reaching-definition store opcodes that belong to loadWeb.
func (d *Decompiler) reachingStoreOpsByWeb(load *OpCode, slot, loadWeb int, webs *slotWeb) []*OpCode {
	stores, _ := reachingStoresOf(load, slot)
	out := stores[:0]
	for _, st := range stores {
		if w, ok := webs.webOf[st]; ok && w == loadWeb {
			out = append(out, st)
		}
	}
	return out
}

// reachingSlotStoreContinuationByWeb decides, for a local store, whether it CONTINUES an existing
// variable (same reaching-definition web as a unique dominating prior definition) rather than
// starting a fresh one. It generalizes reachingStoreVersion / reachingRefSlotPhiMerge: instead of
// matching on a single type-equal reaching definition, it uses the precomputed web to recognize that
// this store and its (unique) prior same-slot definition are the SAME source variable, so the store
// must reuse that definition's ref even when DFS order left an unrelated version in the slot table
// (fastjson2 JdbcSupport$TimeReader `var5 = var5 * 1000` mis-split into var5_1). Returns the ref to
// continue, or nil to leave the legacy mint/reuse decision untouched.
//
// Safety: disjoint live ranges that merely reuse a JVM slot (the try-with-resources `primaryExc`
// counter-example the legacy comments guard against) share no downstream load, so they fall in
// DIFFERENT webs; reachingStoresOf filtered by storeWeb excludes them and canon stays nil, leaving the
// legacy split intact. Only definitions the dataflow proves to be one variable are coalesced. Gated
// via slotWebs() (kill-switch JDEC_LIVEINTERVAL_OFF).
func (d *Decompiler) reachingSlotStoreContinuationByWeb(store *OpCode, slot int, current *values.JavaRef) *values.JavaRef {
	// Default ON (kill-switch JDEC_LIVEINTERVAL_WEB_OFF); see reachingSlotVersionByWeb for the empirical
	// rationale and the current 8-jar tree-inventory A/B that justifies the default flip.
	if os.Getenv("JDEC_LIVEINTERVAL_WEB_OFF") != "" {
		return nil
	}
	webs := d.slotWebs()
	if webs == nil || store == nil {
		return nil
	}
	storeWeb, ok := webs.webOf[store]
	if !ok {
		return nil
	}
	stores, _ := reachingStoresOf(store, slot) // prior same-slot definitions reaching this store
	var canon *values.JavaRef
	for _, st := range stores {
		if w, ok := webs.webOf[st]; !ok || w != storeWeb {
			continue
		}
		refs, ok := d.opcodeIdToRef[st]
		if !ok || len(refs) == 0 {
			continue
		}
		ref, ok := refs[len(refs)-1][0].(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		if canon == nil {
			canon = ref
		} else if canon.VarUid != ref.VarUid {
			return nil // multiple distinct prior versions in this web: leave to legacy handling
		}
	}
	if canon == nil || (current != nil && current.VarUid == canon.VarUid) {
		return nil
	}
	return canon
}
