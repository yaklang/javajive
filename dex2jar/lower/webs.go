package lower

import (
	"math"
	"sort"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/internal/jdecenv"
)

type regUseKey struct {
	index int
	reg   uint16
}

type webPolyKey struct {
	root int
	kind classfilebuilder.LocalKind
}

func categoryMaskBit(kind ir.CategoryKind, alloc uint32, desc string) (narrowBit uint8, isWide bool) {
	switch kind {
	case ir.Int:
		return ir.PolyZeroInt, false
	case ir.Bits32:
		return 0, false
	case ir.Float:
		return ir.PolyZeroFloat, false
	case ir.Reference, ir.Uninitialized:
		return ir.PolyZeroRef, false
	case ir.Null:
		if desc != "" || (alloc != 0 && alloc != ir.PolyZeroAllocation) {
			return ir.PolyZeroRef, false
		}
		return 0, false
	case ir.Long, ir.Double, ir.Bits64:
		return 0, true
	default:
		return 0, false
	}
}

func hasMultipleNarrowBits(mask uint8) bool {
	return mask&(mask-1) != 0
}

type defKindInfo struct {
	reg         uint16
	width       int
	isEntry     bool
	paramLocal  uint16
	instruction int
}

// defSet is an immutable ascending set of definition IDs reaching a register.
type defSet []int

func unionDefSets(left, right defSet) defSet {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	if defSetsEqual(left, right) {
		return left
	}
	merged := make(defSet, 0, len(left)+len(right))
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		if left[i] < right[j] {
			merged = append(merged, left[i])
			i++
		} else if right[j] < left[i] {
			merged = append(merged, right[j])
			j++
		} else {
			merged = append(merged, left[i])
			i++
			j++
		}
	}
	merged = append(merged, left[i:]...)
	merged = append(merged, right[j:]...)
	return merged
}

func defSetsEqual(left, right defSet) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

type defState map[uint16]defSet

func copyDefState(src defState) defState {
	out := make(defState, len(src))
	for r, s := range src {
		out[r] = s
	}
	return out
}

func mergeDefState(dst, src defState) bool {
	changed := false
	for r, s := range src {
		u := unionDefSets(dst[r], s)
		if !defSetsEqual(dst[r], u) {
			dst[r] = u
			changed = true
		}
	}
	return changed
}

func defStatesEqual(left, right defState) bool {
	if len(left) != len(right) {
		return false
	}
	for r, s := range left {
		if !defSetsEqual(s, right[r]) {
			return false
		}
	}
	return true
}

func (l *methodLowerer) instructionDefWidth(idx int, reg uint16, defs []uint16) int {
	if len(defs) == 2 && defs[0] == reg && defs[1] == reg+1 {
		return 2
	}
	if def, ok := l.method.DefinitionAt(idx); ok && def.Register == int32(reg) {
		return def.Category.Kind.Words()
	}
	return 1
}

// assignHomes splits disjoint DEX register def-use webs into independent JVM
// locals while falling back cleanly to assignHomesLegacy when disabled or on
// pinned/oversized methods.
func (l *methodLowerer) assignHomes() error {
	if jdecenv.Get("JDEC_DEX_WEBSPLIT_OFF") == "1" {
		return l.assignHomesLegacy()
	}
	if !l.assignWebs() {
		return l.assignHomesLegacy()
	}
	return nil
}

func (l *methodLowerer) assignWebs() bool {
	registerCount := int(l.method.Code.RegistersSize)
	if registerCount == 0 || len(l.method.Blocks) == 0 {
		return false
	}
	if len(l.method.Blocks)*registerCount > 200000 {
		return false
	}

	widths := make([]int, registerCount)
	for r := range widths {
		widths[r] = 1
	}
	for r, cat := range l.method.RegisterTypes {
		if r < registerCount && cat.Kind.Words() == 2 {
			widths[r] = 2
		}
	}
	if stride := l.method.CategoryStride; stride > 0 {
		for offset, cat := range l.method.Categories {
			if cat.Kind.Words() == 2 {
				if r := offset % stride; r < registerCount {
					widths[r] = 2
				}
			}
		}
	}

	pinned := make([]bool, registerCount)
	unreachableTouched := make([]bool, registerCount)
	for r, mask := range l.method.PolyZeroRegisters {
		if mask != 0 && r < registerCount {
			pinned[r] = true
		}
	}
	for _, alloc := range l.method.Allocations {
		if alloc.NewInstruction >= 0 && alloc.NewInstruction < len(l.method.Instructions) {
			regs := l.method.Instructions[alloc.NewInstruction].Regs
			if len(regs) > 0 && int(regs[0]) < registerCount {
				pinned[regs[0]] = true
			}
		}
		for _, site := range alloc.Constructors {
			if site >= 0 && site < len(l.method.Instructions) {
				regs := l.method.Instructions[site].Regs
				if len(regs) > 0 && int(regs[0]) < registerCount {
					pinned[regs[0]] = true
				}
			}
		}
		for _, alias := range alloc.AliasRegisters {
			if int(alias) < registerCount {
				pinned[alias] = true
			}
		}
	}

	for _, block := range l.method.Blocks {
		if !block.Reachable {
			for _, idx := range block.Instructions {
				uses, defs := l.method.EffectsAt(idx)
				for _, u := range uses {
					if int(u) < registerCount {
						unreachableTouched[u] = true
					}
				}
				for _, d := range defs {
					if int(d) < registerCount {
						unreachableTouched[d] = true
					}
				}
			}
			continue
		}
		for _, idx := range block.Instructions {
			insn := l.method.Instructions[idx]
			if insn.Opcode >= 0x01 && insn.Opcode <= 0x03 && len(insn.Regs) >= 2 {
				if srcCat := l.method.CategoryAt(idx, insn.Regs[1]); srcCat.Kind == ir.Null && srcCat.Allocation == ir.PolyZeroAllocation {
					if int(insn.Regs[0]) < registerCount {
						pinned[insn.Regs[0]] = true
					}
					if int(insn.Regs[1]) < registerCount {
						pinned[insn.Regs[1]] = true
					}
				}
			}
			uses, defs := l.method.EffectsAt(idx)
			for _, r := range uses {
				if int(r) < registerCount && l.method.CategoryAt(idx, r).Kind == ir.Uninitialized {
					pinned[r] = true
				}
			}
			for _, r := range defs {
				if int(r) < registerCount && l.method.CategoryAt(idx, r).Kind == ir.Uninitialized {
					pinned[r] = true
				}
			}
		}
	}

	// Collect definitions: entry parameter pseudo-defs first, then instruction defs.
	var defs []defKindInfo
	entryDefByReg := make([]int, registerCount)
	for i := range entryDefByReg {
		entryDefByReg[i] = -1
	}
	paramStart := registerCount - int(l.method.Code.InsSize)
	type paramEntry struct {
		reg      uint16
		jvmLocal uint16
		words    int
		lk       classfilebuilder.LocalKind
	}
	var params []paramEntry
	pReg := paramStart
	jvmLocal := 0
	if l.encoded.AccessFlags&dexStatic == 0 {
		if pReg >= 0 && pReg < registerCount {
			params = append(params, paramEntry{reg: uint16(pReg), jvmLocal: uint16(jvmLocal), words: 1, lk: classfilebuilder.RefLocal})
		}
		pReg++
		jvmLocal++
	}
	for _, descriptor := range l.method.Proto.Parameters {
		words := descriptorWords(descriptor)
		pk := classfilebuilder.RefLocal
		if k, err := fieldKind(descriptor); err == nil {
			pk = localKind(k)
		}
		if pReg >= 0 && pReg+words <= registerCount {
			params = append(params, paramEntry{reg: uint16(pReg), jvmLocal: uint16(jvmLocal), words: words, lk: pk})
		}
		pReg += words
		jvmLocal += words
	}
	for _, p := range params {
		if pinned[p.reg] {
			continue
		}
		id := len(defs)
		defs = append(defs, defKindInfo{
			reg:         p.reg,
			width:       p.words,
			isEntry:     true,
			paramLocal:  p.jvmLocal,
			instruction: -1,
		})
		entryDefByReg[p.reg] = id
	}

	instDefID := make(map[regUseKey]int)
	wideInsnDef := make(map[int]uint16) // insn idx -> base reg if wide def
	for _, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		for _, idx := range block.Instructions {
			_, effDefs := l.method.EffectsAt(idx)
			if len(effDefs) == 0 {
				if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 && int(def.Register) < registerCount {
					r := uint16(def.Register)
					w := def.Category.Kind.Words()
					if w == 2 {
						wideInsnDef[idx] = r
					}
					if !pinned[r] {
						id := len(defs)
						defs = append(defs, defKindInfo{reg: r, width: w, instruction: idx})
						instDefID[regUseKey{index: idx, reg: r}] = id
					}
				}
				continue
			}
			r := effDefs[0]
			if int(r) >= registerCount {
				continue
			}
			w := l.instructionDefWidth(idx, r, effDefs)
			if w == 2 {
				wideInsnDef[idx] = r
			}
			if !pinned[r] {
				id := len(defs)
				defs = append(defs, defKindInfo{reg: r, width: w, instruction: idx})
				instDefID[regUseKey{index: idx, reg: r}] = id
			}
		}
	}
	if len(defs) > 65535 || len(l.method.Instructions)*len(defs) > 100000 {
		return false
	}

	transferDefs := func(idx int, state defState) {
		_, effDefs := l.method.EffectsAt(idx)
		var base uint16
		hasDef := false
		width := 1
		if len(effDefs) > 0 {
			base = effDefs[0]
			hasDef = true
			width = l.instructionDefWidth(idx, base, effDefs)
		} else if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 {
			base = uint16(def.Register)
			hasDef = true
			width = def.Category.Kind.Words()
		}
		if !hasDef || int(base) >= registerCount {
			return
		}
		// Writing base kills any active wide def that starts at base-1.
		if base > 0 {
			if prev, ok := state[base-1]; ok {
				var filtered defSet
				for _, d := range prev {
					if defs[d].width < 2 {
						filtered = append(filtered, d)
					}
				}
				if len(filtered) == 0 {
					delete(state, base-1)
				} else if len(filtered) != len(prev) {
					state[base-1] = filtered
				}
			}
		}
		if width == 2 && int(base)+1 < registerCount {
			delete(state, base+1)
		}
		if pinned[base] {
			delete(state, base)
			return
		}
		if d, ok := instDefID[regUseKey{index: idx, reg: base}]; ok {
			state[base] = defSet{d}
		}
	}

	numBlocks := len(l.method.Blocks)
	reachIn := make([]defState, numBlocks)
	reachOut := make([]defState, numBlocks)
	for i := range reachIn {
		reachIn[i] = make(defState)
		reachOut[i] = make(defState)
	}
	entryState := make(defState)
	for _, p := range params {
		if id := entryDefByReg[p.reg]; id >= 0 {
			entryState[p.reg] = defSet{id}
		}
	}
	reachIn[0] = copyDefState(entryState)

	handlerEntryDefs := func(blockIdx int) defState {
		merged := make(defState)
		protected := l.method.Blocks[blockIdx].Protected
		if len(protected) == 0 {
			for _, pred := range l.method.Blocks[blockIdx].Predecessors {
				if l.method.Blocks[pred].Reachable {
					mergeDefState(merged, reachOut[pred])
				}
			}
			return merged
		}
		byBlock := make(map[int]map[int]bool)
		for _, insnIdx := range protected {
			b := l.blockOfInstruction(insnIdx)
			if b < 0 || !l.method.Blocks[b].Reachable {
				continue
			}
			if byBlock[b] == nil {
				byBlock[b] = make(map[int]bool)
			}
			byBlock[b][insnIdx] = true
		}
		for _, pred := range l.method.Blocks[blockIdx].Predecessors {
			if !l.method.Blocks[pred].Reachable {
				continue
			}
			if byBlock[pred] == nil || l.blockHasNormalEdgeTo(pred, blockIdx) {
				mergeDefState(merged, reachOut[pred])
			}
		}
		for b, wanted := range byBlock {
			st := copyDefState(reachIn[b])
			for _, insnIdx := range l.method.Blocks[b].Instructions {
				if wanted[insnIdx] {
					mergeDefState(merged, st)
				}
				transferDefs(insnIdx, st)
			}
		}
		return merged
	}

	protectedHandlersByBlock := make([][]int, numBlocks)
	protectedSeen := make([]map[int]bool, numBlocks)
	for blockIdx, block := range l.method.Blocks {
		if !block.Handler || !block.Reachable {
			continue
		}
		addHandlerTarget := func(b int) {
			if b < 0 || b >= numBlocks || !l.method.Blocks[b].Reachable {
				return
			}
			if protectedSeen[b] == nil {
				protectedSeen[b] = make(map[int]bool)
			}
			if !protectedSeen[b][blockIdx] {
				protectedSeen[b][blockIdx] = true
				protectedHandlersByBlock[b] = append(protectedHandlersByBlock[b], blockIdx)
			}
		}
		for _, pred := range block.Predecessors {
			addHandlerTarget(pred)
		}
		for _, insnIdx := range block.Protected {
			addHandlerTarget(l.blockOfInstruction(insnIdx))
		}
	}

	inQueue := make([]bool, numBlocks)
	worklist := make([]int, 0, numBlocks)
	for i := range l.method.Blocks {
		if l.method.Blocks[i].Reachable {
			inQueue[i] = true
			worklist = append(worklist, i)
		}
	}
	maxSteps := numBlocks*32 + 512
	steps := 0
	for len(worklist) > 0 {
		steps++
		if steps > maxSteps {
			return false
		}
		b := worklist[0]
		worklist = worklist[1:]
		inQueue[b] = false
		var merged defState
		if l.method.Blocks[b].Handler {
			merged = handlerEntryDefs(b)
		} else {
			merged = make(defState)
			for _, pred := range l.method.Blocks[b].Predecessors {
				if l.method.Blocks[pred].Reachable {
					mergeDefState(merged, reachOut[pred])
				}
			}
		}
		if b == 0 {
			mergeDefState(merged, entryState)
		}
		inChanged := !defStatesEqual(reachIn[b], merged)
		reachIn[b] = merged
		st := copyDefState(reachIn[b])
		for _, insnIdx := range l.method.Blocks[b].Instructions {
			transferDefs(insnIdx, st)
		}
		outChanged := !defStatesEqual(reachOut[b], st)
		if outChanged {
			reachOut[b] = st
			for _, succ := range l.method.Blocks[b].Successors {
				if l.method.Blocks[succ].Reachable && !inQueue[succ] {
					inQueue[succ] = true
					worklist = append(worklist, succ)
				}
			}
		}
		if inChanged || outChanged {
			for _, h := range protectedHandlersByBlock[b] {
				if !inQueue[h] {
					inQueue[h] = true
					worklist = append(worklist, h)
				}
			}
		}
	}
	_ = wideInsnDef

	// Build Union-Find webs across all instruction uses.
	ufParent := make([]int, len(defs))
	webWidth := make([]int, len(defs))
	webHasInstDef := make([]bool, len(defs))
	for i, d := range defs {
		ufParent[i] = i
		webWidth[i] = d.width
		webHasInstDef[i] = !d.isEntry
	}
	var ufFind func(int) int
	ufFind = func(x int) int {
		for ufParent[x] != x {
			ufParent[x] = ufParent[ufParent[x]]
			x = ufParent[x]
		}
		return x
	}
	ufUnion := func(a, b int) int {
		ra, rb := ufFind(a), ufFind(b)
		if ra == rb {
			return ra
		}
		if ra > rb {
			ra, rb = rb, ra
		}
		ufParent[rb] = ra
		if webWidth[rb] > webWidth[ra] {
			webWidth[ra] = webWidth[rb]
		}
		if webHasInstDef[rb] {
			webHasInstDef[ra] = true
		}
		return ra
	}

	useWebDef := make(map[regUseKey]int)
	for blockIdx, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		st := copyDefState(reachIn[blockIdx])
		for _, idx := range block.Instructions {
			uses, _ := l.method.EffectsAt(idx)
			prevWide := false
			var prevReg uint16
			for _, u := range uses {
				if prevWide && u == prevReg+1 {
					prevWide = false
					continue
				}
				cat := l.method.CategoryAt(idx, u)
				if cat.Kind == ir.WideUpper {
					prevWide = false
					continue
				}
				prevWide = isWideKind(cat.Kind)
				prevReg = u
				if int(u) >= registerCount || pinned[u] {
					continue
				}
				reaching := st[u]
				if len(reaching) == 0 {
					pinned[u] = true
					continue
				}
				root := reaching[0]
				for _, d := range reaching[1:] {
					root = ufUnion(root, d)
				}
				root = ufFind(root)
				if w := cat.Kind.Words(); w > webWidth[root] {
					webWidth[root] = w
				}
				useWebDef[regUseKey{index: idx, reg: u}] = reaching[0]
			}
			transferDefs(idx, st)
		}
	}

	// Collect per-web and per-pinned-register concrete kind masks so untyped
	// zero constants stay consistent with their web and mixed-kind webs or
	// pinned registers get separate JVM locals per LocalKind.
	webKindMask := make([]uint8, len(defs))
	webHasWide := make([]bool, len(defs))
	pinnedNarrowMask := make([]uint8, registerCount)
	pinnedHasWide := make([]bool, registerCount)
	for _, p := range params {
		d := entryDefByReg[p.reg]
		if d < 0 {
			continue
		}
		root := ufFind(d)
		switch p.lk {
		case classfilebuilder.IntLocal:
			webKindMask[root] |= ir.PolyZeroInt
		case classfilebuilder.FloatLocal:
			webKindMask[root] |= ir.PolyZeroFloat
		case classfilebuilder.RefLocal:
			webKindMask[root] |= ir.PolyZeroRef
		case classfilebuilder.LongLocal, classfilebuilder.DoubleLocal:
			webHasWide[root] = true
		}
	}
	recordCat := func(pinnedReg int, defIdx int, hasWeb bool, cat ir.Category) {
		bit, wide := categoryMaskBit(cat.Kind, cat.Allocation, cat.Descriptor)
		if bit == 0 && !wide {
			return
		}
		if hasWeb {
			root := ufFind(defIdx)
			webKindMask[root] |= bit
			if wide {
				webHasWide[root] = true
			}
			return
		}
		if pinnedReg >= 0 && pinnedReg < registerCount && pinned[pinnedReg] {
			pinnedNarrowMask[pinnedReg] |= bit
			if wide {
				pinnedHasWide[pinnedReg] = true
			}
		}
	}
	for _, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		for _, idx := range block.Instructions {
			_, effDefs := l.method.EffectsAt(idx)
			var defReg int = -1
			if len(effDefs) > 0 {
				defReg = int(effDefs[0])
			} else if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 {
				defReg = int(def.Register)
			}
			if defReg >= 0 && defReg < registerCount {
				cat := l.definitionCategoryAt(idx, uint16(defReg))
				if !pinned[defReg] {
					if d, ok := instDefID[regUseKey{index: idx, reg: uint16(defReg)}]; ok {
						recordCat(-1, d, true, cat)
					}
				} else {
					recordCat(defReg, 0, false, cat)
				}
			}
			uses, _ := l.method.EffectsAt(idx)
			for _, u := range uses {
				if int(u) >= registerCount {
					continue
				}
				cat := l.method.CategoryAt(idx, u)
				if cat.Kind == ir.WideUpper {
					continue
				}
				if !pinned[u] {
					if d, ok := useWebDef[regUseKey{index: idx, reg: u}]; ok {
						recordCat(-1, d, true, cat)
					}
				} else {
					recordCat(int(u), 0, false, cat)
				}
			}
		}
	}

	// Assign JVM slots to webs and fallback homes for pinned/unreachable registers.
	l.directParamSlots = make([]int, registerCount)
	l.paramPrologueSlot = make([]int, registerCount)
	for i := range l.directParamSlots {
		l.directParamSlots[i] = -1
		l.paramPrologueSlot[i] = -1
	}

	webSlots := make([]uint16, len(defs))
	webAssigned := make([]bool, len(defs))
	directParamRootKind := make(map[int]classfilebuilder.LocalKind)
	directWideParam := make([]bool, registerCount)
	next := 0

	// First assign parameter entry webs (direct incoming local if it matches the
	// incoming parameter width and does not alias an <init> receiver slot,
	// otherwise prologue copy into a dedicated home >= homeBase).
	for _, p := range params {
		if pinned[p.reg] {
			continue
		}
		d := entryDefByReg[p.reg]
		if d < 0 {
			continue
		}
		root := ufFind(d)
		if webAssigned[root] {
			continue
		}
		isInitThis := l.encoded.AccessFlags&dexStatic == 0 && p.jvmLocal == 0 && l.encoded.Method.Name == "<init>"
		if webWidth[root] == p.words && !(isInitThis && webHasInstDef[root]) && (!unreachableTouched[p.reg] || widths[p.reg] == p.words) && (p.words == 1 || !unreachableTouched[p.reg+1]) {
			webSlots[root] = p.jvmLocal
			webAssigned[root] = true
			l.directParamSlots[p.reg] = int(p.jvmLocal)
			directParamRootKind[root] = p.lk
			if p.words == 2 {
				directWideParam[p.reg] = true
			}
		} else {
			if int(l.homeBase)+next+webWidth[root] > math.MaxUint16 {
				return false
			}
			slot := l.homeBase + uint16(next)
			next += webWidth[root]
			webSlots[root] = slot
			webAssigned[root] = true
			l.paramPrologueSlot[p.reg] = int(slot)
		}
	}

	// Assign remaining webs in deterministic defID order.
	for i := range defs {
		root := ufFind(i)
		if pinned[defs[i].reg] || webAssigned[root] {
			continue
		}
		if int(l.homeBase)+next+webWidth[root] > math.MaxUint16 {
			return false
		}
		webSlots[root] = l.homeBase + uint16(next)
		webAssigned[root] = true
		next += webWidth[root]
	}

	// Allocate companion 1-word slots for webs that mix multiple narrow LocalKinds
	// or combine wide and narrow operations on the same Union-Find root.
	l.webPolySlots = nil
	l.webPolyMask = nil
	for i := range defs {
		root := ufFind(i)
		if root != i || !webAssigned[root] {
			continue
		}
		mask := webKindMask[root]
		if !hasMultipleNarrowBits(mask) && !(webHasWide[root] && mask != 0) {
			continue
		}
		if l.webPolySlots == nil {
			l.webPolySlots = make(map[webPolyKey]uint16)
			l.webPolyMask = make(map[int]uint8)
		}
		l.webPolyMask[root] = mask
		directKind, hasDirectKind := directParamRootKind[root]
		reusedBase := false
		if hasDirectKind && !webHasWide[root] {
			for _, entry := range []struct {
				bit  uint8
				kind classfilebuilder.LocalKind
			}{
				{ir.PolyZeroRef, classfilebuilder.RefLocal},
				{ir.PolyZeroInt, classfilebuilder.IntLocal},
				{ir.PolyZeroFloat, classfilebuilder.FloatLocal},
			} {
				if mask&entry.bit != 0 && entry.kind == directKind {
					l.webPolySlots[webPolyKey{root: root, kind: entry.kind}] = webSlots[root]
					reusedBase = true
				}
			}
		}
		first := !reusedBase && !hasDirectKind && !webHasWide[root]
		for _, entry := range []struct {
			bit  uint8
			kind classfilebuilder.LocalKind
		}{
			{ir.PolyZeroRef, classfilebuilder.RefLocal},
			{ir.PolyZeroInt, classfilebuilder.IntLocal},
			{ir.PolyZeroFloat, classfilebuilder.FloatLocal},
		} {
			if mask&entry.bit == 0 {
				continue
			}
			if _, exists := l.webPolySlots[webPolyKey{root: root, kind: entry.kind}]; exists {
				continue
			}
			var slot uint16
			if first {
				slot = webSlots[root]
				first = false
			} else {
				if int(l.homeBase)+next >= math.MaxUint16 {
					return false
				}
				slot = l.homeBase + uint16(next)
				next++
			}
			l.webPolySlots[webPolyKey{root: root, kind: entry.kind}] = slot
		}
	}

	// Check which pinned registers can still be direct read-only parameters,
	// and allocate homeSlots/polySlots for all pinned or unreachable-only registers.
	writtenPinned := make([]bool, registerCount)
	for idx := range l.method.Instructions {
		_, effDefs := l.method.EffectsAt(idx)
		for _, r := range effDefs {
			if int(r) < registerCount {
				writtenPinned[r] = true
			}
		}
		if def, ok := l.method.DefinitionAt(idx); ok && def.Register >= 0 && int(def.Register) < registerCount {
			writtenPinned[def.Register] = true
		}
	}
	for _, p := range params {
		if !pinned[p.reg] {
			continue
		}
		if widths[p.reg] == p.words && !writtenPinned[p.reg] && (p.words == 1 || (!writtenPinned[p.reg+1] && widths[p.reg+1] == 1)) {
			l.directParamSlots[p.reg] = int(p.jvmLocal)
			if p.words == 2 {
				directWideParam[p.reg] = true
			}
		}
	}

	l.registerWidths = widths
	l.homeSlots = make([]uint16, registerCount)
	hasLegacyHome := make([]bool, registerCount)
	for r := 0; r < registerCount; r++ {
		if (!pinned[r] && !unreachableTouched[r]) || l.directParamSlots[r] >= 0 {
			continue
		}
		// Keep upper half free when already covered as a genuine 2-word direct
		// wide param that is never independently written or polymorphic-zero.
		if r > 0 && directWideParam[r-1] && !writtenPinned[r] && (r >= len(l.method.PolyZeroRegisters) || l.method.PolyZeroRegisters[r] == 0) {
			continue
		}
		if int(l.homeBase)+next+widths[r] > math.MaxUint16 {
			return false
		}
		l.homeSlots[r] = uint16(next)
		hasLegacyHome[r] = true
		next += widths[r]
	}
	l.polySlots = nil
	for r := 0; r < registerCount; r++ {
		var polyMask uint8
		if r < len(l.method.PolyZeroRegisters) {
			polyMask = l.method.PolyZeroRegisters[r]
		}
		if pinned[r] && (hasMultipleNarrowBits(pinnedNarrowMask[r]) || (pinnedNarrowMask[r] != 0 && pinnedHasWide[r])) {
			polyMask |= pinnedNarrowMask[r]
		}
		if polyMask == 0 || l.directParamSlots[r] >= 0 {
			continue
		}
		if !hasLegacyHome[r] {
			if int(l.homeBase)+next+widths[r] > math.MaxUint16 {
				return false
			}
			l.homeSlots[r] = uint16(next)
			hasLegacyHome[r] = true
			next += widths[r]
		}
		if l.polySlots == nil {
			l.polySlots = make(map[polySlotKey]uint16)
		}
		first := widths[r] == 1 && !pinnedHasWide[r]
		for _, entry := range []struct {
			bit  uint8
			kind classfilebuilder.LocalKind
		}{
			{ir.PolyZeroRef, classfilebuilder.RefLocal},
			{ir.PolyZeroInt, classfilebuilder.IntLocal},
			{ir.PolyZeroFloat, classfilebuilder.FloatLocal},
		} {
			if polyMask&entry.bit == 0 {
				continue
			}
			var slot uint16
			if first {
				slot = l.home(uint16(r))
				first = false
			} else {
				if int(l.homeBase)+next >= math.MaxUint16 {
					return false
				}
				slot = l.homeBase + uint16(next)
				next++
			}
			l.polySlots[polySlotKey{reg: uint16(r), kind: entry.kind}] = slot
		}
	}
	if int(l.homeBase)+next > math.MaxUint16 {
		return false
	}

	// Record resolved single narrow kinds for webs and pinned registers whose
	// only concrete uses/defs are Int or Float (and never Wide/Ref/PolyZero),
	// plus resolve untyped zero instructions (const/4..const #0).
	l.webZeroKind = nil
	l.webSingleZeroKind = nil
	l.pinnedZeroKind = nil
	for i := range defs {
		root := ufFind(i)
		if root != i || !webAssigned[root] || webHasWide[root] {
			continue
		}
		if webKindMask[root] == ir.PolyZeroInt || webKindMask[root] == ir.PolyZeroFloat {
			if l.webSingleZeroKind == nil {
				l.webSingleZeroKind = make(map[int]ir.CategoryKind)
			}
			k := ir.Int
			if webKindMask[root] == ir.PolyZeroFloat {
				k = ir.Float
			}
			l.webSingleZeroKind[root] = k
		}
	}
	for r := 0; r < registerCount; r++ {
		if !pinned[r] || pinnedHasWide[r] {
			continue
		}
		if r < len(l.method.PolyZeroRegisters) && l.method.PolyZeroRegisters[r] != 0 {
			continue
		}
		if pinnedNarrowMask[r] == ir.PolyZeroInt || pinnedNarrowMask[r] == ir.PolyZeroFloat {
			if l.pinnedZeroKind == nil {
				l.pinnedZeroKind = make(map[uint16]ir.CategoryKind)
			}
			k := ir.Int
			if pinnedNarrowMask[r] == ir.PolyZeroFloat {
				k = ir.Float
			}
			l.pinnedZeroKind[uint16(r)] = k
		}
	}
	for _, block := range l.method.Blocks {
		if !block.Reachable {
			continue
		}
		for _, idx := range block.Instructions {
			insn := &l.method.Instructions[idx]
			if insn.Opcode < 0x12 || insn.Opcode > 0x15 || len(insn.Regs) == 0 {
				continue
			}
			r := insn.Regs[0]
			if int(r) >= registerCount {
				continue
			}
			defCat := l.definitionCategoryAt(idx, r)
			isUntypedZero := insn.Literal == 0 && defCat.Kind == ir.Null && defCat.Allocation == 0
			isBits32 := defCat.Kind == ir.Bits32
			if !isUntypedZero && !isBits32 {
				continue
			}
			var k ir.CategoryKind
			if !pinned[r] {
				if d, ok := instDefID[regUseKey{index: idx, reg: r}]; ok {
					k = l.webSingleZeroKind[ufFind(d)]
				}
			} else {
				k = l.pinnedZeroKind[r]
			}
			if isBits32 && k != ir.Int && k != ir.Float {
				continue
			}
			if k != ir.Unknown {
				if l.webZeroKind == nil {
					l.webZeroKind = make(map[int]ir.CategoryKind)
				}
				l.webZeroKind[idx] = k
			}
		}
	}

	// Build sorted offset -> block instructions lookup for emitDebug resolution.
	l.webSplitting = true
	l.webPinned = pinned
	l.hasLegacyHome = hasLegacyHome
	l.webDefs = defs
	l.ufParent = ufParent
	l.webSlots = webSlots
	l.webAssigned = webAssigned
	l.instDefID = instDefID
	l.useWebDef = useWebDef
	l.entryDefByReg = entryDefByReg
	l.webReachIn = reachIn
	l.homeLimit = uint16(next)
	return true
}

func (l *methodLowerer) ufFindRoot(x int) int {
	for l.ufParent[x] != x {
		l.ufParent[x] = l.ufParent[l.ufParent[x]]
		x = l.ufParent[x]
	}
	return x
}

func (l *methodLowerer) loadSlotAt(index int, register uint16, kind classfilebuilder.LocalKind) uint16 {
	if l.webSplitting && int(register) < len(l.webPinned) && !l.webPinned[register] {
		if d, ok := l.useWebDef[regUseKey{index: index, reg: register}]; ok {
			root := l.ufFindRoot(d)
			if kind != classfilebuilder.LongLocal && kind != classfilebuilder.DoubleLocal {
				if slot, exists := l.webPolySlots[webPolyKey{root: root, kind: kind}]; exists {
					return slot
				}
			}
			if l.webAssigned[root] {
				return l.webSlots[root]
			}
		}
	}
	return l.slotFor(register, kind)
}

func (l *methodLowerer) storeSlotAt(index int, register uint16, kind classfilebuilder.LocalKind) uint16 {
	if l.webSplitting && int(register) < len(l.webPinned) && !l.webPinned[register] {
		if d, ok := l.instDefID[regUseKey{index: index, reg: register}]; ok {
			root := l.ufFindRoot(d)
			if kind != classfilebuilder.LongLocal && kind != classfilebuilder.DoubleLocal {
				if slot, exists := l.webPolySlots[webPolyKey{root: root, kind: kind}]; exists {
					return slot
				}
			}
			if l.webAssigned[root] {
				return l.webSlots[root]
			}
		}
	}
	return l.slotFor(register, kind)
}

// debugWebSlot locates a concrete JVM local slot for DEX debug local range [start, end).
// Returns (slot, true) if an exact active web or pinned home owns the local.
func (l *methodLowerer) debugWebSlot(register uint16, start, end uint32, kind ir.CategoryKind) (uint16, bool) {
	if !l.webSplitting {
		return l.slotFor(register, localKind(kind)), true
	}
	if int(register) >= len(l.webPinned) {
		return 0, false
	}
	if l.webPinned[register] {
		if l.directParamSlots[register] >= 0 || (int(register) < len(l.hasLegacyHome) && l.hasLegacyHome[register]) {
			return l.slotFor(register, localKind(kind)), true
		}
		return 0, false
	}
	// Check instruction defs and uses inside [start, end), ordered by offset, plus
	// defs ending cleanly at start (d8 emits DBG_START_LOCAL right after def).
	insns := l.method.Instructions
	firstIdx := sort.Search(len(insns), func(i int) bool {
		return insns[i].End() >= start
	})
	for i := firstIdx; i < len(insns) && insns[i].Offset < end; i++ {
		if insns[i].End() == start || (insns[i].Offset >= start && insns[i].Offset < end) {
			if d, ok := l.instDefID[regUseKey{index: i, reg: register}]; ok {
				root := l.ufFindRoot(d)
				if l.webAssigned[root] {
					return l.webSlots[root], true
				}
			}
		}
		if insns[i].Offset >= start && insns[i].Offset < end {
			if d, ok := l.useWebDef[regUseKey{index: i, reg: register}]; ok {
				root := l.ufFindRoot(d)
				if l.webAssigned[root] {
					return l.webSlots[root], true
				}
			}
		}
	}
	if start == 0 {
		if d := l.entryDefByReg[register]; d >= 0 {
			root := l.ufFindRoot(d)
			if l.webAssigned[root] {
				return l.webSlots[root], true
			}
		}
	}
	return 0, false
}

func (l *methodLowerer) blockHasNormalEdgeTo(pred, target int) bool {
	insns := l.method.Blocks[pred].Instructions
	if len(insns) == 0 {
		return false
	}
	last := l.method.Instructions[insns[len(insns)-1]]
	targetStart := l.method.Blocks[target].Start
	switch {
	case last.Opcode >= 0x28 && last.Opcode <= 0x2a:
		return last.Target >= 0 && uint32(last.Target) == targetStart
	case last.Opcode >= 0x32 && last.Opcode <= 0x3d:
		if last.Target >= 0 && uint32(last.Target) == targetStart {
			return true
		}
		return pred+1 == target
	case last.Opcode == 0x2b || last.Opcode == 0x2c:
		for _, caseTarget := range last.Targets {
			if caseTarget >= 0 && uint32(caseTarget) == targetStart {
				return true
			}
		}
		return pred+1 == target
	case last.Opcode >= 0x0e && last.Opcode <= 0x11 || last.Opcode == 0x27:
		return false
	default:
		return pred+1 == target
	}
}
