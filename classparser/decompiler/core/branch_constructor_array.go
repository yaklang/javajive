package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A reconstructed constructor arm must own its fresh array argument. Keeping
// the DUP temporary as a statement splits that arm from its value; a catch's
// final throw can then escape its handler and both arrays can run eagerly.
// Recover ownership before statement structuring, after initializer folding.
func (d *Decompiler) inlineBranchConstructorArrays() {
	if d == nil || d.getenv("JDEC_BRANCH_CONSTRUCTOR_ARRAY_OFF") != "" {
		return
	}
	seen := map[values.JavaValue]bool{}
	var visit func(values.JavaValue)
	visit = func(value values.JavaValue) {
		value = values.UnpackSoltValue(value)
		if value == nil || seen[value] {
			return
		}
		seen[value] = true
		if tern, ok := value.(*values.TernaryExpression); ok {
			visit(tern.TrueValue)
			visit(tern.FalseValue)
			return
		}
		allocation, ok := value.(*values.NewExpression)
		if !ok || allocation == nil || allocation.IsArray() || allocation.ConstructorCall == nil {
			return
		}
		call := allocation.ConstructorCall
		last := len(call.Arguments) - 1
		if last < 0 {
			return
		}
		ref, ok := values.UnpackSoltValue(call.Arguments[last]).(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.IsParam {
			return
		}
		var definition *Node
		var array *values.NewExpression
		count := 0
		WalkGraph[*Node](d.RootNode, func(n *Node) ([]*Node, error) {
			if a, ok := n.Statement.(*statements.AssignStatement); ok && a.ArrayMember == nil {
				if r, ok := values.UnpackSoltValue(a.LeftValue).(*values.JavaRef); ok && values.SameLocal(r, ref) {
					count++
					definition = n
					array, _ = values.UnpackSoltValue(a.JavaValue).(*values.NewExpression)
				}
			}
			return n.Next, nil
		})
		if count != 1 || definition == nil || definition == d.RootNode || len(definition.Next) != 1 ||
			!d.branchConstructorArrayIsolated(allocation, call, ref, array) {
			return
		}
		// Every original use is an initializer DUP/store or this constructor's
		// final argument. Replace that use together with its one declaration.
		call.Arguments[last] = array
		next := definition.Next[0]
		for _, source := range append([]*Node(nil), definition.Source...) {
			source.ReplaceNextSliceKeepOrder(definition, []*Node{next})
			source.ReplaceSwitchTarget(definition, next)
			if source.JmpNode == definition {
				source.JmpNode = next
			}
		}
		definition.RemoveAllSource()
		definition.RemoveAllNext()
		d.varUserMap.Delete(ref)
	}
	for tern := range d.valueTernaryMerges {
		visit(tern)
	}
}

// Prove a single newly allocated reference-array literal, consumed only as
// the last constructor argument. A private forward suffix and unchanged
// handlers keep allocation, element effects and failure order on the arm.
// Replaying each element's decoded operands prevents discarded or duplicated
// calls from being hidden by a syntactically similar initializer.
func (d *Decompiler) branchConstructorArrayIsolated(object *values.NewExpression, call *values.FunctionCallExpression, ref *values.JavaRef, array *values.NewExpression) bool {
	if d == nil || object == nil || call == nil || ref == nil || array == nil ||
		!object.HasOriginPC || !array.HasOriginPC || !array.HasEvaluationEndPC || !array.IsArray() ||
		len(array.Initializer) == 0 || len(array.Initializer) > 128 || len(array.Length) != 1 ||
		!call.IsSpecialInvoke || call.FunctionName != "<init>" || call.FuncType == nil ||
		len(call.Arguments) != len(call.FuncType.ParamTypes) || len(call.Arguments) == 0 {
		return false
	}
	last := len(call.Arguments) - 1
	method, err := types.ParseMethodDescriptor(call.Descriptor)
	if err != nil || method.FunctionType() == nil || method.FunctionType().ReturnType == nil ||
		len(method.FunctionType().ParamTypes) != len(call.Arguments) {
		return false
	}
	ret, ok := method.FunctionType().ReturnType.RawType().(*types.JavaPrimer)
	if !ok || ret.Name != types.JavaVoid || values.UnpackSoltValue(call.Object) != object {
		return false
	}
	owner, ok := types.ClassFQNOf(object.Type())
	if !ok || owner != call.ClassName || values.UnpackSoltValue(call.Arguments[last]) != ref ||
		!sameExactArrayType(array.Type(), call.FuncType.ParamTypes[last]) ||
		!sameExactArrayType(array.Type(), method.FunctionType().ParamTypes[last]) {
		return false
	}
	for _, earlier := range call.Arguments[:last] {
		if !branchArrayInertElement(earlier) {
			return false
		}
	}
	newOp, alloc, end, invoke := d.opcodeAtOffset(object.OriginPC), d.opcodeAtOffset(array.OriginPC), d.opcodeAtOffset(array.EvaluationEndPC), d.opcodeAtOffset(call.OriginPC)
	if newOp == nil || newOp.Instr == nil || newOp.Instr.OpCode != OP_NEW || alloc == nil || alloc.Instr == nil || alloc.Instr.OpCode != OP_ANEWARRAY ||
		end == nil || invoke == nil || invoke.Instr == nil || invoke.Instr.OpCode != OP_INVOKESPECIAL ||
		object.OriginPC >= array.OriginPC || array.EvaluationEndPC >= call.OriginPC ||
		!branchArraySinglePath(d, newOp, invoke) || len(invoke.stackConsumed) != len(call.Arguments)+1 ||
		values.UnpackSoltValue(invoke.stackConsumed[0]) != ref {
		return false
	}
	if len(newOp.stackProduced) != 1 || values.UnpackSoltValue(newOp.stackProduced[0]) != object {
		return false
	}
	decoded := d.invokeFuncCall[invoke]
	if decoded == nil || decoded.Descriptor != call.Descriptor || decoded.FunctionName != "<init>" || decoded.ClassName != call.ClassName ||
		values.UnpackSoltValue(decoded.Object) != values.UnpackSoltValue(call.Object) {
		return false
	}
	for i, arg := range call.Arguments {
		if values.UnpackSoltValue(arg) != values.UnpackSoltValue(invoke.stackConsumed[last-i]) {
			return false
		}
	}
	length, ok := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	if !ok || length == nil {
		return false
	}
	n, ok := length.Data.(int)
	if !ok || n != len(array.Initializer) {
		return false
	}
	if len(alloc.stackProduced) != 1 || values.UnpackSoltValue(alloc.stackProduced[0]) != array {
		return false
	}
	stack := []values.JavaValue{array}
	filled, steps := 0, 0
	for cur := alloc.Target[0]; cur != invoke; cur = cur.Target[0] {
		steps++
		if steps > 256 || cur == nil || cur.Instr == nil {
			return false
		}
		if filled == len(array.Initializer) {
			if cur.Instr.OpCode != OP_NOP || len(cur.stackConsumed) != 0 || len(cur.stackProduced) != 0 {
				return false
			}
			continue
		}
		switch cur.Instr.OpCode {
		case OP_DUP:
			if len(stack) != 1 || stack[0] != array && stack[0] != ref {
				return false
			}
			if stack[0] == array {
				// Materialization pops the allocation, then pushes its new
				// temporary twice. Subsequent DUPs only push another copy.
				if len(cur.stackConsumed) != 1 || values.UnpackSoltValue(cur.stackConsumed[0]) != array || len(cur.stackProduced) != 2 ||
					values.UnpackSoltValue(cur.stackProduced[0]) != ref || values.UnpackSoltValue(cur.stackProduced[1]) != ref {
					return false
				}
			} else if len(cur.stackConsumed) != 0 || len(cur.stackProduced) != 1 || values.UnpackSoltValue(cur.stackProduced[0]) != ref {
				return false
			}
			stack = []values.JavaValue{ref, ref}
		case OP_AASTORE:
			if len(stack) != 4 || stack[0] != ref || stack[1] != ref || len(cur.stackConsumed) != 3 || len(cur.stackProduced) != 0 ||
				values.UnpackSoltValue(cur.stackConsumed[2]) != ref || values.UnpackSoltValue(cur.stackConsumed[0]) != stack[3] ||
				values.UnpackSoltValue(cur.stackConsumed[1]) != stack[2] || values.UnpackSoltValue(array.Initializer[filled]) != stack[3] {
				return false
			}
			index, ok := stack[2].(*values.JavaLiteral)
			if !ok || index == nil || index.Data != filled {
				return false
			}
			filled++
			stack = stack[:1]
			if filled == len(array.Initializer) && cur != end {
				return false
			}
		case OP_NOP:
			if len(cur.stackConsumed) != 0 || len(cur.stackProduced) != 0 {
				return false
			}
		default:
			var ok bool
			stack, ok = d.branchExpressionStackStep(cur, stack)
			if !ok {
				return false
			}
		}
	}
	if filled != len(array.Initializer) || len(stack) != 1 || stack[0] != ref {
		return false
	}
	uses := 0
	for _, op := range d.opCodes {
		for i, value := range op.stackConsumed {
			if values.UnpackSoltValue(value) != ref {
				continue
			}
			if op == invoke && i == 0 {
				uses++
				continue
			}
			if op.Instr != nil && op.Instr.OpCode == OP_AASTORE && i == 2 && int(op.CurrentOffset) > array.OriginPC && int(op.CurrentOffset) <= array.EvaluationEndPC {
				continue
			}
			return false
		}
	}
	return uses == 1
}
