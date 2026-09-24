package core

import (
	"fmt"

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
	allowedPair := srcOp.Instr.OpCode == OP_DUP && dstOp.Instr.OpCode == OP_PUTSTATIC
	if !allowedPair || len(srcOp.Target) != 1 || srcOp.Target[0] != dstOp || !sameHandlerCoverage(d.handlersAt(srcOp), d.handlersAt(dstOp)) {
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

// countCallOperandLocalUses counts a local read in the receiver or argument
// expressions evaluated by this invocation, but does not count reads captured
// inside an earlier nested invocation. Otherwise a later call such as
// escape(value).replace(...) looks like a second consumer of value and makes
// the exact CHECKCAST producer ambiguous.
func countCallOperandLocalUses(call *values.FunctionCallExpression, ref *values.JavaRef) (int, bool) {
	if call == nil || ref == nil {
		return 0, false
	}
	operands := make([]values.JavaValue, 0, len(call.Arguments)+1)
	operands = append(operands, call.Object)
	operands = append(operands, call.Arguments...)
	var count int
	var walk func(values.JavaValue, map[values.JavaValue]bool) (int, bool)
	walk = func(value values.JavaValue, path map[values.JavaValue]bool) (int, bool) {
		value = values.UnpackSoltValue(value)
		if value == nil {
			return 0, true
		}
		if local, ok := value.(*values.JavaRef); ok && values.SameLocal(local, ref) {
			return 1, true
		}
		if _, isInvocation := value.(*values.FunctionCallExpression); isInvocation {
			return 0, true
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
		uses := 0
		for _, child := range children {
			n, childKnown := walk(child, path)
			if !childKnown {
				return 0, false
			}
			uses += n
			if uses > 1 {
				return uses, true
			}
		}
		return uses, true
	}
	for _, operand := range operands {
		n, known := walk(operand, map[values.JavaValue]bool{})
		if !known {
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
	storedCast, ok := values.UnpackSoltValue(srcAssign.JavaValue).(*values.CastExpression)
	foldedCast, foldedOK := values.UnpackSoltValue(value).(*values.CastExpression)
	if !ok || !foldedOK || storedCast.OriginPC != int(sourceOp.CurrentOffset) || foldedCast.OriginPC != storedCast.OriginPC ||
		!castPreservesLocalStaticType(foldedRef, storedCast, d.FunctionContext) ||
		!castPreservesLocalStaticType(foldedRef, foldedCast, d.FunctionContext) {
		return false
	}
	if sourceOp.CurrentOffset >= targetOp.CurrentOffset ||
		!isUnconditionalTransferOrStore(targetOp.Instr.OpCode) && !isInvokeOpcode(targetOp.Instr.OpCode) ||
		!d.opcodeProducesLocal(sourceOp, foldedRef) ||
		!d.hasUniqueCheckcastProducerForLocal(sourceOp, foldedRef) {
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
		uses, known := countCallOperandLocalUses(call, foldedRef)
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
	branchStackUse := hasBranchMergeStackUse(d, targetOp, selectedCall)
	roots, known := statementValueRoots(target.Statement)
	if !known && !branchStackUse {
		return false
	}
	callUses := 0
	if known {
		for _, root := range roots {
			n, known := countCallIdentity(root, selectedCall, map[values.JavaValue]bool{})
			if !known {
				return false
			}
			callUses += n
		}
	}
	if branchStackUse {
		if known && callUses != 0 {
			return false
		}
		callUses = 1
	}
	linearNodePath := singleLinearNodePath(source, target)
	callInTernaryArm := known && containsCallInOneTernaryArm(roots, selectedCall)
	d.tracef("var-fold", "checkcast merge producerPC=%d consumerPC=%d invokePC=%d branchStack=%t roots=%t callUses=%d linearNodePath=%t callInTernaryArm=%t", sourceOp.CurrentOffset, targetOp.CurrentOffset, selectedOp.CurrentOffset, branchStackUse, known, callUses, linearNodePath, callInTernaryArm)
	if callUses != 1 || !linearNodePath && !branchStackUse && !callInTernaryArm {
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

func hasBranchMergeStackUse(d *Decompiler, target *OpCode, call *values.FunctionCallExpression) bool {
	if d == nil || target == nil || target.Instr == nil || call == nil || target.StackEntry == nil {
		return false
	}
	if target.Instr.OpCode != OP_GOTO && target.Instr.OpCode != OP_GOTO_W || len(target.Target) != 1 {
		return false
	}
	merge := target.Target[0]
	if merge == nil || len(merge.Source) < 2 || !sameHandlerCoverage(d.handlersAt(target), d.handlersAt(merge)) {
		return false
	}
	fromMerge, otherPredecessor := false, false
	for _, source := range merge.Source {
		if source == target {
			fromMerge = true
		} else if source != nil {
			otherPredecessor = true
		}
	}
	return fromMerge && otherPredecessor && values.UnpackSoltValue(target.StackEntry.value) == call
}

func statementValueRoots(statement statements.Statement) ([]values.JavaValue, bool) {
	switch s := statement.(type) {
	case *statements.ExpressionStatement:
		return []values.JavaValue{s.Expression}, true
	case *statements.AssignStatement:
		roots := []values.JavaValue{s.LeftValue, s.JavaValue}
		if s.ArrayMember != nil {
			roots = append(roots, s.ArrayMember)
		}
		return roots, true
	case *statements.ConditionStatement:
		return []values.JavaValue{s.Condition}, true
	case *statements.ReturnStatement:
		return []values.JavaValue{s.JavaValue}, true
	case *statements.StackAssignStatement:
		return []values.JavaValue{s.JavaValue}, true
	default:
		return nil, false
	}
}

func countCallIdentity(value values.JavaValue, target *values.FunctionCallExpression, active map[values.JavaValue]bool) (int, bool) {
	if value == nil {
		return 0, true
	}
	if value == target {
		return 1, true
	}
	if active[value] {
		return 0, false
	}
	active[value] = true
	defer delete(active, value)
	children, known := values.Children(value)
	if !known {
		return 0, false
	}
	count := 0
	for _, child := range children {
		n, known := countCallIdentity(child, target, active)
		if !known {
			return 0, false
		}
		count += n
	}
	return count, true
}

func containsCallInOneTernaryArm(roots []values.JavaValue, target *values.FunctionCallExpression) bool {
	var visit func(values.JavaValue, map[values.JavaValue]bool) bool
	visit = func(value values.JavaValue, active map[values.JavaValue]bool) bool {
		if value == nil || active[value] {
			return false
		}
		active[value] = true
		defer delete(active, value)
		if ternary, ok := value.(*values.TernaryExpression); ok && ternary != nil {
			trueCount, trueKnown := countCallIdentity(ternary.TrueValue, target, map[values.JavaValue]bool{})
			falseCount, falseKnown := countCallIdentity(ternary.FalseValue, target, map[values.JavaValue]bool{})
			if trueKnown && falseKnown && (trueCount == 1 && falseCount == 0 || trueCount == 0 && falseCount == 1) {
				return true
			}
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if visit(child, active) {
				return true
			}
		}
		return false
	}
	for _, root := range roots {
		if visit(root, map[values.JavaValue]bool{}) {
			return true
		}
	}
	return false
}

// hasUniqueCheckcastProducerForLocal refuses to fold one checkcast from a
// local that is also produced by a different checkcast. Alternate bytecode
// arms can assign the same logical local at a join; folding only one arm
// removes a definition while leaving the other arm's use behind.
func (d *Decompiler) hasUniqueCheckcastProducerForLocal(source *OpCode, ref *values.JavaRef) bool {
	if d == nil || source == nil || source.Instr == nil || source.Instr.OpCode != OP_CHECKCAST || ref == nil {
		return false
	}
	seenSource := false
	for op := range d.opcodeToSimulateStack {
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || !d.opcodeProducesLocal(op, ref) {
			continue
		}
		if op != source || seenSource {
			return false
		}
		seenSource = true
	}
	return seenSource
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

// castPreservesLocalStaticType makes cast folding fail closed when a JVM
// CHECKCAST's erased target differs from the Java source type carried by the
// temp. Keeping that temp can be necessary for generic overload resolution and
// target typing even when the runtime cast itself is redundant.
func castPreservesLocalStaticType(ref *values.JavaRef, cast *values.CastExpression, ctx *class_context.ClassContext) bool {
	if ref == nil || cast == nil || cast.TargetType == nil {
		return false
	}
	if ctx == nil {
		ctx = &class_context.ClassContext{}
	}
	localType := ref.Type()
	return localType != nil && localType.String(ctx) == cast.TargetType.String(ctx)
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

// hasProvenAllocationPrefix is used only for constructor-delegation arguments,
// where the bytecode path must prove that NEW completed before the spilled
// value. Generic expression motion retains the allocation barrier because
// branch-local variable scopes and lambda-capture typing are not represented
// by bytecode offsets alone.
func (d *Decompiler) hasProvenAllocationPrefix(value *values.NewExpression, sourceOp *OpCode) bool {
	if d == nil || value == nil || sourceOp == nil || sourceOp.Instr == nil || !value.HasOriginPC || value.OriginPC >= int(sourceOp.CurrentOffset) {
		return false
	}
	allocation := d.opcodeAtOffset(value.OriginPC)
	if allocation == nil || allocation.Instr == nil {
		return false
	}
	switch allocation.Instr.OpCode {
	case OP_NEW, OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY:
	default:
		return false
	}
	handlers := d.handlersAt(sourceOp)
	return sameHandlerCoverage(handlers, d.handlersAt(allocation)) &&
		singleLinearOpcodePathInHandlers(d, allocation, sourceOp, handlers)
}

// singleLinearNodePath applies the same fail-closed rule to the source graph:
// every step to the consumer must have one successor and one matching
// predecessor, including the consumer itself.
func singleLinearNodePath(from, to *Node) bool {
	if from == nil || to == nil || from == to {
		return false
	}
	seen := map[*Node]bool{}
	cur := from
	for cur != to {
		if cur == nil || seen[cur] || len(cur.Next) != 1 {
			return false
		}
		seen[cur] = true
		next := cur.Next[0]
		if next == nil || len(next.Source) != 1 || next.Source[0] != cur {
			return false
		}
		cur = next
	}
	return true
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

// canInlineDelegationValue permits only a unique, eager use inside this(...)/super(...).
// Earlier constructor arguments must be pure: bytecode can legally compute them out of
// source order, and moving an effectful spill past one would change Java's left-to-right
// argument evaluation. A direct producer/local read and an unchanged handler domain are
// also required; unknown captures, deferred uses, and ambiguous origins fail closed.
func (d *Decompiler) canInlineDelegationValue(value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) bool {
	return d.canInlineDelegationValueProof(value, ref, sourceOp, false)
}

// canInlineDelegatingArrayValue is reserved for the constructor-entry array
// rewrite, which separately proves that every removed declaration is a
// contiguous, single-use entry spill and that the constructor call consumes
// those spills in source order.
func (d *Decompiler) canInlineDelegatingArrayValue(value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) bool {
	return d.canInlineDelegationValueProof(value, ref, sourceOp, true)
}

func (d *Decompiler) canInlineDelegationValueProof(value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode, allowInitializedArray bool) bool {
	if d == nil || ref == nil || sourceOp == nil || sourceOp.Instr == nil {
		return false
	}
	if isInitializedArrayLiteral(value) && !allowInitializedArray {
		return false
	}
	var arraySpill *values.NewExpression
	if allowInitializedArray {
		var directArray bool
		arraySpill, directArray = values.UnpackSoltValue(value).(*values.NewExpression)
		if !isInitializedArrayLiteral(value) {
			d.tracef("ctor-array-inline", "array proof reject: value is not an initialized literal: %T", values.UnpackSoltValue(value))
			return false
		}
		if !directArray || arraySpill == nil {
			d.tracef("ctor-array-inline", "array proof reject: value is not a direct allocation: %T", values.UnpackSoltValue(value))
			return false
		}
		if !d.provesDelegatingArraySpillSpan(arraySpill, sourceOp) {
			d.tracef("ctor-array-inline", "array proof reject: incomplete linear span origin=%d end=%d producer=%d", arraySpill.OriginPC, arraySpill.EvaluationEndPC, sourceOp.CurrentOffset)
			return false
		}
	}
	stackProducer := isDupFamily(sourceOp.Instr.OpCode) || sourceOp.Instr.OpCode == OP_CHECKCAST
	parameterRead := isLocalLoadOpcode(sourceOp.Instr.OpCode) && ref.IsParam && values.IsPure(value)
	arrayProducer := arraySpill != nil && d.opcodeProducesLocal(sourceOp, ref)
	if !stackProducer && !parameterRead && !arrayProducer {
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
		if arraySpill != nil {
			arrayEnd := d.opcodeAtOffset(arraySpill.EvaluationEndPC)
			handlers := d.handlersAt(sourceOp)
			if arrayEnd == nil || arrayEnd.CurrentOffset >= targetOp.CurrentOffset ||
				!sameHandlerCoverage(handlers, d.handlersAt(arrayEnd)) ||
				!singleLinearOpcodePathInHandlers(d, arrayEnd, targetOp, handlers) {
				continue
			}
		}
		uses := 0
		ordered := true
		useSeen := false
		moving := values.InspectAccess(value)
		moving.Handlers = d.handlersAt(sourceOp)
		for argIndex, arg := range call.Arguments {
			n, known := countLocalUses(arg, ref, map[values.JavaValue]bool{})
			if !known {
				d.tracef("ctor-array-inline", "argument order reject pc=%d arg=%d reason=unknown local-use shape", sourceOp.CurrentOffset, argIndex)
				ordered = false
				break
			}
			uses += n
			if uses > 1 {
				d.tracef("ctor-array-inline", "argument order reject pc=%d reason=multiple temp reads", sourceOp.CurrentOffset)
				ordered = false
				break
			}
			if n == 0 {
				// Only arguments before the selected use form a prefix. Later
				// arguments naturally remain after the inlined expression and must
				// not be treated as work that the array would cross.
				if useSeen {
					continue
				}
				prefix := values.InspectAccess(arg)
				prefix.Handlers = d.handlersAt(targetOp)
				if arraySpill != nil && (prefix.Effects != 0 || len(prefix.Writes) != 0) {
					arrayStart := d.opcodeAtOffset(arraySpill.OriginPC)
					if d.valueWasProducedBefore(arg, arrayStart) || d.evaluationCompletedBefore(arg, arrayStart) {
						continue
					}
					d.tracef("ctor-array-inline", "argument order reject pc=%d arg=%d reason=effectful prefix was not proven complete before array allocation type=%T details=%s access=%+v", sourceOp.CurrentOffset, argIndex, arg, delegationPrefixTraceDetails(arg), prefix)
					ordered = false
					break
				}
				if !canMoveInlineAcrossPrefix(moving, prefix) {
					d.tracef("ctor-array-inline", "argument order reject pc=%d arg=%d reason=move crosses prefix access=%+v", sourceOp.CurrentOffset, argIndex, prefix)
					ordered = false
					break
				}
				continue
			}
			prefix, count, known := prefixBeforeDelegationUse(d, arg, ref, sourceOp)
			if !known || count != 1 {
				d.tracef("ctor-array-inline", "argument order reject pc=%d arg=%d reason=use prefix proof known=%t count=%d", sourceOp.CurrentOffset, argIndex, known, count)
				ordered = false
				break
			}
			prefix.Handlers = d.handlersAt(targetOp)
			if !canMoveInlineAcrossPrefix(moving, prefix) {
				d.tracef("ctor-array-inline", "argument order reject pc=%d arg=%d reason=move crosses use prefix access=%+v", sourceOp.CurrentOffset, argIndex, prefix)
				ordered = false
				break
			}
			useSeen = true
		}
		if ordered && uses == 1 {
			return true
		}
	}
	return false
}

// delegationPrefixTraceDetails reports bytecode origin metadata that is useful
// when an effectful earlier constructor argument cannot be proven complete
// before a later array spill. It is only called under the opt-in trace flag.
func delegationPrefixTraceDetails(value values.JavaValue) string {
	switch v := values.UnpackSoltValue(value).(type) {
	case *values.NewExpression:
		if v == nil {
			return "<nil-new>"
		}
		callPC := -1
		callName := "<nil>"
		callArgs := -1
		if v.ConstructorCall != nil {
			callPC = v.ConstructorCall.OriginPC
			callName = v.ConstructorCall.FunctionName
			callArgs = len(v.ConstructorCall.Arguments)
		}
		return fmt.Sprintf("newPC=%d hasNewPC=%t arrayElements=%d ctor=%s ctorPC=%d ctorArgs=%d", v.OriginPC, v.HasOriginPC, len(v.Initializer), callName, callPC, callArgs)
	case *values.FunctionCallExpression:
		if v == nil {
			return "<nil-call>"
		}
		return fmt.Sprintf("call=%s pc=%d args=%d", v.FunctionName, v.OriginPC, len(v.Arguments))
	case *values.CastExpression:
		if v == nil {
			return "<nil-cast>"
		}
		return fmt.Sprintf("castPC=%d", v.OriginPC)
	default:
		return "no-origin-fields"
	}
}

// provesDelegatingArraySpillSpan ties a synthetic local back to one complete,
// straight-line array initializer. The producer can occur inside its DUP/store
// sequence, so checking only that opcode's offset would miss later element
// stores and could move allocation ahead of an earlier constructor argument.
func (d *Decompiler) provesDelegatingArraySpillSpan(array *values.NewExpression, sourceOp *OpCode) bool {
	reject := func(reason string) bool {
		d.tracef("ctor-array-inline", "array span reject: %s", reason)
		return false
	}
	if d == nil || array == nil || sourceOp == nil || sourceOp.Instr == nil {
		return reject("missing decompiler, initializer, or producer")
	}
	if !array.HasOriginPC || !array.HasEvaluationEndPC || len(array.Initializer) == 0 {
		return reject("initializer does not have a complete origin span")
	}
	if array.OriginPC > int(sourceOp.CurrentOffset) || array.EvaluationEndPC < int(sourceOp.CurrentOffset) {
		return reject("producer is outside the initializer span")
	}
	start := d.opcodeAtOffset(array.OriginPC)
	end := d.opcodeAtOffset(array.EvaluationEndPC)
	if start == nil || start.Instr == nil || end == nil || end.Instr == nil {
		return reject("allocation or final store opcode cannot be resolved")
	}
	switch start.Instr.OpCode {
	case OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY:
	default:
		return reject(fmt.Sprintf("span starts at %s, not an allocation", start.Instr.Name))
	}
	switch end.Instr.OpCode {
	case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
	default:
		return reject(fmt.Sprintf("span ends at %s, not an array store", end.Instr.Name))
	}
	if !sameHandlerCoverage(d.handlersAt(start), d.handlersAt(sourceOp)) || !sameHandlerCoverage(d.handlersAt(start), d.handlersAt(end)) {
		return reject("handler coverage differs across array construction")
	}
	if !singleLinearOpcodePathInHandlers(d, start, sourceOp, d.handlersAt(start)) {
		return reject("allocation to local producer crosses a control-flow or handler boundary")
	}
	if !singleLinearOpcodePathInHandlers(d, sourceOp, end, d.handlersAt(start)) {
		return reject("local producer to final array store crosses a control-flow or handler boundary")
	}
	return true
}

func sameStackProducedValue(want, produced values.JavaValue) bool {
	want = values.UnpackSoltValue(want)
	produced = values.UnpackSoltValue(produced)
	if want == produced {
		return true
	}
	wantMember, wantOK := want.(*values.JavaClassMember)
	producedMember, producedOK := produced.(*values.JavaClassMember)
	if !wantOK || !producedOK || wantMember == nil || producedMember == nil || wantMember.Description == "" {
		return false
	}
	return wantMember.Name == producedMember.Name &&
		wantMember.Member == producedMember.Member &&
		wantMember.Description == producedMember.Description &&
		wantMember.RefKind == producedMember.RefKind
}

// valueWasProducedBefore locates a stack value's unique bytecode producer and
// verifies a straight-line path to the array allocation. This preserves the
// original order when an earlier constructor argument is itself effectful;
// the source graph alone cannot express that a GETSTATIC or call ran before a
// synthetic array local was rendered. Field references may be rebuilt while
// stack values are translated, so compare their complete constant-pool identity
// when pointer identity is unavailable; duplicate reads remain ambiguous.
func (d *Decompiler) valueWasProducedBefore(value values.JavaValue, before *OpCode) bool {
	if d == nil || before == nil || before.Instr == nil {
		return false
	}
	want := values.UnpackSoltValue(value)
	if want == nil {
		return true
	}
	var producer *OpCode
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || op.CurrentOffset >= before.CurrentOffset {
			continue
		}
		for _, produced := range op.stackProduced {
			if sameStackProducedValue(want, produced) {
				if producer != nil {
					return false
				}
				producer = op
			}
		}
	}
	if producer == nil {
		return false
	}
	handlers := d.handlersAt(before)
	return sameHandlerCoverage(handlers, d.handlersAt(producer)) && singleLinearOpcodePathInHandlers(d, producer, before, handlers)
}

// Initialized arrays used as this()/super() arguments are handled by the
// dedicated constructor-entry rewrite, which proves the declaration is the
// first real statement. Folding them through the generic spill path can leave
// their declaration after the mandatory delegation call.
func isInitializedArrayLiteral(value values.JavaValue) bool {
	switch x := values.UnpackSoltValue(value).(type) {
	case *values.NewExpression:
		return x != nil && x.IsArray() && len(x.Initializer) > 0
	case *values.CastExpression:
		return x != nil && isInitializedArrayLiteral(x.Value)
	default:
		return false
	}
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
	if d.getenv("JDEC_IMMEDIATE_STORE_FOLD_OFF") == "" && d.canInlineImmediateStore(source, target, origins) {
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
		} else {
			var ok bool
			endPC, ok = d.constructorCompletionBefore(x, sourceOp)
			if !ok {
				d.tracef("ctor-array-inline", "completed-prefix reject: NEW at pc=%d has no unique matching invokespecial before pc=%d", x.OriginPC, sourceOp.CurrentOffset)
				return false
			}
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
		} else if endOp.Instr.OpCode != OP_INVOKESPECIAL {
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

// constructorCompletionBefore locates the invokespecial that consumes this
// exact NEW value. In particular, an allocation is not evidence that the
// object is fully constructed: a zero-argument constructor still has a
// throwing invocation that must precede a later array allocation before its
// expression can safely remain earlier in a reconstructed constructor call.
func (d *Decompiler) constructorCompletionBefore(value *values.NewExpression, before *OpCode) (int, bool) {
	reject := func(reason string) (int, bool) {
		if d != nil {
			d.tracef("ctor-array-inline", "constructor completion reject: %s", reason)
		}
		return -1, false
	}
	if d == nil || value == nil || before == nil || !value.HasOriginPC {
		return reject("missing decompiler, allocation, spill boundary, or allocation PC")
	}
	var completion *OpCode
	if value.ConstructorCall != nil {
		call := value.ConstructorCall
		if call.FunctionName != "<init>" {
			return reject("attached constructor call has a different method name")
		}
		op := d.opcodeAtOffset(call.OriginPC)
		mappedCall := d.invokeFuncCall[op]
		attachedObject := values.UnpackSoltValue(GetRealValue(call.Object))
		var mappedObject values.JavaValue
		if mappedCall != nil {
			mappedObject = values.UnpackSoltValue(GetRealValue(mappedCall.Object))
		}
		if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL || mappedCall == nil ||
			mappedCall.FunctionName != "<init>" || mappedCall.OriginPC != call.OriginPC ||
			attachedObject != value || mappedObject != value {
			return reject(fmt.Sprintf("attached call mismatch callPC=%d opcode=%v mapped=%t attachedObject=%T mappedObject=%T", call.OriginPC, op, mappedCall != nil, attachedObject, mappedObject))
		}
		completion = op
	} else {
		for op, call := range d.invokeFuncCall {
			if op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESPECIAL || call == nil ||
				call.FunctionName != "<init>" || call.OriginPC <= value.OriginPC || call.OriginPC >= int(before.CurrentOffset) {
				continue
			}
			if values.UnpackSoltValue(GetRealValue(call.Object)) != value {
				continue
			}
			if completion != nil {
				return reject("more than one invokespecial consumes the allocation")
			}
			completion = op
		}
	}
	if completion == nil || int(completion.CurrentOffset) <= value.OriginPC || completion.CurrentOffset >= before.CurrentOffset {
		return reject(fmt.Sprintf("no completion before spill boundary: match=%v allocationPC=%d beforePC=%d", completion, value.OriginPC, before.CurrentOffset))
	}
	start := d.opcodeAtOffset(value.OriginPC)
	handlers := d.handlersAt(start)
	if start == nil || start.Instr == nil || start.Instr.OpCode != OP_NEW ||
		!sameHandlerCoverage(handlers, d.handlersAt(completion)) ||
		!singleLinearOpcodePathInHandlers(d, start, completion, handlers) {
		return reject(fmt.Sprintf("allocation-to-invokespecial path is not linear under one handler domain: start=%v completion=%v", start, completion))
	}
	return int(completion.CurrentOffset), true
}

// prefixBeforeUse computes Java's evaluated prefix before one value use. It
// rejects deferred/conditional uses, allocation boundaries and opaque closures.
func prefixBeforeUse(d *Decompiler, value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) (values.Access, int, bool) {
	return prefixBeforeUseWithAllocationProof(d, value, ref, sourceOp, false)
}

// Constructor delegation has an additional bytecode proof that can preserve a
// completed allocation before an inlined argument. Keep it separate from the
// generic motion path so that proof never relaxes lambda/generic branch safety.
func prefixBeforeDelegationUse(d *Decompiler, value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode) (values.Access, int, bool) {
	return prefixBeforeUseWithAllocationProof(d, value, ref, sourceOp, true)
}

func prefixBeforeUseWithAllocationProof(d *Decompiler, value values.JavaValue, ref *values.JavaRef, sourceOp *OpCode, allowProvenAllocation bool) (values.Access, int, bool) {
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
			// Allocation and class initialization precede constructor arguments.
			// An older OriginPC alone is not proof that the allocation stayed before
			// this spill across every branch and handler boundary.
			// Keep allocation and class initialization at their bytecode position
			// unless this is a constructor-delegation argument whose complete NEW path
			// is proven before the spill. A linear bytecode path is not enough for
			// generic expression motion across branch-local or lambda scopes.
			if !allowProvenAllocation || !d.hasProvenAllocationPrefix(x, sourceOp) {
				leading.Effects = values.EffectAllocate | values.EffectThrow | values.EffectClassInit
			}
			if x.IsArray() && len(x.Initializer) > 0 {
				// Filling an array is observable and may still be in progress even
				// when its allocation is proven to precede the spilled value.
				leading.Effects |= values.EffectWriteMemory | values.EffectThrow
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
