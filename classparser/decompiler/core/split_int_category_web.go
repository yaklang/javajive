package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type splitWebConsumer struct {
	op  *OpCode
	idx int
}

type splitWebIndex struct {
	d                 *Decompiler
	webs              *slotWeb
	funcCtx           *class_context.ClassContext
	entries           map[int]bool
	decoded           map[*OpCode]bool
	webOps            map[int][]*OpCode
	stores            map[int][]*OpCode
	loads             map[int][]*OpCode
	members           map[int]map[*values.JavaRef]bool
	slots             map[int]map[int]bool
	invalid           map[int]bool
	solvedBoolean     map[int]bool
	solvedInteger     map[int]bool
	incrementedSlots  map[int]bool
	order             []int
	loadByProduced    map[values.JavaValue]*OpCode
	ambiguousProduced map[values.JavaValue]bool
	consumersOf       map[values.JavaValue][]splitWebConsumer
	ternaryEscapes    map[values.JavaValue]bool
}

// unifySplitIntCategoryWebs repairs int-category reaching-definition webs whose
// stores/loads were split across distinct JavaRefs (for example mixing boolean
// field loads with int 0/1 constants). Each web is proved independently without
// mutating other webs: either all consumers and stores form a closed 0/1
// boolean domain (-> boolean), or every boolean RHS is safely coercible by
// CoerceIntAssignRHS while avoiding uncoercible boolean sinks (-> int).
func (d *Decompiler) unifySplitIntCategoryWebs() {
	if d == nil || d.getenv("JDEC_SPLIT_INTCAT_WEB_OFF") != "" {
		return
	}
	if d.semanticCFG != nil && d.semanticCFG.Err != nil {
		return
	}
	if len(d.ExceptionTable) > 0 && d.semanticCFG == nil {
		return
	}
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	idx := buildSplitWebIndex(d, webs)
	if idx == nil {
		return
	}
	const maxBoolPasses = 8
	for pass := 0; pass < maxBoolPasses; pass++ {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(idx.order))) != nil {
			return
		}
		changed := false
		for _, web := range idx.order {
			if idx.solvedBoolean[web] || idx.solvedInteger[web] || idx.invalid[web] || idx.entries[web] {
				continue
			}
			if len(idx.stores[web]) == 0 || len(idx.loads[web]) == 0 || len(idx.slots[web]) != 1 {
				continue
			}
			if len(idx.members[web]) < 2 && !idx.hasBoolIntoIntStore(web) {
				continue
			}
			slot := singleSetKey(idx.slots[web])
			if idx.incrementedSlots[slot] && !d.originalIncrementsOutsideWeb(webs, web, slot) {
				continue
			}
			if idx.canUnifyToBoolean(web) {
				d.bindProvedPrimitiveWeb(webs, web, idx.stores[web], types.JavaBoolean)
				idx.solvedBoolean[web] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for _, web := range idx.order {
		if idx.solvedBoolean[web] || idx.solvedInteger[web] || idx.invalid[web] || idx.entries[web] {
			continue
		}
		if len(idx.stores[web]) == 0 || len(idx.loads[web]) == 0 || len(idx.slots[web]) != 1 || len(idx.members[web]) < 2 {
			continue
		}
		slot := singleSetKey(idx.slots[web])
		if idx.incrementedSlots[slot] && !d.originalIncrementsOutsideWeb(webs, web, slot) {
			continue
		}
		if idx.canUnifyToInteger(web) {
			d.bindProvedPrimitiveWeb(webs, web, idx.stores[web], types.JavaInteger)
			idx.solvedInteger[web] = true
		}
	}
}

func buildSplitWebIndex(d *Decompiler, webs *slotWeb) *splitWebIndex {
	if d.Work != nil {
		cost := int64(len(d.opCodes) + len(webs.webOf))
		if d.Work.Charge(workbudget.CounterGraphScans, cost) != nil || d.Work.CheckAlloc((cost+1)*192) != nil {
			return nil
		}
	}
	funcCtx := d.FunctionContext
	if funcCtx == nil {
		funcCtx = &class_context.ClassContext{}
	}
	idx := &splitWebIndex{
		d:                 d,
		webs:              webs,
		funcCtx:           funcCtx,
		entries:           map[int]bool{},
		decoded:           map[*OpCode]bool{},
		webOps:            map[int][]*OpCode{},
		stores:            map[int][]*OpCode{},
		loads:             map[int][]*OpCode{},
		members:           map[int]map[*values.JavaRef]bool{},
		slots:             map[int]map[int]bool{},
		invalid:           map[int]bool{},
		solvedBoolean:     map[int]bool{},
		solvedInteger:     map[int]bool{},
		incrementedSlots:  map[int]bool{},
		loadByProduced:    map[values.JavaValue]*OpCode{},
		ambiguousProduced: map[values.JavaValue]bool{},
		consumersOf:       map[values.JavaValue][]splitWebConsumer{},
		ternaryEscapes:    map[values.JavaValue]bool{},
	}
	for _, web := range webs.entryWeb {
		idx.entries[web] = true
	}
	seenOrder := map[int]bool{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		idx.decoded[op] = true
		if op.Instr.OpCode == OP_IINC {
			if slot := GetStoreIdx(op); slot >= 0 {
				idx.incrementedSlots[slot] = true
			}
		}
		if isLocalLoadOpcode(op.Instr.OpCode) && len(op.stackProduced) == 1 && op.stackProduced[0] != nil {
			raw := op.stackProduced[0]
			orig := values.OriginalStackLifetimeUse(raw)
			for _, key := range []values.JavaValue{raw, orig} {
				if key == nil {
					continue
				}
				if idx.loadByProduced[key] != nil && idx.loadByProduced[key] != op {
					idx.ambiguousProduced[key] = true
				} else {
					idx.loadByProduced[key] = op
				}
			}
		}
		for i, consumed := range op.stackConsumed {
			orig := values.OriginalStackLifetimeUse(consumed)
			if orig != nil {
				idx.consumersOf[orig] = append(idx.consumersOf[orig], splitWebConsumer{op: op, idx: i})
			}
			markTernaryEscapes(consumed, false, idx.ternaryEscapes, map[values.JavaValue]bool{}, 0)
		}
		web, ok := webs.webOf[op]
		if !ok {
			continue
		}
		if !seenOrder[web] {
			seenOrder[web] = true
			idx.order = append(idx.order, web)
		}
		idx.recordWebOp(web, op)
	}
	for op, web := range webs.webOf {
		idx.webOps[web] = append(idx.webOps[web], op)
		if !idx.decoded[op] || op == nil || op.Instr == nil {
			idx.invalid[web] = true
		}
	}
	return idx
}

func (idx *splitWebIndex) recordWebOp(web int, op *OpCode) {
	code := op.Instr.OpCode
	if code == OP_IINC {
		idx.invalid[web] = true
		return
	}
	if isLocalStoreOpcode(code) {
		infos := idx.d.opcodeIdToRef[op]
		if op.IsCustom || !dynamicPrimitiveStore(code, types.JavaInteger) || len(infos) != 1 || len(op.stackConsumed) != 1 {
			idx.invalid[web] = true
			return
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || !splitWebEligibleRef(ref) || (!isExactPrimer(ref.Type(), types.JavaBoolean) && !isIntCategoryNumeric(ref.Type())) {
			idx.invalid[web] = true
			return
		}
		idx.stores[web] = append(idx.stores[web], op)
		idx.addWebMember(web, GetStoreIdx(op), ref)
		return
	}
	if isLocalLoadOpcode(code) {
		if !dynamicPrimitiveLoad(code, types.JavaInteger) || len(op.stackProduced) != 1 {
			idx.invalid[web] = true
			return
		}
		slotVal, ok := op.stackProduced[0].(*values.SlotValue)
		if !ok || slotVal == nil {
			idx.invalid[web] = true
			return
		}
		ref, ok := values.UnpackSoltValue(slotVal).(*values.JavaRef)
		if !ok || !splitWebEligibleRef(ref) || (!isExactPrimer(ref.Type(), types.JavaBoolean) && !isIntCategoryNumeric(ref.Type())) {
			idx.invalid[web] = true
			return
		}
		idx.loads[web] = append(idx.loads[web], op)
		idx.addWebMember(web, GetRetrieveIdx(op), ref)
		return
	}
	idx.invalid[web] = true
}

func (idx *splitWebIndex) addWebMember(web, slot int, ref *values.JavaRef) {
	if idx.members[web] == nil {
		idx.members[web] = map[*values.JavaRef]bool{}
	}
	if idx.slots[web] == nil {
		idx.slots[web] = map[int]bool{}
	}
	idx.members[web][ref] = true
	idx.slots[web][slot] = true
}

func splitWebEligibleRef(ref *values.JavaRef) bool {
	return ref != nil &&
		ref.Id != nil &&
		ref.Type() != nil &&
		!ref.IsParam &&
		!ref.IsThis &&
		ref.CustomValue == nil &&
		ref.StackVar == nil &&
		ref.WebDeclType == nil &&
		ref.SolvedWebIdentity == nil
}

func markTernaryEscapes(v values.JavaValue, inTernary bool, out map[values.JavaValue]bool, seen map[values.JavaValue]bool, depth int) {
	if v == nil || depth > 16 || seen[v] {
		return
	}
	seen[v] = true
	if inTernary {
		if orig := values.OriginalStackLifetimeUse(v); orig != nil {
			out[orig] = true
		}
	}
	switch x := values.UnpackSoltValue(v).(type) {
	case *values.TernaryExpression:
		if x == nil {
			return
		}
		markTernaryEscapes(x.Condition, true, out, seen, depth+1)
		markTernaryEscapes(x.TrueValue, true, out, seen, depth+1)
		markTernaryEscapes(x.FalseValue, true, out, seen, depth+1)
	case *values.JavaExpression:
		if x != nil {
			for _, sub := range x.Values {
				markTernaryEscapes(sub, inTernary, out, seen, depth+1)
			}
		}
	case *values.JavaCompare:
		if x != nil {
			markTernaryEscapes(x.JavaValue1, inTernary, out, seen, depth+1)
			markTernaryEscapes(x.JavaValue2, inTernary, out, seen, depth+1)
		}
	}
}

func (idx *splitWebIndex) canUnifyToBoolean(web int) bool {
	hasBoolEvidence := false
	baseOps := map[*OpCode]bool{}
	for _, store := range idx.stores[web] {
		baseOps[store] = true
	}
	for _, store := range idx.stores[web] {
		active := map[int]bool{web: true}
		activeOps := map[*OpCode]bool{}
		for op := range baseOps {
			activeOps[op] = true
		}
		isBool, ok := idx.storeValueIsClosedBoolean01(store.stackConsumed[0], active, activeOps, 0)
		if !ok {
			return false
		}
		if isBool {
			hasBoolEvidence = true
		}
	}
	for _, load := range idx.loads[web] {
		consumers, ok := idx.collectLoadConsumers(load)
		if !ok {
			return false
		}
		for _, c := range consumers {
			strictBool, allowed := idx.isBooleanModeConsumer(c)
			if !allowed {
				return false
			}
			if strictBool {
				hasBoolEvidence = true
			}
		}
	}
	return hasBoolEvidence
}

func (idx *splitWebIndex) storeValueIsClosedBoolean01(val values.JavaValue, active map[int]bool, activeOps map[*OpCode]bool, depth int) (isBool, ok bool) {
	operand := safeSeedOperand(val)
	if operand == nil || operand.Type() == nil {
		return false, false
	}
	unpacked := values.UnpackSoltValue(operand)
	if unpacked == nil || unpacked.Type() == nil {
		return false, false
	}
	if _, lit01 := intLiteral01(unpacked); lit01 {
		return false, true
	}
	if lit, isLit := unpacked.(*values.JavaLiteral); isLit && lit != nil {
		switch word := lit.Data.(type) {
		case bool:
			return true, true
		case int:
			if isExactPrimer(lit.Type(), types.JavaBoolean) && (word == 0 || word == 1) {
				return true, true
			}
		}
		return false, false
	}
	if values.OriginalBooleanStackWord(val) || values.IntrinsicBooleanValue(val) {
		return true, true
	}
	if tern, isTern := unpacked.(*values.TernaryExpression); isTern && tern != nil {
		if depth >= 4 {
			return false, false
		}
		tb, tok := idx.storeValueIsClosedBoolean01(tern.TrueValue, active, activeOps, depth+1)
		fb, fok := idx.storeValueIsClosedBoolean01(tern.FalseValue, active, activeOps, depth+1)
		if tok && fok {
			return tb || fb, true
		}
		return false, false
	}
	load := idx.findLocalLoadProducer(val)
	if load != nil && load.Instr != nil && dynamicPrimitiveLoad(load.Instr.OpCode, types.JavaInteger) {
		if depth >= 4 {
			return false, false
		}
		slot := GetRetrieveIdx(load)
		srcWeb, known := idx.webs.webOf[load]
		if !known {
			return false, false
		}
		if idx.incrementedSlots[slot] && !idx.d.originalIncrementsOutsideWeb(idx.webs, srcWeb, slot) {
			return false, false
		}
		if idx.solvedBoolean[srcWeb] {
			return true, true
		}
		if len(load.Source) > 0 || (idx.d.semanticCFG != nil && idx.d.semanticCFG.Err == nil) {
			defs, reachesEntry := idx.d.allocationReachingStores(load, slot)
			if reachesEntry || len(defs) == 0 {
				return false, false
			}
			srcBool := false
			for _, def := range defs {
				if def == nil || !idx.decoded[def] || def.Instr == nil || def.IsCustom ||
					!dynamicPrimitiveStore(def.Instr.OpCode, types.JavaInteger) || len(def.stackConsumed) != 1 || activeOps[def] {
					return false, false
				}
				activeOps[def] = true
				b, valid := idx.storeValueIsClosedBoolean01(def.stackConsumed[0], active, activeOps, depth+1)
				delete(activeOps, def)
				if !valid {
					return false, false
				}
				if b {
					srcBool = true
				}
			}
			return srcBool, true
		}
		if idx.entries[srcWeb] || idx.invalid[srcWeb] || active[srcWeb] || len(idx.stores[srcWeb]) == 0 {
			return false, false
		}
		active[srcWeb] = true
		defer delete(active, srcWeb)
		srcBool := false
		for _, st := range idx.stores[srcWeb] {
			b, valid := idx.storeValueIsClosedBoolean01(st.stackConsumed[0], active, activeOps, depth+1)
			if !valid {
				return false, false
			}
			if b {
				srcBool = true
			}
		}
		return srcBool, true
	}
	return false, false
}

func (idx *splitWebIndex) findLocalLoadProducer(val values.JavaValue) *OpCode {
	if idx.d.stackValueProducers != nil {
		if orig := values.OriginalStackLifetimeUse(val); orig != nil {
			if p := idx.d.stackValueProducers[orig]; p != nil && p.Instr != nil && isLocalLoadOpcode(p.Instr.OpCode) {
				return p
			}
		}
	}
	cur := val
	for step := 0; step < 16 && cur != nil; step++ {
		if idx.ambiguousProduced[cur] {
			return nil
		}
		if load := idx.loadByProduced[cur]; load != nil {
			return load
		}
		if orig := values.OriginalStackLifetimeUse(cur); orig != nil && orig != cur {
			if idx.ambiguousProduced[orig] {
				return nil
			}
			if load := idx.loadByProduced[orig]; load != nil {
				return load
			}
		}
		slot, ok := cur.(*values.SlotValue)
		if !ok || slot == nil {
			return nil
		}
		cur = slot.GetValue()
	}
	return nil
}

func (idx *splitWebIndex) collectLoadConsumers(load *OpCode) ([]splitWebConsumer, bool) {
	if load == nil || len(load.stackProduced) != 1 || load.stackProduced[0] == nil {
		return nil, false
	}
	orig := values.OriginalStackLifetimeUse(load.stackProduced[0])
	if orig == nil || idx.ternaryEscapes[orig] {
		return nil, false
	}
	var out []splitWebConsumer
	seenCons := map[splitWebConsumer]bool{}
	emit := func(op *OpCode, pos int) {
		if op == nil || op.Instr == nil {
			return
		}
		pair := splitWebConsumer{op: op, idx: pos}
		if !seenCons[pair] {
			seenCons[pair] = true
			out = append(out, pair)
		}
	}
	queue := []values.JavaValue{orig}
	seenVals := map[values.JavaValue]bool{orig: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range idx.consumersOf[cur] {
			if c.op == nil || c.op.Instr == nil {
				continue
			}
			if isStackForwarderOpcode(c.op.Instr.OpCode) {
				matched := false
				for _, p := range c.op.stackProduced {
					po := values.OriginalStackLifetimeUse(p)
					if po == nil {
						continue
					}
					if c.op.Instr.OpCode != OP_DUP && po != cur {
						continue
					}
					matched = true
					if !seenVals[po] {
						if len(seenVals) >= 256 {
							return nil, false
						}
						seenVals[po] = true
						queue = append(queue, po)
					}
				}
				if !matched {
					return nil, false
				}
				continue
			}
			emit(c.op, c.idx)
		}
	}
	if idx.d != nil {
		idx.d.walkLinearConsumers(load, func(op *OpCode, pos int) bool {
			emit(op, pos)
			return true
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func isStackForwarderOpcode(code int) bool {
	switch code {
	case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2, OP_SWAP:
		return true
	}
	return false
}

func (idx *splitWebIndex) isBooleanModeConsumer(c splitWebConsumer) (strictBool, allowed bool) {
	if c.op == nil || c.op.Instr == nil {
		return false, false
	}
	switch c.op.Instr.OpCode {
	case OP_POP:
		return false, true
	case OP_IFEQ, OP_IFNE:
		return false, c.idx == 0
	case OP_PUTFIELD, OP_PUTSTATIC:
		if c.idx != 0 || idx.d.constantPoolGetter == nil || len(c.op.Data) < 2 {
			return false, false
		}
		return true, idx.d.opcodeStoresBooleanField(c.op)
	case OP_IRETURN:
		if c.idx != 0 {
			return false, false
		}
		return true, idx.d.functionReturnsBoolean()
	case OP_INVOKEVIRTUAL, OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
		params, _, ok := idx.d.invokeMethodDescriptor(c.op)
		if !ok || c.idx < 0 || c.idx >= len(params) {
			return false, false
		}
		return true, params[len(params)-1-c.idx] == "Z"
	case OP_BASTORE:
		if c.idx != 0 || len(c.op.stackConsumed) != 3 {
			return false, false
		}
		arr := c.op.stackConsumed[2]
		if arr == nil || arr.Type() == nil || !arr.Type().IsArray() || arr.Type().ArrayDim() != 1 {
			return false, false
		}
		return true, isExactPrimer(arr.Type().ElementType(), types.JavaBoolean)
	case OP_ISTORE, OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3:
		if c.idx != 0 {
			return false, false
		}
		destWeb, ok := idx.webs.webOf[c.op]
		if !ok {
			return false, false
		}
		return true, idx.isBooleanDestinationWeb(destWeb, c.op)
	}
	return false, false
}

func (idx *splitWebIndex) hasBoolIntoIntStore(web int) bool {
	for _, st := range idx.stores[web] {
		infos := idx.d.opcodeIdToRef[st]
		if len(infos) != 1 || len(st.stackConsumed) != 1 {
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || !isExactPrimer(ref.Type(), types.JavaInteger) {
			continue
		}
		rhs := safeSeedOperand(st.stackConsumed[0])
		if rhs != nil && isExactPrimer(rhs.Type(), types.JavaBoolean) {
			return true
		}
	}
	return false
}

func (idx *splitWebIndex) isBooleanDestinationWeb(destWeb int, storeOp *OpCode) bool {
	if idx.entries[destWeb] || idx.solvedInteger[destWeb] {
		return false
	}
	if idx.solvedBoolean[destWeb] {
		return true
	}
	ops := idx.webOps[destWeb]
	if len(ops) == 0 {
		return false
	}
	destSlot := -1
	var destRef *values.JavaRef
	checkRef := func(r *values.JavaRef) bool {
		if r == nil || r.Id == nil || r.Type() == nil || r.IsParam || r.IsThis ||
			r.CustomValue != nil || r.StackVar != nil || !isExactPrimer(r.Type(), types.JavaBoolean) {
			return false
		}
		if r.WebDeclType != nil && !isExactPrimer(r.WebDeclType, types.JavaBoolean) {
			return false
		}
		if destRef != nil && destRef != r {
			return false
		}
		destRef = r
		return true
	}
	for _, op := range ops {
		if !idx.decoded[op] || op == nil || op.Instr == nil {
			return false
		}
		code := op.Instr.OpCode
		if code == OP_IINC {
			return false
		}
		if isLocalStoreOpcode(code) {
			infos := idx.d.opcodeIdToRef[op]
			if op.IsCustom || !dynamicPrimitiveStore(code, types.JavaInteger) || len(infos) != 1 {
				return false
			}
			r, ok := infos[0][0].(*values.JavaRef)
			if !ok || !checkRef(r) {
				return false
			}
			slot := GetStoreIdx(op)
			if slot < 0 || (destSlot >= 0 && destSlot != slot) {
				return false
			}
			destSlot = slot
		} else if isLocalLoadOpcode(code) {
			if !dynamicPrimitiveLoad(code, types.JavaInteger) || len(op.stackProduced) != 1 {
				return false
			}
			r, ok := values.UnpackSoltValue(op.stackProduced[0]).(*values.JavaRef)
			if !ok || r == nil || !checkRef(r) {
				return false
			}
			slot := GetRetrieveIdx(op)
			if slot < 0 || (destSlot >= 0 && destSlot != slot) {
				return false
			}
			destSlot = slot
		} else {
			return false
		}
	}
	if destRef == nil || destSlot < 0 {
		return false
	}
	if idx.incrementedSlots[destSlot] && !idx.d.originalIncrementsOutsideWeb(idx.webs, destWeb, destSlot) {
		return false
	}
	infos := idx.d.opcodeIdToRef[storeOp]
	if len(infos) != 1 || infos[0][0] != destRef {
		return false
	}
	return !idx.d.webHasNumericConsumer(destWeb, idx.webs)
}

func (idx *splitWebIndex) canUnifyToInteger(web int) bool {
	for ref := range idx.members[web] {
		if !isExactPrimer(ref.Type(), types.JavaBoolean) && !isExactPrimer(ref.Type(), types.JavaInteger) {
			return false
		}
	}
	intType := types.NewJavaPrimer(types.JavaInteger)
	for _, store := range idx.stores[web] {
		rhs := safeSeedOperand(store.stackConsumed[0])
		if rhs == nil || rhs.Type() == nil {
			return false
		}
		unpacked := values.UnpackSoltValue(rhs)
		if unpacked == nil || unpacked.Type() == nil {
			return false
		}
		prim, ok := unpacked.Type().RawType().(*types.JavaPrimer)
		if !ok || prim == nil {
			return false
		}
		switch prim.Name {
		case types.JavaInteger, types.JavaByte, types.JavaShort, types.JavaChar:
		case types.JavaBoolean:
			coerced := values.CoerceIntAssignRHS(intType, store.stackConsumed[0], idx.funcCtx)
			if coerced == nil || coerced == store.stackConsumed[0] || isExactPrimer(coerced.Type(), types.JavaBoolean) {
				return false
			}
		default:
			return false
		}
	}
	for _, load := range idx.loads[web] {
		if len(load.stackProduced) != 1 || load.stackProduced[0] == nil {
			return false
		}
		if orig := values.OriginalStackLifetimeUse(load.stackProduced[0]); orig == nil || idx.ternaryEscapes[orig] {
			return false
		}
		consumers, ok := idx.collectLoadConsumers(load)
		if !ok {
			return false
		}
		for _, c := range consumers {
			if !idx.isIntegerModeConsumer(c) {
				return false
			}
		}
	}
	return true
}

func (idx *splitWebIndex) isIntegerModeConsumer(c splitWebConsumer) bool {
	if c.op == nil || c.op.Instr == nil {
		return false
	}
	switch c.op.Instr.OpCode {
	case OP_BASTORE:
		if c.idx == 0 {
			if len(c.op.stackConsumed) != 3 {
				return false
			}
			arr := c.op.stackConsumed[2]
			if arr == nil || arr.Type() == nil || !arr.Type().IsArray() || arr.Type().ArrayDim() != 1 {
				return false
			}
			if isExactPrimer(arr.Type().ElementType(), types.JavaBoolean) {
				return false
			}
		}
	case OP_INVOKEVIRTUAL, OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
		params, _, ok := idx.d.invokeMethodDescriptor(c.op)
		if !ok {
			return false
		}
		if c.idx >= 0 && c.idx < len(params) {
			switch params[len(params)-1-c.idx] {
			case "Z", "B", "S", "C":
				return false
			}
		}
	case OP_PUTFIELD, OP_PUTSTATIC:
		if c.idx == 0 {
			if idx.d.constantPoolGetter == nil || len(c.op.Data) < 2 {
				return false
			}
			m, ok := idx.d.constantPoolGetter(int(Convert2bytesToInt(c.op.Data))).(*values.JavaClassMember)
			if !ok || m == nil || m.JavaType == nil {
				return false
			}
			if isExactPrimer(m.JavaType, types.JavaByte) || isExactPrimer(m.JavaType, types.JavaShort) || isExactPrimer(m.JavaType, types.JavaChar) {
				return false
			}
		}
	case OP_IRETURN:
		if c.idx == 0 {
			ft := idx.d.FunctionType
			if ft == nil && idx.funcCtx != nil {
				ft, _ = idx.funcCtx.FunctionType.(*types.JavaFuncType)
			}
			if ft == nil || ft.ReturnType == nil {
				return false
			}
			rt := ft.ReturnType
			if isExactPrimer(rt, types.JavaByte) || isExactPrimer(rt, types.JavaShort) || isExactPrimer(rt, types.JavaChar) {
				return false
			}
		}
	}
	return true
}

func singleSetKey(m map[int]bool) int {
	for k := range m {
		return k
	}
	return 0
}
