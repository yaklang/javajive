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
	if call.Object != nil && !values.IsPure(call.Object) {
		return nil
	}
	invoke, check := d.opcodeAtOffset(call.OriginPC), d.opcodeAtOffset(cast.OriginPC)
	if invoke == nil || check == nil || check.Instr == nil || check.Instr.OpCode != OP_CHECKCAST ||
		d.invokeFuncCall[invoke] != call || !d.opcodeProducesLocal(check, ref) ||
		!d.hasUniqueCheckcastProducerForLocal(check, ref) ||
		len(invoke.Target) != 1 || invoke.Target[0] != check || len(check.Source) != 1 || check.Source[0] != invoke ||
		!singleLinearOpcodePathInHandlers(d, entry, check, d.handlersAt(check)) {
		return nil
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
