package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Array ownership ends at its original consuming invocation, which can be a
// producer of the delegation argument. A static producer directly followed by
// initialization keeps its complete operand packet on the original JVM stack:
// replacing only its uniquely consumed last array operand changes no evaluation
// position. Do not infer this relationship from names or printed expressions.
func (d *Decompiler) privateDelegationArrayConsumer(delegate *values.FunctionCallExpression, initialization *OpCode, ref *values.JavaRef, array *values.NewExpression) (*values.FunctionCallExpression, *OpCode, bool) {
	if delegate == nil || initialization == nil || initialization.Instr == nil || initialization.Instr.OpCode != OP_INVOKESPECIAL ||
		!delegate.HasOriginPC || int(initialization.CurrentOffset) != delegate.OriginPC || delegate.FuncType == nil ||
		len(delegate.Arguments) == 0 || len(delegate.Arguments) != len(delegate.FuncType.ParamTypes) {
		return nil, nil, false
	}
	method, err := types.ParseMethodDescriptor(delegate.Descriptor)
	if err != nil || method.FunctionType() == nil || len(method.FunctionType().ParamTypes) != len(delegate.Arguments) {
		return nil, nil, false
	}
	ret, ok := method.FunctionType().ReturnType.RawType().(*types.JavaPrimer)
	if !ok || ret.Name != types.JavaVoid || !d.delegationArrayCallOperands(delegate, initialization) {
		return nil, nil, false
	}
	// The private operand DAG below proves one use at the consumer's last
	// raw operand and rejects every other original stack consumption. Avoid
	// a second, potentially expanding traversal of the shared source tree.
	consumer, invoke := delegate, initialization
	last := len(delegate.Arguments) - 1
	if !delegationArraySameRef(delegate.Arguments[last], ref) {
		// A different operand after this producer requires a larger packet
		// proof. This case admits just one result immediately consumed by the
		// original initialization, without a source statement or branch gap.
		if len(delegate.Arguments) != 1 {
			return nil, nil, false
		}
		consumer, ok = values.UnpackSoltValue(delegate.Arguments[0]).(*values.FunctionCallExpression)
		if !ok || consumer == nil || !consumer.IsStatic || consumer.Kind != values.InvokeStatic ||
			consumer.FunctionName == "<init>" || !consumer.HasOriginPC || consumer.FuncType == nil || len(consumer.Arguments) == 0 {
			return nil, nil, false
		}
		invoke = d.opcodeAtOffset(consumer.OriginPC)
		if invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESTATIC ||
			len(invoke.Target) != 1 || invoke.Target[0] != initialization || len(initialization.Source) != 1 || initialization.Source[0] != invoke ||
			len(invoke.stackProduced) != 1 || values.UnpackSoltValue(invoke.stackProduced[0]) != consumer ||
			!d.delegationArrayCallOperands(consumer, invoke) {
			return nil, nil, false
		}
	}
	last = len(consumer.Arguments) - 1
	method, err = types.ParseMethodDescriptor(consumer.Descriptor)
	if err != nil || method.FunctionType() == nil || len(method.FunctionType().ParamTypes) != len(consumer.Arguments) ||
		len(consumer.FuncType.ParamTypes) != len(consumer.Arguments) || !delegationArraySameRef(consumer.Arguments[last], ref) ||
		!sameExactArrayType(array.Type(), consumer.FuncType.ParamTypes[last]) || !sameExactArrayType(array.Type(), method.FunctionType().ParamTypes[last]) {
		return nil, nil, false
	}
	return consumer, invoke, true
}

// Reconstructed calls retain the exact decoded original operand identities.
// Static calls have no receiver word; delegation has the original THIS word.
func (d *Decompiler) delegationArrayCallOperands(call *values.FunctionCallExpression, op *OpCode) bool {
	decoded := d.invokeFuncCall[op]
	if decoded == nil || call == nil || decoded.Descriptor != call.Descriptor || decoded.ClassName != call.ClassName ||
		decoded.FunctionName != call.FunctionName || decoded.Kind != call.Kind || decoded.IsStatic != call.IsStatic ||
		values.UnpackSoltValue(decoded.Object) != values.UnpackSoltValue(call.Object) {
		return false
	}
	words := len(call.Arguments)
	if !call.IsStatic {
		words++
	}
	if len(op.stackConsumed) != words {
		return false
	}
	for i, argument := range call.Arguments {
		if values.UnpackSoltValue(argument) != values.UnpackSoltValue(op.stackConsumed[len(call.Arguments)-1-i]) {
			return false
		}
	}
	if !call.IsStatic && values.UnpackSoltValue(op.stackConsumed[len(call.Arguments)]) != values.UnpackSoltValue(call.Object) {
		return false
	}
	return true
}
