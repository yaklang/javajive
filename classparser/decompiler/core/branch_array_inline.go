package core

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A handler cannot access an array that exists only on the protected operand
// stack: that stack is discarded on exceptional entry. Prove that every use
// is an initializer store or the sole later invocation, with no local store,
// publication, alias, branch entry or handler-domain change. Named arrays keep
// the conservative rule because their partially filled state can be observed.
func (d *Decompiler) privateArrayFillUnobservable(first, last *Node, firstOp, lastOp *OpCode) bool {
	if d == nil || first == nil || last == nil || firstOp == nil || lastOp == nil {
		return false
	}
	assign, ok := first.Statement.(*statements.AssignStatement)
	if !ok || assign.ArrayMember != nil {
		return false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	array, arrayOK := assign.JavaValue.(*values.NewExpression)
	if !ok || ref == nil || ref.IsParam || ref.IsThis || !arrayOK || !array.HasOriginPC {
		return false
	}
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY ||
		!branchArraySinglePath(d, allocation, lastOp) {
		return false
	}
	uses := 0
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return false
		}
		for index, consumed := range op.stackConsumed {
			if values.UnpackSoltValue(consumed) != ref {
				continue
			}
			switch {
			case op.Instr.OpCode == OP_AASTORE && index == 2 && op.CurrentOffset > allocation.CurrentOffset:
				// The caller checks all element indices and removes the stores
				// atomically. Reject stores outside the one protected prefix.
				if !branchArraySinglePath(d, allocation, op) {
					return false
				}
			case op.Instr.OpCode == OP_DUP && len(op.stackConsumed) == 1 && op.CurrentOffset > allocation.CurrentOffset && op.CurrentOffset < lastOp.CurrentOffset:
			case isInvokeOpcode(op.Instr.OpCode) && op.CurrentOffset > lastOp.CurrentOffset && branchArraySinglePath(d, allocation, op):
				uses++
			default:
				return false
			}
		}
	}
	return uses == 1
}

// Constructor operands join the value DAG only during ParseStatement. Revisit
// proved ternary roots after that phase so calls nested in constructors are
// visible. Registration alone permits no motion; both inlining passes still
// require a private allocation/store/invocation path and complete use evidence.
func (d *Decompiler) RegisterNestedBranchArrayCalls() {
	dupRefs := map[string]bool{}
	for op, infos := range d.opcodeIdToRef {
		if op == nil || op.Instr == nil {
			continue
		}
		switch op.Instr.OpCode {
		case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
			for _, info := range infos {
				if ref, ok := info[0].(*values.JavaRef); ok && ref != nil {
					dupRefs[ref.VarUid] = true
				}
			}
		}
	}
	seenCalls := map[*values.FunctionCallExpression]bool{}
	for _, candidate := range d.branchArrayCalls {
		seenCalls[candidate.call] = true
	}
	seen := map[values.JavaValue]bool{}
	pending := []values.JavaValue{}
	for root := range d.valueTernaryMerges {
		pending = append(pending, root)
	}
	for budget := 512; len(pending) > 0 && budget > 0; budget-- {
		v := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if v == nil || seen[v] {
			continue
		}
		seen[v] = true
		if call, ok := v.(*values.FunctionCallExpression); ok && !seenCalls[call] {
			seenCalls[call] = true
			for i, arg := range call.Arguments {
				ref, ok := values.UnpackSoltValue(arg).(*values.JavaRef)
				if !ok || ref == nil || !dupRefs[ref.VarUid] {
					continue
				}
				array, ok := GetRealValue(ref).(*values.NewExpression)
				if ok && array != nil && array.HasOriginPC {
					d.branchArrayCalls = append(d.branchArrayCalls, branchArrayCall{call, i, ref, array})
				}
			}
		}
		if children, known := values.Children(v); known {
			pending = append(pending, children...)
		}
	}
}

// Transfer a private branch array to its invocation before condition callbacks
// flatten the branch graph. A delayed transfer leaves the array definition as
// a second normal successor of a try node; it must never become a catch body.
func (d *Decompiler) InlinePrivateBranchArrayDefinitions() int {
	if d == nil || d.getenv("JDEC_BRANCH_ARRAY_INLINE_OFF") != "" {
		return 0
	}
	changed := 0
	for _, candidate := range d.branchArrayCalls {
		call, ref, array := candidate.call, candidate.ref, candidate.array
		if call == nil || ref == nil || array == nil || call.FuncType == nil || candidate.argIndex < 0 ||
			candidate.argIndex >= len(call.Arguments) || candidate.argIndex >= len(call.FuncType.ParamTypes) ||
			values.UnpackSoltValue(call.Arguments[candidate.argIndex]) != ref || !array.HasOriginPC ||
			!array.HasEvaluationEndPC || len(array.Initializer) == 0 || array.EvaluationEndPC >= call.OriginPC ||
			!branchArrayKeepsArgumentView(array, ref, call.FuncType.ParamTypes[candidate.argIndex]) {
			continue
		}
		valid := true
		for _, item := range array.Initializer {
			_, known := values.Children(item)
			effect, _ := values.InspectValue(item)
			valid = valid && known && effect&values.EffectOpaque == 0
		}
		allocation, invocation := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
		if !valid || allocation == nil || invocation == nil || !branchArraySinglePath(d, allocation, invocation) {
			continue
		}
		valid = d.branchOperandPrecedesArray(call.Object, allocation)
		for _, earlier := range call.Arguments[:candidate.argIndex] {
			valid = valid && d.branchOperandPrecedesArray(earlier, allocation)
		}
		uses, stores := 0, 0
		for _, op := range d.opCodes {
			if op == nil || op.Instr == nil {
				valid = false
				continue
			}
			if op.CurrentOffset >= allocation.CurrentOffset && op.CurrentOffset < invocation.CurrentOffset && LocalAccessOf(op.Instr.OpCode).Write {
				valid = false
			}
			for index, v := range op.stackConsumed {
				if values.UnpackSoltValue(v) != ref {
					continue
				}
				if op == invocation {
					uses++
				} else if op.Instr.OpCode == OP_AASTORE && index == 2 && int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC {
					stores++
				} else {
					valid = false
				}
			}
		}
		if !valid || uses != 1 || stores != len(array.Initializer) {
			continue
		}
		nodes, _ := guardGraph(d.RootNode)
		var definition *Node
		for _, node := range nodes {
			assign, ok := node.Statement.(*statements.AssignStatement)
			if ok && assign.ArrayMember == nil && values.UnpackSoltValue(assign.LeftValue) == ref && assign.JavaValue == array {
				if definition != nil {
					valid = false
				}
				definition = node
			}
		}
		if !valid || definition == nil || definition.IsTryCatch || definition.IsCatchStart || len(definition.Next) > 1 {
			continue
		}
		original := call.Arguments[candidate.argIndex]
		call.Arguments[candidate.argIndex] = array
		for _, node := range nodes {
			if assign, ok := node.Statement.(*statements.AssignStatement); ok && node.IsCatchStart {
				if exception, ok := values.UnpackSoltValue(assign.JavaValue).(*values.CustomValue); ok && exception.Flag == "exception" &&
					len(exception.Captures) == 0 && values.UnpackSoltValue(assign.LeftValue) != ref {
					// The handler's synthetic stack exception has no local input.
					// Its source closure is opaque but cannot hide an array read.
					continue
				}
			}
			if node != definition && statementReferencesLocal(node.Statement, ref) {
				valid = false
			}
		}
		if !valid {
			call.Arguments[candidate.argIndex] = original
			continue
		}
		for _, source := range slices.Clone(definition.Source) {
			source.ReplaceNextSliceKeepOrder(definition, slices.Clone(definition.Next))
		}
		definition.RemoveAllNext()
		definition.RemoveAllSource()
		changed++
	}
	return changed
}

// Java evaluates a receiver and earlier arguments before this array argument.
// Effectful operands are admitted only when their original producer already
// lies on that same private prefix, so moving the array into the call retains
// their relative order. Identity alone is insufficient across a branch.
func (d *Decompiler) branchOperandPrecedesArray(value values.JavaValue, allocation *OpCode) bool {
	effect, _ := values.InspectValue(value)
	if effect == 0 {
		return true
	}
	if effect&values.EffectOpaque != 0 || allocation == nil {
		return false
	}
	matches := 0
	for _, producer := range d.opCodes {
		if producer == nil || producer.Instr == nil || producer.CurrentOffset >= allocation.CurrentOffset {
			continue
		}
		for _, produced := range producer.stackProduced {
			if values.UnpackSoltValue(produced) == values.UnpackSoltValue(value) &&
				singleLinearOpcodePathInHandlers(d, producer, allocation, d.handlersAt(allocation)) {
				matches++
			}
		}
	}
	return matches == 1
}

// valueOccurrences counts an exact expression object in the unreduced value tree.
// A cycle or an opaque value is not proof that a call occurs only once.
func valueOccurrences(value, target values.JavaValue, path map[values.JavaValue]bool) (int, bool) {
	if value == nil {
		return 0, true
	}
	if path[value] {
		return 0, false
	}
	if value == target {
		return 1, true
	}
	path[value] = true
	defer delete(path, value)
	children, known := values.Children(value)
	if !known {
		return 0, false
	}
	count := 0
	for _, child := range children {
		n, ok := valueOccurrences(child, target, path)
		if !ok {
			return 0, false
		}
		count += n
	}
	return count, true
}

type branchArrayReturn struct {
	list  *[]statements.Statement
	index int
}

// The allocation-to-invocation suffix must have no alternate entry. A single
// successor alone would allow an unrelated branch to join after allocation and
// execute the newly inlined allocation when the original path never did.
func branchArraySinglePath(d *Decompiler, allocation, invocation *OpCode) bool {
	if !singleLinearOpcodePathInHandlers(d, allocation, invocation, d.handlersAt(allocation)) {
		return false
	}
	for cur := allocation; cur != invocation; {
		next := cur.Target[0]
		if len(next.Source) != 1 || next.Source[0] != cur {
			return false
		}
		cur = next
	}
	return true
}

// findBranchArrayReturn accepts only statement/value trees it can inspect.
// Boolean reduction can fold a shared call that occurs twice in the value DAG
// into one rendered occurrence; require that final occurrence to be unique.
func findBranchArrayReturn(body *[]statements.Statement, call *values.FunctionCallExpression, ctx *class_context.ClassContext) (*branchArrayReturn, bool) {
	var found *branchArrayReturn
	count := 0
	callText := call.String(ctx)
	remaining := 512
	var walk func(*[]statements.Statement) bool
	walk = func(list *[]statements.Statement) bool {
		for i, statement := range *list {
			remaining--
			if remaining < 0 || statement == nil {
				return false
			}
			var valuesToCheck []values.JavaValue
			switch s := statement.(type) {
			case *statements.ReturnStatement:
				valuesToCheck = []values.JavaValue{s.JavaValue}
			case *statements.AssignStatement:
				valuesToCheck = []values.JavaValue{s.JavaValue, s.ArrayMember}
			case *statements.ExpressionStatement:
				valuesToCheck = []values.JavaValue{s.Expression}
			case *statements.ConditionStatement:
				valuesToCheck = []values.JavaValue{s.Condition}
			case *statements.IfStatement:
				valuesToCheck = []values.JavaValue{s.Condition}
				if !walk(&s.IfBody) || !walk(&s.ElseBody) {
					return false
				}
			case *statements.TryCatchStatement:
				if !walk(&s.TryBody) {
					return false
				}
				for i := range s.CatchBodies {
					if !walk(&s.CatchBodies[i]) {
						return false
					}
				}
			case *statements.DoWhileStatement:
				valuesToCheck = []values.JavaValue{s.ConditionValue}
				if !walk(&s.Body) {
					return false
				}
			case *statements.SynchronizedStatement:
				valuesToCheck = []values.JavaValue{s.Argument}
				if !walk(&s.Body) {
					return false
				}
			case *statements.MiddleStatement, *statements.NewStatement, *statements.GOTOStatement:
				continue
			default:
				return false
			}
			for _, value := range valuesToCheck {
				n, known := valueOccurrences(value, call, map[values.JavaValue]bool{})
				if !known {
					return false
				}
				if n != 0 {
					valueConsumer := false
					switch consumer := statement.(type) {
					case *statements.ReturnStatement:
						valueConsumer = true
					case *statements.AssignStatement:
						_, local := values.UnpackSoltValue(consumer.LeftValue).(*values.JavaRef)
						valueConsumer = local && consumer.ArrayMember == nil && value == consumer.JavaValue
					}
					if !valueConsumer || strings.Count(value.String(ctx), callText) != 1 {
						return false
					}
					count++
					found = &branchArrayReturn{list: list, index: i}
				}
			}
		}
		return true
	}
	ok := walk(body)
	return found, ok && count == 1 && found != nil
}

func branchArrayInertElement(item values.JavaValue) bool {
	switch v := values.UnpackSoltValue(item).(type) {
	case *values.JavaLiteral:
		return v != nil
	case *values.JavaRef:
		return v != nil && v.CustomValue == nil && v.StackVar == nil
	default:
		return false
	}
}

// InlineDroppedBranchArrayCalls repairs only a branch-local DUP temporary whose
// definition was removed when a short-circuit value merge became one return
// expression. A live allocation in the final statement tree is never
// duplicated. The bytecode checks also keep the allocation in its original arm,
// after all earlier call operands and under the same exception handlers.
func (d *Decompiler) InlineDroppedBranchArrayCalls(body []statements.Statement) int {
	if d == nil || d.getenv("JDEC_BRANCH_ARRAY_INLINE_OFF") != "" {
		return 0
	}
	changed := 0
	for _, candidate := range d.branchArrayCalls {
		call, ref, array := candidate.call, candidate.ref, candidate.array
		if call == nil || ref == nil || array == nil || call.FuncType == nil ||
			candidate.argIndex < 0 || candidate.argIndex >= len(call.Arguments) ||
			candidate.argIndex >= len(call.FuncType.ParamTypes) ||
			values.UnpackSoltValue(call.Arguments[candidate.argIndex]) != ref ||
			!array.IsArray() || !array.HasOriginPC || !array.HasEvaluationEndPC ||
			len(array.Initializer) == 0 || array.EvaluationEndPC >= call.OriginPC ||
			!branchArrayKeepsArgumentView(array, ref, call.FuncType.ParamTypes[candidate.argIndex]) {
			continue
		}
		// The whole private allocation-to-call path preserves initializer
		// order. Closed field/call operands can remain inside that expression;
		// opaque captures cannot establish the complete evaluation tree.
		literal := true
		for _, item := range array.Initializer {
			if !branchArrayInertElement(item) {
				effect, _ := values.InspectValue(item)
				_, known := values.Children(item)
				if !known || effect&values.EffectOpaque != 0 {
					literal = false
					break
				}
			}
		}
		if !call.IsStatic && call.Object != nil {
			if effect, _ := values.InspectValue(call.Object); effect != 0 {
				literal = false
			}
		}

		for _, earlier := range call.Arguments[:candidate.argIndex] {
			effect, _ := values.InspectValue(earlier)
			if effect != 0 {
				literal = false
				break
			}
		}
		if !literal {
			continue
		}
		allocation, invocation := d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(call.OriginPC)
		if allocation == nil || invocation == nil || allocation.Instr == nil || invocation.Instr == nil ||
			allocation.Instr.OpCode != OP_ANEWARRAY ||
			!branchArraySinglePath(d, allocation, invocation) {
			continue
		}
		// A local element must retain its value at the original array store.
		// Even an unrelated slot write is conservatively rejected here; this
		// avoids changing captured values while moving the initializer.
		writesLocal := false
		for cur := allocation; cur != invocation; cur = cur.Target[0] {
			if cur.Instr != nil && LocalAccessOf(cur.Instr.OpCode).Write {
				writesLocal = true
				break
			}
		}
		if writesLocal {
			continue
		}
		uses, stores := 0, 0
		for _, op := range d.opCodes {
			for index, consumed := range op.stackConsumed {
				if values.UnpackSoltValue(consumed) != ref {
					continue
				}
				if op == invocation {
					uses++
				} else if op.Instr != nil && op.Instr.OpCode == OP_AASTORE && index == 2 &&
					int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC {
					stores++
				} else {
					uses += 2
				}
			}
		}
		if uses != 1 || stores != len(array.Initializer) {
			continue
		}
		// A stranded array assignment immediately AFTER its return cannot run;
		// the CFG builder placed it there when flattening the short-circuit arm.
		// Remove exactly that dead definition as the call takes ownership of its
		// allocation. An earlier, live definition blocks this rewrite.
		location, ok := findBranchArrayReturn(&body, call, d.FunctionContext)
		if !ok {
			continue
		}
		var stranded *statements.AssignStatement
		items := *location.list
		if location.index+1 < len(items) {
			assign, isAssign := items[location.index+1].(*statements.AssignStatement)
			if isAssign && assign.ArrayMember == nil && assign.JavaValue == array {
				left, isRef := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
				if !isRef || !values.SameLocal(left, ref) || location.index+2 != len(items) {
					continue
				}
				stranded = assign
			} else if _, returned := items[location.index].(*statements.ReturnStatement); returned {
				continue
			}
		}
		original := call.Arguments[candidate.argIndex]
		call.Arguments[candidate.argIndex] = array
		if statementTreeReferencesLocal(body, ref, stranded) {
			call.Arguments[candidate.argIndex] = original
			continue
		}
		if stranded != nil {
			*location.list = items[:len(items)-1]
		}
		changed++
	}
	return changed
}

// Inline the same array view that the existing argument already presents.
// A generic/covariant array parameter may have a wider erased component type;
// replacing a Token[] local with a Token[] allocation changes no overload or
// type inference input. A widened local declaration does not prove this.
func branchArrayKeepsArgumentView(array *values.NewExpression, ref *values.JavaRef, parameter types.JavaType) bool {
	if array == nil || ref == nil || parameter == nil || !parameter.IsArray() || !sameExactArrayType(array.Type(), ref.Type()) {
		return false
	}
	return ref.WebDeclType == nil || sameExactArrayType(array.Type(), ref.WebDeclType)
}
