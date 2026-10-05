package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// EvaluationSnapshot materializes one already-evaluated bytecode operand. The
// original SlotValue remains on the RHS for phase-two reaching-def rebinding;
// the reconstructed concat/lambda sees only the dedicated immutable temporary.
type EvaluationSnapshot struct {
	Ref      *values.JavaRef
	Value    values.JavaValue
	OriginPC int
	Operand  bool
	// Call-site descriptor type, used only for a proved constant-null copy.
	ExpectedType types.JavaType
}

func (d *Decompiler) snapshotDynamicOperands(op *OpCode, sim StackSimulation, args []values.JavaValue, parameters []types.JavaType) ([]values.JavaValue, error) {
	if err := d.chargeNodeCopies(len(args) + 1); err != nil {
		return nil, err
	}
	if d.evaluationSnapshots == nil {
		d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{}
	}
	result := make([]values.JavaValue, len(args))
	// JVM stack-pop order is reversed; source evaluation order must be restored.
	for i := len(args) - 1; i >= 0; i-- {
		ref := sim.NewVar(args[i])
		ref.ResetVarType(ref.Type().Copy())
		var expected types.JavaType
		if len(parameters) == len(args) {
			want := parameters[len(args)-1-i]
			expected = want.Copy()
			if _, ok := want.RawType().(*types.JavaPrimer); ok {
				ref.ResetVarType(want.Copy())
			}
		}
		d.disFoldRef = append(d.disFoldRef, ref)
		d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: ref, Value: args[i], OriginPC: int(op.CurrentOffset), Operand: true, ExpectedType: expected})
		result[i] = ref
	}
	return result, nil
}

// Operand snapshots are immutable value copies, but their declaration type was
// copied before reference webs were solved. Refresh direct reference copies
// from the final source declaration; never follow JavaRef.Val (an earlier store)
// or change the copied value, evaluation point, primitive descriptor, or poly
// result target. This also lets a downstream local copy see the solved type on
// the next web iteration.
func (d *Decompiler) refreshReferenceOperandSnapshotTypes() bool {
	changed := false
	for _, op := range d.opCodes {
		for _, snapshot := range d.evaluationSnapshots[op] {
			if !snapshot.Operand || snapshot.Ref == nil {
				continue
			}
			// Constant propagation has proved this operand to be null; the
			// snapshot may adopt its call-site reference descriptor without a
			// CHECKCAST. Preserve that capture type in the inlined lambda body
			// instead of exposing an Object placeholder to overload inference.
			if values.IsNullLiteral(values.UnpackSoltValue(snapshot.Value)) && snapshot.ExpectedType != nil {
				if _, primitive := snapshot.ExpectedType.RawType().(*types.JavaPrimer); !primitive && !reflect.DeepEqual(snapshot.Ref.Type().RawType(), snapshot.ExpectedType.RawType()) {
					snapshot.Ref.ResetVarType(snapshot.ExpectedType.Copy())
					changed = true
				}
				continue
			}
			source, ok := values.UnpackSoltValue(snapshot.Value).(*values.JavaRef)
			if !ok || source == nil || source == snapshot.Ref {
				continue
			}
			if _, primitive := snapshot.Ref.Type().RawType().(*types.JavaPrimer); primitive {
				continue
			}
			typ := source.Type()
			if _, primitive := typ.RawType().(*types.JavaPrimer); primitive {
				continue
			}
			if !reflect.DeepEqual(snapshot.Ref.Type().RawType(), typ.RawType()) {
				snapshot.Ref.ResetVarType(typ.Copy())
				changed = true
			}
		}
	}
	return changed
}

func (d *Decompiler) snapshotDynamicResult(op *OpCode, sim StackSimulation, value values.JavaValue) values.JavaValue {
	if d.canInlineImmediateMethodRef(op, value) {
		// An explicit target preserves the bootstrap's existing SAM adaptation
		// even when the consuming declaration is outside this compilation unit.
		// Only adjacent consumption permits replacing eager operand snapshots.
		for _, snapshot := range d.evaluationSnapshots[op] {
			if !snapshot.Operand {
				continue
			}
			source := snapshot.Value
			ref := snapshot.Ref
			// Preserve this already-proved adjacent capture as an explicit value
			// dependency. A forwarding text closure hides its scoped local
			// binding from namespace and effect visitors.
			ref.StackVar = source
		}
		delete(d.evaluationSnapshots, op)
		typed := &values.CastExpression{Value: value, TargetType: value.Type().Copy(), Binding: true, OriginPC: int(op.CurrentOffset)}
		name, _ := types.RawClassFQN(value.Type())
		return &values.CastExpression{Value: typed, TargetType: types.NewJavaClass(name), Binding: true, OriginPC: int(op.CurrentOffset)}
	}
	ref := sim.NewVar(value)
	ref.ResetVarType(ref.Type().Copy())
	d.disFoldRef = append(d.disFoldRef, ref)
	d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: ref, Value: value, OriginPC: int(op.CurrentOffset)})
	return ref
}

// canInlineImmediateMethodRef keeps a method reference as a poly expression
// only when the next instruction consumes it as the final argument of a
// invocation whose functional-interface parameter exactly matches the
// invokedynamic result erasure. The explicit original SAM target prevents an
// external raw receiver from changing parameter or result adaptation. Pure
// capture copies can be read at this adjacent use; effectful captures stay.
func (d *Decompiler) canInlineImmediateMethodRef(op *OpCode, value values.JavaValue) bool {
	ref, ok := value.(*values.CustomValue)
	if !ok || ref == nil || (!ref.IsMethodRef && ref.Flag != "lambda") || !ref.CapturesKnown || d == nil || d.constantPoolGetter == nil ||
		d.FunctionContext == nil || d.FunctionContext.ClassName == "" ||
		op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC {
		return false
	}
	if len(op.Target) != 1 {
		return false
	}
	next := op.Target[0]
	if next == nil || next.Instr == nil || len(next.Source) != 1 || next.Source[0] != op ||
		int(next.CurrentOffset) != int(op.CurrentOffset)+1+len(op.Data) {
		return false
	}
	if !sameIntSlice(d.handlersAt(op), d.handlersAt(next)) || len(next.Data) < 2 {
		return false
	}
	switch next.Instr.OpCode {
	case OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
	default:
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(next.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.JavaType == nil || member.JavaType.FunctionType() == nil {
		return false
	}
	params := member.JavaType.FunctionType().ParamTypes
	if !ref.IsMethodRef {
		result := member.JavaType.FunctionType().ReturnType
		if result == nil {
			return false
		}
		primitive, ok := result.RawType().(*types.JavaPrimer)
		if !ok || primitive.Name != types.JavaBoolean {
			return false
		}
	}
	if len(params) == 0 || params[len(params)-1] == nil || ref.Type() == nil {
		return false
	}
	ownerType, ownerOK := types.RawClassFQN(params[len(params)-1])
	refType, refOK := types.RawClassFQN(ref.Type())
	if !ownerOK || !refOK || normalizeJavaClassName(ownerType) != normalizeJavaClassName(refType) {
		return false
	}
	for _, snapshot := range d.evaluationSnapshots[op] {
		if !snapshot.Operand || snapshot.Ref == nil {
			return false
		}
		switch v := values.UnpackSoltValue(snapshot.Value).(type) {
		case *values.JavaLiteral:
		case *values.JavaRef:
			if v == nil || v.CustomValue != nil || v.StackVar != nil {
				return false
			}
			if !ref.IsMethodRef && !d.immutableCaptureParameter(v) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Lambda source captures must be effectively final even if the consumer
// retains the closure after this call. A parameter with no writes anywhere in
// the complete opcode stream is stable; arbitrary locals remain snapshots.
func (d *Decompiler) immutableCaptureParameter(ref *values.JavaRef) bool {
	if ref.IsThis {
		return true
	}
	if !ref.IsParam || len(d.opCodes) == 0 {
		return false
	}
	slot := -1
	index := 0
	for _, value := range d.Params {
		param, ok := values.UnpackSoltValue(value).(*values.JavaRef)
		if !ok || param == nil || param.Type() == nil {
			return false
		}
		if values.SameLocal(ref, param) {
			if slot >= 0 {
				return false
			}
			slot = index
		}
		index++
		if primitive, ok := param.Type().RawType().(*types.JavaPrimer); ok && (primitive.Name == types.JavaLong || primitive.Name == types.JavaDouble) {
			index++
		}
	}
	if slot < 0 {
		return false
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		if LocalAccessOf(op.Instr.OpCode).Write && GetStoreIdx(op) == slot {
			return false
		}
	}
	return true
}

func normalizeJavaClassName(name string) string {
	return strings.ReplaceAll(name, ".", "/")
}

// A lambda selected on the operand stack must remain in its selected arm.
// Materializing it as a statement before reconstructing the ternary strands
// one arm's definition (or hoists creation from the other arm). Zero captures,
// `this`, and literals need no snapshot: their values cannot change later.
// Other local captures retain snapshots because deferred lambda execution must
// not observe a reassigned local. Only an immediate merge consumer and equal
// exception coverage prove that no intervening evaluation is crossed.
func (d *Decompiler) canInlineConditionalLambda(op *OpCode, args []values.JavaValue) bool {
	if d == nil || op == nil || len(op.Target) != 1 {
		return false
	}
	for _, arg := range args {
		switch v := values.UnpackSoltValue(arg).(type) {
		case *values.JavaLiteral:
		case *values.JavaRef:
			if v == nil || !v.IsThis {
				return false
			}
		default:
			return false
		}
	}
	merge := op.Target[0]
	if merge == nil || merge.Instr == nil {
		return false
	}
	if merge.Instr.OpCode == OP_GOTO || merge.Instr.OpCode == OP_GOTO_W {
		if len(merge.Target) != 1 || len(merge.Source) != 1 || merge.Source[0] != op ||
			!sameHandlerCoverage(d.handlersAt(op), d.handlersAt(merge)) {
			return false
		}
		merge = merge.Target[0]
	}
	if merge == nil || merge.Instr == nil || len(merge.Source) < 2 || merge.CurrentOffset <= op.CurrentOffset ||
		!sameHandlerCoverage(d.handlersAt(op), d.handlersAt(merge)) {
		return false
	}
	switch merge.Instr.OpCode {
	case OP_ASTORE, OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3,
		OP_PUTFIELD, OP_PUTSTATIC, OP_ARETURN:
		return true
	}
	return false
}
