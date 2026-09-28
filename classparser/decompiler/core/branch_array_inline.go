package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

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
	var walk func(*[]statements.Statement) bool
	walk = func(list *[]statements.Statement) bool {
		for i, statement := range *list {
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
					if _, isReturn := statement.(*statements.ReturnStatement); !isReturn ||
						strings.Count(value.String(ctx), callText) != 1 {
						return false
					}
					count++
					found = &branchArrayReturn{list: list, index: i}
				}
			}
		}
		return true
	}
	return found, walk(body) && count == 1 && found != nil
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
			array.Type().String(d.FunctionContext) != "String[]" ||
			call.FuncType.ParamTypes[candidate.argIndex].String(d.FunctionContext) != "String[]" {
			continue
		}
		// The completed initializer must be a bytecode literal. An effectful
		// element could change timing if moved into the call expression.
		literal := true
		for _, item := range array.Initializer {
			v, ok := item.(*values.JavaLiteral)
			if !ok {
				literal = false
				break
			}
			if _, ok := v.Data.(string); !ok {
				literal = false
				break
			}
		}
		if !literal {
			continue
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
			if location.index+2 != len(items) {
				continue
			}
			assign, isAssign := items[location.index+1].(*statements.AssignStatement)
			if !isAssign || assign.ArrayMember != nil || assign.JavaValue != array {
				continue
			}
			left, isRef := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
			if !isRef || !values.SameLocal(left, ref) {
				continue
			}
			stranded = assign
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
