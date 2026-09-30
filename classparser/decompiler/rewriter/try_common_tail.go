package rewriter

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A shared handler can protect two disjoint branch prefixes while excluding
// their identical normal continuation. Factoring that continuation out of the
// catch preserves the exception-table gaps: a failure in the normal tail must
// not run the handler. Compare typed dependencies and invoke descriptors, never
// method names or rendered Java. Unknown effects and non-terminal handlers fail
// closed. Both original branch results feed one fresh, explicitly declared phi.
func factorUnprotectedTryTail(region *core.Node, tr *statements.TryCatchStatement) (*statements.AssignStatement, []statements.Statement) {
	if region == nil || tr == nil || !region.HasProtectedRange || !region.SharedProtectedHandler || len(region.SharedProtectedRanges) < 2 || len(region.SharedProtectedRanges) > 32 ||
		len(tr.TryBody) != 1 || len(tr.Exception) != 1 || len(tr.CatchBodies) != 1 {
		return nil, nil
	}
	rows := region.SharedProtectedRanges
	for _, row := range rows {
		if row.StartPc >= row.EndPc || row.HandlerPc != rows[0].HandlerPc || row.CatchType != rows[0].CatchType {
			return nil, nil
		}
	}
	branch, ok := tr.TryBody[0].(*statements.IfStatement)
	if !ok || branch == nil || !coveredTryValue(region, branch.Condition) || len(branch.IfBody) < 3 || len(branch.IfBody) > 10 || len(branch.IfBody) != len(branch.ElseBody) {
		return nil, nil
	}
	left, lok := branch.IfBody[0].(*statements.AssignStatement)
	right, rok := branch.ElseBody[0].(*statements.AssignStatement)
	if !lok || !rok || left == nil || right == nil || left.ArrayMember != nil || right.ArrayMember != nil || !left.HasOriginPC || !right.HasOriginPC {
		return nil, nil
	}
	lref, lvalid := plainTryValue(left.LeftValue).(*values.JavaRef)
	rref, rvalid := plainTryValue(right.LeftValue).(*values.JavaRef)
	if !lvalid || !rvalid || lref.IsParam || rref.IsParam || lref.IsThis || rref.IsThis || lref.Id == nil || rref.Id == nil {
		return nil, nil
	}
	for _, store := range []*statements.AssignStatement{left, right} {
		covered := false
		for _, row := range rows {
			if store.OriginPC >= int(row.StartPc) && store.OriginPC < int(row.EndPc) {
				proof := &core.Node{HasProtectedRange: true, ProtectedStartPC: int(row.StartPc), ProtectedEndPC: int(row.EndPc)}
				covered = coveredTryValue(proof, store.JavaValue)
				break
			}
		}
		if !covered {
			return nil, nil
		}
	}
	last := len(branch.IfBody) - 1
	lret, lok := branch.IfBody[last].(*statements.ReturnStatement)
	rret, rok := branch.ElseBody[last].(*statements.ReturnStatement)
	if !lok || !rok || lret == nil || rret == nil || !sameTryLocal(lret.JavaValue, lref) || !sameTryLocal(rret.JavaValue, rref) ||
		!tryHandlerIndependent(tr.CatchBodies[0], lref, rref) {
		return nil, nil
	}
	for _, ex := range tr.Exception {
		if ex == nil || ex.VarUid == lref.VarUid || ex.VarUid == rref.VarUid {
			return nil, nil
		}
	}
	for i := 1; i < last; i++ {
		a, aok := branch.IfBody[i].(*statements.ExpressionStatement)
		b, bok := branch.ElseBody[i].(*statements.ExpressionStatement)
		if !aok || !bok || a == nil || b == nil || !sameUnprotectedTryCall(rows, a.Expression, b.Expression, lref, rref) {
			return nil, nil
		}
		if plainTryValue(a.Expression).(*values.FunctionCallExpression).OriginPC <= left.OriginPC ||
			plainTryValue(b.Expression).(*values.FunctionCallExpression).OriginPC <= right.OriginPC {
			return nil, nil
		}
	}
	lt, rt := left.JavaValue.Type(), right.JavaValue.Type()
	if lt == nil || rt == nil {
		return nil, nil
	}
	typ := lt
	lnull, rnull := values.IsNullLiteral(plainTryValue(left.JavaValue)), values.IsNullLiteral(plainTryValue(right.JavaValue))
	if lnull {
		typ = rt
	}
	if lnull || rnull {
		if _, primitive := typ.RawType().(*types.JavaPrimer); primitive {
			return nil, nil
		}
	} else if lt.String(&class_context.ClassContext{}) != rt.String(&class_context.ClassContext{}) {
		return nil, nil
	}
	result := values.NewJavaRef(utils.NewRootVariableId(), nil, typ.Copy())
	declaration := &statements.AssignStatement{LeftValue: result, IsDeclare: true}
	lstore, rstore := *left, *right
	lstore.LeftValue, lstore.IsDeclare, lstore.IsFirst = result, false, false
	rstore.LeftValue, rstore.IsDeclare, rstore.IsFirst = result, false, false
	tail := append([]statements.Statement{}, branch.IfBody[1:last]...)
	tail = append(tail, statements.NewReturnStatement(result))
	// Mutation begins only after every witness has passed. The condition stays
	// inside the try and is still evaluated exactly once, before either prefix.
	branch.IfBody, branch.ElseBody = []statements.Statement{&lstore}, []statements.Statement{&rstore}
	return declaration, tail
}

func plainTryValue(v values.JavaValue) values.JavaValue {
	for steps := 0; steps < 32; steps++ {
		if slot, ok := v.(*values.SlotValue); ok && slot != nil {
			v = slot.GetValue()
			continue
		}
		if ref, ok := v.(*values.JavaRef); ok && (ref == nil || ref.CustomValue != nil || ref.StackVar != nil) {
			return nil
		}
		return v
	}
	return nil
}

func sameTryLocal(v values.JavaValue, ref *values.JavaRef) bool {
	other, ok := plainTryValue(v).(*values.JavaRef)
	return ok && other != nil && ref != nil && other.Id == ref.Id && other.VarUid == ref.VarUid
}

func coveredTryValue(region *core.Node, v values.JavaValue) bool {
	type frame struct {
		value values.JavaValue
		leave bool
	}
	queue := []frame{{value: v}}
	seen := map[values.JavaValue]uint8{}
	for len(queue) > 0 && len(seen) < 512 {
		f := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if f.value == nil {
			return false
		}
		if f.leave {
			seen[f.value] = 2
			continue
		}
		if seen[f.value] == 1 {
			return false
		}
		if seen[f.value] == 2 {
			continue
		}
		seen[f.value] = 1
		queue = append(queue, frame{value: f.value, leave: true})
		var children []values.JavaValue
		switch x := f.value.(type) {
		case *values.JavaRef:
			if x == nil || x.CustomValue != nil || x.StackVar != nil {
				return false
			}
		case *values.JavaLiteral:
			if x == nil {
				return false
			}
		case *values.SlotValue:
			if x == nil {
				return false
			}
			children = []values.JavaValue{x.GetValue()}
		case *values.FunctionCallExpression:
			if x == nil || !x.HasOriginPC || x.Kind >= values.InvokeDynamic || x.FuncType == nil || x.FuncType.ReturnType == nil || x.Descriptor == "" || x.ClassName == "" || x.FunctionName == "" ||
				(x.Kind == values.InvokeStatic) != x.IsStatic || (!x.IsStatic && x.Object == nil) ||
				x.OriginPC < region.ProtectedStartPC || x.OriginPC >= region.ProtectedEndPC {
				return false
			}
			children = append(children, x.Arguments...)
			if x.Object != nil {
				q, qualifier := x.Object.(*values.JavaClassValue)
				if !qualifier || q == nil || q.JavaType == nil || !x.IsStatic || x.Kind != values.InvokeStatic {
					children = append(children, x.Object)
				} else if c, ok := q.JavaType.RawType().(*types.JavaClass); !ok || c == nil || strings.ReplaceAll(c.Name, ".", "/") != strings.ReplaceAll(x.ClassName, ".", "/") {
					return false
				}
			}
		default:
			return false
		}
		if len(children)+len(queue)+len(seen) > 512 {
			return false
		}
		for _, child := range children {
			queue = append(queue, frame{value: child})
		}
	}
	return len(queue) == 0
}

func sameUnprotectedTryCall(rows []core.HandlerRange, a, b values.JavaValue, excluded ...*values.JavaRef) bool {
	x, xok := plainTryValue(a).(*values.FunctionCallExpression)
	y, yok := plainTryValue(b).(*values.FunctionCallExpression)
	if !xok || !yok || x == nil || y == nil || !x.HasOriginPC || !y.HasOriginPC || x.Kind >= values.InvokeDynamic || x.Descriptor == "" ||
		!strings.HasSuffix(x.Descriptor, ")V") || x.FunctionName == "" || x.ClassName == "" || x.Kind != y.Kind || x.IsStatic != y.IsStatic ||
		x.IsSpecialInvoke != y.IsSpecialInvoke || (x.Kind == values.InvokeStatic) != x.IsStatic || (!x.IsStatic && (x.Object == nil || y.Object == nil)) ||
		x.Descriptor != y.Descriptor || x.ClassName != y.ClassName || x.FunctionName != y.FunctionName || len(x.Arguments) != len(y.Arguments) || len(x.Arguments) > 32 {
		return false
	}
	for _, row := range rows {
		if (x.OriginPC >= int(row.StartPc) && x.OriginPC < int(row.EndPc)) || (y.OriginPC >= int(row.StartPc) && y.OriginPC < int(row.EndPc)) {
			return false
		}
	}
	same := func(a, b values.JavaValue) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		a, b = plainTryValue(a), plainTryValue(b)
		if a == nil || b == nil {
			return false
		}
		for _, ref := range excluded {
			if sameTryLocal(a, ref) || sameTryLocal(b, ref) {
				return false
			}
		}
		switch a := a.(type) {
		case *values.JavaRef:
			return sameTryLocal(b, a)
		case *values.JavaLiteral:
			other, ok := b.(*values.JavaLiteral)
			return a != nil && ok && other != nil && a.Type() != nil && other.Type() != nil && reflect.DeepEqual(a.Data, other.Data) &&
				a.Type().String(&class_context.ClassContext{}) == other.Type().String(&class_context.ClassContext{})
		}
		return false
	}
	if !(finallyStaticQualifier(x) && finallyStaticQualifier(y)) && !same(x.Object, y.Object) {
		return false
	}
	for i := range x.Arguments {
		if !same(x.Arguments[i], y.Arguments[i]) {
			return false
		}
	}
	return true
}

func finallyStaticQualifier(call *values.FunctionCallExpression) bool {
	q, ok := plainTryValue(call.Object).(*values.JavaClassValue)
	if !ok || q == nil || q.JavaType == nil || !call.IsStatic || call.Kind != values.InvokeStatic {
		return false
	}
	c, ok := q.JavaType.RawType().(*types.JavaClass)
	return ok && c != nil && strings.ReplaceAll(c.Name, ".", "/") == strings.ReplaceAll(call.ClassName, ".", "/")
}

// Removing branch-private result definitions is safe only when the handler
// cannot read them. Typed ATHROW retains its operand; opaque statements and
// values cannot supply an independence or terminality proof.
func tryHandlerIndependent(body []statements.Statement, excluded ...*values.JavaRef) bool {
	for len(body) > 0 {
		end, ok := body[len(body)-1].(*statements.MiddleStatement)
		if !ok || end == nil || end.Flag != "end" || end.Data != nil {
			break
		}
		body = body[:len(body)-1]
	}
	if len(body) == 0 {
		return false
	}
	throw, ok := body[len(body)-1].(*statements.CustomStatement)
	if !ok || throw == nil || throw.ThrownValue == nil {
		return false
	}
	queue := append([]statements.Statement{}, body...)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 256 {
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || seen[st] {
			return false
		}
		seen[st] = true
		var roots []values.JavaValue
		switch st := st.(type) {
		case *statements.ExpressionStatement:
			if st == nil {
				return false
			}
			roots = []values.JavaValue{st.Expression}
		case *statements.AssignStatement:
			if st == nil || (st.JavaValue == nil && !st.IsDeclare) {
				return false
			}
			roots = []values.JavaValue{st.LeftValue, st.JavaValue}
			if st.ArrayMember != nil {
				roots = append(roots, st.ArrayMember)
			}
		case *statements.CustomStatement:
			if st == nil || st.ThrownValue == nil {
				return false
			}
			roots = []values.JavaValue{st.ThrownValue}
		case *statements.TryCatchStatement:
			if st == nil {
				return false
			}
			for _, ex := range st.Exception {
				roots = append(roots, ex)
			}
		case *statements.MiddleStatement:
			if st == nil || st.Flag != "end" || st.Data != nil {
				return false
			}
		default:
			return false
		}
		for _, root := range roots {
			if root == nil {
				continue
			}
			if !tryValueIndependent(root, excluded) {
				return false
			}
		}
		for _, list := range childStatementLists(st) {
			if len(queue)+len(seen)+len(*list) > 256 {
				return false
			}
			queue = append(queue, (*list)...)
		}
	}
	return len(queue) == 0
}

func tryValueIndependent(root values.JavaValue, excluded []*values.JavaRef) bool {
	type frame struct {
		value values.JavaValue
		leave bool
	}
	queue := []frame{{value: root}}
	seen := map[values.JavaValue]uint8{}
	for len(queue) > 0 && len(seen) < 512 {
		f := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if f.value == nil {
			continue
		}
		if value := reflect.ValueOf(f.value); value.Kind() == reflect.Pointer && value.IsNil() {
			return false
		}
		if f.leave {
			seen[f.value] = 2
			continue
		}
		if seen[f.value] == 1 {
			return false
		}
		if seen[f.value] == 2 {
			continue
		}
		seen[f.value] = 1
		if ref, ok := f.value.(*values.JavaRef); ok && ref != nil {
			for _, other := range excluded {
				if ref.VarUid == other.VarUid {
					return false
				}
			}
		}
		children, known := values.Children(f.value)
		if !known || len(children)+len(queue)+len(seen)+1 > 512 {
			return false
		}
		queue = append(queue, frame{value: f.value, leave: true})
		for _, child := range children {
			queue = append(queue, frame{value: child})
		}
	}
	return len(queue) == 0
}
