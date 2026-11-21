package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type intConsumerKind int

const (
	intConsumerBooleanOrNeutral intConsumerKind = iota
	intConsumerNumeric
	intConsumerCopy
)

// localWebHasNumericConsumer reports whether the local variable (or any local
// web it is copied into via iload;istore) has a numeric or iinc consumer, or a
// definitively non-boolean numeric store RHS. When true, a boolean field store
// (putfield/putstatic Z) must NOT retype the shared int variable to boolean; it
// must instead narrow only its own consumer view via NarrowBooleanStackWord.
// Kill-switch: JDEC_BOOL_STORE_GUARD_OFF=1.
func (d *Decompiler) localWebHasNumericConsumer(storeOp *OpCode, value values.JavaValue) bool {
	if d == nil || d.getenv("JDEC_BOOL_STORE_GUARD_OFF") != "" || storeOp == nil || value == nil {
		return false
	}
	valueRef, ok := UnpackSoltValue(value).(*values.JavaRef)
	if !ok || valueRef == nil {
		return false
	}
	webs := d.slotWebs()
	if webs == nil {
		return false
	}
	var startWebs []int
	if loadOp := d.localProducerLoad(storeOp, value); loadOp != nil {
		if w, known := webs.webOf[loadOp]; known {
			startWebs = append(startWebs, w)
		}
	}
	if valueRef.VarUid != "" {
		for op, infos := range d.opcodeIdToRef {
			w, known := webs.webOf[op]
			if !known {
				continue
			}
			for _, info := range infos {
				if r, ok := info[0].(*values.JavaRef); ok && r != nil && r.VarUid == valueRef.VarUid {
					startWebs = append(startWebs, w)
					break
				}
			}
		}
	}
	return d.websHaveNumericConsumer(startWebs, webs)
}

func (d *Decompiler) storeWebHasNumericConsumer(storeOp *OpCode) bool {
	if d == nil || storeOp == nil || d.getenv("JDEC_BOOL_STORE_GUARD_OFF") != "" {
		return false
	}
	webs := d.slotWebs()
	if webs == nil {
		return false
	}
	var startWebs []int
	if w, known := webs.webOf[storeOp]; known {
		startWebs = append(startWebs, w)
	}
	if infos := d.opcodeIdToRef[storeOp]; len(infos) > 0 {
		uids := map[string]bool{}
		for _, info := range infos {
			if r, ok := info[0].(*values.JavaRef); ok && r != nil && r.VarUid != "" {
				uids[r.VarUid] = true
			}
		}
		if len(uids) > 0 {
			for op, opInfos := range d.opcodeIdToRef {
				w, known := webs.webOf[op]
				if !known {
					continue
				}
				for _, info := range opInfos {
					if r, ok := info[0].(*values.JavaRef); ok && r != nil && uids[r.VarUid] {
						startWebs = append(startWebs, w)
						break
					}
				}
			}
		}
	}
	return d.websHaveNumericConsumer(startWebs, webs)
}

func (d *Decompiler) localProducerLoad(consumerOp *OpCode, value values.JavaValue) *OpCode {
	if d == nil || value == nil {
		return nil
	}
	unwrapped := values.OriginalStackLifetimeUse(value)
	if unwrapped != nil {
		if op := d.stackValueProducers[unwrapped]; op != nil && op.Instr != nil && isLocalLoadOpcode(op.Instr.OpCode) {
			return op
		}
	}
	if op := d.stackValueProducers[value]; op != nil && op.Instr != nil && isLocalLoadOpcode(op.Instr.OpCode) {
		return op
	}
	if consumerOp != nil && len(consumerOp.Source) == 1 && consumerOp.Source[0] != nil && consumerOp.Source[0].Instr != nil && isLocalLoadOpcode(consumerOp.Source[0].Instr.OpCode) {
		return consumerOp.Source[0]
	}
	return nil
}

func (d *Decompiler) loadOrCopyHasNumericConsumer(load *OpCode, webs *slotWeb) bool {
	if d == nil || load == nil || load.Instr == nil {
		return false
	}
	if webs == nil {
		webs = d.slotWebs()
	}
	if webs == nil {
		return false
	}
	startWeb, known := webs.webOf[load]
	if !known {
		return false
	}
	return d.webHasNumericConsumer(startWeb, webs)
}

func (d *Decompiler) webHasNumericConsumer(startWeb int, webs *slotWeb) bool {
	return d.websHaveNumericConsumer([]int{startWeb}, webs)
}

func (d *Decompiler) websHaveNumericConsumer(startWebs []int, webs *slotWeb) bool {
	if d == nil || webs == nil || len(startWebs) == 0 || d.getenv("JDEC_BOOL_STORE_GUARD_OFF") != "" {
		return false
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(webs.webOf)+len(d.opCodes)+1)) != nil || d.Work.CheckAlloc(int64(len(webs.webOf)+1)*128) != nil) {
		return false
	}

	nodes := d.opCodes
	if d.semanticCFG != nil && len(d.semanticCFG.Nodes) > 0 {
		nodes = d.semanticCFG.Nodes
	}

	webOps := map[int][]*OpCode{}
	for op, w := range webs.webOf {
		if op != nil && op.Instr != nil {
			webOps[w] = append(webOps[w], op)
		}
	}
	entrySlotsByWeb := map[int][]int{}
	for slot, w := range webs.entryWeb {
		entrySlotsByWeb[w] = append(entrySlotsByWeb[w], slot)
	}

	iincBySlot := map[int][]*OpCode{}
	type valUse struct {
		op  *OpCode
		idx int
	}
	consumersByVal := map[values.JavaValue][]valUse{}
	for _, op := range nodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if op.Instr.OpCode == OP_IINC {
			if slot := GetStoreIdx(op); slot >= 0 {
				iincBySlot[slot] = append(iincBySlot[slot], op)
			}
		}
		for idx, consumed := range op.stackConsumed {
			if consumed == nil {
				continue
			}
			orig := values.OriginalStackLifetimeUse(consumed)
			if orig != nil {
				consumersByVal[orig] = append(consumersByVal[orig], valUse{op: op, idx: idx})
			}
			if orig != consumed {
				consumersByVal[consumed] = append(consumersByVal[consumed], valUse{op: op, idx: idx})
			}
		}
	}

	const maxWebs = 64
	visitedWebs := map[int]bool{}
	queue := make([]int, 0, len(startWebs))
	for _, w := range startWebs {
		if !visitedWebs[w] && len(visitedWebs) < maxWebs {
			visitedWebs[w] = true
			queue = append(queue, w)
		}
	}

	for len(queue) > 0 && len(visitedWebs) <= maxWebs {
		w := queue[0]
		queue = queue[1:]
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}

		slots := map[int]bool{}
		storeSet := map[*OpCode]bool{}
		isEntry := false
		for _, s := range entrySlotsByWeb[w] {
			slots[s] = true
			isEntry = true
		}

		var loads, stores []*OpCode
		for _, op := range webOps[w] {
			code := op.Instr.OpCode
			if dynamicPrimitiveStore(code, types.JavaInteger) {
				stores = append(stores, op)
				storeSet[op] = true
				if s := GetStoreIdx(op); s >= 0 {
					slots[s] = true
				}
			} else if dynamicPrimitiveLoad(code, types.JavaInteger) {
				loads = append(loads, op)
				if s := GetRetrieveIdx(op); s >= 0 {
					slots[s] = true
				}
			}
		}

		// Any IINC whose reaching definitions touch this web (or entry slot if entry web)
		// proves an integer counter/accumulator.
		for slot := range slots {
			for _, inc := range iincBySlot[slot] {
				defs, entry := d.allocationReachingStores(inc, slot)
				if isEntry && entry {
					return true
				}
				for _, def := range defs {
					if storeSet[def] {
						return true
					}
				}
			}
		}

		// Defining stores in this web can prove numeric domain even before all loads are simulated.
		for _, st := range stores {
			if len(st.stackConsumed) == 1 && d.isNumericStoreRHS(st.stackConsumed[0]) {
				return true
			}
			if d.isNumericRawPredecessor(st) {
				return true
			}
		}

		// Consumers reached via already-simulated stackConsumed or raw linear CFG walk.
		for _, ld := range loads {
			if len(ld.stackProduced) == 1 && ld.stackProduced[0] != nil {
				for _, use := range consumersByVal[ld.stackProduced[0]] {
					kind, targetWeb := d.classifyIntConsumer(use.op, use.idx, webs)
					switch kind {
					case intConsumerNumeric:
						return true
					case intConsumerCopy:
						if !visitedWebs[targetWeb] && len(visitedWebs) < maxWebs {
							visitedWebs[targetWeb] = true
							queue = append(queue, targetWeb)
						}
					}
				}
			}
			var numeric bool
			d.walkLinearConsumers(ld, func(op *OpCode, idx int) bool {
				kind, targetWeb := d.classifyIntConsumer(op, idx, webs)
				switch kind {
				case intConsumerNumeric:
					numeric = true
					return false
				case intConsumerCopy:
					if !visitedWebs[targetWeb] && len(visitedWebs) < maxWebs {
						visitedWebs[targetWeb] = true
						queue = append(queue, targetWeb)
					}
				}
				return true
			})
			if numeric {
				return true
			}
		}
	}
	return false
}

func (d *Decompiler) isNumericStoreRHS(v values.JavaValue) bool {
	if v == nil {
		return false
	}
	u := safeSeedOperand(v)
	if u == nil {
		return false
	}
	switch t := u.(type) {
	case *values.JavaLiteral:
		if _, ok := intLiteral01(t); ok {
			return false
		}
		if _, ok := t.Data.(int); ok && isIntCategoryNumeric(t.Type()) {
			return true
		}
	case *values.RefMember, *values.JavaClassMember, *values.FunctionCallExpression, *values.JavaArrayMember:
		return isIntCategoryNumeric(t.Type())
	case *values.ArrayLengthExpression:
		return true
	case *values.JavaExpression:
		switch t.Op {
		case values.ADD, values.SUB, values.MUL, values.DIV, values.REM, values.SHL, values.SHR, values.USHR, values.INC, values.DEC:
			return true
		case values.AND, values.OR, values.XOR, values.EQ, values.NEQ, values.LT, values.LTE, values.GT, values.GTE, values.Not, values.LOGICAL_AND, values.LOGICAL_OR:
			return false
		default:
			return isIntCategoryNumeric(t.Type())
		}
	}
	return false
}

func (d *Decompiler) isNumericRawPredecessor(store *OpCode) bool {
	if store == nil || len(store.Source) != 1 || store.Source[0] == nil || store.Source[0].Instr == nil {
		return false
	}
	src := store.Source[0]
	switch src.Instr.OpCode {
	case OP_ICONST_M1, OP_ICONST_2, OP_ICONST_3, OP_ICONST_4, OP_ICONST_5,
		OP_IADD, OP_ISUB, OP_IMUL, OP_IDIV, OP_IREM, OP_ISHL, OP_ISHR, OP_IUSHR, OP_INEG,
		OP_L2I, OP_F2I, OP_D2I, OP_I2B, OP_I2C, OP_I2S,
		OP_IALOAD, OP_CALOAD, OP_SALOAD, OP_ARRAYLENGTH,
		OP_LCMP, OP_FCMPL, OP_FCMPG, OP_DCMPL, OP_DCMPG:
		return true
	case OP_BIPUSH:
		if len(src.Data) >= 1 {
			v := int(int8(src.Data[0]))
			return v != 0 && v != 1
		}
	case OP_SIPUSH:
		if len(src.Data) >= 2 {
			v := int(int16(Convert2bytesToInt(src.Data)))
			return v != 0 && v != 1
		}
	case OP_GETFIELD, OP_GETSTATIC:
		if d.constantPoolGetter != nil && len(src.Data) >= 2 {
			if m, ok := d.constantPoolGetter(int(Convert2bytesToInt(src.Data))).(*values.JavaClassMember); ok && m != nil {
				return numericJoinDescriptor(m.Description)
			}
		}
	case OP_INVOKEVIRTUAL, OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
		if _, ret, ok := d.invokeMethodDescriptor(src); ok {
			return numericJoinDescriptor(ret)
		}
	}
	return false
}

func (d *Decompiler) invokeMethodDescriptor(op *OpCode) (params []string, ret string, ok bool) {
	if op == nil {
		return nil, "", false
	}
	if call := d.invokeFuncCall[op]; call != nil && call.Descriptor != "" {
		if ps, r, err := callbinding.Descriptor(call.Descriptor); err == nil {
			return ps, r, true
		}
	}
	if d.constantPoolGetter != nil && len(op.Data) >= 2 {
		if m, isMem := d.constantPoolGetter(int(Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember); isMem && m != nil && m.Description != "" {
			if ps, r, err := callbinding.Descriptor(m.Description); err == nil {
				return ps, r, true
			}
		}
	}
	return nil, "", false
}

func (d *Decompiler) walkLinearConsumers(load *OpCode, emit func(op *OpCode, idx int) bool) {
	if d == nil || load == nil || len(load.Target) != 1 {
		return
	}
	type frame struct {
		op    *OpCode
		depth int
		steps int
	}
	work := []frame{{op: load.Target[0], depth: 0, steps: 0}}
	for len(work) > 0 {
		f := work[len(work)-1]
		work = work[:len(work)-1]
		cur := f.op
		depth := f.depth
		steps := f.steps
		for cur != nil && cur.Instr != nil && steps < 32 {
			if steps > 0 && (len(cur.Source) != 1 || cur.IsCatch) {
				break
			}
			code := cur.Instr.OpCode
			switch code {
			case OP_NOP, OP_GOTO, OP_GOTO_W, OP_IINC:
				if len(cur.Target) != 1 {
					cur = nil
					continue
				}
				cur = cur.Target[0]
				steps++
				continue
			case OP_DUP:
				if depth == 0 {
					if len(cur.Target) == 1 {
						// One duplicate sits at depth 1, the other at depth 0.
						work = append(work, frame{op: cur.Target[0], depth: 1, steps: steps + 1})
						cur = cur.Target[0]
						depth = 0
						steps++
						continue
					}
					cur = nil
					continue
				}
				depth++
				if len(cur.Target) != 1 {
					cur = nil
					continue
				}
				cur = cur.Target[0]
				steps++
				continue
			case OP_SWAP:
				if depth == 0 {
					depth = 1
				} else if depth == 1 {
					depth = 0
				}
				if len(cur.Target) != 1 {
					cur = nil
					continue
				}
				cur = cur.Target[0]
				steps++
				continue
			}

			pops, pushes, known := d.opcodeStackDelta(cur)
			if !known {
				break
			}
			if depth < pops {
				if !emit(cur, depth) {
					return
				}
				break
			}
			depth = depth - pops + pushes
			if len(cur.Target) != 1 {
				break
			}
			cur = cur.Target[0]
			steps++
		}
	}
}

func (d *Decompiler) opcodeStackDelta(op *OpCode) (pops, pushes int, known bool) {
	if op == nil || op.Instr == nil {
		return 0, 0, false
	}
	switch op.Instr.OpCode {
	case OP_ACONST_NULL, OP_ICONST_M1, OP_ICONST_0, OP_ICONST_1, OP_ICONST_2, OP_ICONST_3, OP_ICONST_4, OP_ICONST_5,
		OP_LCONST_0, OP_LCONST_1, OP_FCONST_0, OP_FCONST_1, OP_FCONST_2, OP_DCONST_0, OP_DCONST_1,
		OP_BIPUSH, OP_SIPUSH, OP_LDC, OP_LDC_W, OP_LDC2_W,
		OP_ILOAD, OP_ILOAD_0, OP_ILOAD_1, OP_ILOAD_2, OP_ILOAD_3,
		OP_LLOAD, OP_LLOAD_0, OP_LLOAD_1, OP_LLOAD_2, OP_LLOAD_3,
		OP_FLOAD, OP_FLOAD_0, OP_FLOAD_1, OP_FLOAD_2, OP_FLOAD_3,
		OP_DLOAD, OP_DLOAD_0, OP_DLOAD_1, OP_DLOAD_2, OP_DLOAD_3,
		OP_ALOAD, OP_ALOAD_0, OP_ALOAD_1, OP_ALOAD_2, OP_ALOAD_3,
		OP_GETSTATIC, OP_NEW:
		return 0, 1, true

	case OP_GETFIELD, OP_ARRAYLENGTH, OP_CHECKCAST, OP_INSTANCEOF,
		OP_INEG, OP_LNEG, OP_FNEG, OP_DNEG,
		OP_I2L, OP_I2F, OP_I2D, OP_L2I, OP_L2F, OP_L2D, OP_F2I, OP_F2L, OP_F2D, OP_D2I, OP_D2L, OP_D2F,
		OP_I2B, OP_I2C, OP_I2S,
		OP_NEWARRAY, OP_ANEWARRAY:
		return 1, 1, true

	case OP_IADD, OP_LADD, OP_FADD, OP_DADD,
		OP_ISUB, OP_LSUB, OP_FSUB, OP_DSUB,
		OP_IMUL, OP_LMUL, OP_FMUL, OP_DMUL,
		OP_IDIV, OP_LDIV, OP_FDIV, OP_DDIV,
		OP_IREM, OP_LREM, OP_FREM, OP_DREM,
		OP_ISHL, OP_LSHL, OP_ISHR, OP_LSHR, OP_IUSHR, OP_LUSHR,
		OP_IAND, OP_LAND, OP_IOR, OP_LOR, OP_IXOR, OP_LXOR,
		OP_LCMP, OP_FCMPL, OP_FCMPG, OP_DCMPL, OP_DCMPG,
		OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD:
		return 2, 1, true

	case OP_ISTORE, OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3,
		OP_LSTORE, OP_LSTORE_0, OP_LSTORE_1, OP_LSTORE_2, OP_LSTORE_3,
		OP_FSTORE, OP_FSTORE_0, OP_FSTORE_1, OP_FSTORE_2, OP_FSTORE_3,
		OP_DSTORE, OP_DSTORE_0, OP_DSTORE_1, OP_DSTORE_2, OP_DSTORE_3,
		OP_ASTORE, OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3,
		OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE,
		OP_IFNULL, OP_IFNONNULL,
		OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN,
		OP_ATHROW, OP_PUTSTATIC, OP_POP, OP_MONITORENTER, OP_MONITOREXIT,
		OP_TABLESWITCH, OP_LOOKUPSWITCH:
		return 1, 0, true

	case OP_RETURN:
		return 0, 0, true

	case OP_PUTFIELD,
		OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
		OP_IF_ACMPEQ, OP_IF_ACMPNE:
		return 2, 0, true

	case OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE:
		return 3, 0, true

	case OP_MULTIANEWARRAY:
		if len(op.Data) >= 3 {
			return int(op.Data[2]), 1, true
		}
		return 0, 0, false

	case OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKEINTERFACE, OP_INVOKESTATIC:
		params, ret, ok := d.invokeMethodDescriptor(op)
		if !ok {
			return 0, 0, false
		}
		pops := len(params)
		if op.Instr.OpCode != OP_INVOKESTATIC {
			pops++
		}
		pushes := 0
		if ret != "V" && ret != "" {
			pushes = 1
		}
		return pops, pushes, true
	}
	return 0, 0, false
}

func (d *Decompiler) classifyIntConsumer(op *OpCode, idx int, webs *slotWeb) (intConsumerKind, int) {
	if op == nil || op.Instr == nil {
		return intConsumerBooleanOrNeutral, 0
	}
	code := op.Instr.OpCode
	if dynamicPrimitiveStore(code, types.JavaInteger) && idx == 0 {
		if webs != nil {
			if targetWeb, ok := webs.webOf[op]; ok {
				return intConsumerCopy, targetWeb
			}
		}
		return intConsumerBooleanOrNeutral, 0
	}
	switch code {
	case OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
		OP_IFLT, OP_IFLE, OP_IFGT, OP_IFGE,
		OP_IADD, OP_ISUB, OP_IMUL, OP_IDIV, OP_IREM, OP_ISHL, OP_ISHR, OP_IUSHR,
		OP_INEG, OP_I2L, OP_I2F, OP_I2D, OP_I2B, OP_I2C, OP_I2S,
		OP_TABLESWITCH, OP_LOOKUPSWITCH,
		OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY:
		return intConsumerNumeric, 0

	case OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD:
		if idx == 0 {
			return intConsumerNumeric, 0
		}

	case OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_CASTORE, OP_SASTORE:
		if idx == 0 || idx == 1 {
			return intConsumerNumeric, 0
		}

	case OP_AASTORE:
		if idx == 1 {
			return intConsumerNumeric, 0
		}

	case OP_BASTORE:
		if idx == 1 {
			return intConsumerNumeric, 0
		}
		if idx == 0 && len(op.stackConsumed) == 3 && op.stackConsumed[2] != nil && op.stackConsumed[2].Type() != nil {
			arrType := op.stackConsumed[2].Type()
			if arrType.IsArray() && arrType.ArrayDim() == 1 && isExactPrimer(arrType.ElementType(), types.JavaByte) {
				return intConsumerNumeric, 0
			}
		}

	case OP_PUTFIELD, OP_PUTSTATIC:
		if idx == 0 && d.constantPoolGetter != nil && len(op.Data) >= 2 {
			if m, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember); ok && m != nil && m.JavaType != nil {
				if !isExactPrimer(m.JavaType, types.JavaBoolean) {
					return intConsumerNumeric, 0
				}
			}
		}

	case OP_IRETURN:
		if idx == 0 && !d.functionReturnsBoolean() {
			return intConsumerNumeric, 0
		}

	case OP_INVOKEVIRTUAL, OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
		params, _, ok := d.invokeMethodDescriptor(op)
		if ok && idx >= 0 && idx < len(params) {
			if params[len(params)-1-idx] != "Z" {
				return intConsumerNumeric, 0
			}
		}
	}
	return intConsumerBooleanOrNeutral, 0
}
