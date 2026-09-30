package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// branchCallCastLeaf proves that a stack-only cast belongs to the selected
// ternary arm, rather than being a local snapshot evaluated before selection.
// No effect is moved across another instruction: call -> checkcast -> optional
// goto -> merge. Its operands are already values, apart from a witnessed final
// static-field load retained in place; the handler domain stays identical.
// The caller also proves unique use and no DUP.
func (d *Decompiler) branchCallCastLeaf(ref *values.JavaRef, cast *values.CastExpression, entry, leaf, merge *OpCode) *OpCode {
	if d == nil || cast == nil || entry == nil || leaf == nil || merge == nil {
		return nil
	}
	call, ok := values.UnpackSoltValue(cast.Value).(*values.FunctionCallExpression)
	if !ok || call == nil || call.OriginPC < int(entry.CurrentOffset) || call.Descriptor == "" {
		return nil
	}
	invoke, check := d.opcodeAtOffset(call.OriginPC), d.branchCastOpcode(ref, cast, entry, leaf, merge)
	if invoke == nil || check == nil || d.invokeFuncCall[invoke] != call ||
		len(invoke.Target) != 1 || invoke.Target[0] != check || len(check.Source) != 1 || check.Source[0] != invoke {
		return nil
	}
	if call.Object != nil && !values.IsPure(call.Object) {
		// A directly represented receiver chain is evaluated on this arm,
		// before the final invocation. Prove every call and throwing cast in
		// that suffix rather than treating the receiver as pure. Arguments
		// would require a separate operand-order proof.
		if len(call.Arguments) != 0 || !d.branchZeroArgReceiverSuffix(call.Object, entry, invoke) {
			return nil
		}
	}
	for i, arg := range call.Arguments {
		if values.IsPure(arg) {
			continue
		}
		// A static field loaded as the last argument remains after the
		// receiver/earlier arguments and immediately before the call. This
		// preserves its class initialization and volatile-read effects; it
		// does not classify the field as pure or move it across a condition.
		field, ok := values.UnpackSoltValue(arg).(*values.JavaClassMember)
		if !ok || field == nil || i != len(call.Arguments)-1 || len(invoke.Source) != 1 {
			return nil
		}
		load := invoke.Source[0]
		if load.Instr == nil || load.Instr.OpCode != OP_GETSTATIC || load.CurrentOffset < entry.CurrentOffset || len(load.stackProduced) != 1 || values.UnpackSoltValue(load.stackProduced[0]) != field {
			return nil
		}
	}
	return check
}

// branchZeroArgReceiverSuffix matches Java receiver evaluation, inside out, to
// one private bytecode suffix. It never follows a local's defining value: a
// local read is already a snapshot, while only directly represented calls and
// already-inline CHECKCASTs belong to the expression being adopted. Every edge
// is adjacent, forward and in the same handler domain, so no call, cast, local
// write or discarded operation can be skipped or moved across selection.
func (d *Decompiler) branchZeroArgReceiverSuffix(value values.JavaValue, entry, consumer *OpCode) bool {
	if d == nil || entry == nil || consumer == nil {
		return false
	}
	next := consumer
	seen := map[values.JavaValue]bool{}
	for steps := 0; steps < 256; steps++ {
		value = values.UnpackSoltValue(value)
		if value == nil || seen[value] {
			return false
		}
		seen[value] = true
		if values.IsPure(value) {
			// Only local/null loads may precede the represented suffix. Their
			// reads cannot cross a local write or any observable operation.
			prefix := map[*OpCode]bool{}
			for cur := entry; cur != next; cur = cur.Target[0] {
				if cur == nil || prefix[cur] || cur.Instr == nil || len(cur.Target) != 1 ||
					cur.CurrentOffset >= next.CurrentOffset || !sameHandlerCoverage(d.handlersAt(cur), d.handlersAt(consumer)) {
					return false
				}
				prefix[cur] = true
				switch cur.Instr.OpCode {
				case OP_ALOAD, OP_ALOAD_0, OP_ALOAD_1, OP_ALOAD_2, OP_ALOAD_3, OP_ACONST_NULL, OP_NOP:
				default:
					return false
				}
				successor := cur.Target[0]
				if successor == nil || len(successor.Source) != 1 || successor.Source[0] != cur {
					return false
				}
			}
			return next != consumer
		}
		var producer *OpCode
		switch v := value.(type) {
		case *values.CastExpression:
			producer = d.opcodeAtOffset(v.OriginPC)
			if producer == nil || producer.Instr == nil || producer.Instr.OpCode != OP_CHECKCAST ||
				!d.inlineCheckcast[producer] || values.UnpackSoltValue(d.checkcastInnerArg[producer]) != values.UnpackSoltValue(v.Value) {
				return false
			}
			value = v.Value
		case *values.FunctionCallExpression:
			producer = d.opcodeAtOffset(v.OriginPC)
			if producer == nil || producer.Instr == nil || v.Descriptor == "" || len(v.Arguments) != 0 ||
				d.invokeFuncCall[producer] != v {
				return false
			}
			// A static call's class initialization needs a separate witness;
			// this proof covers only nested instance receiver evaluation.
			switch producer.Instr.OpCode {
			case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
			default:
				return false
			}
			value = v.Object
		default:
			return false
		}
		if producer.CurrentOffset < entry.CurrentOffset || producer.CurrentOffset >= next.CurrentOffset ||
			len(producer.Target) != 1 || producer.Target[0] != next || len(next.Source) != 1 || next.Source[0] != producer ||
			!sameHandlerCoverage(d.handlersAt(producer), d.handlersAt(consumer)) {
			return false
		}
		next = producer
	}
	return false
}

// A pure operand does not make CHECKCAST pure: even a cast of a parameter can
// throw. Adopt its statement together with the branch value, after proving the
// cast was performed on this arm. A cast evaluated before the condition must
// stay at that earlier point, including on paths that do not use its result.
func (d *Decompiler) branchPureCastLeaf(ref *values.JavaRef, cast *values.CastExpression, entry, leaf, merge *OpCode) *OpCode {
	if cast == nil || !values.IsPure(cast.Value) {
		return nil
	}
	check := d.branchCastOpcode(ref, cast, entry, leaf, merge)
	if check == nil {
		return nil
	}
	for cur := entry; cur != check; cur = cur.Target[0] {
		switch cur.Instr.OpCode {
		case OP_ALOAD, OP_ALOAD_0, OP_ALOAD_1, OP_ALOAD_2, OP_ALOAD_3, OP_ACONST_NULL, OP_NOP:
		default:
			return nil
		}
		next := cur.Target[0]
		if len(next.Source) != 1 || next.Source[0] != cur {
			return nil
		}
	}
	return check
}

func (d *Decompiler) branchCastOpcode(ref *values.JavaRef, cast *values.CastExpression, entry, leaf, merge *OpCode) *OpCode {
	if d == nil || cast == nil || entry == nil || leaf == nil || merge == nil || cast.OriginPC < int(entry.CurrentOffset) {
		return nil
	}
	check := d.opcodeAtOffset(cast.OriginPC)
	if check == nil || check.Instr == nil || check.Instr.OpCode != OP_CHECKCAST ||
		!d.opcodeProducesLocal(check, ref) || !d.hasUniqueCheckcastProducerForLocal(check, ref) ||
		!singleLinearOpcodePathInHandlers(d, entry, check, d.handlersAt(check)) {
		return nil
	}
	if check != leaf {
		if leaf.Instr == nil || (leaf.Instr.OpCode != OP_GOTO && leaf.Instr.OpCode != OP_GOTO_W) ||
			len(check.Target) != 1 || check.Target[0] != leaf || len(leaf.Source) != 1 || leaf.Source[0] != check {
			return nil
		}
	}
	if len(leaf.Target) != 1 || leaf.Target[0] != merge ||
		!sameHandlerCoverage(d.handlersAt(check), d.handlersAt(leaf)) ||
		!sameHandlerCoverage(d.handlersAt(check), d.handlersAt(merge)) {
		return nil
	}
	return check
}
