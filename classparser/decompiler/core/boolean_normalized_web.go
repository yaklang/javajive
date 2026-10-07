package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
)

// Recover boolean accumulator declarations only from a closed 0/1 domain.
// This is the dataflow counterpart of the existing optional accumulator source
// recovery. It never changes entry parameters or a ref shared across webs.
func (d *Decompiler) restoreNormalizedBooleanWebs() {
	if d.getenv("JDEC_ORIG14_REMAINING_OFF") != "" {
		return
	}
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	groups := map[int][]*OpCode{}
	owners := map[*values.JavaRef]map[int]bool{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		w, known := webs.webOf[op]
		if !known {
			continue
		}
		groups[w] = append(groups[w], op)
	}
	for op, w := range webs.webOf {
		if op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		for _, info := range d.opcodeIdToRef[op] {
			if ref, ok := info[0].(*values.JavaRef); ok {
				if owners[ref] == nil {
					owners[ref] = map[int]bool{}
				}
				owners[ref][w] = true
			}
		}
	}
	// Copies and bitwise recurrences connect computational-int webs. Prove the
	// entire connected component before choosing boolean source declarations.
	// A numeric consumer on any member keeps the component numeric; a closed
	// component still keeps separate source identities for distinct locals.
	consumers := map[int]map[int]bool{}
	incomplete := map[int]bool{}
	queue := []int{}
	for w, stores := range groups {
		queue = append(queue, w)
		for _, store := range stores {
			pending := append([]values.JavaValue{}, store.stackConsumed...)
			seen := map[values.JavaValue]bool{}
			for len(pending) > 0 && len(seen) < 512 {
				v := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				if v == nil || seen[v] {
					continue
				}
				seen[v] = true
				v = safeSeedOperand(v)
				switch value := v.(type) {
				case *values.JavaRef:
					for producer := range owners[value] {
						if producer != w {
							if consumers[producer] == nil {
								consumers[producer] = map[int]bool{}
							}
							consumers[producer][w] = true
							if consumers[w] == nil {
								consumers[w] = map[int]bool{}
							}
							consumers[w][producer] = true
						}
					}
				case *values.CustomValue:
					// Opaque closures cannot prove the source Boolean domain.
				case *values.JavaExpression:
					if value != nil && (value.Op == values.AND || value.Op == values.OR || value.Op == values.XOR) {
						pending = append(pending, value.Values...)
					}
				default:
					if operand, known := values.BooleanStackWordOperand(v); known {
						pending = append(pending, operand)
					}
				}
			}
			incomplete[w] = incomplete[w] || len(pending) != 0
		}
	}
	slices.Sort(queue)
	completed := map[int]bool{}
	for index := 0; index < len(queue); index++ {
		w := queue[index]
		if completed[w] {
			continue
		}
		component := map[int]bool{}
		worklist := []int{w}
		for len(worklist) > 0 {
			next := worklist[len(worklist)-1]
			worklist = worklist[:len(worklist)-1]
			if component[next] {
				continue
			}
			component[next] = true
			completed[next] = true
			for dependency := range consumers[next] {
				if !component[dependency] {
					worklist = append(worklist, dependency)
				}
			}
		}
		componentOrder := []int{}
		stores := []*OpCode{}
		slots := map[int]bool{}
		for member := range component {
			componentOrder = append(componentOrder, member)
		}
		slices.Sort(componentOrder)
		for _, member := range componentOrder {
			stores = append(stores, groups[member]...)
		}
		for _, store := range stores {
			slots[GetStoreIdx(store)] = true
		}
		if len(stores) == 0 {
			continue
		}
		entry := false
		for member := range component {
			entry = entry || incomplete[member]
		}
		for _, e := range webs.entryWeb {
			entry = entry || component[e]
		}
		if entry {
			continue
		}
		var ref *values.JavaRef
		members := map[*values.JavaRef]bool{}
		valid := true
		for _, op := range stores {
			infos := d.opcodeIdToRef[op]
			if !(op.Instr.OpCode == OP_ISTORE || op.Instr.OpCode >= OP_ISTORE_0 && op.Instr.OpCode <= OP_ISTORE_3) || len(infos) != 1 || len(op.stackConsumed) != 1 {
				valid = false
				break
			}
			r, ok := infos[0][0].(*values.JavaRef)
			if !ok || r == nil || r.Id == nil || r.IsParam || r.IsThis || len(owners[r]) != 1 ||
				(!isExactPrimer(r.Type(), types.JavaInteger) && !isExactPrimer(r.Type(), types.JavaBoolean)) {
				valid = false
				break
			}
			if ref == nil {
				ref = r
			}
			members[r] = true
		}
		if !valid || ref == nil {
			continue
		}
		// Split simulator names can still represent one immutable reaching-
		// definition web. Require every definition and every load snapshot to
		// be present before making one source declaration for that web.
		count := 0
		loadViews := map[*values.SlotValue]bool{}
		for op, owner := range webs.webOf {
			if !component[owner] || op == nil || op.Instr == nil {
				continue
			}
			if isLocalStoreOpcode(op.Instr.OpCode) {
				count++
			}
			if len(members) > 1 && isLocalLoadOpcode(op.Instr.OpCode) {
				if len(op.stackProduced) != 1 {
					valid = false
					break
				}
				snapshot, replaceable := op.stackProduced[0].(*values.SlotValue)
				if !replaceable || snapshot == nil {
					valid = false
					break
				}
				r, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaRef)
				if !ok || !members[r] {
					valid = false
					break
				}
				loadViews[snapshot] = true
			}
		}
		if !valid || count != len(stores) {
			continue
		}
		seed := false
		var normalized func(values.JavaValue, int) bool
		normalized = func(v values.JavaValue, depth int) bool {
			if v == nil || depth > 32 {
				return false
			}
			original := values.OriginalStackLifetimeUse(v)
			v = safeSeedOperand(v)
			if v == nil {
				return false
			}
			if lit, ok := intLiteral01(v); ok && lit != nil {
				seed = true
				return true
			}
			if literal, ok := v.(*values.JavaLiteral); ok && isExactPrimer(literal.Type(), types.JavaBoolean) {
				switch word := literal.Data.(type) {
				case bool:
					seed = true
					return true
				case int:
					if word == 0 || word == 1 {
						seed = true
						return true
					}
				}
				return false
			}
			if r, ok := v.(*values.JavaRef); ok {
				if members[r] && len(members) > 1 {
					snapshot, ok := original.(*values.SlotValue)
					return ok && loadViews[snapshot]
				}
				if members[r] {
					return true
				}
				// A copy from an independently boolean source is a closed
				// domain root just like 0/1. Include a single-definition exit
				// snapshot: its source declaration and its later Z consumer
				// must agree, even when the snapshot was simulated as int.
				if isExactPrimer(r.Type(), types.JavaBoolean) {
					seed = true
					return true
				}
				return false
			}
			if operand, known := values.BooleanStackWordOperand(v); known {
				return normalized(operand, depth+1)
			}
			if e, ok := v.(*values.JavaExpression); ok && len(e.Values) == 2 && (e.Op == values.AND || e.Op == values.OR || e.Op == values.XOR) {
				return normalized(e.Values[0], depth+1) && normalized(e.Values[1], depth+1)
			}
			if isExactPrimer(v.Type(), types.JavaBoolean) {
				seed = true
				return true
			}
			return false
		}
		for _, op := range stores {
			if !normalized(op.stackConsumed[0], 0) {
				valid = false
				break
			}
		}
		if !valid || !seed {
			continue
		}
		booleanConsumer := false
		for _, op := range d.opCodes {
			if op == nil || op.Instr == nil {
				continue
			}
			if isLocalLoadOpcode(op.Instr.OpCode) {
				for _, value := range op.stackProduced {
					if r, ok := safeSeedOperand(value).(*values.JavaRef); ok && members[r] {
						owner, known := webs.webOf[op]
						valid = valid && known && owners[r][owner]
					}
				}
			}
			// IINC reads and writes a local without consuming an operand-stack
			// value. Inspect its reaching definitions explicitly; otherwise a
			// counter initialized/reset to zero looks like a closed boolean web.
			if op.Instr.OpCode == OP_IINC && slots[GetStoreIdx(op)] {
				defs, entry := reachingStoresOf(op, GetStoreIdx(op))
				if entry || len(defs) == 0 {
					valid = false
				}
				for _, def := range defs {
					owner, known := webs.webOf[def]
					if !known || component[owner] {
						valid = false
					}
				}
			}
			for i, v := range op.stackConsumed {
				r, ok := safeSeedOperand(v).(*values.JavaRef)
				if !ok || !members[r] {
					continue
				}
				if len(members) > 1 {
					snapshot, ok := values.OriginalStackLifetimeUse(v).(*values.SlotValue)
					if !ok || !loadViews[snapshot] {
						valid = false
						continue
					}
				}
				switch op.Instr.OpCode {
				case OP_IAND, OP_IOR, OP_IXOR:
					for _, operand := range op.stackConsumed {
						if !normalized(operand, 0) {
							valid = false
						}
					}
				case OP_IFEQ, OP_IFNE:
					booleanConsumer = true
				case OP_IRETURN:
					valid = valid && d.functionReturnsBoolean()
					booleanConsumer = booleanConsumer || d.functionReturnsBoolean()
				case OP_ISTORE, OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3:
					owner, known := webs.webOf[op]
					valid = valid && known && component[owner]
				case OP_PUTFIELD, OP_PUTSTATIC:
					valid = valid && i == 0 && isExactPrimer(d.GetMethodFromPool(int(Convert2bytesToInt(op.Data))).JavaType, types.JavaBoolean)
					booleanConsumer = booleanConsumer || valid
				default:
					valid = false
				}
			}
		}
		// A dead canonical seed is still an integer definition. Exceptional
		// before-state restoration may later reconnect it to a numeric web;
		// a 0/1 domain alone does not justify a boolean source declaration.
		if !valid || !booleanConsumer {
			continue
		}
		for _, memberWeb := range componentOrder {
			definitions := groups[memberWeb]
			names := map[*values.JavaRef]bool{}
			for _, store := range definitions {
				names[d.opcodeIdToRef[store][0][0].(*values.JavaRef)] = true
			}
			if len(names) > 1 {
				d.bindProvedPrimitiveWeb(webs, memberWeb, definitions, types.JavaBoolean)
			} else {
				for r := range names {
					r.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
					r.WebDeclType = r.Type().Copy()
				}
			}
		}
		for _, op := range stores {
			if lit, ok := intLiteral01(op.stackConsumed[0]); ok {
				lit.JavaType = types.NewJavaPrimer(types.JavaBoolean)
			}
		}
	}
}
