package rewriter

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
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
		if sameTryLocal(a, local) || sameTryLocal(b, local) {
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
		return sameFinallyEffectTree(rows, x, y, excluded, depth+1, remaining)
	}
	switch x := a.(type) {
	case *values.JavaRef:
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
		if !ok || x == nil || y == nil || x.IsArray() || y.IsArray() || !typeSame(x.JavaType, y.JavaType) || !outside(x.OriginPC, x.HasOriginPC) || !outside(y.OriginPC, y.HasOriginPC) {
			return false
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
		if !ok || x == nil || y == nil || x.Op != y.Op || len(x.Values) != len(y.Values) || len(x.Values) != 2 || !typeSame(x.Type(), y.Type()) {
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
	case *values.CastExpression:
		y, ok := b.(*values.CastExpression)
		return ok && x != nil && y != nil && typeSame(x.TargetType, y.TargetType) && x.Binding == y.Binding && outside(x.OriginPC, !x.Binding) && outside(y.OriginPC, !y.Binding) && same(x.Value, y.Value)
	}
	return false
}

func finallyCoveredAssignment(x *statements.AssignStatement, covered func(int) bool) bool {
	if x == nil || x.ArrayMember != nil || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(x.JavaValue, covered) {
		return false
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
// of a terminal cleanup condition. Separate only that exact primary rethrow;
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
		if len(arm) != 1 || !outsideThrow(arm[0], primary) || len(other) == 0 {
			continue
		}
		thrown, ok := other[len(other)-1].(*statements.CustomStatement)
		if !ok || thrown == nil || thrown.ThrownValue == nil || !thrown.HasOriginPC || thrown.OriginPC < 0 || finallyContains(rows, thrown.OriginPC) || sameTryLocal(thrown.ThrownValue, primary) {
			continue
		}
		if first {
			clone.IfBody = nil
		} else {
			clone.ElseBody = nil
		}
		out := append([]statements.Statement{}, body[:last]...)
		return append(out, &clone), true
	}
	return nil, false
}

// A normal return may occupy the empty completion arm of that same copied
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
		if len(arm) != 1 || len(witness) != 0 || len(other) == 0 {
			continue
		}
		ret, ok := arm[0].(*statements.ReturnStatement)
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
			clone.IfBody = nil
		} else {
			clone.ElseBody = nil
		}
		if sameFinallyCleanup(rows, &clone, y, excluded, 0) {
			return &clone, ret
		}
	}
	return candidate, nil
}
