package javaclassparser

import (
	"sort"
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

func collectStmtVarReads(sts []statements.Statement, ctx *class_context.ClassContext, reads map[*coreutils.VariableId]bool, depth int) {
	if depth > 64 || reads == nil {
		return
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.ReturnStatement:
			if s.JavaValue != nil {
				collectValueVarReads(s.JavaValue, ctx, reads, nil, nil, 0)
			}
		case *statements.AssignStatement:
			if s.ArrayMember != nil {
				collectValueVarReads(s.ArrayMember, ctx, reads, nil, nil, 0)
			}
			if s.LeftValue != nil {
				ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
				if !ok || ref == nil || ref.CustomValue != nil || ref.StackVar != nil {
					collectValueVarReads(s.LeftValue, ctx, reads, nil, nil, 0)
				}
			}
			if s.JavaValue != nil {
				collectValueVarReads(s.JavaValue, ctx, reads, nil, nil, 0)
			}
		case *statements.ExpressionStatement:
			if s.Expression != nil {
				collectValueVarReads(s.Expression, ctx, reads, nil, nil, 0)
			}
		case *statements.IfStatement:
			collectValueVarReads(s.Condition, ctx, reads, nil, nil, 0)
			collectStmtVarReads(s.IfBody, ctx, reads, depth+1)
			collectStmtVarReads(s.ElseBody, ctx, reads, depth+1)
		case *statements.WhileStatement:
			collectValueVarReads(s.ConditionValue, ctx, reads, nil, nil, 0)
			collectStmtVarReads(s.Body, ctx, reads, depth+1)
		case *statements.DoWhileStatement:
			collectValueVarReads(s.ConditionValue, ctx, reads, nil, nil, 0)
			collectStmtVarReads(s.Body, ctx, reads, depth+1)
		case *statements.ForStatement:
			if s.InitVar != nil {
				collectStmtVarReads([]statements.Statement{s.InitVar}, ctx, reads, depth+1)
			}
			if s.Condition != nil {
				collectValueVarReads(s.Condition.Condition, ctx, reads, nil, nil, 0)
			}
			if s.EndExp != nil {
				collectStmtVarReads([]statements.Statement{s.EndExp}, ctx, reads, depth+1)
			}
			collectStmtVarReads(s.SubStatements, ctx, reads, depth+1)
		case *statements.SwitchStatement:
			collectValueVarReads(s.Value, ctx, reads, nil, nil, 0)
			for _, item := range s.Cases {
				if item != nil {
					collectStmtVarReads(item.Body, ctx, reads, depth+1)
				}
			}
		case *statements.SynchronizedStatement:
			collectValueVarReads(s.Argument, ctx, reads, nil, nil, 0)
			collectStmtVarReads(s.Body, ctx, reads, depth+1)
		case *statements.TryCatchStatement:
			collectStmtVarReads(s.TryBody, ctx, reads, depth+1)
			for _, body := range s.CatchBodies {
				collectStmtVarReads(body, ctx, reads, depth+1)
			}
		case *statements.CustomStatement:
			if v, ok := s.SourceThrowOperand(); ok && v != nil {
				collectValueVarReads(v, ctx, reads, nil, nil, 0)
			}
			if assign, ok := s.SourceAssignmentForward(); ok && assign != nil {
				collectStmtVarReads([]statements.Statement{assign}, ctx, reads, depth+1)
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
				if s.LeftValue != nil {
					if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); !ok || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
						total += countValueVarRefs(s.LeftValue, targetID, targetName, ctx, 0)
					}
				}
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
		case *statements.CustomStatement:
			if s != nil {
				if v, ok := s.SourceThrowOperand(); ok && v != nil {
					total += countValueVarRefs(v, targetID, targetName, ctx, 0)
				}
				if assign, ok := s.SourceAssignmentForward(); ok && assign != nil {
					total += countStmtVarRefs([]statements.Statement{assign}, targetID, targetName, ctx, depth+1)
				}
			}
		case *statements.WhileStatement:
			if s != nil {
				total += countValueVarRefs(s.ConditionValue, targetID, targetName, ctx, 0)
				total += countStmtVarRefs(s.Body, targetID, targetName, ctx, depth+1)
			}
		case *statements.DoWhileStatement:
			if s != nil {
				total += countValueVarRefs(s.ConditionValue, targetID, targetName, ctx, 0)
				total += countStmtVarRefs(s.Body, targetID, targetName, ctx, depth+1)
			}
		case *statements.ForStatement:
			if s != nil {
				if s.InitVar != nil {
					total += countStmtVarRefs([]statements.Statement{s.InitVar}, targetID, targetName, ctx, depth+1)
				}
				if s.Condition != nil {
					total += countValueVarRefs(s.Condition.Condition, targetID, targetName, ctx, 0)
				}
				if s.EndExp != nil {
					total += countStmtVarRefs([]statements.Statement{s.EndExp}, targetID, targetName, ctx, depth+1)
				}
				total += countStmtVarRefs(s.SubStatements, targetID, targetName, ctx, depth+1)
			}
		case *statements.SwitchStatement:
			if s != nil {
				total += countValueVarRefs(s.Value, targetID, targetName, ctx, 0)
				for _, item := range s.Cases {
					if item != nil {
						total += countStmtVarRefs(item.Body, targetID, targetName, ctx, depth+1)
					}
				}
			}
		case *statements.SynchronizedStatement:
			if s != nil {
				total += countValueVarRefs(s.Argument, targetID, targetName, ctx, 0)
				total += countStmtVarRefs(s.Body, targetID, targetName, ctx, depth+1)
			}
		case *statements.TryCatchStatement:
			if s != nil {
				total += countStmtVarRefs(s.TryBody, targetID, targetName, ctx, depth+1)
				for _, body := range s.CatchBodies {
					total += countStmtVarRefs(body, targetID, targetName, ctx, depth+1)
				}
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
	case *values.CustomValue:
		if x == nil {
			return v, true
		}
		if op, target, pc, ok := x.SourcePrimitiveConversion(); ok {
			sub, sok := substCtorValue(op, env, ctx, depth+1)
			if !sok {
				return nil, false
			}
			if sub == op {
				return v, true
			}
			rebuilt := values.NewOriginalPrimitiveConversion(sub, types.NewJavaPrimer(target), pc)
			if rebuilt == nil {
				return nil, false
			}
			return rebuilt, true
		}
		if op, desc, pc, ok := x.OriginalInstanceOfSource(); ok {
			sub, sok := substCtorValue(op, env, ctx, depth+1)
			if !sok {
				return nil, false
			}
			if sub == op {
				return v, true
			}
			targetType, err := types.ParseDescriptor(desc)
			if err != nil || targetType == nil {
				return nil, false
			}
			rebuilt := values.NewOriginalInstanceOf(sub, targetType, pc)
			if rebuilt == nil {
				return nil, false
			}
			return rebuilt, true
		}
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
	var assign *statements.AssignStatement
	switch s := st.(type) {
	case *statements.AssignStatement:
		assign = s
	case *statements.CustomStatement:
		if s != nil {
			assign, _ = s.SourceAssignmentForward()
		}
	}
	if assign == nil || assign.ArrayMember != nil {
		return nil, nil, false
	}
	ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Id == nil {
		return nil, nil, false
	}
	return ref, assign.JavaValue, true
}

func ctorAssignStmt(st statements.Statement) *statements.AssignStatement {
	switch s := st.(type) {
	case *statements.AssignStatement:
		return s
	case *statements.CustomStatement:
		if s != nil {
			if assign, ok := s.SourceAssignmentForward(); ok {
				return assign
			}
		}
	}
	return nil
}

func ctorDelegateEvalOrder(args []values.JavaValue) (useRank map[*coreutils.VariableId]int, useCount map[*coreutils.VariableId]int, condUsed map[*coreutils.VariableId]bool, firstOriginalBarrierRank int) {
	useRank = map[*coreutils.VariableId]int{}
	useCount = map[*coreutils.VariableId]int{}
	condUsed = map[*coreutils.VariableId]bool{}
	firstOriginalBarrierRank = -1
	nextRank := 0
	recordBarrier := func() {
		if firstOriginalBarrierRank < 0 {
			firstOriginalBarrierRank = nextRank
		}
		nextRank++
	}
	var walk func(v values.JavaValue, inCond bool, depth int)
	walk = func(v values.JavaValue, inCond bool, depth int) {
		if v == nil || depth > 256 {
			if depth > 256 {
				recordBarrier()
			}
			return
		}
		switch x := v.(type) {
		case *values.SlotValue:
			if x != nil && x.GetValue() != nil {
				walk(x.GetValue(), inCond, depth+1)
			}
		case *values.JavaRef:
			if x == nil || x.IsThis {
				if x != nil && x.IsThis {
					recordBarrier()
				}
				return
			}
			if x.CustomValue != nil {
				walk(x.CustomValue, inCond, depth+1)
				return
			}
			if x.StackVar != nil {
				walk(x.StackVar, inCond, depth+1)
				return
			}
			if x.Id != nil {
				useCount[x.Id]++
				useRank[x.Id] = nextRank
				nextRank++
				if inCond {
					condUsed[x.Id] = true
				}
			}
		case *values.JavaLiteral, *types.JavaClass:
		case *values.JavaClassValue:
			if eff, _ := values.InspectValue(x); eff&values.BarrierEffects != 0 {
				recordBarrier()
			}
		case *values.JavaClassMember:
			recordBarrier()
		case *values.CastExpression:
			if x != nil {
				walk(x.Value, inCond, depth+1)
				recordBarrier()
			}
		case *values.RefMember:
			if x != nil {
				walk(x.Object, inCond, depth+1)
				recordBarrier()
			}
		case *values.ArrayLengthExpression:
			if x != nil {
				walk(x.Array, inCond, depth+1)
				recordBarrier()
			}
		case *values.JavaArrayMember:
			if x != nil {
				walk(x.Object, inCond, depth+1)
				walk(x.Index, inCond, depth+1)
				recordBarrier()
			}
		case *values.JavaArray:
			if x != nil {
				walk(x.Length, inCond, depth+1)
				recordBarrier()
			}
		case *values.JavaCompare:
			if x != nil {
				walk(x.JavaValue1, inCond, depth+1)
				walk(x.JavaValue2, inCond, depth+1)
			}
		case *values.JavaExpression:
			if x != nil {
				if (x.Op == "&&" || x.Op == "||") && len(x.Values) > 0 {
					walk(x.Values[0], inCond, depth+1)
					for _, item := range x.Values[1:] {
						walk(item, true, depth+1)
					}
				} else {
					for _, item := range x.Values {
						walk(item, inCond, depth+1)
					}
				}
				if eff, _ := values.InspectValue(x); eff&values.BarrierEffects != 0 {
					recordBarrier()
				}
			}
		case *values.TernaryExpression:
			if x != nil {
				walk(x.Condition, inCond, depth+1)
				walk(x.TrueValue, true, depth+1)
				walk(x.FalseValue, true, depth+1)
			}
		case *values.FunctionCallExpression:
			if x != nil {
				if !x.IsStatic && x.Object != nil {
					walk(x.Object, inCond, depth+1)
				}
				for _, arg := range x.Arguments {
					walk(arg, inCond, depth+1)
				}
				recordBarrier()
			}
		case *values.NewExpression:
			if x != nil {
				for _, l := range x.Length {
					walk(l, inCond, depth+1)
				}
				for _, initVal := range x.Initializer {
					walk(initVal, inCond, depth+1)
				}
				if x.ConstructorCall != nil {
					for _, arg := range x.ConstructorCall.Arguments {
						walk(arg, inCond, depth+1)
					}
				}
				recordBarrier()
			}
		case *values.AssignmentExpression:
			if x != nil {
				walk(x.Target, inCond, depth+1)
				walk(x.Value, inCond, depth+1)
				recordBarrier()
			}
		case *values.EffectTag:
			if x != nil {
				walk(x.Inner, inCond, depth+1)
				if eff, _ := values.InspectValue(x); eff&values.BarrierEffects != 0 {
					recordBarrier()
				}
			}
		case *values.LambdaIntersection:
			if x != nil {
				walk(x.Value, inCond, depth+1)
			}
		case *values.CustomValue:
			if x != nil {
				if op, _, _, ok := x.SourcePrimitiveConversion(); ok {
					walk(op, inCond, depth+1)
				} else if op, _, _, ok := x.OriginalInstanceOfSource(); ok {
					walk(op, inCond, depth+1)
					recordBarrier()
				} else {
					if children, known := values.Children(x); known {
						for _, ch := range children {
							walk(ch, inCond, depth+1)
						}
					}
					recordBarrier()
				}
			}
		default:
			children, known := values.Children(v)
			if known {
				for _, ch := range children {
					walk(ch, inCond, depth+1)
				}
			}
			if eff, _ := values.InspectValue(v); !known || eff&values.BarrierEffects != 0 {
				recordBarrier()
			}
		}
	}
	for _, arg := range args {
		walk(arg, false, 0)
	}
	return useRank, useCount, condUsed, firstOriginalBarrierRank
}

func ctorDelegateWritesVar(args []values.JavaValue, id *coreutils.VariableId) bool {
	if id == nil {
		return false
	}
	seen := map[values.JavaValue]bool{}
	var check func(v values.JavaValue, depth int) bool
	check = func(v values.JavaValue, depth int) bool {
		if v == nil || depth > 256 || seen[v] {
			return false
		}
		seen[v] = true
		if ae, ok := v.(*values.AssignmentExpression); ok && ae != nil {
			if ref, ok := values.UnpackSoltValue(ae.Target).(*values.JavaRef); ok && ref != nil && ref.Id == id {
				return true
			}
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, ch := range children {
			if check(ch, depth+1) {
				return true
			}
		}
		return false
	}
	for _, arg := range args {
		if check(arg, 0) {
			return true
		}
	}
	return false
}

func isPureDeferrableCtorRHS(rhs values.JavaValue, paramByID map[*coreutils.VariableId]*values.JavaRef, prefixWritten map[*coreutils.VariableId]bool, delegateArgs []values.JavaValue) bool {
	if rhs == nil {
		return false
	}
	unpacked := values.UnpackSoltValue(rhs)
	switch v := unpacked.(type) {
	case *values.JavaLiteral, *values.JavaClassValue:
		return v != nil
	case *values.JavaRef:
		if v == nil || v.IsThis || v.CustomValue != nil || v.StackVar != nil || v.Id == nil || paramByID[v.Id] == nil {
			return false
		}
		return !prefixWritten[v.Id] && !ctorDelegateWritesVar(delegateArgs, v.Id)
	default:
		return unpacked == values.JavaNull
	}
}

func ctorIntLiteral(v values.JavaValue) (int, bool) {
	lit, ok := values.UnpackSoltValue(v).(*values.JavaLiteral)
	if !ok || lit == nil {
		return 0, false
	}
	switch n := lit.Data.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func tryFoldCtorArrayInit(prefix []statements.Statement, idx int, arrID *coreutils.VariableId, ne *values.NewExpression, env map[*coreutils.VariableId]values.JavaValue, postIDs map[*coreutils.VariableId]bool, paramByID map[*coreutils.VariableId]*values.JavaRef, useCount map[*coreutils.VariableId]int, ctx *class_context.ClassContext) (*values.NewExpression, []int, int, bool) {
	if arrID == nil || ne == nil || !ne.IsArray() || ne.JavaType == nil || ne.JavaType.ArrayDim() != 1 || len(ne.Length) != 1 || len(ne.Initializer) != 0 {
		return nil, nil, 0, false
	}
	lenVal, ok := substCtorValue(ne.Length[0], env, ctx, 0)
	if !ok {
		return nil, nil, 0, false
	}
	n, ok := ctorIntLiteral(lenVal)
	if !ok || n < 1 || n > 32 {
		return nil, nil, 0, false
	}
	localEnv := make(map[*coreutils.VariableId]values.JavaValue, len(env)+4)
	for k, v := range env {
		localEnv[k] = v
	}
	elems := make([]values.JavaValue, n)
	consumed := []int{idx}
	tempIDs := map[*coreutils.VariableId]bool{}
	var pendingTempID *coreutils.VariableId
	var pendingTempVal values.JavaValue
	nextK := 0
	endIdx := idx
	for j := idx + 1; j < len(prefix) && nextK < n; j++ {
		st := prefix[j]
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		}
		assign := ctorAssignStmt(st)
		if assign == nil {
			return nil, nil, 0, false
		}
		if assign.ArrayMember != nil {
			objRef, ok := values.UnpackSoltValue(assign.ArrayMember.Object).(*values.JavaRef)
			if !ok || objRef == nil || objRef.Id != arrID || assign.JavaValue == nil {
				return nil, nil, 0, false
			}
			if pendingTempID != nil && countValueVarRefs(assign.ArrayMember.Index, pendingTempID, "", ctx, 0) > 0 {
				return nil, nil, 0, false
			}
			subIndex, ok := substCtorValue(assign.ArrayMember.Index, localEnv, ctx, 0)
			if !ok {
				return nil, nil, 0, false
			}
			kVal, ok := ctorIntLiteral(subIndex)
			if !ok || kVal != nextK {
				return nil, nil, 0, false
			}
			var elemVal values.JavaValue
			if pendingTempID != nil {
				rhsRef, ok := values.UnpackSoltValue(assign.JavaValue).(*values.JavaRef)
				if !ok || rhsRef == nil || rhsRef.IsThis || rhsRef.CustomValue != nil || rhsRef.StackVar != nil || rhsRef.Id != pendingTempID {
					return nil, nil, 0, false
				}
				elemVal = pendingTempVal
				pendingTempID = nil
				pendingTempVal = nil
			} else {
				var errOK bool
				elemVal, errOK = substCtorValue(assign.JavaValue, localEnv, ctx, 0)
				if !errOK {
					return nil, nil, 0, false
				}
			}
			var elemThis bool
			collectValueVarReads(elemVal, ctx, nil, nil, &elemThis, 0)
			if elemThis || countValueVarRefs(elemVal, arrID, "", ctx, 0) > 0 {
				return nil, nil, 0, false
			}
			elems[nextK] = elemVal
			consumed = append(consumed, j)
			nextK++
			endIdx = j
			continue
		}
		lhs, rhs, isRefAssign := ctorRefLHS(st)
		if !isRefAssign || rhs == nil || lhs.Id == arrID || postIDs[lhs.Id] || paramByID[lhs.Id] != nil || tempIDs[lhs.Id] || lhs.Id == pendingTempID {
			return nil, nil, 0, false
		}
		if _, shadows := env[lhs.Id]; shadows {
			return nil, nil, 0, false
		}
		if pendingTempID != nil && countValueVarRefs(rhs, pendingTempID, "", ctx, 0) > 0 {
			return nil, nil, 0, false
		}
		subRHS, ok := substCtorValue(rhs, localEnv, ctx, 0)
		if !ok {
			return nil, nil, 0, false
		}
		var rhsThis bool
		collectValueVarReads(subRHS, ctx, nil, nil, &rhsThis, 0)
		if rhsThis || countValueVarRefs(subRHS, arrID, "", ctx, 0) > 0 {
			return nil, nil, 0, false
		}
		eff, _ := values.InspectValue(subRHS)
		if eff&values.BarrierEffects == 0 {
			localEnv[lhs.Id] = subRHS
		} else {
			if pendingTempID != nil {
				return nil, nil, 0, false
			}
			pendingTempID = lhs.Id
			pendingTempVal = subRHS
		}
		tempIDs[lhs.Id] = true
		consumed = append(consumed, j)
		endIdx = j
	}
	if nextK != n || pendingTempID != nil {
		return nil, nil, 0, false
	}
	tailAll := map[*coreutils.VariableId]bool{}
	collectStmtVars(prefix[endIdx+1:], ctx, tailAll, nil, nil, nil, 0)
	if tailAll[arrID] {
		return nil, nil, 0, false
	}
	for tid := range tempIDs {
		if postIDs[tid] || tailAll[tid] || useCount[tid] > 0 {
			return nil, nil, 0, false
		}
	}
	cp := *ne
	cp.Length = nil
	cp.Initializer = elems
	return &cp, consumed, endIdx, true
}

func canCommitWithRemainingThisFieldStores(remaining []statements.Statement, remRewriteByAssign map[*statements.AssignStatement]values.JavaValue, env map[*coreutils.VariableId]values.JavaValue, newArgs []values.JavaValue, delegate *values.FunctionCallExpression, superClass string, ctx *class_context.ClassContext) bool {
	if len(remaining) == 0 || delegate == nil || ctx == nil {
		return false
	}
	allRHSIDs := map[*coreutils.VariableId]bool{}
	for _, st := range remaining {
		if st == nil {
			continue
		}
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		}
		as := ctorAssignStmt(st)
		if as == nil || as.ArrayMember != nil || as.IsDeclare || as.JavaValue == nil {
			return false
		}
		rm, isField := values.UnpackSoltValue(as.LeftValue).(*values.RefMember)
		if !isField || rm == nil {
			return false
		}
		recv, isRef := values.UnpackSoltValue(rm.Object).(*values.JavaRef)
		if !isRef || recv == nil || !recv.IsThis {
			return false
		}
		rhs := as.JavaValue
		if rewritten, ok := remRewriteByAssign[as]; ok && rewritten != nil {
			rhs = rewritten
		}
		if eff, _ := values.InspectValue(rhs); eff&values.BarrierEffects != 0 {
			return false
		}
		rhsIDs := map[*coreutils.VariableId]bool{}
		collectValueVarReads(rhs, ctx, rhsIDs, nil, nil, 0)
		for id := range rhsIDs {
			if _, inEnv := env[id]; inEnv {
				return false
			}
			allRHSIDs[id] = true
		}
	}
	argWritten := map[*coreutils.VariableId]bool{}
	for _, arg := range newArgs {
		acc := values.InspectAccess(arg)
		if len(allRHSIDs) > 0 && acc.Effects&values.EffectOpaque != 0 {
			return false
		}
		for wRef := range acc.Writes {
			if wRef != nil && wRef.Id != nil {
				argWritten[wRef.Id] = true
			}
		}
	}
	for id := range allRHSIDs {
		if argWritten[id] {
			return false
		}
	}
	if len(newArgs) == 0 && strings.ReplaceAll(delegate.ClassName, ".", "/") == superClass {
		return true
	}
	remWritten := map[*coreutils.VariableId]bool{}
	collectStmtVars(remaining, ctx, nil, remWritten, nil, nil, 0)
	if len(remWritten) > 0 {
		argIDs := map[*coreutils.VariableId]bool{}
		for _, arg := range newArgs {
			collectValueVarReads(arg, ctx, argIDs, nil, nil, 0)
		}
		for id := range remWritten {
			if argIDs[id] {
				return false
			}
		}
	}
	return true
}

func (c *ClassObjectDumper) foldRetainedConstructorPrefix(p *constructorSourceBoundary, postSuper []statements.Statement) {
	if c == nil || p == nil || len(p.prefix) == 0 || p.delegate == nil {
		return
	}
	postIDs := map[*coreutils.VariableId]bool{}
	collectStmtVars(postSuper, c.FuncCtx, postIDs, nil, nil, nil, 0)

	prefixWritten := map[*coreutils.VariableId]bool{}
	collectStmtVars(p.prefix, c.FuncCtx, nil, prefixWritten, nil, nil, 0)

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

	useRank, useCount, condUsed, firstOrigBarrier := ctorDelegateEvalOrder(delegateArgs)
	lastInlinedBarrierRank := -1
	hasRemainingBarrier := false
	inlinedAnyBarrier := false

	type remainingAssignRewrite struct {
		assign *statements.AssignStatement
		newRHS values.JavaValue
	}
	var remRewrites []remainingAssignRewrite
	remRewriteByAssign := map[*statements.AssignStatement]values.JavaValue{}
	type remainingExprRewrite struct {
		exprStmt *statements.ExpressionStatement
		newExpr  values.JavaValue
	}
	var remExprRewrites []remainingExprRewrite
	remExprByStmt := map[*statements.ExpressionStatement]values.JavaValue{}
	type deferredEntry struct {
		prefixIdx int
		st        statements.Statement
		lhsID     *coreutils.VariableId
		assign    *statements.AssignStatement
		newRHS    values.JavaValue
	}
	var deferred []deferredEntry
	type remainingPos struct {
		prefixIdx int
		st        statements.Statement
	}
	var remWithPos []remainingPos
	appendRemaining := func(idx int, st statements.Statement) {
		remaining = append(remaining, st)
		remWithPos = append(remWithPos, remainingPos{prefixIdx: idx, st: st})
		hasRemainingBarrier = true
	}

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
		if hasRemainingBarrier {
			return false
		}
		remUses := countStmtVarRefs(p.prefix[idx+1:], targetID, "", c.FuncCtx, 0)
		r, hasRank := useRank[targetID]
		if remUses != 0 || useCount[targetID] != 1 || condUsed[targetID] || !hasRank {
			return false
		}
		if r <= lastInlinedBarrierRank {
			return false
		}
		if firstOrigBarrier >= 0 && r >= firstOrigBarrier {
			return false
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
		if effects, _ := values.InspectValue(subRHS); effects&values.BarrierEffects != 0 {
			if hasRemainingBarrier {
				return false
			}
			if r, hasRank := useRank[lhs.Id]; !hasRank || condUsed[lhs.Id] || r <= lastInlinedBarrierRank || (firstOrigBarrier >= 0 && r >= firstOrigBarrier) {
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
		prevRank, prevHasRank := useRank[lhs.Id]
		delegateArgs[firstArg] = replacedArg
		delete(env, lhs.Id)
		folded[p.prefix[idx]] = true
		mutatedDelegate = true
		useRank, useCount, condUsed, firstOrigBarrier = ctorDelegateEvalOrder(delegateArgs)
		if prevHasRank && prevRank > lastInlinedBarrierRank {
			lastInlinedBarrierRank = prevRank
		}
		if effects, _ := values.InspectValue(subRHS); effects&values.BarrierEffects != 0 {
			inlinedAnyBarrier = true
		}
		return true
	}

	for idx := 0; idx < len(p.prefix); idx++ {
		st := p.prefix[idx]
		switch s := st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		case *statements.AssignStatement, *statements.CustomStatement:
			assignStmt := ctorAssignStmt(st)
			lhs, rhs, ok := ctorRefLHS(st)
			if !ok {
				var pendingAssign *statements.AssignStatement
				var pendingRHS values.JavaValue
				if assignStmt != nil && assignStmt.ArrayMember == nil && assignStmt.JavaValue != nil && len(env) > 0 {
					if rm, isField := values.UnpackSoltValue(assignStmt.LeftValue).(*values.RefMember); isField && rm != nil {
						if recv, isRef := values.UnpackSoltValue(rm.Object).(*values.JavaRef); isRef && recv != nil && recv.IsThis {
							if subVal, subOK := substCtorValue(assignStmt.JavaValue, env, c.FuncCtx, 0); subOK && subVal != assignStmt.JavaValue {
								if eff, _ := values.InspectValue(subVal); eff&values.BarrierEffects == 0 {
									pendingAssign = assignStmt
									pendingRHS = subVal
								}
							}
						}
					}
				}
				written := map[*coreutils.VariableId]bool{}
				collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
				for id := range written {
					delete(env, id)
				}
				appendRemaining(idx, st)
				if pendingAssign != nil && pendingRHS != nil {
					remRewrites = append(remRewrites, remainingAssignRewrite{assign: pendingAssign, newRHS: pendingRHS})
					remRewriteByAssign[pendingAssign] = pendingRHS
				}
				continue
			}
			if rhs == nil {
				// Pure declaration `T varN;` without initializer for a variable unused after prefix.
				if assignStmt != nil && assignStmt.IsDeclare && !postIDs[lhs.Id] {
					folded[st] = true
					continue
				}
				delete(env, lhs.Id)
				appendRemaining(idx, st)
				continue
			}
			subRHS, ok := substCtorValue(rhs, env, c.FuncCtx, 0)
			if !ok {
				delete(env, lhs.Id)
				appendRemaining(idx, st)
				continue
			}
			// Dead `this`-alias receiver local (`T alias = this;`) used solely as the
			// constructor delegation receiver (`this(...)`/`super(...)` renders without it).
			if c.getenv("JDEC_THIS_ALIAS_SUPER_OFF") == "" && lhs.IsThisAlias && !postIDs[lhs.Id] && useCount[lhs.Id] == 0 {
				if rhsRef, isRef := values.UnpackSoltValue(subRHS).(*values.JavaRef); isRef && rhsRef != nil && (rhsRef.IsThis || rhsRef.IsThisAlias) {
					tailIDs := map[*coreutils.VariableId]bool{}
					collectStmtVars(p.prefix[idx+1:], c.FuncCtx, tailIDs, nil, nil, nil, 0)
					if !tailIDs[lhs.Id] {
						folded[st] = true
						continue
					}
				}
			}
			if !hasRemainingBarrier && !postIDs[lhs.Id] && paramByID[lhs.Id] == nil && useCount[lhs.Id] == 1 && !condUsed[lhs.Id] {
				if r, hasRank := useRank[lhs.Id]; hasRank && r > lastInlinedBarrierRank && (firstOrigBarrier < 0 || r < firstOrigBarrier) {
					if ne, isNew := values.UnpackSoltValue(subRHS).(*values.NewExpression); isNew && ne != nil && ne.IsArray() {
						if initArr, consumed, endIdx, foldedOK := tryFoldCtorArrayInit(p.prefix, idx, lhs.Id, ne, env, postIDs, paramByID, useCount, c.FuncCtx); foldedOK {
							env[lhs.Id] = initArr
							for _, cIdx := range consumed {
								folded[p.prefix[cIdx]] = true
							}
							lastInlinedBarrierRank = r
							inlinedAnyBarrier = true
							idx = endIdx
							continue
						}
					}
				}
			}
			if canInlineExpr(idx, lhs.Id, subRHS) {
				if eff, _ := values.InspectValue(subRHS); eff&values.BarrierEffects != 0 {
					lastInlinedBarrierRank = useRank[lhs.Id]
					inlinedAnyBarrier = true
				}
				env[lhs.Id] = subRHS
				folded[st] = true
				continue
			}
			if postIDs[lhs.Id] && tryFoldEscapingParamIntoDelegate(idx, lhs, subRHS) {
				continue
			}
			if postIDs[lhs.Id] && paramByID[lhs.Id] == nil && assignStmt != nil {
				remWritten := map[*coreutils.VariableId]bool{}
				collectStmtVars(p.prefix[idx+1:], c.FuncCtx, nil, remWritten, nil, nil, 0)
				if !remWritten[lhs.Id] && isPureDeferrableCtorRHS(subRHS, paramByID, prefixWritten, delegateArgs) {
					env[lhs.Id] = subRHS
					var rewriteRHS values.JavaValue
					if subRHS != rhs {
						rewriteRHS = subRHS
					}
					deferred = append(deferred, deferredEntry{
						prefixIdx: idx,
						st:        st,
						lhsID:     lhs.Id,
						assign:    assignStmt,
						newRHS:    rewriteRHS,
					})
					continue
				}
			}
			delete(env, lhs.Id)
			appendRemaining(idx, st)
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
				appendRemaining(idx, st)
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
								if eff, _ := values.InspectValue(ternary); eff&values.BarrierEffects != 0 {
									lastInlinedBarrierRank = useRank[lhs.Id]
								}
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
							if eff, _ := values.InspectValue(ternary); eff&values.BarrierEffects != 0 {
								lastInlinedBarrierRank = useRank[lhs1.Id]
								inlinedAnyBarrier = true
							}
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
			appendRemaining(idx, st)
			continue

		case *statements.ExpressionStatement:
			var pendingExpr values.JavaValue
			if s != nil && s.Expression != nil && len(env) > 0 {
				if sub, ok := substCtorValue(s.Expression, env, c.FuncCtx, 0); ok {
					if sub != s.Expression {
						pendingExpr = sub
					}
				} else {
					written := map[*coreutils.VariableId]bool{}
					collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
					for id := range written {
						delete(env, id)
					}
					appendRemaining(idx, st)
					continue
				}
			}
			written := map[*coreutils.VariableId]bool{}
			collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
			for id := range written {
				delete(env, id)
			}
			appendRemaining(idx, st)
			if s != nil && pendingExpr != nil {
				remExprRewrites = append(remExprRewrites, remainingExprRewrite{exprStmt: s, newExpr: pendingExpr})
				remExprByStmt[s] = pendingExpr
			}
			continue
		default:
			written := map[*coreutils.VariableId]bool{}
			collectStmtVars([]statements.Statement{st}, c.FuncCtx, nil, written, nil, nil, 0)
			for id := range written {
				delete(env, id)
			}
			appendRemaining(idx, st)
			continue
		}
	}

	// Never leave half-deferred state if un-folded statements remain and touch a
	// deferred variable or could run in a carrier: keep everything atomic.
	if len(remaining) > 0 && len(deferred) > 0 {
		remAll := map[*coreutils.VariableId]bool{}
		collectStmtVars(remaining, c.FuncCtx, remAll, nil, nil, nil, 0)
		conflict := false
		for _, d := range deferred {
			if remAll[d.lhsID] {
				conflict = true
				break
			}
		}
		if conflict {
			combined := make([]remainingPos, 0, len(remWithPos)+len(deferred))
			combined = append(combined, remWithPos...)
			for _, d := range deferred {
				combined = append(combined, remainingPos{prefixIdx: d.prefixIdx, st: d.st})
				delete(env, d.lhsID)
			}
			for i := 1; i < len(combined); i++ {
				for j := i; j > 0 && combined[j].prefixIdx < combined[j-1].prefixIdx; j-- {
					combined[j], combined[j-1] = combined[j-1], combined[j]
				}
			}
			remaining = remaining[:0]
			for _, item := range combined {
				remaining = append(remaining, item.st)
			}
			deferred = nil
		}
	}

	if len(folded) == 0 && !mutatedDelegate && len(deferred) == 0 && len(remRewrites) == 0 && len(remExprRewrites) == 0 {
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
	if len(folded) > 0 {
		foldedList := make([]statements.Statement, 0, len(folded))
		for st := range folded {
			foldedList = append(foldedList, st)
		}
		foldedWritten := map[*coreutils.VariableId]bool{}
		collectStmtVars(foldedList, c.FuncCtx, nil, foldedWritten, nil, nil, 0)
		// Abort folding if any remaining/deferred statement or rewritten delegate
		// argument still reads a non-parameter variable written in folded statements.
		postFoldReads := map[*coreutils.VariableId]bool{}
		for _, arg := range newArgs {
			collectValueVarReads(arg, c.FuncCtx, postFoldReads, nil, nil, 0)
		}
		if len(remaining) > 0 {
			for _, st := range remaining {
				switch s := st.(type) {
				case *statements.AssignStatement:
					if newRHS, ok := remRewriteByAssign[s]; ok && newRHS != nil {
						cp := *s
						cp.JavaValue = newRHS
						collectStmtVarReads([]statements.Statement{&cp}, c.FuncCtx, postFoldReads, 0)
					} else {
						collectStmtVarReads([]statements.Statement{st}, c.FuncCtx, postFoldReads, 0)
					}
				case *statements.ExpressionStatement:
					if newExpr, ok := remExprByStmt[s]; ok && newExpr != nil {
						collectValueVarReads(newExpr, c.FuncCtx, postFoldReads, nil, nil, 0)
					} else {
						collectStmtVarReads([]statements.Statement{st}, c.FuncCtx, postFoldReads, 0)
					}
				default:
					collectStmtVarReads([]statements.Statement{st}, c.FuncCtx, postFoldReads, 0)
				}
			}
		}
		for _, d := range deferred {
			if d.newRHS != nil && d.assign != nil {
				cp := *d.assign
				cp.JavaValue = d.newRHS
				collectStmtVarReads([]statements.Statement{&cp}, c.FuncCtx, postFoldReads, 0)
			} else if d.st != nil {
				collectStmtVarReads([]statements.Statement{d.st}, c.FuncCtx, postFoldReads, 0)
			}
		}
		for id := range foldedWritten {
			if paramByID[id] == nil && postFoldReads[id] {
				return
			}
		}
	}
	if hasRemainingBarrier && inlinedAnyBarrier && !canCommitWithRemainingThisFieldStores(remaining, remRewriteByAssign, env, newArgs, p.delegate, c.obj.GetSupperClassName(), c.FuncCtx) {
		return
	}
	for _, rw := range remRewrites {
		if rw.assign != nil && rw.newRHS != nil {
			rw.assign.JavaValue = rw.newRHS
		}
	}
	for _, rw := range remExprRewrites {
		if rw.exprStmt != nil && rw.newExpr != nil {
			rw.exprStmt.Expression = rw.newExpr
		}
	}
	for _, d := range deferred {
		if d.newRHS != nil && d.assign != nil {
			d.assign.JavaValue = d.newRHS
		}
	}
	p.delegate.Arguments = newArgs
	p.prefix = remaining
	p.foldedStatements = folded
}

func ctorDefaultInitializer(t types.JavaType) string {
	if t == nil {
		return ""
	}
	if t.IsArray() {
		return "null"
	}
	if primer, ok := t.RawType().(*types.JavaPrimer); ok && primer != nil {
		switch primer.Name {
		case types.JavaBoolean:
			return "false"
		case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger:
			return "0"
		case types.JavaLong:
			return "0L"
		case types.JavaFloat:
			return "0.0f"
		case types.JavaDouble:
			return "0.0d"
		case types.JavaVoid:
			return ""
		}
	}
	return "null"
}

func cloneDeclaredMap(declared map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(declared))
	for k, v := range declared {
		if v {
			cloned[k] = true
		}
	}
	return cloned
}

// collectRetainedPrefixHoistedDecls hoists declarations for non-parameter locals
// written inside nested branches/loops whose live range reaches sibling/outer
// scopes (or delegate target argument) before any prior top-level declaration.
func collectRetainedPrefixHoistedDecls(prefix []statements.Statement, targetArg values.JavaValue, ctx *class_context.ClassContext, declared map[string]bool) (string, bool) {
	if ctx == nil || len(prefix) == 0 {
		return "", true
	}
	type localWriteInfo struct {
		firstTopWriteIdx int
		firstNestedIdx   int
		scopeCount       int
		escapedTopOrArg  bool
		typ              types.JavaType
		typStr           string
		conflict         bool
	}
	infos := map[string]*localWriteInfo{}
	ensureInfo := func(name string) *localWriteInfo {
		if info, ok := infos[name]; ok {
			return info
		}
		info := &localWriteInfo{firstTopWriteIdx: -1, firstNestedIdx: -1}
		infos[name] = info
		return info
	}
	recordWriteType := func(info *localWriteInfo, typ types.JavaType) {
		if typ == nil {
			info.conflict = true
			return
		}
		ts := typ.String(ctx)
		if ts == "" || ctorDefaultInitializer(typ) == "" {
			info.conflict = true
			return
		}
		if info.typStr == "" {
			info.typ = typ
			info.typStr = ts
			return
		}
		if info.typStr != ts {
			info.conflict = true
		}
	}
	seenNamesInTargetArg := map[string]bool{}
	collectValueVarReads(targetArg, ctx, nil, seenNamesInTargetArg, nil, 0)
	for name := range seenNamesInTargetArg {
		ensureInfo(name).escapedTopOrArg = true
	}

	var walkNested func(sts []statements.Statement, topIdx int, scopeHits map[string]bool)
	walkNested = func(sts []statements.Statement, topIdx int, scopeHits map[string]bool) {
		for _, st := range sts {
			if st == nil {
				continue
			}
			var assign *statements.AssignStatement
			switch s := st.(type) {
			case *statements.AssignStatement:
				assign = s
			case *statements.CustomStatement:
				if a, ok := s.SourceAssignmentForward(); ok {
					assign = a
				}
			}
			if assign != nil && assign.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil {
					if name := ref.String(ctx); name != "" {
						info := ensureInfo(name)
						if info.firstNestedIdx < 0 || topIdx < info.firstNestedIdx {
							info.firstNestedIdx = topIdx
						}
						scopeHits[name] = true
						recordWriteType(info, ref.Type())
					}
				}
			}
			readNames := map[string]bool{}
			collectStmtVars([]statements.Statement{st}, ctx, nil, nil, readNames, nil, 0)
			for name := range readNames {
				scopeHits[name] = true
			}
			switch s := st.(type) {
			case *statements.IfStatement:
				walkNested(s.IfBody, topIdx, scopeHits)
				if len(s.ElseBody) > 0 {
					walkNested(s.ElseBody, topIdx, scopeHits)
				}
			case *statements.DoWhileStatement:
				bodyHits := map[string]bool{}
				walkNested(s.Body, topIdx, bodyHits)
				condNames := map[string]bool{}
				collectValueVarReads(s.ConditionValue, ctx, nil, condNames, nil, 0)
				for n := range bodyHits {
					if condNames[n] {
						ensureInfo(n).escapedTopOrArg = true
					}
					scopeHits[n] = true
				}
			}
		}
	}

	for idx, st := range prefix {
		if st == nil {
			continue
		}
		var assign *statements.AssignStatement
		switch s := st.(type) {
		case *statements.AssignStatement:
			assign = s
		case *statements.CustomStatement:
			if a, ok := s.SourceAssignmentForward(); ok {
				assign = a
			}
		}
		if assign != nil {
			if assign.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil {
					if name := ref.String(ctx); name != "" {
						info := ensureInfo(name)
						if info.firstTopWriteIdx < 0 {
							info.firstTopWriteIdx = idx
						}
						if info.firstNestedIdx >= 0 && idx > info.firstNestedIdx {
							info.escapedTopOrArg = true
						}
						info.scopeCount++
						recordWriteType(info, ref.Type())
					}
				}
			}
			rhsNames := map[string]bool{}
			collectStmtVars([]statements.Statement{assign}, ctx, nil, nil, rhsNames, nil, 0)
			for name := range rhsNames {
				if info, ok := infos[name]; ok && info.firstNestedIdx >= 0 && idx > info.firstNestedIdx {
					info.escapedTopOrArg = true
				}
			}
			continue
		}
		switch s := st.(type) {
		case *statements.ExpressionStatement, *statements.ReturnStatement:
			names := map[string]bool{}
			collectStmtVars([]statements.Statement{st}, ctx, nil, nil, names, nil, 0)
			for name := range names {
				if info, ok := infos[name]; ok && info.firstNestedIdx >= 0 && idx > info.firstNestedIdx {
					info.escapedTopOrArg = true
				}
			}
		case *statements.IfStatement:
			condNames := map[string]bool{}
			collectValueVarReads(s.Condition, ctx, nil, condNames, nil, 0)
			for name := range condNames {
				if info, ok := infos[name]; ok && info.firstNestedIdx >= 0 && idx > info.firstNestedIdx {
					info.escapedTopOrArg = true
				}
			}
			stmtHits := map[string]bool{}
			walkNested(s.IfBody, idx, stmtHits)
			if len(s.ElseBody) > 0 {
				walkNested(s.ElseBody, idx, stmtHits)
			}
			for n := range stmtHits {
				ensureInfo(n).scopeCount++
			}
		case *statements.DoWhileStatement:
			bodyHits := map[string]bool{}
			walkNested(s.Body, idx, bodyHits)
			condNames := map[string]bool{}
			collectValueVarReads(s.ConditionValue, ctx, nil, condNames, nil, 0)
			for n := range bodyHits {
				info := ensureInfo(n)
				info.scopeCount++
				if condNames[n] {
					info.escapedTopOrArg = true
				}
			}
		}
	}

	var hoistNames []string
	for name, info := range infos {
		if declared[name] || info.firstNestedIdx < 0 {
			continue
		}
		if info.firstTopWriteIdx >= 0 && info.firstTopWriteIdx < info.firstNestedIdx {
			continue
		}
		if !info.escapedTopOrArg && info.scopeCount < 2 {
			continue
		}
		if info.conflict || info.typ == nil || info.typStr == "" {
			return "", false
		}
		hoistNames = append(hoistNames, name)
	}
	sort.Strings(hoistNames)
	var b strings.Builder
	for _, name := range hoistNames {
		info := infos[name]
		init := ctorDefaultInitializer(info.typ)
		if init == "" {
			return "", false
		}
		declared[name] = true
		b.WriteString(info.typStr)
		b.WriteByte(' ')
		b.WriteString(name)
		b.WriteString(" = ")
		b.WriteString(init)
		b.WriteString(";\n")
	}
	return b.String(), true
}

func renderRetainedPrefixStatements(sts []statements.Statement, ctx *class_context.ClassContext, declared map[string]bool) (string, bool) {
	var b strings.Builder
	writeAssign := func(s *statements.AssignStatement) {
		prefix := ""
		renderStmt := s
		if s.ArrayMember == nil {
			if ref, ok := values.UnpackSoltValue(s.LeftValue).(*values.JavaRef); ok && ref != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil && ctx != nil {
				name := ref.String(ctx)
				if name != "" && declared[name] {
					if s.IsDeclare && s.JavaValue == nil {
						return
					}
					if s.IsFirst || s.IsDeclare {
						cp := *s
						cp.IsFirst = false
						cp.IsDeclare = false
						renderStmt = &cp
					}
				} else if name != "" && (s.IsFirst || s.IsDeclare) {
					declared[name] = true
				} else if name != "" && ref.Type() != nil {
					declared[name] = true
					prefix = ref.Type().String(ctx) + " "
				}
			}
		}
		b.WriteString(prefix)
		b.WriteString(renderStmt.String(ctx))
		b.WriteString(";\n")
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		switch s := st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		case *statements.AssignStatement:
			writeAssign(s)
		case *statements.CustomStatement:
			if assign, ok := s.SourceAssignmentForward(); ok && assign != nil {
				writeAssign(assign)
			} else {
				return "", false
			}
		case *statements.ExpressionStatement, *statements.ReturnStatement:
			b.WriteString(st.String(ctx))
			b.WriteString(";\n")
		case *statements.IfStatement:
			cond := values.SimplifyConditionValue(s.Condition).String(ctx)
			thenStr, ok1 := renderRetainedPrefixStatements(s.IfBody, ctx, cloneDeclaredMap(declared))
			if !ok1 {
				return "", false
			}
			b.WriteString("if (" + cond + ") {\n" + thenStr + "}")
			if len(s.ElseBody) > 0 {
				elseStr, ok2 := renderRetainedPrefixStatements(s.ElseBody, ctx, cloneDeclaredMap(declared))
				if !ok2 {
					return "", false
				}
				b.WriteString(" else {\n" + elseStr + "}")
			}
			b.WriteString("\n")
		case *statements.DoWhileStatement:
			bodyStr, okB := renderRetainedPrefixStatements(s.Body, ctx, cloneDeclaredMap(declared))
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
	for prev := 0; prev < targetArgIdx; prev++ {
		if eff, _ := values.InspectValue(p.delegate.Arguments[prev]); eff&values.BarrierEffects != 0 {
			return false
		}
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
	paramIDs := map[*coreutils.VariableId]bool{}
	declarations := make([]string, 0, len(p.params))
	for i, param := range p.params {
		ref, ok := param.(*values.JavaRef)
		if !ok || ref == nil || ref.IsThis || ref.Id == nil {
			return false
		}
		name := ref.String(c.FuncCtx)
		declared[name] = true
		paramIDs[ref.Id] = true
		declarations = append(declarations, mt.FunctionType().ParamTypes[i].String(c.FuncCtx)+" "+name)
	}
	helperReads := map[*coreutils.VariableId]bool{}
	collectStmtVarReads(p.prefix, c.FuncCtx, helperReads, 0)
	collectValueVarReads(p.delegate.Arguments[targetArgIdx], c.FuncCtx, helperReads, nil, nil, 0)
	for id := range helperReads {
		if !paramIDs[id] && !prefixWrittenIDs[id] {
			return false
		}
	}
	hoistedDecls, ok := collectRetainedPrefixHoistedDecls(p.prefix, p.delegate.Arguments[targetArgIdx], c.FuncCtx, declared)
	if !ok {
		return false
	}
	prefixCode, ok := renderRetainedPrefixStatements(p.prefix, c.FuncCtx, declared)
	if !ok {
		return false
	}
	prefixCode = hoistedDecls + prefixCode
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
