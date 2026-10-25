package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A byte/short/char/int invocation produces a computational int, even when a
// sibling definition is the literal zero. Preserve that complete reaching web
// as one numeric declaration. Inferring each branch separately can turn the
// zero arm into boolean and disconnect the other arm from the joined reads.
// Only original descriptor calls and integer literals supply domain evidence;
// mutable local types, names and inferred expression types do not.
func (d *Decompiler) unifyDescriptorIntJoins() {
	if d == nil || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || len(d.ExceptionTable) != 0 || len(d.evaluationSnapshots) != 0 || len(d.opCodes) > 8192 || len(d.invokeFuncCall) == 0 {
		return
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes)+len(d.invokeFuncCall))) != nil || d.Work.CheckAlloc(int64(len(d.opCodes)+len(d.invokeFuncCall))*192) != nil) {
		return
	}
	calls := map[*values.FunctionCallExpression]bool{}
	decodedCalls := map[*OpCode]bool{}
	for _, op := range d.opCodes {
		if op != nil && op.Instr != nil && (op.Instr.OpCode == OP_INVOKEVIRTUAL || op.Instr.OpCode == OP_INVOKESTATIC || op.Instr.OpCode == OP_INVOKESPECIAL || op.Instr.OpCode == OP_INVOKEINTERFACE) {
			decodedCalls[op] = true
		}
	}
	for op, call := range d.invokeFuncCall {
		if !decodedCalls[op] || call == nil || !call.HasOriginPC || call.OriginPC != int(op.CurrentOffset) {
			continue
		}
		typ, err := types.ParseMethodDescriptor(call.Descriptor)
		if err != nil || typ == nil {
			continue
		}
		fn, ok := typ.RawType().(*types.JavaFuncType)
		if !ok || fn.ReturnType == nil {
			continue
		}
		for _, kind := range []string{types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger} {
			if isExactPrimer(fn.ReturnType, kind) {
				calls[call] = true
			}
		}
	}
	if len(calls) == 0 {
		return
	}
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	decoded, entries, incremented := map[*OpCode]bool{}, map[int]bool{}, map[int]bool{}
	groups := map[int][]*OpCode{}
	order := []int{}
	for _, web := range webs.entryWeb {
		entries[web] = true
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		decoded[op] = true
		if op.Instr.OpCode == OP_IINC {
			incremented[GetStoreIdx(op)] = true
		}
		if web, known := webs.webOf[op]; known && isLocalStoreOpcode(op.Instr.OpCode) {
			if len(groups[web]) == 0 {
				order = append(order, web)
			}
			groups[web] = append(groups[web], op)
		}
	}
	for _, web := range order {
		stores := groups[web]
		if entries[web] || len(stores) < 2 || incremented[GetStoreIdx(stores[0])] {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(stores)+len(webs.webOf))) != nil {
			return
		}
		valid, hasCall, uses := true, false, 0
		for op, owner := range webs.webOf {
			if owner != web {
				continue
			}
			if !decoded[op] || op == nil || op.Instr == nil || op.Instr.OpCode == OP_IINC {
				valid = false
				break
			}
			if isLocalLoadOpcode(op.Instr.OpCode) {
				if !dynamicPrimitiveLoad(op.Instr.OpCode, types.JavaInteger) || len(op.stackProduced) != 1 {
					valid = false
					break
				}
				if slot, ok := op.stackProduced[0].(*values.SlotValue); !ok || slot == nil {
					valid = false
					break
				}
				uses++
			}
		}
		for _, store := range stores {
			infos := d.opcodeIdToRef[store]
			if !valid || !dynamicPrimitiveStore(store.Instr.OpCode, types.JavaInteger) || len(infos) != 1 || len(store.stackConsumed) != 1 {
				valid = false
				break
			}
			ref, ok := infos[0][0].(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || d.localHasBooleanDescriptorUse(ref) {
				valid = false
				break
			}
			switch rhs := safeSeedOperand(store.stackConsumed[0]).(type) {
			case *values.JavaLiteral:
				word, integer := rhs.Data.(int)
				// Simulation can already have annotated an original 0/1
				// word as boolean. Its unchanged integer payload still proves
				// the value; a bool-valued literal supplies no numeric proof.
				valid = integer && (isExactPrimer(rhs.Type(), types.JavaInteger) || ((word == 0 || word == 1) && isExactPrimer(rhs.Type(), types.JavaBoolean)))
			case *values.FunctionCallExpression:
				valid = calls[rhs]
				hasCall = hasCall || valid
			default:
				valid = false
			}
		}
		if valid && hasCall && uses > 0 {
			d.bindProvedPrimitiveWeb(webs, web, stores, types.JavaInteger)
		}
	}
}
