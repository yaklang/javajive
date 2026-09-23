package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func statementAccess(s statements.Statement) values.Access {
	switch v := s.(type) {
	case *statements.AssignStatement:
		target := v.LeftValue
		if v.ArrayMember != nil {
			target = v.ArrayMember
		}
		return values.InspectAccess(&values.AssignmentExpression{Target: target, Value: v.JavaValue})
	case *statements.ReturnStatement:
		return values.InspectAccess(v.JavaValue)
	case *statements.ExpressionStatement:
		return values.InspectAccess(v.Expression)
	case *statements.ConditionStatement:
		return values.InspectAccess(v.Condition)
	default:
		return values.Access{Effects: values.EffectOpaque}
	}
}
func (d *Decompiler) handlersAt(op *OpCode) []int {
	var out []int
	for i, h := range d.ExceptionTable {
		if op.CurrentOffset >= h.StartPc && op.CurrentOffset < h.EndPc {
			out = append(out, i)
		}
	}
	return out
}

// canInlineImmediateStore recognizes a value temp consumed immediately by a
// store opcode. Substituting the defining expression into the store RHS keeps
// JVM order: evaluate the value, then perform the field store. No expression is
// moved across a statement or a handler boundary.
func (d *Decompiler) canInlineImmediateStore(source, target *Node, origins map[int]*OpCode) bool {
	if source == nil || target == nil || len(source.Next) != 1 || source.Next[0] != target || len(target.Source) != 1 || target.Source[0] != source {
		return false
	}
	srcOp, dstOp := origins[source.Id], origins[target.Id]
	if srcOp == nil || dstOp == nil || srcOp.Instr == nil || dstOp.Instr == nil {
		return false
	}
	allowedPair := srcOp.Instr.OpCode == OP_DUP && dstOp.Instr.OpCode == OP_PUTSTATIC || srcOp.Instr.OpCode == OP_CHECKCAST && dstOp.Instr.OpCode == OP_PUTFIELD
	if !allowedPair || !sameHandlerCoverage(d.handlersAt(srcOp), d.handlersAt(dstOp)) {
		return false
	}
	srcAssign, ok := source.Statement.(*statements.AssignStatement)
	if !ok {
		return false
	}
	dstAssign, ok := target.Statement.(*statements.AssignStatement)
	if !ok {
		return false
	}
	srcRef, ok := values.UnpackSoltValue(srcAssign.LeftValue).(*values.JavaRef)
	if !ok || srcRef == nil {
		return false
	}
	dstRef, ok := values.UnpackSoltValue(dstAssign.JavaValue).(*values.JavaRef)
	return ok && values.SameLocal(srcRef, dstRef)
}

// countLocalUses counts rendered local reads under a Java value. Unknown
// closures and cycles fail closed; JavaRef.Val is deliberately not followed,
// because that is the definition rather than a rendered use.
func countLocalUses(value values.JavaValue, ref *values.JavaRef, path map[values.JavaValue]bool) (int, bool) {
	value = values.UnpackSoltValue(value)
	if value == nil || ref == nil {
		return 0, value == nil
	}
	if local, ok := value.(*values.JavaRef); ok && values.SameLocal(local, ref) {
		return 1, true
	}
	if path[value] {
		return 0, false
	}
	path[value] = true
	defer delete(path, value)
	children, known := values.Children(value)
	if !known {
		return 0, false
	}
	count := 0
	for _, child := range children {
		n, childKnown := countLocalUses(child, ref, path)
		if !childKnown {
			return 0, false
		}
		count += n
		if count > 1 {
			return count, true
		}
	}
	return count, true
}

func isDupFamily(opcode int) bool {
	switch opcode {
	case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
		return true
	default:
		return false
	}
}

func singleLinearOpcodePath(from, to *OpCode) bool {
	if from == nil || to == nil {
		return false
	}
	cur := from
	seen := map[*OpCode]bool{}
	for cur != to {
		if cur == nil || seen[cur] || cur.Instr == nil || len(cur.Target) != 1 {
			return false
		}
		seen[cur] = true
		switch cur.Instr.OpCode {
		case OP_GOTO, OP_GOTO_W, OP_TABLESWITCH, OP_LOOKUPSWITCH,
			OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE,
			OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
			OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
			return false
		}
		cur = cur.Target[0]
	}
	return true
}

// canInlineCheckcastAtInvocation keeps a throwing cast at its exact invocation
// operand position. The bytecode checkcast must directly feed that invocation,
// its local must occur once in the invocation tree, and the invocation must stay
// on a straight-line path to the graph consumer under the same handlers.
func (d *Decompiler) canInlineCheckcastAtInvocation(value values.JavaValue, source, target *Node, origins map[int]*OpCode, foldedRef *values.JavaRef) bool {
	if d == nil || source == nil || target == nil || foldedRef == nil {
		return false
	}
	srcAssign, ok := source.Statement.(*statements.AssignStatement)
	if !ok {
		return false
	}
	srcRef, ok := values.UnpackSoltValue(srcAssign.LeftValue).(*values.JavaRef)
	if !ok || !values.SameLocal(srcRef, foldedRef) {
		return false
	}
	sourceOp, targetOp := origins[source.Id], origins[target.Id]
	if sourceOp == nil || targetOp == nil || sourceOp.Instr == nil || targetOp.Instr == nil || sourceOp.Instr.OpCode != OP_CHECKCAST {
		return false
	}
	if sourceOp.CurrentOffset >= targetOp.CurrentOffset || !isUnconditionalTransferOrStore(targetOp.Instr.OpCode) ||
		!d.opcodeProducesLocal(sourceOp, foldedRef) {
		return false
	}
	handlers := d.handlersAt(sourceOp)
	if !sameHandlerCoverage(handlers, d.handlersAt(targetOp)) {
		return false
	}
	access := values.InspectAccess(value)
	allowed := values.EffectReadMemory | values.EffectWriteMemory | values.EffectCall | values.EffectAllocate | values.EffectThrow
	if access.Effects&^allowed != 0 || len(access.Writes) != 0 || access.Effects&values.EffectWriteMemory != 0 && access.Effects&values.EffectCall == 0 {
		return false
	}
	var selectedOp *OpCode
	var selectedCall *values.FunctionCallExpression
	for invokeOp, call := range d.invokeFuncCall {
		if invokeOp == nil || invokeOp.Instr == nil || call == nil || !isInvokeOpcode(invokeOp.Instr.OpCode) ||
			invokeOp.CurrentOffset <= sourceOp.CurrentOffset || invokeOp.CurrentOffset >= targetOp.CurrentOffset ||
			!singleLinearOpcodePathInHandlers(d, sourceOp, invokeOp, handlers) ||
			!singleLinearOpcodePathInHandlers(d, invokeOp, targetOp, handlers) {
			continue
		}
		uses, known := countLocalUses(call, foldedRef, map[values.JavaValue]bool{})
		if !known || uses != 1 {
			continue
		}
		if selectedOp != nil {
			return false
		}
		selectedOp, selectedCall = invokeOp, call
	}
	if selectedOp == nil || selectedCall == nil || !sameHandlerCoverage(handlers, d.handlersAt(selectedOp)) {
		return false
	}
	prefix, count, known := prefixBeforeUse(d, selectedCall, foldedRef, sourceOp)
	if !known || count != 1 {
		return false
	}
	moving := values.InspectAccess(value)
	moving.Handlers = handlers
	prefix.Handlers = d.handlersAt(selectedOp)
	return canMoveInlineAcrossPrefix(moving, prefix)
}

func isUnconditionalTransferOrStore(opcode int) bool {
	if isLocalStoreOpcode(opcode) {
		return true
	}
	return opcode == OP_GOTO || opcode == OP_GOTO_W
}

func isInvokeOpcode(opcode int) bool {
	switch opcode {
	case OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE, OP_INVOKEDYNAMIC:
		return true
	default:
		return false
	}
}

// sameHandlerCoverage preserves exception behavior when an expression stays in one
// ordered handler domain.
func sameHandlerCoverage(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func singleLinearOpcodePathInHandlers(d *Decompiler, from, to *OpCode, handlers []int) bool {
	if d == nil || from == nil || to == nil {
		return false
	}
	cur := from
	seen := map[*OpCode]bool{}
	for cur != to {
		if cur == nil || seen[cur] || cur.Instr == nil || len(cur.Target) != 1 || !sameHandlerCoverage(d.handlersAt(cur), handlers) {
			return false
		}
		seen[cur] = true
		switch cur.Instr.OpCode {
		case OP_GOTO, OP_GOTO_W, OP_TABLESWITCH, OP_LOOKUPSWITCH,
			OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE,
			OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
			OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
			return false
		}
		cur = cur.Target[0]
	}
	return sameHandlerCoverage(d.handlersAt(to), handlers)
}

// canMoveInlineAcrossPrefix permits a side-effect-free value to cross observable
// evaluation only when that evaluation does not overwrite a local the value reads.
// Effectful values retain the stricter shared motion proof.
func canMoveInlineAcrossPrefix(operation, prefix values.Access) bool {
	if operation.Effects != 0 || len(operation.Writes) != 0 {
		return values.CanMoveAcrossPrefix(operation, prefix)
	}
	if len(operation.Handlers) != len(prefix.Handlers) {
		return false
	}
	for i := range operation.Handlers {
		if operation.Handlers[i] != prefix.Handlers[i] {
			return false
		}
	}
	for read := range operation.Reads {
		for write := range prefix.Writes {
			if values.SameLocal(read, write) {
				return false
			}
		}
	}
	return true
}

// opcodeProducesLocal proves that the candidate stack-producing instruction actually
// produced the folded local. Stack snapshots retain SlotValue wrappers, so compare
// their unwrapped JavaRef identities rather than names or rendered source text.
func (d *Decompiler) opcodeProducesLocal(op *OpCode, ref *values.JavaRef) bool {
	if d == nil || op == nil || ref == nil {
		return false
	}
	for _, produced := range op.stackProduced {
		if candidate, ok := values.UnpackSoltValue(produced).(*values.JavaRef); ok && values.SameLocal(candidate, ref) {
			return true
		}
	}
	for _, info := range d.opcodeIdToRef[op] {
		if len(info) == 0 {
			continue
		}
		if candidate, ok := info[0].(*values.JavaRef); ok && values.SameLocal(candidate, ref) {
			return true
		}
	}
	return false
}

// canInlineEffectfulCastAtMergeLeaf proves that an effectful cast temp is the
// value of one specific ternary/short-circuit arm. The cast itself must produce
// the referenced local and flow directly to that arm's unconditional merge edge;
// this keeps calls and throws at the same selected-arm evaluation point.
func (d *Decompiler) canInlineEffectfulCastAtMergeLeaf(ref *values.JavaRef, cast *values.CastExpression, leaf *OpCode) bool {
	if d == nil || ref == nil || cast == nil || leaf == nil || leaf.Instr == nil {
		return false
	}
	if leaf.Instr.OpCode != OP_GOTO && leaf.Instr.OpCode != OP_GOTO_W {
		return false
	}
	var source *OpCode
	for op := range d.opcodeToSimulateStack {
		if op != nil && int(op.CurrentOffset) == cast.OriginPC && op.Instr != nil && op.Instr.OpCode == OP_CHECKCAST {
			source = op
			break
		}
	}
	if source == nil || source.CurrentOffset >= leaf.CurrentOffset || len(source.Target) != 1 || source.Target[0] != leaf ||
		!sameHandlerCoverage(d.handlersAt(source), d.handlersAt(leaf)) || !d.opcodeProducesLocal(source, ref) {
		return false
	}
	access := values.InspectAccess(cast.Value)
	if access.Effects&values.EffectOpaque != 0 || access.Effects&(values.EffectMonitor|values.EffectVolatile) != 0 || len(access.Writes) != 0 {
		return false
	}
	if access.Effects&values.EffectWriteMemory != 0 && access.Effects&values.EffectCall == 0 {
		return false
	}
	return true
}

// canInlineDelegationValue permits only a unique, eager use inside this(...)/super(...).
// Earlier constructor arguments must be pure: bytecode can legally compute them out of
// source order, and moving an effectful spill past one would change Java's left-to-right
// argument evaluation. A direct producer/local read and an unchanged handler domain are
// also required; unknown captures, deferred uses, and ambiguous origins fail closed.
func (d *Decompiler) canInlineDelegationValue(value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) bool {
	if d == nil || ref == nil || sourceOp == nil || sourceOp.Instr == nil {
		return false
	}
	stackProducer := isDupFamily(sourceOp.Instr.OpCode) || sourceOp.Instr.OpCode == OP_CHECKCAST
	parameterRead := isLocalLoadOpcode(sourceOp.Instr.OpCode) && ref.IsParam && values.IsPure(value)
	if !stackProducer && !parameterRead {
		return false
	}
	if stackProducer && !d.opcodeProducesLocal(sourceOp, ref) {
		return false
	}
	for targetOp, call := range d.invokeFuncCall {
		if targetOp == nil || targetOp.Instr == nil || targetOp.Instr.OpCode != OP_INVOKESPECIAL || call == nil || call.FunctionName != "<init>" {
			continue
		}
		receiver, ok := values.UnpackSoltValue(call.Object).(*values.JavaRef)
		if !ok || receiver == nil || !receiver.IsThis || sourceOp.CurrentOffset > targetOp.CurrentOffset ||
			!sameHandlerCoverage(d.handlersAt(sourceOp), d.handlersAt(targetOp)) {
			continue
		}
		uses := 0
		ordered := true
		moving := values.InspectAccess(value)
		moving.Handlers = d.handlersAt(sourceOp)
		for _, arg := range call.Arguments {
			n, known := countLocalUses(arg, ref, map[values.JavaValue]bool{})
			if !known {
				ordered = false
				break
			}
			uses += n
			if uses > 1 {
				ordered = false
				break
			}
			if n == 0 {
				prefix := values.InspectAccess(arg)
				prefix.Handlers = d.handlersAt(targetOp)
				if !canMoveInlineAcrossPrefix(moving, prefix) {
					ordered = false
					break
				}
				continue
			}
			prefix, count, known := prefixBeforeUse(d, arg, ref, sourceOp)
			if !known || count != 1 {
				ordered = false
				break
			}
			prefix.Handlers = d.handlersAt(targetOp)
			if !canMoveInlineAcrossPrefix(moving, prefix) {
				ordered = false
				break
			}
		}
		if ordered && uses == 1 {
			return true
		}
	}
	return false
}

// isProvableNull recognizes casts of the null literal. Such a cast has no
// effects, cannot throw, and can be placed at its eventual use without changing
// Java evaluation order (notably, constructor delegation requires its call first).
func isProvableNull(value values.JavaValue) bool {
	for depth := 0; depth < 32; depth++ {
		value = values.UnpackSoltValue(value)
		if value == values.JavaNull {
			return true
		}
		switch v := value.(type) {
		case *values.JavaLiteral:
			return v != nil && (v.Data == nil || v.Data == "null")
		case *values.CastExpression:
			if v == nil {
				return false
			}
			value = v.Value
		case *values.JavaRef:
			if v == nil {
				return false
			}
			if v.Val != nil {
				value = v.Val
				continue
			}
			if v.CustomValue != nil {
				value = v.CustomValue
				continue
			}
			if v.StackVar != nil {
				value = v.StackVar
				continue
			}
			return false
		default:
			return false
		}
	}
	return false
}

// canInlineValue proves the complete proposed motion before graph or value
// mutation. A pure local read is checked against every crossed local write.
// Ambiguous paths, unavailable origins and changed handler coverage keep temps.
func (d *Decompiler) canInlineValue(value values.JavaValue, source, target *Node, origins map[int]*OpCode, foldedRef *values.JavaRef, originHint *OpCode) bool {
	if target == nil {
		return false
	}
	if d.canInlineDelegationValue(value, foldedRef, originHint) || isProvableNull(value) {
		return true
	}
	if source == nil || source == target {
		return false
	}
	if d.canInlineImmediateStore(source, target, origins) {
		return true
	}
	if d.canInlineCheckcastAtInvocation(value, source, target, origins, foldedRef) {
		return true
	}
	moving := values.InspectAccess(value)
	assignment, ok := source.Statement.(*statements.AssignStatement)
	if !ok {
		return false
	}
	ref, ok := values.UnpackSoltValue(assignment.LeftValue).(*values.JavaRef)
	if !ok || ref == nil {
		return false
	}
	origin := origins[source.Id]
	if origin == nil {
		return false
	}
	moving.Handlers = d.handlersAt(origin)
	cur := source
	seen := map[*Node]bool{}
	for cur != target {
		if seen[cur] || len(cur.Next) != 1 {
			return false
		}
		seen[cur] = true
		cur = cur.Next[0]
		if cur == nil || hasDistinctPredecessors(cur) {
			return false
		}
		if d.Work != nil {
			if err := d.Work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
				return false
			}
		}
		op := origins[cur.Id]
		if op == nil {
			return false
		}
		if cur == target {
			prefix, ok := inlineUsePrefix(d, cur.Statement, ref, origin)
			if !ok {
				return false
			}
			prefix.Handlers = d.handlersAt(op)
			return values.CanMoveAcrossPrefix(moving, prefix)
		}
		crossed := statementAccess(cur.Statement)
		crossed.Handlers = d.handlersAt(op)
		if !values.CanSwap(moving, crossed) {
			return false
		}
	}
	return true
}

// opcodeAtOffset returns only an opcode from this method's analyzed stack map.
// A matching integer offset from a different method or incomplete parse is not proof.
func (d *Decompiler) opcodeAtOffset(offset int) *OpCode {
	if d == nil {
		return nil
	}
	for op := range d.opcodeToSimulateStack {
		if op != nil && int(op.CurrentOffset) == offset {
			return op
		}
	}
	return nil
}

// evaluationCompletedBefore proves that a side-effecting child was fully
// evaluated before sourceOp. This lets a constructor argument remain before a
// later spill without treating its already-completed effects as crossed work.
// Unknown expression shapes, control-flow joins, and handler changes fail closed.
func (d *Decompiler) evaluationCompletedBefore(value values.JavaValue, sourceOp *OpCode) bool {
	if d == nil || sourceOp == nil || sourceOp.Instr == nil {
		return false
	}
	value = values.UnpackSoltValue(value)
	access := values.InspectAccess(value)
	if access.Effects == 0 && len(access.Writes) == 0 {
		return true
	}
	endPC := -1
	expected := func(op *OpCode) bool { return op != nil && op.Instr != nil }
	switch x := value.(type) {
	case *values.NewExpression:
		if x == nil || !x.HasOriginPC {
			return false
		}
		if x.IsArray() {
			if len(x.Initializer) == 0 {
				endPC = x.OriginPC
			} else if x.HasEvaluationEndPC {
				endPC = x.EvaluationEndPC
			} else {
				return false
			}
		} else if x.ConstructorCall != nil && x.ConstructorCall.FunctionName == "<init>" {
			endPC = x.ConstructorCall.OriginPC
		} else {
			return false
		}
	case *values.FunctionCallExpression:
		if x == nil || x.OriginPC < 0 {
			return false
		}
		endPC = x.OriginPC
	case *values.CastExpression:
		if x == nil || x.OriginPC < 0 {
			return false
		}
		endPC = x.OriginPC
	default:
		return false
	}
	if endPC < 0 || endPC >= int(sourceOp.CurrentOffset) {
		return false
	}
	endOp := d.opcodeAtOffset(endPC)
	if !expected(endOp) || endOp == sourceOp || !sameHandlerCoverage(d.handlersAt(endOp), d.handlersAt(sourceOp)) {
		return false
	}
	switch x := value.(type) {
	case *values.NewExpression:
		if x.IsArray() {
			if len(x.Initializer) == 0 {
				switch endOp.Instr.OpCode {
				case OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY:
				default:
					return false
				}
			} else {
				switch endOp.Instr.OpCode {
				case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
				default:
					return false
				}
			}
		} else if !isInvokeOpcode(endOp.Instr.OpCode) || x.ConstructorCall == nil || x.ConstructorCall.OriginPC != endPC {
			return false
		}
	case *values.FunctionCallExpression:
		if !isInvokeOpcode(endOp.Instr.OpCode) {
			return false
		}
	case *values.CastExpression:
		if endOp.Instr.OpCode != OP_CHECKCAST {
			return false
		}
	}
	return singleLinearOpcodePathInHandlers(d, endOp, sourceOp, d.handlersAt(sourceOp))
}

// prefixBeforeUse computes Java's evaluated prefix before one value use. It
// rejects deferred/conditional uses, allocation boundaries and opaque closures.
func prefixBeforeUse(d *Decompiler, value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) (values.Access, int, bool) {
	evaluationPC := -1
	if sourceOp != nil {
		evaluationPC = int(sourceOp.CurrentOffset)
	}
	empty := func() values.Access {
		return values.Access{Reads: map[*values.JavaRef]bool{}, Writes: map[*values.JavaRef]bool{}}
	}
	seen := map[values.JavaValue]bool{}
	var walk func(values.JavaValue) (values.Access, int, bool)
	walk = func(v values.JavaValue) (values.Access, int, bool) {
		if v == nil {
			return empty(), 0, true
		}
		if seen[v] {
			return empty(), 0, false
		}
		seen[v] = true
		defer delete(seen, v)
		if r, ok := v.(*values.JavaRef); ok && values.SameLocal(r, ref) {
			return empty(), 1, true
		}
		if d != nil && sourceOp != nil {
			uses, known := countLocalUses(v, ref, map[values.JavaValue]bool{})
			if known && uses == 0 && d.evaluationCompletedBefore(v, sourceOp) {
				return empty(), 0, true
			}
		}
		var children []values.JavaValue
		conditional := false
		leading := empty()
		switch x := v.(type) {
		case *values.SlotValue:
			children = []values.JavaValue{x.GetValue()}
		case *values.FunctionCallExpression:
			children = append([]values.JavaValue{x.Object}, x.Arguments...)
		case *values.NewExpression:
			children = append(children, x.Length...)
			children = append(children, x.Initializer...)
			if x.ConstructorCall != nil {
				children = append(children, x.ConstructorCall.Arguments...)
			}
			// Allocation and class initialization precede constructor arguments. If
			// bytecode proves the spill was produced after NEW, the reconstructed
			// nested argument remains after that same allocation.
			if evaluationPC < 0 || !x.HasOriginPC || x.OriginPC >= evaluationPC {
				leading.Effects = values.EffectAllocate | values.EffectThrow | values.EffectClassInit
			}
		case *values.CustomValue:
			var known bool
			children, known = values.Children(x)
			if !known {
				return empty(), 0, false
			}
		case *values.JavaExpression:
			children = x.Values
			conditional = x.Op == "&&" || x.Op == "||"
			// String + may perform conversion between operands.
			if x.Op == "+" && x.Type() != nil && x.Type().String(&class_context.ClassContext{}) == "String" {
				conditional = true
			}
		case *values.CastExpression:
			children = []values.JavaValue{x.Value}
		case *values.JavaArrayMember:
			children = []values.JavaValue{x.Object, x.Index}
		case *values.RefMember:
			children = []values.JavaValue{x.Object}
		case *values.JavaCompare:
			children = []values.JavaValue{x.JavaValue1, x.JavaValue2}
		case *values.TernaryExpression:
			children = []values.JavaValue{x.Condition, x.TrueValue, x.FalseValue}
			conditional = true
		default:
			a := values.InspectAccess(v)
			for r := range a.Reads {
				if values.SameLocal(r, ref) {
					return empty(), 0, false
				}
			}
			return a, 0, true
		}
		prefix := leading
		count := 0
		for i, c := range children {
			a, n, ok := walk(c)
			if !ok {
				return empty(), 0, false
			}
			if n > 0 && conditional && i > 0 {
				return empty(), 0, false
			}
			if count == 0 {
				prefix.Effects |= a.Effects
				for r := range a.Reads {
					prefix.Reads[r] = true
				}
				for r := range a.Writes {
					prefix.Writes[r] = true
				}
			}
			count += n
		}
		if count == 0 {
			return values.InspectAccess(v), 0, true
		}
		return prefix, count, true
	}
	return walk(value)
}
func inlineUsePrefix(d *Decompiler, statement statements.Statement, ref *values.JavaRef, sourceOp *OpCode) (values.Access, bool) {
	var expression values.JavaValue
	var lhsPrefix values.JavaValue
	switch s := statement.(type) {
	case *statements.ReturnStatement:
		expression = s.JavaValue
	case *statements.ExpressionStatement:
		expression = s.Expression
	case *statements.ConditionStatement:
		expression = s.Condition
	case *statements.AssignStatement:
		expression = s.JavaValue
		if s.ArrayMember != nil {
			lhsPrefix = values.NewJavaCompare(s.ArrayMember.Object, s.ArrayMember.Index)
		} else if target, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); !ok || target == nil {
			lhsPrefix = s.LeftValue
		}
	default:
		return values.Access{}, false
	}
	p, count, ok := prefixBeforeUse(d, expression, ref, sourceOp)
	if !ok || count != 1 {
		return values.Access{}, false
	}
	if lhsPrefix != nil {
		a := values.InspectAccess(lhsPrefix)
		p.Effects |= a.Effects
		for r := range a.Reads {
			p.Reads[r] = true
		}
		for r := range a.Writes {
			p.Writes[r] = true
		}
	}
	return p, true
}
