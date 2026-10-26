package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A byte/short/char/int invocation or decoded integer arithmetic produces a
// computational int, even when a sibling definition is the literal zero/one.
// Preserve that complete reaching web
// as one numeric declaration. Inferring each branch separately can turn the
// zero arm into boolean and disconnect the other arm from the joined reads.
// Only original typed producers and integer literals supply domain evidence;
// mutable local types, names and inferred expression types do not.
func (d *Decompiler) unifyDescriptorIntJoins() {
	if d == nil || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || len(d.evaluationSnapshots) != 0 || len(d.opCodes) > 8192 {
		return
	}
	// Handler joins need the frozen exceptional graph: a throwing
	// producer contributes its INPUT local state to a catch edge. The
	// mutable source graph cannot establish which stores reach that use.
	// Canonicalizing a complete web changes no evaluations or handler ranges.
	if len(d.ExceptionTable) != 0 && (d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 8192) {
		return
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes)+len(d.invokeFuncCall))) != nil || d.Work.CheckAlloc(int64(len(d.opCodes)+len(d.invokeFuncCall))*192) != nil) {
		return
	}
	producers := d.originalNumericJoinProducers()
	copyProof := d.numericCopyProvenance(producers)
	if len(producers) == 0 && copyProof == nil {
		return
	}
	webs := d.slotWebs()
	if webs == nil || (d.semanticCFG != nil && d.semanticCFG.Err != nil) {
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
	// A structuring pass may remove an increment from the mutable list.
	// Its original read/write still constrains source lifetime ownership.
	if d.semanticCFG != nil {
		for _, op := range d.semanticCFG.Nodes {
			if op != nil && op.Instr != nil && op.Instr.OpCode == OP_IINC {
				incremented[GetStoreIdx(op)] = true
			}
		}
	}
	for _, web := range order {
		stores := groups[web]
		if entries[web] || len(stores) < 2 {
			continue
		}
		if slot := GetStoreIdx(stores[0]); incremented[slot] && !d.originalIncrementsOutsideWeb(webs, web, slot) {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(stores)+len(webs.webOf))) != nil {
			return
		}
		valid, hasNumericProducer, uses := true, false, 0
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
			if copyProof != nil {
				load, _, bounded := copyProof.operand(store.stackConsumed[0])
				if !bounded {
					valid = false
					break
				}
				if load != nil {
					valid = copyProof.provesStore(store)
					hasNumericProducer = hasNumericProducer || valid
					if !valid {
						break
					}
					continue
				}
			}
			switch rhs := safeSeedOperand(store.stackConsumed[0]).(type) {
			case *values.JavaLiteral:
				word, integer := rhs.Data.(int)
				// Simulation can already have annotated an original 0/1
				// word as boolean. Its unchanged integer payload still proves
				// the value; a bool-valued literal supplies no numeric proof.
				valid = integer && (isExactPrimer(rhs.Type(), types.JavaInteger) || ((word == 0 || word == 1) && isExactPrimer(rhs.Type(), types.JavaBoolean)))
			default:
				valid = producers[rhs]
				hasNumericProducer = hasNumericProducer || valid
			}
		}
		if valid && hasNumericProducer && uses > 0 {
			d.bindProvedPrimitiveWeb(webs, web, stores, types.JavaInteger)
		}
	}
}

// The numeric result witness belongs to the decoded producer, rather than a
// mutable local type. Calls and fields use their original descriptor; typed
// array reads use the opcode category. Arithmetic must retain the decoded
// operation's exact result and operand identities. BALOAD and bitwise boolean
// operations cannot alone establish a numeric source domain.
func (d *Decompiler) originalNumericJoinProducers() map[values.JavaValue]bool {
	result := map[values.JavaValue]bool{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if expression, proved := originalIntArithmeticResult(op); proved {
			result[expression] = true
			continue
		}
		switch op.Instr.OpCode {
		case OP_INVOKEVIRTUAL, OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
			call := d.invokeFuncCall[op]
			if call == nil || !call.HasOriginPC || call.OriginPC != int(op.CurrentOffset) {
				continue
			}
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(call.Descriptor))) != nil {
				return nil
			}
			_, ret, err := callbinding.Descriptor(call.Descriptor)
			if err == nil && numericJoinDescriptor(ret) {
				result[call] = true
			}
		case OP_IALOAD, OP_CALOAD, OP_SALOAD:
			if len(op.stackProduced) != 1 {
				continue
			}
			value, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaArrayMember)
			if ok && value != nil && value.HasOriginPC && value.OriginPC == int(op.CurrentOffset) && value.Object != nil && value.Index != nil {
				result[value] = true
			}
		case OP_GETFIELD, OP_GETSTATIC:
			if len(op.stackProduced) != 1 || len(op.Data) != 2 || d.constantPoolGetter == nil {
				continue
			}
			member, ok := d.constantPoolGetter(int(Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
			if !ok || member == nil || !numericJoinDescriptor(member.Description) {
				continue
			}
			value := safeSeedOperand(op.stackProduced[0])
			known := false
			switch field := value.(type) {
			case *values.RefMember:
				known = op.Instr.OpCode == OP_GETFIELD && field.OriginalInstanceFieldRead(int(op.CurrentOffset), member.Name, member.Member, member.Description)
			case *values.JavaClassMember:
				known = op.Instr.OpCode == OP_GETSTATIC && field.OriginalStaticFieldRead(int(op.CurrentOffset), member.Name, member.Member, member.Description)
			}
			if known {
				result[value] = true
			}
		}
	}
	return result
}

// This is a result-category proof, not an expression rewrite. The original
// operation (including division/remainder traps) stays at the original store.
// Requiring the simulator's exact produced value prevents a similarly shaped
// expression or a mutable inferred local type from supplying numeric evidence.
func originalIntArithmeticResult(op *OpCode) (*values.JavaExpression, bool) {
	if op == nil || op.Instr == nil || op.IsCustom || len(op.stackProduced) != 1 || len(op.stackConsumed) != 2 {
		return nil, false
	}
	operator := ""
	switch op.Instr.OpCode {
	case OP_IADD:
		operator = values.ADD
	case OP_ISUB:
		operator = values.SUB
	case OP_IMUL:
		operator = values.MUL
	case OP_IDIV:
		operator = values.DIV
	case OP_IREM:
		operator = values.REM
	case OP_ISHL:
		operator = values.SHL
	case OP_ISHR:
		operator = values.SHR
	case OP_IUSHR:
		operator = values.USHR
	default:
		return nil, false
	}
	expression, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaExpression)
	if !ok || expression == nil || expression.Op != operator || len(expression.Values) != 2 || !isExactPrimer(expression.Type(), types.JavaInteger) {
		return nil, false
	}
	for index, operand := range expression.Values {
		if operand == nil || op.stackConsumed[1-index] == nil || values.OriginalStackLifetimeUse(operand) != values.OriginalStackLifetimeUse(op.stackConsumed[1-index]) {
			return nil, false
		}
	}
	if operator == values.DIV || operator == values.REM {
		pc, originalOp, descriptor, known := expression.OriginalIntegerTrap()
		if !known || pc != int(op.CurrentOffset) || originalOp != operator || descriptor != "I" {
			return nil, false
		}
	}
	return expression, true
}
func numericJoinDescriptor(descriptor string) bool {
	switch descriptor {
	case "B", "S", "C", "I":
		return true
	}
	return false
}
