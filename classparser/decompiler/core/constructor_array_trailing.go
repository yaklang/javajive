package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// A private array may occupy an earlier argument as long as later operands
// still finish in their original source order, after its last store. The
// complete private DAG separately accounts for every original allocation/call
// in these intervals and rejects publications, aliases and alternate entries.
// Returned integers are exact completion PCs, not invented instruction sites.
func (d *Decompiler) delegationArrayTrailingEnd(value values.JavaValue, lower int, invoke *OpCode) (int, bool) {
	value = values.UnpackSoltValue(value)
	effect, _ := values.InspectValue(value)
	if effect&values.EffectOpaque != 0 {
		return 0, false
	}
	if effect == 0 {
		return lower, true
	}
	end := -1
	switch v := value.(type) {
	case *values.NewExpression:
		if !v.HasOriginPC {
			return 0, false
		}
		create := d.opcodeAtOffset(v.OriginPC)
		if create == nil || create.Instr == nil || v.OriginPC <= lower || len(create.stackProduced) != 1 || values.UnpackSoltValue(create.stackProduced[0]) != v {
			return 0, false
		}
		if v.IsArray() {
			if create.Instr.OpCode != OP_ANEWARRAY && create.Instr.OpCode != OP_NEWARRAY && create.Instr.OpCode != OP_MULTIANEWARRAY {
				return 0, false
			}
			end = v.OriginPC
			if len(v.Initializer) != 0 {
				if !v.HasEvaluationEndPC || v.EvaluationEndPC <= v.OriginPC {
					return 0, false
				}
				last := d.opcodeAtOffset(v.EvaluationEndPC)
				if last == nil || last.Instr == nil || !isArrayElementStore(last.Instr.OpCode) || len(last.stackConsumed) != 3 || len(create.Target) != 1 || invoke == nil {
					return 0, false
				}
				ref, known := values.UnpackSoltValue(last.stackConsumed[2]).(*values.JavaRef)
				use := -1
				for index, original := range invoke.stackConsumed {
					if delegationArraySameRef(original, ref) {
						if use >= 0 {
							return 0, false
						}
						use = index
					}
				}
				if !known || use < 0 || !d.privateCompletedArrayAtUse(v, ref, create.Target[0], invoke, use, map[*values.NewExpression]bool{}, 0) {
					return 0, false
				}
				end = v.EvaluationEndPC
			}
		} else {
			call := v.ConstructorCall
			if create.Instr.OpCode != OP_NEW || call == nil || !call.HasOriginPC || call.OriginPC <= v.OriginPC {
				return 0, false
			}
			ctor := d.opcodeAtOffset(call.OriginPC)
			if ctor == nil || ctor.Instr == nil || ctor.Instr.OpCode != OP_INVOKESPECIAL || !sameBranchArrayInvocation(d.invokeFuncCall[ctor], call) {
				return 0, false
			}
			end = call.OriginPC
		}
	case *values.FunctionCallExpression:
		if !v.HasOriginPC {
			return 0, false
		}
		call := d.opcodeAtOffset(v.OriginPC)
		if call == nil || call.Instr == nil || !isInvokeOpcode(call.Instr.OpCode) || !sameBranchArrayInvocation(d.invokeFuncCall[call], v) {
			return 0, false
		}
		end = v.OriginPC
	default:
		return 0, false
	}
	if end <= lower || invoke == nil || end >= int(invoke.CurrentOffset) || !d.evaluationCompletedBefore(value, invoke) {
		return 0, false
	}
	return end, true
}
