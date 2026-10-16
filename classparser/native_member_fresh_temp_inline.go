package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	u "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeFreshTempCandidate struct {
	decl   *statements.AssignStatement
	check  *statements.ExpressionStatement
	ref    *values.JavaRef
	value  values.JavaValue
	target *values.NewExpression
}

// Only the evaluation-leading receiver spine may absorb a preceding producer.
// A symbolic static qualifier has no evaluated receiver, so its first
// argument is also leading. Constructor arguments, later static arguments,
// evaluated qualifiers and conditions remain closed.
func nativeFreshLeadingAllocation(s statements.Statement, work *workbudget.Budget) *values.NewExpression {
	var v values.JavaValue
	switch s := s.(type) {
	case *statements.ReturnStatement:
		v = s.JavaValue
	case *statements.ExpressionStatement:
		v = s.Expression
	case *statements.AssignStatement:
		if ref, ok := s.LeftValue.(*values.JavaRef); !ok || ref == nil || ref.CustomValue != nil || ref.StackVar != nil || s.ArrayMember != nil {
			return nil
		}
		v = s.JavaValue
	default:
		return nil
	}
	for depth := 0; depth < 32; depth++ {
		var known bool
		v, known = nativeMemberEnclosingUnpack(v, work)
		if !known {
			return nil
		}
		switch x := v.(type) {
		case *values.NewExpression:
			return x
		case *values.FunctionCallExpression:
			if x.IsStatic || x.Kind == values.InvokeStatic {
				if !x.IsStatic || x.Kind != values.InvokeStatic || x.IsSpecialInvoke || !x.HasOriginPC || len(x.Arguments) == 0 {
					return nil
				}
				qualifier, known := x.Object.(*values.JavaClassValue)
				if !known || qualifier == nil || qualifier.HasOriginPC || qualifier.JavaType == nil {
					return nil
				}
				owner, known := qualifier.JavaType.RawType().(*types.JavaClass)
				if !known || owner == nil || owner.Name == "" || x.ClassName == "" || qualifier.JavaType.IsArray() || !nativeProofWork(work, int64(len(owner.Name)+len(x.ClassName)+1)) || work != nil && work.CheckAlloc(int64(len(owner.Name)+len(x.ClassName))*2) != nil {
					return nil
				}
				if strings.ReplaceAll(owner.Name, ".", "/") != strings.ReplaceAll(x.ClassName, ".", "/") {
					return nil
				}
				v = x.Arguments[0]
			} else {
				v = x.Object
			}
		case *values.CastExpression:
			v = x.Value
		default:
			return nil
		}
	}
	return nil
}

// recoverNativeFreshEnclosingTemps restores a compiler-materialized enclosing
// NEW only when the original allocation packet and a whole-body use count agree.
// The adjacent getClass discard is a proven nonnull compiler check. No producer
// crosses another evaluated operand, statement or control-flow boundary, and
// the original NEW/initialization sites remain the final renderer's authority.
// Discovery and the bounded opaque-aware census finish before any AST mutation.
func (c *ClassObjectDumper) recoverNativeFreshEnclosingTemps(body []statements.Statement) []statements.Statement {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.nativeMemberRoot == nil || c.nativeMemberRoot.owner == "" || c.nativeMemberRoot.failed {
		return body
	}
	if !c.nativeFreshCallerClosed() || strings.ReplaceAll(c.FuncCtx.ClassName, ".", "/") != c.obj.GetClassName() {
		return body
	}
	plans := c.nativeMemberCalls[c.FuncCtx.FunctionName+c.FuncCtx.CurrentMethodDesc]
	if len(plans) == 0 {
		return body
	}
	hasFresh := false
	if !nativeProofWork(c.Work, int64(len(plans))) {
		return body
	}
	for _, plan := range plans {
		if plan != nil && plan.freshEnclosing != nil {
			hasFresh = true
			break
		}
	}
	if !hasFresh {
		return body
	}
	var candidates []nativeFreshTempCandidate
	discoveredEntries := 0
	discover := func(body []statements.Statement) bool {
		if len(body) > 8192-discoveredEntries {
			return false
		}
		discoveredEntries += len(body)
		for i := 0; i+2 < len(body); i++ {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			a, ok := body[i].(*statements.AssignStatement)
			if !ok || a.ArrayMember != nil || !(a.IsFirst || a.IsDeclare) {
				continue
			}
			left, bounded := nativeMemberEnclosingUnpack(a.LeftValue, c.Work)
			if !bounded {
				continue
			}
			ref, ok := left.(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
				continue
			}
			check, ok := body[i+1].(*statements.ExpressionStatement)
			if !ok {
				continue
			}
			cv, bounded := nativeMemberEnclosingUnpack(check.Expression, c.Work)
			if !bounded {
				continue
			}
			call, ok := cv.(*values.FunctionCallExpression)
			if !ok || !call.HasOriginPC || call.IsStatic || call.Kind != values.InvokeVirtual || strings.ReplaceAll(call.ClassName, ".", "/") != "java/lang/Object" || call.FunctionName != "getClass" || call.Descriptor != "()Ljava/lang/Class;" || len(call.Arguments) != 0 {
				continue
			}
			rv, bounded := nativeMemberEnclosingUnpack(call.Object, c.Work)
			if !bounded {
				continue
			}
			receiver, ok := rv.(*values.JavaRef)
			if !ok || receiver == nil || receiver.Id != ref.Id || receiver.CustomValue != nil || receiver.StackVar != nil {
				continue
			}
			target := nativeFreshLeadingAllocation(body[i+2], c.Work)
			if target == nil || target.ConstructorCall == nil || !target.HasOriginPC {
				continue
			}
			ctor := target.ConstructorCall
			if target.Type() == nil || target.IsArray() || ctor.FunctionName != "<init>" || ctor.Kind != values.InvokeSpecial || !ctor.IsSpecialInvoke || ctor.IsStatic || ctor.Object != target {
				continue
			}
			targetOwner, targetKnown := types.RawClassFQN(target.Type())
			if !targetKnown || strings.ReplaceAll(targetOwner, ".", "/") != strings.ReplaceAll(ctor.ClassName, ".", "/") {
				continue
			}
			plan := plans[ctor.OriginPC]
			if !ctor.HasOriginPC || plan == nil || plan.freshEnclosing == nil || plan.child == nil || plan.child.object == nil || plan.child.static || c.nativeMemberRoot.lexicalObjects[plan.child.object.GetClassName()] != plan.child.object || plan.rootObject != nil || target.OriginPC != plan.newPC || ctor.Descriptor != plan.descriptor || strings.ReplaceAll(ctor.ClassName, ".", "/") != plan.child.object.GetClassName() || len(ctor.Arguments) == 0 || call.OriginPC != plan.checkPC {
				continue
			}
			targetType, err := types.ParseMethodDescriptor(ctor.Descriptor)
			if err != nil || len(targetType.FunctionType().ParamTypes) != len(ctor.Arguments) {
				continue
			}
			av, bounded := nativeMemberEnclosingUnpack(ctor.Arguments[0], c.Work)
			if !bounded {
				continue
			}
			argument, ok := av.(*values.JavaRef)
			if !ok || argument == nil || argument.Id != ref.Id || argument.CustomValue != nil || argument.StackVar != nil || !nativeMemberFreshEnclosingOperand(a.JavaValue, plan.freshEnclosing, c.Work) {
				continue
			}
			if c.Work != nil && c.Work.CheckAlloc(int64(len(candidates)+1)*192+int64(len(body))*16) != nil {
				return false
			}
			if !c.nativeFreshQualifierHasNoRegistration(a.JavaValue, plan.freshEnclosing) {
				continue
			}
			candidates = append(candidates, nativeFreshTempCandidate{a, check, ref, a.JavaValue, target})
		}
		return true
	}
	uses := map[*u.VariableId]int{}
	sites := map[int]int{}
	activeV := map[values.JavaValue]bool{}
	activeS := map[statements.Statement]bool{}
	steps := 0
	step := func() bool {
		steps++
		return steps <= 8192 && nativeProofWork(c.Work, 1) && (c.Work == nil || c.Work.CheckAlloc(int64(steps)*128+int64(len(body))*16+int64(len(candidates))*192) == nil)
	}
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		if sourceProofNil(v) {
			return true
		}
		if !step() || activeV[v] {
			return false
		}
		activeV[v] = true
		defer delete(activeV, v)
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil {
			uses[ref.Id]++
		}
		if n, ok := v.(*values.NewExpression); ok && n.ConstructorCall != nil && n.ConstructorCall.HasOriginPC {
			sites[n.ConstructorCall.OriginPC]++
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, x := range children {
			if !visit(x) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement) bool
	walk = func(ss []statements.Statement) bool {
		if !discover(ss) {
			return false
		}
		for _, s := range ss {
			if !step() || sourceProofNil(s) || activeS[s] {
				return false
			}
			activeS[s] = true
			roots, children, known := nativeSourceNameChildren(s)
			if !known {
				return false
			}
			for _, v := range roots {
				if !visit(v) {
					return false
				}
			}
			for _, ss := range children {
				if !walk(ss) {
					return false
				}
			}
			delete(activeS, s)
		}
		return true
	}
	if !walk(body) {
		return body
	}
	if len(candidates) == 0 {
		return body
	}
	removed := map[statements.Statement]bool{}
	var accepted []nativeFreshTempCandidate
	for _, x := range candidates {
		if uses[x.ref.Id] != 3 || sites[x.target.ConstructorCall.OriginPC] != 1 {
			continue
		}
		removed[x.decl], removed[x.check] = true, true
		accepted = append(accepted, x)
		uses[x.ref.Id] = 0
	}
	if len(accepted) == 0 {
		return body
	}
	// Rebuild containers before mutating a single constructor argument. The
	// complete method census above, not a per-block count, licenses each removal.
	projectedSteps := 0
	var project func([]statements.Statement) ([]statements.Statement, bool)
	project = func(list []statements.Statement) ([]statements.Statement, bool) {
		out := make([]statements.Statement, 0, len(list))
		for _, st := range list {
			projectedSteps++
			if projectedSteps > 8192 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(projectedSteps)*256+int64(len(candidates))*192) != nil {
				return nil, false
			}
			if removed[st] {
				continue
			}
			mapped, known := nativeProjectStructuredBlocks(st, project)
			if !known {
				return nil, false
			}
			out = append(out, mapped)
		}
		return out, true
	}
	out, closed := project(body)
	if !closed {
		return body
	}
	for _, x := range accepted {
		x.target.ConstructorCall.Arguments[0] = x.value
	}
	return out
}

// Restoring an enclosing expression must not exchange compiler registration
// events with the member's arguments. This domain has an owned nonsynthetic
// constructor and scalar/local argument expressions, with no private-call,
// lambda, nested-NEW or field-access projection in the qualifier.
func (c *ClassObjectDumper) nativeFreshQualifierHasNoRegistration(value values.JavaValue, proof *nativeMemberFreshEnclosing) bool {
	v, known := nativeMemberEnclosingUnpack(value, c.Work)
	n, ok := v.(*values.NewExpression)
	if !known || !ok || n == nil || n.ConstructorCall == nil {
		return false
	}
	object := c.nativeMemberRoot.lexicalObjects[proof.owner]
	if object == nil || object.GetClassName() != proof.owner || len(object.Methods) > 4096 {
		return false
	}
	found := 0
	for _, m := range object.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		name, nok := sourceBridgeUTF8(object, m.NameIndex)
		desc, dok := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		if name != "<init>" || desc != proof.descriptor {
			continue
		}
		found++
		if m.AccessFlags&0x1000 != 0 {
			return false
		}
		for _, a := range m.Attributes {
			if sourceProofNil(a) || !nativeProofWork(c.Work, 1) {
				return false
			}
			if _, synthetic := a.(*SyntheticAttribute); synthetic {
				return false
			}
		}
	}
	mt, err := types.ParseMethodDescriptor(proof.descriptor)
	if found != 1 || err != nil || len(mt.FunctionType().ParamTypes) != len(n.ConstructorCall.Arguments) {
		return false
	}
	active := map[values.JavaValue]bool{}
	steps := 0
	var visit func(values.JavaValue, int) bool
	visit = func(v values.JavaValue, depth int) bool {
		var known bool
		v, known = nativeMemberEnclosingUnpack(v, c.Work)
		if !known || sourceProofNil(v) || depth >= 32 || steps >= 512 || active[v] || !nativeProofWork(c.Work, 1) {
			return false
		}
		steps++
		active[v] = true
		defer delete(active, v)
		switch x := v.(type) {
		case *values.JavaRef:
			return x != nil && x.CustomValue == nil && x.StackVar == nil
		case *values.JavaLiteral:
			return x != nil
		case *values.JavaExpression:
			if x.Typ == nil {
				return false
			}
			if _, primitive := x.Typ.RawType().(*types.JavaPrimer); !primitive {
				return false
			}
		case *values.CastExpression:
		default:
			return false
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, x := range children {
			if !visit(x, depth+1) {
				return false
			}
		}
		return true
	}
	for _, arg := range n.ConstructorCall.Arguments {
		if !visit(arg, 0) {
			return false
		}
	}
	return true
}
