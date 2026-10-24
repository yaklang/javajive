package rewriter

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A copied cleanup is an effect tree, not necessarily a void call on a local.
// Match exact typed operations and operand identities in evaluation order.
// Every possibly throwing evaluation must have an original PC outside every
// protected interval. Nothing is evaluated early and the tree is retained once.
func sameFinallyEffectValue(rows []core.HandlerRange, a, b values.JavaValue, excluded []*values.JavaRef, depth int) bool {
	remaining := 2048
	return sameFinallyEffectTree(rows, a, b, excluded, depth, &remaining)
}

func sameFinallyEffectTree(rows []core.HandlerRange, a, b values.JavaValue, excluded []*values.JavaRef, depth int, remaining *int) bool {
	return sameFinallyEffectTreeBound(rows, a, b, excluded, depth, remaining, nil, nil)
}

func sameFinallyEffectTreeBound(rows []core.HandlerRange, a, b values.JavaValue, excluded []*values.JavaRef, depth int, remaining *int, left, right map[*utils.VariableId]int) bool {
	*remaining--
	if depth > 32 || *remaining < 0 {
		return false
	}
	a, b = plainTryValue(a), plainTryValue(b)
	if a == nil || b == nil {
		return false
	}
	if a == values.JavaNull || b == values.JavaNull {
		return a == values.JavaNull && b == values.JavaNull
	}
	for _, local := range excluded {
		ar, aRef := a.(*values.JavaRef)
		br, bRef := b.(*values.JavaRef)
		if sameTryLocal(a, local) && !(aRef && left[ar.Id] != 0) || sameTryLocal(b, local) && !(bRef && right[br.Id] != 0) {
			return false
		}
	}
	outside := func(pc int, known bool) bool {
		if !known || pc < 0 {
			return false
		}
		for _, row := range rows {
			if pc >= int(row.StartPc) && pc < int(row.EndPc) {
				return false
			}
		}
		return true
	}
	typeSame := func(x, y types.JavaType) bool {
		return x != nil && y != nil && reflect.DeepEqual(x.RawType(), y.RawType())
	}
	same := func(x, y values.JavaValue) bool {
		return sameFinallyEffectTreeBound(rows, x, y, excluded, depth+1, remaining, left, right)
	}
	switch x := a.(type) {
	case *values.JavaRef:
		if x != nil {
			y, ref := b.(*values.JavaRef)
			xid, xbound := left[x.Id]
			if ref && y != nil {
				yid, ybound := right[y.Id]
				if xbound || ybound {
					return xbound && ybound && xid == yid && finallyPureLocal(x) && finallyPureLocal(y) && typeSame(x.Type(), y.Type())
				}
			} else if xbound {
				return false
			}
		}
		return x != nil && finallyPureLocal(x) && sameTryLocal(b, x)
	case *values.JavaLiteral:
		y, ok := b.(*values.JavaLiteral)
		return ok && x != nil && y != nil && typeSame(x.Type(), y.Type()) && reflect.DeepEqual(x.Data, y.Data) && reflect.DeepEqual(x.Units, y.Units)
	case *values.JavaClassMember:
		y, ok := b.(*values.JavaClassMember)
		return ok && x != nil && y != nil && x.Name != "" && x.Member != "" && x.Description != "" && x.Name == y.Name && x.Member == y.Member && x.Description == y.Description && typeSame(x.JavaType, y.JavaType) && outside(x.OriginPC, x.HasOriginPC) && outside(y.OriginPC, y.HasOriginPC)
	case *values.RefMember:
		y, ok := b.(*values.RefMember)
		return ok && x != nil && y != nil && x.Member != "" && x.Member == y.Member && typeSame(x.JavaType, y.JavaType) && outside(x.OriginPC, x.HasOriginPC) && outside(y.OriginPC, y.HasOriginPC) && same(x.Object, y.Object)
	case *values.FunctionCallExpression:
		y, ok := b.(*values.FunctionCallExpression)
		if !ok || x == nil || y == nil || x.Kind >= values.InvokeDynamic || x.Kind != y.Kind || x.IsStatic != y.IsStatic || (x.Kind == values.InvokeStatic) != x.IsStatic || x.IsSpecialInvoke != y.IsSpecialInvoke || x.ClassName == "" || x.ClassName != y.ClassName || x.FunctionName == "" || x.FunctionName != y.FunctionName || x.Descriptor == "" || x.Descriptor != y.Descriptor || len(x.Arguments) != len(y.Arguments) || len(x.Arguments) > 32 || !outside(x.OriginPC, x.HasOriginPC) || !outside(y.OriginPC, y.HasOriginPC) {
			return false
		}
		parsed, err := types.ParseMethodDescriptor(x.Descriptor)
		if err != nil || parsed == nil {
			return false
		}
		abi, ok := parsed.RawType().(*types.JavaFuncType)
		if !ok || len(abi.ParamTypes) != len(x.Arguments) || x.FuncType == nil || y.FuncType == nil || !typeSame(abi.ReturnType, x.FuncType.ReturnType) || !typeSame(abi.ReturnType, y.FuncType.ReturnType) {
			return false
		}
		if x.IsStatic {
			if !((x.Object == nil && y.Object == nil) || (finallyStaticQualifier(x) && finallyStaticQualifier(y))) {
				return false
			}
		} else if !same(x.Object, y.Object) {
			return false
		}
		for i, operand := range x.Arguments {
			if !same(operand, y.Arguments[i]) {
				return false
			}
		}
		return true
	case *values.NewExpression:
		y, ok := b.(*values.NewExpression)
		if !ok || x == nil || y == nil || !typeSame(x.JavaType, y.JavaType) || !outside(x.OriginPC, x.HasOriginPC) || !outside(y.OriginPC, y.HasOriginPC) {
			return false
		}
		if x.IsArray() || y.IsArray() {
			if !finallyArrayEvaluationOutside(rows, x) || !finallyArrayEvaluationOutside(rows, y) || len(x.Length) != len(y.Length) || len(x.Initializer) != len(y.Initializer) {
				return false
			}
			for i, operand := range x.Length {
				if !same(operand, y.Length[i]) {
					return false
				}
			}
			for i, operand := range x.Initializer {
				if !same(operand, y.Initializer[i]) {
					return false
				}
			}
			return true
		}
		cx, cy := x.ConstructorCall, y.ConstructorCall
		if cx == nil || cy == nil || cx.Kind != values.InvokeSpecial || cy.Kind != values.InvokeSpecial || cx.FunctionName != "<init>" || cy.FunctionName != "<init>" {
			return false
		}
		allocationReceiver := func(allocation *values.NewExpression, ctor *values.FunctionCallExpression) bool {
			receiver, ok := plainTryValue(ctor.Object).(*values.NewExpression)
			owner, known := types.RawClassFQN(allocation.JavaType)
			return ok && receiver != nil && receiver.HasOriginPC && receiver.OriginPC == allocation.OriginPC && typeSame(receiver.JavaType, allocation.JavaType) && known && strings.ReplaceAll(owner, ".", "/") == strings.ReplaceAll(ctor.ClassName, ".", "/")
		}
		if !allocationReceiver(x, cx) || !allocationReceiver(y, cy) {
			return false
		}
		// Constructor receivers point back to the allocation. Compare the ABI
		// and arguments without following that intentional graph back edge.
		copyX, copyY := *cx, *cy
		copyX.Object, copyY.Object = values.JavaNull, values.JavaNull
		return same(&copyX, &copyY)
	case *values.JavaExpression:
		y, ok := b.(*values.JavaExpression)
		if !ok || x == nil || y == nil || x.Op != y.Op || len(x.Values) != len(y.Values) || !typeSame(x.Type(), y.Type()) {
			return false
		}
		if x.Op == values.Not {
			return sameFinallyBooleanNotShape(x, y) && same(x.Values[0], y.Values[0])
		}
		if x.Op == values.LOGICAL_AND || x.Op == values.LOGICAL_OR {
			return finallyShortCircuitBooleanShape(x) && finallyShortCircuitBooleanShape(y) && same(x.Values[0], y.Values[0]) && same(x.Values[1], y.Values[1])
		}
		if len(x.Values) != 2 {
			return false
		}
		switch x.Op {
		case values.ADD, values.SUB, values.MUL, values.AND, values.OR, values.XOR, values.SHL, values.SHR, values.USHR, values.EQ, values.NEQ, values.LT, values.LTE, values.GT, values.GTE:
		default:
			return false
		}
		if x.Op != values.EQ && x.Op != values.NEQ {
			for _, operand := range append(append([]values.JavaValue{}, x.Values...), y.Values...) {
				if operand == nil || operand.Type() == nil {
					return false
				}
				p, ok := operand.Type().RawType().(*types.JavaPrimer)
				if !ok {
					return false
				}
				switch p.Name {
				case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
				default:
					return false
				}
			}
		}
		for i, operand := range x.Values {
			if !same(operand, y.Values[i]) {
				return false
			}
		}
		return true
	case *values.CustomValue:
		y, ok := b.(*values.CustomValue)
		if !ok {
			return false
		}
		if x.SameOriginalLambdaSource(y) {
			xpc, xc, xknown := x.OriginalLambdaFactoryEvaluation()
			ypc, yc, yknown := y.OriginalLambdaFactoryEvaluation()
			if !xknown || !yknown || !outside(xpc, true) || !outside(ypc, true) || len(xc) != len(yc) {
				return false
			}
			for i := range xc {
				left := finallyLambdaCaptureOperand(xc[i], xpc, i)
				right := finallyLambdaCaptureOperand(yc[i], ypc, i)
				if left == nil || right == nil || !same(left, right) {
					return false
				}
			}
			return true
		}
		if operand, target, pc, known := x.OriginalInstanceOfSource(); known {
			other, otherTarget, otherPC, otherKnown := y.OriginalInstanceOfSource()
			return otherKnown && target == otherTarget && outside(pc, true) && outside(otherPC, true) && finallyInstanceOfReferenceOperand(operand) && finallyInstanceOfReferenceOperand(other) && same(operand, other)
		}
		xarg, xtarget, _, xknown := x.SourcePrimitiveConversion()
		yarg, ytarget, _, yknown := y.SourcePrimitiveConversion()
		return xknown && yknown && xtarget == ytarget && totalPrimitiveConversionOperand(xarg) && totalPrimitiveConversionOperand(yarg) && same(xarg, yarg)
	case *values.CastExpression:
		y, ok := b.(*values.CastExpression)
		return ok && x != nil && y != nil && typeSame(x.TargetType, y.TargetType) && x.Binding == y.Binding && outside(x.OriginPC, !x.Binding) && outside(y.OriginPC, !y.Binding) && same(x.Value, y.Value)
	}
	return false
}

// Named compiler metadata can require a proved immutable entry operand rather
// than a new source snapshot. Reopen only that exact original inline receipt;
// ref.Val, StackVar, source spelling and a lambda flag confer no such relation.
// The caller still compares the projected eager evaluation in its own domain.
func finallyLambdaCaptureOperand(value values.JavaValue, factoryPC, index int) values.JavaValue {
	for depth := 0; depth < 32; depth++ {
		if slot, known := value.(*values.SlotValue); known && slot != nil {
			value = slot.GetValue()
			continue
		}
		if ref, local := value.(*values.JavaRef); local && ref != nil && ref.StackVar != nil {
			pc, ordinal, original := ref.OriginalInlinedDynamicOperandWitness(ref.StackVar)
			if !original || pc != factoryPC || ordinal != index {
				return nil
			}
			return ref.StackVar
		}
		return value
	}
	return nil
}

// Preserve the short-circuit tree and evaluation order. Boolean source types
// rule out implicit unboxing, while each child's own effect proof retains its
// original domain. Numeric truthiness and boxed Boolean are not this operation.
func finallyShortCircuitBooleanShape(x *values.JavaExpression) bool {
	if x == nil || x.Op != values.LOGICAL_AND && x.Op != values.LOGICAL_OR || len(x.Values) != 2 || x.Type() == nil {
		return false
	}
	for _, v := range []values.JavaValue{x, x.Values[0], x.Values[1]} {
		if v == nil || v.Type() == nil {
			return false
		}
		primitive, ok := v.Type().RawType().(*types.JavaPrimer)
		if !ok || primitive == nil || primitive.Name != types.JavaBoolean {
			return false
		}
	}
	return true
}

func finallyInstanceOfReferenceOperand(v values.JavaValue) bool {
	v = plainTryValue(v)
	return v != nil && (v == values.JavaNull || values.IsNullLiteral(v) || v.Type() != nil && values.ReferenceTypeDescriptor(v.Type(), nil) != "")
}

func finallyCoveredAssignment(x *statements.AssignStatement, covered func(int) bool) bool {
	if x != nil && x.IsDeclare {
		// A declaration inserted for a local join is a source binder, not
		// a JVM evaluation. Do not invent a PC or an initialization value.
		// Every actual store still follows the original effect-domain proof.
		ref, known := x.LeftValue.(*values.JavaRef)
		return known && ref != nil && ref.Id != nil && !ref.IsThis && !ref.IsParam && ref.CustomValue == nil && ref.StackVar == nil && ref.Type() != nil &&
			!x.IsFirst && x.JavaValue == nil && x.ArrayMember == nil
	}
	if x != nil && x.ArrayMember == nil {
		// A source local copy/literal store cannot throw. Some materialized
		// stack copies have no separate JVM store PC; do not invent one merely
		// to keep the surrounding exception region. Exact declared types rule
		// out hidden boxing, unboxing, narrowing casts and erased source views.
		ref, local := x.LeftValue.(*values.JavaRef)
		if local && ref != nil && !ref.IsThis && finallyPureLocal(ref) && ref.Type() != nil && (x.JavaValue == values.JavaNull || values.IsNullLiteral(x.JavaValue)) && (ref.WebDeclType == nil || reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType())) {
			_, reference := types.RawClassFQN(ref.Type())
			if reference || ref.Type().IsArray() {
				return true // null-to-reference has no unboxing or checkcast effect
			}
		}
		if local && ref != nil && !ref.IsThis && finallyPureLocal(ref) && finallyPureLocal(x.JavaValue) && ref.Type() != nil && x.JavaValue.Type() != nil && reflect.DeepEqual(ref.Type().RawType(), x.JavaValue.Type().RawType()) && (ref.WebDeclType == nil || reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType())) {
			if operand, local := plainTryValue(x.JavaValue).(*values.JavaRef); !local || operand.WebDeclType == nil || reflect.DeepEqual(operand.WebDeclType.RawType(), operand.Type().RawType()) {
				return true
			}
		}
	}
	if x == nil || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(x.JavaValue, covered) {
		return false
	}
	// The store itself can throw for null, bounds and reference compatibility.
	// Keep its decoded PC and all array/index/RHS evaluations inside the same
	// handler domain; no computation or check is moved across cleanup.
	if x.ArrayMember != nil {
		return (x.LeftValue == nil || x.LeftValue == x.ArrayMember) && finallyCoveredValue(x.ArrayMember.Object, covered) && finallyCoveredValue(x.ArrayMember.Index, covered)
	}
	switch target := plainTryValue(x.LeftValue).(type) {
	case *values.JavaRef:
		return finallyPureLocal(target)
	case *values.JavaClassMember:
		return target != nil && target.Name != "" && target.Member != "" && target.Description != "" && target.JavaType != nil
	case *values.RefMember:
		return target != nil && target.Member != "" && target.JavaType != nil && finallyCoveredValue(target.Object, covered)
	}
	return false
}

// CFG rendering can absorb the synthetic rethrow into the fall-through arm
// of a terminal cleanup condition. Retain the cleanup prefix of the completion
// arm and separate only its exact primary rethrow;
// the other arm must itself be an abrupt cleanup throw. Its identity remains.
func finallyHandlerCleanup(body []statements.Statement, primary *values.JavaRef, rows [][2]int) ([]statements.Statement, bool) {
	if len(body) == 0 || primary == nil {
		return nil, false
	}
	outsideThrow := func(st statements.Statement, ref *values.JavaRef) bool {
		x, ok := st.(*statements.CustomStatement)
		return ok && x != nil && x.HasOriginPC && x.OriginPC >= 0 && !finallyContains(rows, x.OriginPC) && sameTryLocal(x.ThrownValue, ref)
	}
	last := len(body) - 1
	if outsideThrow(body[last], primary) {
		return body[:last], true
	}
	x, ok := body[last].(*statements.IfStatement)
	if !ok || x == nil {
		return nil, false
	}
	clone := *x
	for _, first := range []bool{true, false} {
		arm, other := finallyWithoutEnd(x.IfBody), finallyWithoutEnd(x.ElseBody)
		if !first {
			arm, other = other, arm
		}
		if len(arm) == 0 || !outsideThrow(arm[len(arm)-1], primary) || len(other) == 0 {
			continue
		}
		if len(arm) > 1 {
			if _, sealed := arm[len(arm)-1].(*statements.CustomStatement).SourceThrowOperand(); !sealed {
				continue
			}
		}
		thrown, ok := other[len(other)-1].(*statements.CustomStatement)
		if !ok || thrown == nil || thrown.ThrownValue == nil || !thrown.HasOriginPC || thrown.OriginPC < 0 || finallyContains(rows, thrown.OriginPC) || sameTryLocal(thrown.ThrownValue, primary) {
			continue
		}
		if first {
			clone.IfBody = arm[:len(arm)-1]
		} else {
			clone.ElseBody = arm[:len(arm)-1]
		}
		out := append([]statements.Statement{}, body[:last]...)
		return append(out, &clone), true
	}
	return nil, false
}

// A normal return may follow the completion arm of that same copied
// conditional cleanup. Lift only its inert local/literal return, after proving
// the rest of the candidate is exactly the canonical cleanup effect tree.
func finallyConditionalReturn(candidate, cleanup statements.Statement, rows []core.HandlerRange, excluded []*values.JavaRef) (statements.Statement, *statements.ReturnStatement) {
	x, ok := candidate.(*statements.IfStatement)
	if !ok || x == nil {
		return candidate, nil
	}
	y, ok := cleanup.(*statements.IfStatement)
	if !ok || y == nil {
		return candidate, nil
	}
	for _, first := range []bool{true, false} {
		arm, witness, other := finallyWithoutEnd(x.IfBody), finallyWithoutEnd(y.IfBody), finallyWithoutEnd(y.ElseBody)
		if !first {
			arm, witness, other = finallyWithoutEnd(x.ElseBody), finallyWithoutEnd(y.ElseBody), finallyWithoutEnd(y.IfBody)
		}
		if len(arm) != len(witness)+1 || len(other) == 0 {
			continue
		}
		ret, ok := arm[len(arm)-1].(*statements.ReturnStatement)
		if !ok || ret == nil || !ret.HasOriginPC || ret.OriginPC < 0 || (ret.JavaValue != nil && !finallyPureLocal(ret.JavaValue)) {
			continue
		}
		outside := true
		for _, r := range rows {
			if ret.OriginPC >= int(r.StartPc) && ret.OriginPC < int(r.EndPc) {
				outside = false
			}
		}
		if !outside {
			continue
		}
		clone := *x
		if first {
			clone.IfBody = arm[:len(arm)-1]
		} else {
			clone.ElseBody = arm[:len(arm)-1]
		}
		if sameFinallyCleanup(rows, &clone, y, excluded, 0) {
			return &clone, ret
		}
	}
	return candidate, nil
}

// Source boolean represents a proved JVM int-category 0/1 word. The sealed
// conversion renderer explicitly recreates that word before the numeric cast.
func totalPrimitiveConversionOperand(v values.JavaValue) bool {
	if v == nil || v.Type() == nil {
		return false
	}
	p, ok := v.Type().RawType().(*types.JavaPrimer)
	if !ok || p == nil {
		return false
	}
	switch p.Name {
	case types.JavaBoolean, types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
		return true
	}
	return false
}
