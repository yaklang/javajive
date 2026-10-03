package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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
	for w, stores := range groups {
		if len(stores) < 2 {
			continue
		}
		entry := false
		for _, e := range webs.entryWeb {
			entry = entry || e == w
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
			if owner != w || op == nil || op.Instr == nil {
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
			original := v
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
				return members[r] || isExactPrimer(r.Type(), types.JavaBoolean)
			}
			if c, ok := v.(*values.CustomValue); ok && c.Flag == "boolean_stack_word" && c.CapturesKnown && len(c.Captures) == 1 {
				return normalized(c.Captures[0], depth+1)
			}
			if e, ok := v.(*values.JavaExpression); ok && len(e.Values) == 2 && (e.Op == values.AND || e.Op == values.OR || e.Op == values.XOR) {
				return normalized(e.Values[0], depth+1) && normalized(e.Values[1], depth+1)
			}
			return isExactPrimer(v.Type(), types.JavaBoolean)
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
		for _, op := range d.opCodes {
			if op == nil || op.Instr == nil {
				continue
			}
			if isLocalLoadOpcode(op.Instr.OpCode) {
				for _, value := range op.stackProduced {
					if r, ok := safeSeedOperand(value).(*values.JavaRef); ok && members[r] {
						owner, known := webs.webOf[op]
						valid = valid && known && owner == w
					}
				}
			}
			// IINC reads and writes a local without consuming an operand-stack
			// value. Inspect its reaching definitions explicitly; otherwise a
			// counter initialized/reset to zero looks like a closed boolean web.
			if op.Instr.OpCode == OP_IINC && GetStoreIdx(op) == GetStoreIdx(stores[0]) {
				defs, entry := reachingStoresOf(op, GetStoreIdx(op))
				if entry || len(defs) == 0 {
					valid = false
				}
				for _, def := range defs {
					owner, known := webs.webOf[def]
					if !known || owner == w {
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
					snapshot, ok := v.(*values.SlotValue)
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
				case OP_IRETURN:
					valid = valid && d.functionReturnsBoolean()
				case OP_ISTORE, OP_ISTORE_0, OP_ISTORE_1, OP_ISTORE_2, OP_ISTORE_3:
					valid = valid && webs.webOf[op] == w
				case OP_PUTFIELD, OP_PUTSTATIC:
					valid = valid && i == 0 && isExactPrimer(d.GetMethodFromPool(int(Convert2bytesToInt(op.Data))).JavaType, types.JavaBoolean)
				default:
					valid = false
				}
			}
		}
		if !valid {
			continue
		}
		if len(members) > 1 {
			ref = d.bindProvedPrimitiveWeb(webs, w, stores, types.JavaBoolean)
		} else {
			ref.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			ref.WebDeclType = ref.Type().Copy()
		}
		for _, op := range stores {
			if lit, ok := intLiteral01(op.stackConsumed[0]); ok {
				lit.JavaType = types.NewJavaPrimer(types.JavaBoolean)
			}
		}
	}
}
