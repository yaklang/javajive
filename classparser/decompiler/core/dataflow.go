package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
	webOf    map[*OpCode]int // store/load opcode -> web id
	entryWeb map[int]int     // slot -> web containing the method-entry definition
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
// deterministic (keyed by original bytecode order) so repeated decompiles are stable.
func (d *Decompiler) computeSlotWebs() *slotWeb {
	opcodes := d.opCodes
	if d.semanticCFG != nil {
		// Structuring may have removed a store from the mutable opcode list.
		// Reaching definitions and their identities must use the same snapshot.
		opcodes = d.semanticCFG.Nodes
	}
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
	for _, op := range opcodes {
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

	for _, op := range opcodes {
		if op == nil || op.Instr == nil || !isLocalLoadOpcode(op.Instr.OpCode) {
			continue
		}
		slot := GetRetrieveIdx(op)
		if slot < 0 {
			continue
		}
		stores, reachesEntry := d.reachingStores(op, slot)
		li := idx[op]
		for _, st := range stores {
			if si, found := idx[st]; found && GetStoreIdx(st) == slot {
				uf.union(li, si)
			}
		}
		if reachesEntry && slot <= maxSlot {
			uf.union(li, entryNode(slot))
		}
	}

	webOf := make(map[*OpCode]int, base)
	for op, i := range idx {
		webOf[op] = uf.find(i)
	}
	entryWeb := make(map[int]int, maxSlot+1)
	for slot := 0; slot <= maxSlot; slot++ {
		entryWeb[slot] = uf.find(entryNode(slot))
	}
	return &slotWeb{webOf: webOf, entryWeb: entryWeb}
}

// A parameter is a real reaching definition even though it has no store opcode.
// Preserve its identity when a conditional assignment rejoins the live-in value.
func (d *Decompiler) parameterWebRefs(webs *slotWeb) map[int]*values.JavaRef {
	result := map[int]*values.JavaRef{}
	slot := 0
	for _, value := range d.Params {
		ref, ok := value.(*values.JavaRef)
		if !ok {
			continue
		}
		if web, ok := webs.entryWeb[slot]; ok {
			result[web] = ref
			d.tracef("var-fold", "parameter slot=%d web=%d type=%s", slot, web, ref.Type().String(d.FunctionContext))
		}
		slot++
		if p, ok := ref.Type().RawType().(*types.JavaPrimer); ok && (p.Name == types.JavaLong || p.Name == types.JavaDouble) {
			slot++
		}
	}
	return result
}

// slotWebs returns the cached web partition, computing it on first use. Returns nil when the
// kill-switch JDEC_LIVEINTERVAL_OFF is set, so all web-driven repairs fall back to the legacy passes.
func (d *Decompiler) slotWebs() *slotWeb {
	if d.getenv("JDEC_LIVEINTERVAL_OFF") != "" {
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
	if d.getenv("JDEC_LIVEINTERVAL_WEB_OFF") != "" {
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
	stores, _ := d.reachingStores(load, slot)
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
	if d.getenv("JDEC_LIVEINTERVAL_WEB_OFF") != "" {
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
	stores, _ := d.reachingStores(store, slot) // prior same-slot definitions reaching this store
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

// isPureThisAliasStore reports whether storing `this` into `slot` at `store` belongs to a
// reaching-definition web isolated from method-entry slot state and free of non-`this` stores.
func (d *Decompiler) isPureThisAliasStore(store *OpCode, slot int) bool {
	if store == nil || slot <= 0 {
		return false
	}
	webs := d.slotWebs()
	if webs == nil {
		return false
	}
	storeWeb, ok := webs.webOf[store]
	if !ok {
		return false
	}
	if entryWeb, hasEntry := webs.entryWeb[slot]; hasEntry && entryWeb == storeWeb {
		return false
	}
	for op, w := range webs.webOf {
		if w != storeWeb || op == store || op == nil || op.Instr == nil {
			continue
		}
		if !isLocalStoreOpcode(op.Instr.OpCode) || GetStoreIdx(op) != slot {
			continue
		}
		if prevVal, seen := d.slotStoreValue[op]; seen {
			unpacked := values.UnpackSoltValue(prevVal)
			sr, ok := unpacked.(*values.JavaRef)
			if !ok || sr == nil || !(sr.IsThis || (sr.IsThisAlias && !d.nonThisAliasUids[sr.VarUid])) {
				return false
			}
		}
	}
	return true
}

// reachingPhiConvergentSameOrSuperStore resolves T4b diamond/computeIfAbsent stores:
// when a branch store `store` flows forward to a downstream phi load where all already-simulated
// reaching definitions share one non-null-initialized canonical variable `canon` whose declared type
// is equal to or a proven supertype of `val`, continue `canon` instead of splitting off a fresh
// block-scoped local. Kill-switch: JDEC_T4B_PHI_MERGE_OFF=1.
func (d *Decompiler) reachingPhiConvergentSameOrSuperStore(store *OpCode, slot int, val values.JavaValue) *values.JavaRef {
	if d.getenv("JDEC_T4B_PHI_MERGE_OFF") == "1" {
		return nil
	}
	if store == nil || val == nil || val.Type() == nil {
		return nil
	}
	vt := slotDeclType(val)
	if vt == nil {
		return nil
	}
	if values.IsNullLiteral(values.UnpackSoltValue(val)) {
		return nil
	}
	webs := d.slotWebs()
	ctx := &class_context.ClassContext{}
	vtName := vt.String(ctx)
	_, vtPrim := vt.RawType().(*types.JavaPrimer)
	var provider types.SuperTypeProvider
	if d.FunctionContext != nil {
		provider = d.FunctionContext.SiblingSuperTypes
	}

	visited := map[*OpCode]bool{store: true}
	queue := append([]*OpCode{}, store.Target...)
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		if cur == nil || cur.Instr == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		if isLocalStoreOpcode(cur.Instr.OpCode) && GetStoreIdx(cur) == slot {
			continue
		}
		if cur.Instr.OpCode == OP_IINC && GetStoreIdx(cur) == slot {
			continue
		}
		if isLocalLoadOpcode(cur.Instr.OpCode) && GetRetrieveIdx(cur) == slot {
			stores, reachesEntry := d.reachingStores(cur, slot)
			if reachesEntry {
				goto nextEdges
			}
			var (
				sawSelf   bool
				ambiguous bool
				canon     *values.JavaRef
				canonOp   *OpCode
			)
			for _, st := range stores {
				if st == store {
					sawSelf = true
					continue
				}
				refs, ok := d.opcodeIdToRef[st]
				if !ok || len(refs) == 0 {
					continue
				}
				stRef, ok := refs[len(refs)-1][0].(*values.JavaRef)
				if !ok || stRef == nil || stRef.IsThis || stRef.IsParam {
					ambiguous = true
					break
				}
				if canon == nil {
					canon = stRef
					canonOp = st
				} else if canon.VarUid != stRef.VarUid {
					ambiguous = true
					break
				}
			}
			if !sawSelf || ambiguous || canon == nil || canonOp == nil || canon.IsNullInitialized() {
				goto nextEdges
			}
			ct := canon.Type()
			if ct == nil {
				goto nextEdges
			}
			if webs != nil {
				sw, ok1 := webs.webOf[store]
				cw, ok2 := webs.webOf[canonOp]
				if !ok1 || !ok2 || sw != cw {
					goto nextEdges
				}
			}
			_, ctPrim := ct.RawType().(*types.JavaPrimer)
			if ctPrim != vtPrim {
				goto nextEdges
			}
			ctName := ct.String(ctx)
			compatible := ctName == vtName
			if !compatible && !ctPrim {
				if cFQN, ok1 := types.RawClassFQN(ct); ok1 {
					if vFQN, ok2 := types.RawClassFQN(vt); ok2 {
						compatible = types.IsReferenceSubtypeBridged(vFQN, cFQN, provider)
					}
				}
			}
			if compatible {
				return canon
			}
		}
	nextEdges:
		queue = append(queue, cur.Target...)
	}
	return nil
}

// singleUseStoreReachesOtherLoad reports whether a local store candidate for single-use folding
// reaches any slot load (or IINC) other than `soleUseOp`, so deleting the store statement would leave
// a downstream read on an unskipped/unbranching path uninitialized (T4b).
func (d *Decompiler) singleUseStoreReachesOtherLoad(storeOp, soleUseOp *OpCode, slot int, arrayVal *values.NewExpression) bool {
	if storeOp == nil || slot < 0 {
		return false
	}
	visited := map[*OpCode]bool{storeOp: true}
	queue := append([]*OpCode{}, storeOp.Target...)
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		if cur == nil || cur.Instr == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		if isLocalStoreOpcode(cur.Instr.OpCode) && GetStoreIdx(cur) == slot {
			continue
		}
		isLoad := isLocalLoadOpcode(cur.Instr.OpCode) && GetRetrieveIdx(cur) == slot
		isIinc := cur.Instr.OpCode == OP_IINC && GetStoreIdx(cur) == slot
		if isLoad || isIinc {
			skipWindow := arrayVal != nil && arrayVal.IsArray() && arrayVal.HasEvaluationEndPC && int(cur.CurrentOffset) <= arrayVal.EvaluationEndPC
			if cur != soleUseOp && !skipWindow {
				stores, _ := d.reachingStores(cur, slot)
				for _, st := range stores {
					if st == storeOp {
						return true
					}
				}
			}
		}
		if isIinc {
			continue
		}
		queue = append(queue, cur.Target...)
	}
	return false
}

func (d *Decompiler) isThisOrAliasRef(ref *values.JavaRef) bool {
	if ref == nil {
		return false
	}
	if ref.IsThis {
		return true
	}
	return d.getenv("JDEC_THIS_ALIAS_SUPER_OFF") == "" && ref.IsThisAlias && !d.nonThisAliasUids[ref.VarUid]
}

func (d *Decompiler) markThisAliasStore(opcode *OpCode, slot int, value values.JavaValue, oldRef, ref *values.JavaRef, isFirst bool) {
	if d.getenv("JDEC_THIS_ALIAS_SUPER_OFF") != "" || d.FunctionContext == nil || d.FunctionContext.IsStatic || ref == nil {
		return
	}
	if d.nonThisAliasUids == nil {
		d.nonThisAliasUids = map[string]bool{}
	}
	if d.thisAliasRefs == nil {
		d.thisAliasRefs = map[string][]*values.JavaRef{}
	}
	clearUidAlias := func(uid string) {
		if uid == "" {
			return
		}
		d.nonThisAliasUids[uid] = true
		ref.IsThisAlias = false
		for _, tracked := range d.thisAliasRefs[uid] {
			if tracked != nil {
				tracked.IsThisAlias = false
			}
		}
	}
	if slot == 0 {
		if oldRef != nil && oldRef.IsThis && oldRef.VarUid == ref.VarUid {
			clearUidAlias(ref.VarUid)
		}
		return
	}
	unpacked := values.UnpackSoltValue(value)
	srcRef, ok := unpacked.(*values.JavaRef)
	isThisVal := ok && srcRef != nil && (srcRef.IsThis || (srcRef.IsThisAlias && !d.nonThisAliasUids[srcRef.VarUid]))
	if isFirst {
		if isThisVal && !d.nonThisAliasUids[ref.VarUid] && d.isPureThisAliasStore(opcode, slot) {
			ref.IsThisAlias = true
			d.thisAliasRefs[ref.VarUid] = append(d.thisAliasRefs[ref.VarUid], ref)
		} else if !isThisVal {
			d.nonThisAliasUids[ref.VarUid] = true
		}
		return
	}
	if !isThisVal {
		clearUidAlias(ref.VarUid)
		return
	}
	if ref.IsThisAlias && !d.nonThisAliasUids[ref.VarUid] {
		d.thisAliasRefs[ref.VarUid] = append(d.thisAliasRefs[ref.VarUid], ref)
	}
}
