package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func collectValueVarReads(v values.JavaValue, ctx *class_context.ClassContext, outIDs map[*coreutils.VariableId]bool, outNames map[string]bool, usesThis *bool, depth int) {
	if v == nil || depth > 256 {
		return
	}
	switch x := values.UnpackSoltValue(v).(type) {
	case *values.JavaRef:
		if x == nil {
			return
		}
		if x.IsThis {
			if usesThis != nil {
				*usesThis = true
			}
			return
		}
		if x.CustomValue != nil {
			collectValueVarReads(x.CustomValue, ctx, outIDs, outNames, usesThis, depth+1)
			return
		}
		if x.StackVar != nil {
			collectValueVarReads(x.StackVar, ctx, outIDs, outNames, usesThis, depth+1)
			return
		}
		if x.Id != nil && outIDs != nil {
			outIDs[x.Id] = true
		}
		if outNames != nil && ctx != nil {
			outNames[x.String(ctx)] = true
		}
		return
	}
	children, known := values.Children(v)
	if !known {
		if n, ok := v.(*values.NewExpression); ok && n != nil && n.ConstructorCall != nil {
			for _, arg := range n.ConstructorCall.Arguments {
				collectValueVarReads(arg, ctx, outIDs, outNames, usesThis, depth+1)
			}
		}
		return
	}
	for _, ch := range children {
		collectValueVarReads(ch, ctx, outIDs, outNames, usesThis, depth+1)
	}
}

func collectStmtVars(sts []statements.Statement, ctx *class_context.ClassContext, ids, writtenIDs map[*coreutils.VariableId]bool, names map[string]bool, usesThis *bool, depth int) {
	if depth > 64 {
		return
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.ReturnStatement:
			if s.JavaValue != nil {
				collectValueVarReads(s.JavaValue, ctx, ids, names, usesThis, 0)
			}
		case *statements.AssignStatement:
			if s.ArrayMember != nil {
				collectValueVarReads(s.ArrayMember, ctx, ids, names, usesThis, 0)
			}
			if s.LeftValue != nil {
				if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil {
					if ref.Id != nil {
						if ids != nil {
							ids[ref.Id] = true
						}
						if writtenIDs != nil {
							writtenIDs[ref.Id] = true
						}
					}
					if names != nil && ctx != nil {
						names[ref.String(ctx)] = true
					}
				} else {
					collectValueVarReads(s.LeftValue, ctx, ids, names, usesThis, 0)
				}
			}
			if s.JavaValue != nil {
				collectValueVarReads(s.JavaValue, ctx, ids, names, usesThis, 0)
			}
		case *statements.ExpressionStatement:
			if s.Expression != nil {
				collectValueVarReads(s.Expression, ctx, ids, names, usesThis, 0)
			}
		case *statements.IfStatement:
			collectValueVarReads(s.Condition, ctx, ids, names, usesThis, 0)
			collectStmtVars(s.IfBody, ctx, ids, writtenIDs, names, usesThis, depth+1)
			collectStmtVars(s.ElseBody, ctx, ids, writtenIDs, names, usesThis, depth+1)
		case *statements.WhileStatement:
			collectValueVarReads(s.ConditionValue, ctx, ids, names, usesThis, 0)
			collectStmtVars(s.Body, ctx, ids, writtenIDs, names, usesThis, depth+1)
		case *statements.DoWhileStatement:
			collectValueVarReads(s.ConditionValue, ctx, ids, names, usesThis, 0)
			collectStmtVars(s.Body, ctx, ids, writtenIDs, names, usesThis, depth+1)
		case *statements.ForStatement:
			if s.InitVar != nil {
				collectStmtVars([]statements.Statement{s.InitVar}, ctx, ids, writtenIDs, names, usesThis, depth+1)
			}
			if s.Condition != nil {
				collectValueVarReads(s.Condition.Condition, ctx, ids, names, usesThis, 0)
			}
			if s.EndExp != nil {
				collectStmtVars([]statements.Statement{s.EndExp}, ctx, ids, writtenIDs, names, usesThis, depth+1)
			}
			collectStmtVars(s.SubStatements, ctx, ids, writtenIDs, names, usesThis, depth+1)
		case *statements.SwitchStatement:
			collectValueVarReads(s.Value, ctx, ids, names, usesThis, 0)
			for _, item := range s.Cases {
				if item != nil {
					collectStmtVars(item.Body, ctx, ids, writtenIDs, names, usesThis, depth+1)
				}
			}
		case *statements.SynchronizedStatement:
			collectValueVarReads(s.Argument, ctx, ids, names, usesThis, 0)
			collectStmtVars(s.Body, ctx, ids, writtenIDs, names, usesThis, depth+1)
		case *statements.TryCatchStatement:
			collectStmtVars(s.TryBody, ctx, ids, writtenIDs, names, usesThis, depth+1)
			for _, body := range s.CatchBodies {
				collectStmtVars(body, ctx, ids, writtenIDs, names, usesThis, depth+1)
			}
		case *statements.CustomStatement:
			if v, ok := s.SourceThrowOperand(); ok && v != nil {
				collectValueVarReads(v, ctx, ids, names, usesThis, 0)
			}
			if assign, ok := s.SourceAssignmentForward(); ok && assign != nil {
				collectStmtVars([]statements.Statement{assign}, ctx, ids, writtenIDs, names, usesThis, depth+1)
			}
		}
	}
}

func countValueVarRefs(v values.JavaValue, targetID *coreutils.VariableId, targetName string, ctx *class_context.ClassContext, depth int) int {
	if v == nil || depth > 256 {
		return 0
	}
	if ref, ok := values.UnpackSoltValue(v).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil {
		if (targetID != nil && ref.Id == targetID) || (ctx != nil && targetName != "" && ref.String(ctx) == targetName) {
			return 1
		}
		return 0
	}
	children, _ := values.Children(v)
	total := 0
	for _, ch := range children {
		total += countValueVarRefs(ch, targetID, targetName, ctx, depth+1)
	}
	return total
}

func countStmtVarRefs(sts []statements.Statement, targetID *coreutils.VariableId, targetName string, ctx *class_context.ClassContext, depth int) int {
	if depth > 64 {
		return 0
	}
	total := 0
	for _, st := range sts {
		switch s := st.(type) {
		case *statements.AssignStatement:
			if s != nil {
				total += countValueVarRefs(s.ArrayMember, targetID, targetName, ctx, 0)
				total += countValueVarRefs(s.JavaValue, targetID, targetName, ctx, 0)
			}
		case *statements.ExpressionStatement:
			if s != nil {
				total += countValueVarRefs(s.Expression, targetID, targetName, ctx, 0)
			}
		case *statements.IfStatement:
			if s != nil {
				total += countValueVarRefs(s.Condition, targetID, targetName, ctx, 0)
				total += countStmtVarRefs(s.IfBody, targetID, targetName, ctx, depth+1)
				total += countStmtVarRefs(s.ElseBody, targetID, targetName, ctx, depth+1)
			}
		case *statements.ReturnStatement:
			if s != nil {
				total += countValueVarRefs(s.JavaValue, targetID, targetName, ctx, 0)
			}
		}
	}
	return total
}

func substCtorValue(v values.JavaValue, env map[*coreutils.VariableId]values.JavaValue, ctx *class_context.ClassContext, depth int) (values.JavaValue, bool) {
	if v == nil {
		return nil, true
	}
	if depth > 256 {
		return nil, false
	}
	switch x := v.(type) {
	case *values.JavaRef:
		if x == nil || x.IsThis {
			return v, true
		}
		if x.CustomValue == nil && x.StackVar == nil && x.Id != nil {
			if rep, ok := env[x.Id]; ok {
				return rep, true
			}
			return v, true
		}
		return v, true
	case *values.SlotValue:
		if x == nil || x.GetValue() == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.GetValue(), env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.GetValue() {
			return v, true
		}
		cp := &values.SlotValue{TmpType: x.TmpType}
		cp.ResetValue(sub)
		return cp, true
	case *values.JavaLiteral, *values.JavaClassValue, *values.JavaClassMember, *types.JavaClass:
		return v, true
	case *values.CastExpression:
		if x == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Value, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Value {
			return v, true
		}
		cp := *x
		cp.Value = sub
		return &cp, true
	case *values.JavaExpression:
		if x == nil {
			return v, true
		}
		changed := false
		vals := make([]values.JavaValue, len(x.Values))
		for i, item := range x.Values {
			sub, ok := substCtorValue(item, env, ctx, depth+1)
			if !ok {
				return nil, false
			}
			vals[i] = sub
			if sub != item {
				changed = true
			}
		}
		if !changed {
			return v, true
		}
		cp := *x
		cp.Values = vals
		return &cp, true
	case *values.TernaryExpression:
		if x == nil {
			return v, true
		}
		sc, ok1 := substCtorValue(x.Condition, env, ctx, depth+1)
		st, ok2 := substCtorValue(x.TrueValue, env, ctx, depth+1)
		sf, ok3 := substCtorValue(x.FalseValue, env, ctx, depth+1)
		if !ok1 || !ok2 || !ok3 {
			return nil, false
		}
		if sc == x.Condition && st == x.TrueValue && sf == x.FalseValue {
			return v, true
		}
		return values.NewTernaryExpression(sc, st, sf), true
	case *values.FunctionCallExpression:
		if x == nil {
			return v, true
		}
		changed := false
		sObj := x.Object
		if x.Object != nil {
			var ok bool
			sObj, ok = substCtorValue(x.Object, env, ctx, depth+1)
			if !ok {
				return nil, false
			}
			if sObj != x.Object {
				changed = true
			}
		}
		sArgs := make([]values.JavaValue, len(x.Arguments))
		for i, arg := range x.Arguments {
			sub, ok := substCtorValue(arg, env, ctx, depth+1)
			if !ok {
				return nil, false
			}
			sArgs[i] = sub
			if sub != arg {
				changed = true
			}
		}
		if !changed {
			return v, true
		}
		cp := x.Clone()
		cp.Object = sObj
		cp.Arguments = sArgs
		return cp, true
	case *values.NewExpression:
		if x == nil {
			return v, true
		}
		changed := false
		sLen := make([]values.JavaValue, len(x.Length))
		for i, item := range x.Length {
			sub, ok := substCtorValue(item, env, ctx, depth+1)
			if !ok {
				return nil, false
			}
			sLen[i] = sub
			if sub != item {
				changed = true
			}
		}
		sInit := make([]values.JavaValue, len(x.Initializer))
		for i, item := range x.Initializer {
			sub, ok := substCtorValue(item, env, ctx, depth+1)
			if !ok {
				return nil, false
			}
			sInit[i] = sub
			if sub != item {
				changed = true
			}
		}
		var sCtor *values.FunctionCallExpression
		if x.ConstructorCall != nil {
			sArgs := make([]values.JavaValue, len(x.ConstructorCall.Arguments))
			ctorChanged := false
			for i, arg := range x.ConstructorCall.Arguments {
				sub, ok := substCtorValue(arg, env, ctx, depth+1)
				if !ok {
					return nil, false
				}
				sArgs[i] = sub
				if sub != arg {
					ctorChanged = true
					changed = true
				}
			}
			if ctorChanged {
				sCtor = x.ConstructorCall.Clone()
				sCtor.Arguments = sArgs
			}
		} else if x.ArgumentsGetter != nil && !x.IsArray() {
			return nil, false
		}
		if !changed {
			return v, true
		}
		cp := *x
		cp.Length = sLen
		cp.Initializer = sInit
		if sCtor != nil {
			sCtor.Object = &cp
			cp.ConstructorCall = sCtor
		}
		return &cp, true
	case *values.RefMember:
		if x == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Object, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Object {
			return v, true
		}
		cp := *x
		cp.Object = sub
		return &cp, true
	case *values.ArrayLengthExpression:
		if x == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Array, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Array {
			return v, true
		}
		cp := *x
		cp.Array = sub
		return &cp, true
	case *values.JavaArrayMember:
		if x == nil {
			return v, true
		}
		sObj, ok1 := substCtorValue(x.Object, env, ctx, depth+1)
		sIdx, ok2 := substCtorValue(x.Index, env, ctx, depth+1)
		if !ok1 || !ok2 {
			return nil, false
		}
		if sObj == x.Object && sIdx == x.Index {
			return v, true
		}
		cp := *x
		cp.Object = sObj
		cp.Index = sIdx
		return &cp, true
	case *values.JavaArray:
		if x == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Length, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Length {
			return v, true
		}
		cp := *x
		cp.Length = sub
		return &cp, true
	case *values.JavaCompare:
		if x == nil {
			return v, true
		}
		s1, ok1 := substCtorValue(x.JavaValue1, env, ctx, depth+1)
		s2, ok2 := substCtorValue(x.JavaValue2, env, ctx, depth+1)
		if !ok1 || !ok2 {
			return nil, false
		}
		if s1 == x.JavaValue1 && s2 == x.JavaValue2 {
			return v, true
		}
		cp := *x
		cp.JavaValue1 = s1
		cp.JavaValue2 = s2
		return &cp, true
	case *values.EffectTag:
		if x == nil || x.Inner == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Inner, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Inner {
			return v, true
		}
		cp := *x
		cp.Inner = sub
		return &cp, true
	case *values.LambdaIntersection:
		if x == nil {
			return v, true
		}
		sub, ok := substCtorValue(x.Value, env, ctx, depth+1)
		if !ok {
			return nil, false
		}
		if sub == x.Value {
			return v, true
		}
		cp := *x
		cp.Value = sub
		return &cp, true
	}
	// Unknown / custom leaf (including javaNull): safe only if it references no replaced variable.
	for id := range env {
		if countValueVarRefs(v, id, "", ctx, depth+1) > 0 {
			return nil, false
		}
	}
	return v, true
}

func ctorRefLHS(st statements.Statement) (*values.JavaRef, values.JavaValue, bool) {
	assign, ok := st.(*statements.AssignStatement)
	if !ok || assign == nil || assign.ArrayMember != nil {
		return nil, nil, false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Id == nil {
		return nil, nil, false
	}
	return ref, assign.JavaValue, true
}

func (c *ClassObjectDumper) foldRetainedConstructorPrefix(p *constructorSourceBoundary, postSuper []statements.Statement) {
	if c == nil || p == nil || len(p.prefix) == 0 || p.delegate == nil {
		return
	}
	postIDs := map[*coreutils.VariableId]bool{}
	collectStmtVars(postSuper, c.FuncCtx, postIDs, nil, nil, nil, 0)

	paramByID := map[*coreutils.VariableId]*values.JavaRef{}
	for _, param := range p.params {
		if ref, ok := param.(*values.JavaRef); ok && ref != nil && ref.Id != nil && !ref.IsThis {
			paramByID[ref.Id] = ref
		}
	}

	env := map[*coreutils.VariableId]values.JavaValue{}
	folded := map[statements.Statement]bool{}
	var remaining []statements.Statement
	mutatedDelegate := false
	delegateArgs := append([]values.JavaValue(nil), p.delegate.Arguments...)

	canInlineExpr := func(idx int, targetID *coreutils.VariableId, rhs values.JavaValue) bool {
		if targetID == nil || rhs == nil || postIDs[targetID] {
			return false
		}
		var rhsThis bool
		collectValueVarReads(rhs, c.FuncCtx, nil, nil, &rhsThis, 0)
		if rhsThis {
			return false
		}
		effects, _ := values.InspectValue(rhs)
		if effects&values.BarrierEffects == 0 {
			return true
		}
		remUses := countStmtVarRefs(p.prefix[idx+1:], targetID, "", c.FuncCtx, 0)
		argUses := 0
		firstArgWithVar := -1
		for ai, arg := range delegateArgs {
			u := countValueVarRefs(arg, targetID, "", c.FuncCtx, 0)
			if u > 0 {
				if firstArgWithVar < 0 {
					firstArgWithVar = ai
				}
				argUses += u
			}
		}
		if remUses+argUses != 1 || remUses != 0 || firstArgWithVar < 0 {
			return false
		}
		for prev := 0; prev < firstArgWithVar; prev++ {
			ae, _ := values.InspectValue(delegateArgs[prev])
			if ae&values.BarrierEffects != 0 {
				return false
			}
		}
		return true
	}

	tryFoldEscapingParamIntoDelegate := func(idx int, lhs *values.JavaRef, subRHS values.JavaValue) bool {
		if lhs == nil || lhs.Id == nil || subRHS == nil || paramByID[lhs.Id] == nil {
			return false
		}
		var rhsThis bool
		collectValueVarReads(subRHS, c.FuncCtx, nil, nil, &rhsThis, 0)
		if rhsThis {
			return false
		}
		if countStmtVarRefs(p.prefix[idx+1:], lhs.Id, "", c.FuncCtx, 0) > 0 {
			return false
		}
		remWritten := map[*coreutils.VariableId]bool{}
		collectStmtVars(p.prefix[idx+1:], c.FuncCtx, nil, remWritten, nil, nil, 0)
		if remWritten[lhs.Id] {
			return false
		}
		firstArg := -1
		for ai, arg := range delegateArgs {
			if countValueVarRefs(arg, lhs.Id, "", c.FuncCtx, 0) > 0 {
				firstArg = ai
				break
			}
		}
		if firstArg < 0 || countValueVarRefs(delegateArgs[firstArg], lhs.Id, "", c.FuncCtx, 0) != 1 {
			return false
		}
		for prev := 0; prev < firstArg; prev++ {
			ae, _ := values.InspectValue(delegateArgs[prev])
			if ae&values.BarrierEffects != 0 {
				return false
			}
		}
		assignExpr := values.NewAssignmentExpression(lhs, subRHS, -1, func(ctx *class_context.ClassContext) string {
			return "(" + lhs.String(ctx) + " = (" + subRHS.String(ctx) + "))"
		})
		replacedArg, ok := substCtorValue(delegateArgs[firstArg], map[*coreutils.VariableId]values.JavaValue{lhs.Id: assignExpr}, c.FuncCtx, 0)
		if !ok {
			return false
		}
		delegateArgs[firstArg] = replacedArg
		delete(env, lhs.Id)
		folded[p.prefix[idx]] = true
		mutatedDelegate = true
		return true
	}

	for idx, st := range p.prefix {
		switch s := st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		case *statements.AssignStatement:
			lhs, rhs, ok := ctorRefLHS(s)
			if !ok {
				remaining = append(remaining, st)
				continue
			}
			if rhs == nil {
				// Pure declaration `T varN;` without initializer for a variable unused after prefix.
				if s.IsDeclare && !postIDs[lhs.Id] {
					folded[st] = true
					continue
				}
				delete(env, lhs.Id)
				remaining = append(remaining, st)
				continue
			}
			subRHS, ok := substCtorValue(rhs, env, c.FuncCtx, 0)
			if !ok {
				delete(env, lhs.Id)
				remaining = append(remaining, st)
				continue
			}
			if canInlineExpr(idx, lhs.Id, subRHS) {
				env[lhs.Id] = subRHS
				folded[st] = true
				continue
			}
			if postIDs[lhs.Id] && tryFoldEscapingParamIntoDelegate(idx, lhs, subRHS) {
				continue
			}
			delete(env, lhs.Id)
			remaining = append(remaining, st)
			continue

		case *statements.IfStatement:
			if s == nil {
				continue
			}
			subCond, ok := substCtorValue(s.Condition, env, c.FuncCtx, 0)
			if !ok {
				written := map[*coreutils.VariableId]bool{}
				collectStmtVars([]statements.Statement{s}, c.FuncCtx, nil, written, nil, nil, 0)
				for id := range written {
					delete(env, id)
				}
				remaining = append(remaining, st)
				continue
			}
			if len(s.IfBody) == 1 && len(s.ElseBody) == 0 {
				lhs, rhs, isAssign := ctorRefLHS(s.IfBody[0])
				if isAssign && rhs != nil {
					var prevVal values.JavaValue
					if v, ok := env[lhs.Id]; ok {
						prevVal = v
					} else if pRef, isParam := paramByID[lhs.Id]; isParam {
						prevVal = pRef
					}
					if prevVal != nil {
						subThen, okThen := substCtorValue(rhs, env, c.FuncCtx, 0)
						ternary := values.NewTernaryExpression(subCond, subThen, prevVal)
						if okThen {
							if canInlineExpr(idx, lhs.Id, ternary) {
								env[lhs.Id] = ternary
								folded[st] = true
								continue
							}
							if postIDs[lhs.Id] && tryFoldEscapingParamIntoDelegate(idx, lhs, ternary) {
								continue
							}
						}
					}
				}
			}
			if len(s.IfBody) == 1 && len(s.ElseBody) == 1 {
				lhs1, rhs1, ok1 := ctorRefLHS(s.IfBody[0])
				lhs2, rhs2, ok2 := ctorRefLHS(s.ElseBody[0])
				if ok1 && ok2 && rhs1 != nil && rhs2 != nil && lhs1.Id == lhs2.Id {
					subThen, okT := substCtorValue(rhs1, env, c.FuncCtx, 0)
					subElse, okE := substCtorValue(rhs2, env, c.FuncCtx, 0)
					if okT && okE {
						ternary := values.NewTernaryExpression(subCond, subThen, subElse)
						if canInlineExpr(idx, lhs1.Id, ternary) {
							env[lhs1.Id] = ternary
							folded[st] = true
							continue
						}
						if postIDs[lhs1.Id] && tryFoldEscapingParamIntoDelegate(idx, lhs1, ternary) {
							continue
						}
					}
				}
			}
			written := map[*coreutils.VariableId]bool{}
			collectStmtVars([]statements.Statement{s}, c.FuncCtx, nil, written, nil, nil, 0)
			for id := range written {
				delete(env, id)
			}
			remaining = append(remaining, st)
			continue

		case *statements.ExpressionStatement:
			if s != nil && s.Expression != nil && len(env) > 0 {
				if sub, ok := substCtorValue(s.Expression, env, c.FuncCtx, 0); ok {
					s.Expression = sub
				} else {
					written := map[*coreutils.VariableId]bool{}
					collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
					for id := range written {
						delete(env, id)
					}
					remaining = append(remaining, st)
					continue
				}
			}
			written := map[*coreutils.VariableId]bool{}
			collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
			for id := range written {
				delete(env, id)
			}
			remaining = append(remaining, st)
			continue
		default:
			written := map[*coreutils.VariableId]bool{}
			collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
			for id := range written {
				delete(env, id)
			}
			remaining = append(remaining, st)
			continue
		}
	}

	if len(folded) == 0 && !mutatedDelegate {
		return
	}
	newArgs := make([]values.JavaValue, len(delegateArgs))
	for i, arg := range delegateArgs {
		sub, ok := substCtorValue(arg, env, c.FuncCtx, 0)
		if !ok {
			return
		}
		newArgs[i] = sub
	}
	p.delegate.Arguments = newArgs
	p.prefix = remaining
	p.foldedStatements = folded
}

func renderRetainedPrefixStatements(sts []statements.Statement, ctx *class_context.ClassContext, declared map[string]bool) (string, bool) {
	var b strings.Builder
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		case *statements.AssignStatement:
			prefix := ""
			if s.ArrayMember == nil && !s.IsDeclare {
				if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil && ctx != nil {
					name := ref.String(ctx)
					if name != "" && !declared[name] && ref.Type() != nil {
						declared[name] = true
						prefix = ref.Type().String(ctx) + " "
					}
				}
			} else if s.IsDeclare && s.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && ref != nil && ctx != nil {
					declared[ref.String(ctx)] = true
				}
			}
			b.WriteString(prefix)
			b.WriteString(s.String(ctx))
			b.WriteString(";\n")
		case *statements.ExpressionStatement, *statements.ReturnStatement:
			b.WriteString(st.String(ctx))
			b.WriteString(";\n")
		case *statements.IfStatement:
			cond := values.SimplifyConditionValue(s.Condition).String(ctx)
			thenStr, ok1 := renderRetainedPrefixStatements(s.IfBody, ctx, declared)
			if !ok1 {
				return "", false
			}
			b.WriteString("if (" + cond + ") {\n" + thenStr + "}")
			if len(s.ElseBody) > 0 {
				elseStr, ok2 := renderRetainedPrefixStatements(s.ElseBody, ctx, declared)
				if !ok2 {
					return "", false
				}
				b.WriteString(" else {\n" + elseStr + "}")
			}
			b.WriteString("\n")
		case *statements.DoWhileStatement:
			bodyStr, okB := renderRetainedPrefixStatements(s.Body, ctx, declared)
			if !okB {
				return "", false
			}
			if s.Label != "" {
				b.WriteString(s.Label + ":\n")
			}
			b.WriteString("do {\n" + bodyStr + "} while (" + values.SimplifyConditionValue(s.ConditionValue).String(ctx) + ");\n")
		default:
			return "", false
		}
	}
	return b.String(), true
}

func (c *ClassObjectDumper) planRetainedConstructorPrefixFallback(p *constructorSourceBoundary, code *CodeAttribute, method *MemberInfo, postSuper []statements.Statement) bool {
	if c == nil || p == nil || p.delegate == nil {
		return false
	}
	// Subcase A: explicit zero-argument `super()` can be omitted so prefix runs after implicit `super()`.
	if len(p.delegate.Arguments) == 0 && strings.ReplaceAll(p.delegate.ClassName, ".", "/") == c.obj.GetSupperClassName() {
		p.omitExplicitSuper = true
		return true
	}

	// Subcase B: none of the variables written anywhere in p.prefix are read by p.delegate arguments.
	prefixWrittenIDs := map[*coreutils.VariableId]bool{}
	prefixAllIDs := map[*coreutils.VariableId]bool{}
	var prefixUsesThis bool
	collectStmtVars(p.prefix, c.FuncCtx, prefixAllIDs, prefixWrittenIDs, nil, &prefixUsesThis, 0)

	argIDs := map[*coreutils.VariableId]bool{}
	for _, arg := range p.delegate.Arguments {
		collectValueVarReads(arg, c.FuncCtx, argIDs, nil, nil, 0)
	}
	usesWrittenInDelegate := false
	for id := range prefixWrittenIDs {
		if argIDs[id] {
			usesWrittenInDelegate = true
			break
		}
	}
	if !usesWrittenInDelegate {
		p.emitPrefixAfterSuper = true
		return true
	}

	// Subcase C: all prefix effects compute into one argument `k` and zero locals escape to postSuper,
	// without referencing `this`.
	if prefixUsesThis || len(p.delegate.Arguments) == 0 {
		return false
	}
	postIDs := map[*coreutils.VariableId]bool{}
	collectStmtVars(postSuper, c.FuncCtx, postIDs, nil, nil, nil, 0)
	for id := range prefixWrittenIDs {
		if postIDs[id] {
			return false
		}
	}

	targetArgIdx := -1
	for ai, arg := range p.delegate.Arguments {
		perArg := map[*coreutils.VariableId]bool{}
		var argUsesThis bool
		collectValueVarReads(arg, c.FuncCtx, perArg, nil, &argUsesThis, 0)
		if argUsesThis {
			return false
		}
		hitsWritten := false
		for id := range prefixWrittenIDs {
			if perArg[id] {
				hitsWritten = true
				break
			}
		}
		if hitsWritten {
			if targetArgIdx >= 0 {
				return false
			}
			targetArgIdx = ai
		}
	}
	if targetArgIdx < 0 {
		return false
	}

	desc, _ := c.obj.getUtf8(method.DescriptorIndex)
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil || len(mt.FunctionType().ParamTypes) != len(p.params) {
		return false
	}
	target, err := types.ParseMethodDescriptor(p.delegate.Descriptor)
	if err != nil || len(target.FunctionType().ParamTypes) != len(p.delegate.Arguments) {
		return false
	}
	declared := map[string]bool{}
	declarations := make([]string, 0, len(p.params))
	for i, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.Id == nil {
			return false
		}
		name := ref.String(c.FuncCtx)
		declared[name] = true
		declarations = append(declarations, mt.FunctionType().ParamTypes[i].String(c.FuncCtx)+" "+name)
	}
	prefixCode, ok := renderRetainedPrefixStatements(p.prefix, c.FuncCtx, declared)
	if !ok {
		return false
	}
	helperName, err := c.constructorBoundaryHelperName("prefix", p.pc)
	if err != nil {
		return false
	}
	retType := target.FunctionType().ParamTypes[targetArgIdx].String(c.FuncCtx)
	argStrings := p.delegate.ArgumentStrings(c.FuncCtx)
	if targetArgIdx >= len(argStrings) {
		return false
	}
	helperBody := prefixCode + "return (" + retType + ")(" + argStrings[targetArgIdx] + ");"
	helperBody = c.wrapCheckedEscapeBody(helperBody)
	p.carrier = helperName
	p.carrierArgIndex = targetArgIdx
	p.retainCarrier = true
	p.helpers = append(p.helpers, &dumpedMethods{
		methodName:    helperName,
		code:          "private static " + retType + " " + helperName + "(" + strings.Join(declarations, ",") + ") {" + helperBody + "}",
		bodyCode:      helperBody,
		checkedEscape: true,
	})
	return true
}
