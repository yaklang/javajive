package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A blank local can be initialized on multiple mutually exclusive edges.
// Count writes along paths, rather than counting assignment syntax: every
// normal predecessor of the capture must be initialized exactly once. This
// proves source declaration stability only; original allocation, descriptor,
// capture field and lexical ownership are still checked independently.
func nativeCaptureJoinedDeclaration(body []statements.Statement, ref *values.JavaRef, allocation *values.NewExpression, work *workbudget.Budget) (*statements.AssignStatement, bool) {
	if ref == nil || ref.Id == nil || ref.IsThis || allocation == nil || allocation.ConstructorCall == nil || !allocation.HasOriginPC || !allocation.ConstructorCall.HasOriginPC || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(4096*128) != nil {
		return nil, false
	}
	captured := false
	for _, arg := range allocation.ConstructorCall.Arguments {
		v, known := nativeMemberEnclosingUnpack(arg, work)
		if !known || sourceProofNil(v) {
			return nil, false
		}
		r, ok := v.(*values.JavaRef)
		if ok && r.Id == nil {
			return nil, false
		}
		captured = captured || ok && r.Id == ref.Id
	}
	if !captured {
		return nil, false
	}
	return nativeCaptureJoinedSource(body, ref, work, func(v values.JavaValue) bool { return v == allocation }, nil, nil)
}

// The same definite-assignment transfer proves a blank local at either an
// original constructor capture or an original dynamic operand declaration.
// A lambda additionally checks each physical STORE's retained source witness.
func nativeCaptureJoinedSource(body []statements.Statement, ref *values.JavaRef, work *workbudget.Budget, captureValue func(values.JavaValue) bool, captureStatement func(statements.Statement) bool, store func(*statements.AssignStatement) bool) (*statements.AssignStatement, bool) {
	if ref == nil || ref.Id == nil || ref.IsThis || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(4096*128) != nil {
		return nil, false
	}
	const absent, blank, initialized uint8 = 1, 2, 4
	var declaration *statements.AssignStatement
	writes, captures, remaining := 0, 0, 4096
	loopDepth := 0
	activeValues := map[values.JavaValue]bool{}
	activeStatements := map[statements.Statement]bool{}
	enter := func() bool {
		remaining--
		return remaining >= 0 && nativeProofWork(work, 1) && (work == nil || work.Enter(workbudget.CounterASTDepth) == nil)
	}
	leave := func() {
		if work != nil {
			work.Leave(workbudget.CounterASTDepth)
		}
	}
	same := func(v values.JavaValue) (bool, bool) {
		v, known := nativeMemberEnclosingUnpack(v, work)
		if !known || sourceProofNil(v) {
			return false, false
		}
		r, ok := v.(*values.JavaRef)
		return ok && r.Id == ref.Id, !ok || r.Id != nil
	}
	var value func(values.JavaValue, uint8) bool
	value = func(v values.JavaValue, state uint8) bool {
		if sourceProofNil(v) || activeValues[v] || !enter() {
			return false
		}
		defer leave()
		activeValues[v] = true
		defer delete(activeValues, v)
		if captureValue != nil && captureValue(v) {
			captures++
			if captures != 1 || state != initialized {
				return false
			}
		}
		if r, ok := v.(*values.JavaRef); ok && (r.Id == nil || r.Id == ref.Id) {
			return r.Id != nil && state == initialized && r.CustomValue == nil && r.StackVar == nil
		}
		switch x := v.(type) {
		case *values.AssignmentExpression:
			if matched, known := same(x.Target); !known || matched {
				return false
			}
		case *values.JavaExpression:
			if x.Op == "=" || strings.HasSuffix(string(x.Op), "=") && x.Op != "==" && x.Op != "!=" && x.Op != "<=" && x.Op != ">=" || x.Op == values.INC || x.Op == values.DEC {
				if len(x.Values) == 0 {
					return false
				}
				if matched, known := same(x.Values[0]); !known || matched {
					return false
				}
			}
		case *values.FunctionCallExpression:
			if !x.IsStatic && !value(x.Object, state) {
				return false
			}
			for _, arg := range x.Arguments {
				if !value(arg, state) {
					return false
				}
			}
			return true
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, state) {
				return false
			}
		}
		return true
	}
	var walk func([]statements.Statement, uint8) (uint8, bool)
	// A preinitialized capture remains initialized through an unrelated loop.
	// A declaration scoped inside the loop is a fresh local each iteration and
	// does not escape its body. A blank declaration outside the loop cannot be
	// repeatedly assigned and claim effective finality. This extension belongs
	// only to the separately witnessed lambda snapshot, not allocation motion.
	loopBody := func(list []statements.Statement, state uint8) bool {
		if captureStatement == nil || state != absent && state != initialized {
			return false
		}
		beforeDeclaration, beforeWrites := declaration, writes
		loopDepth++
		after, known := walk(list, state)
		loopDepth--
		if !known {
			return false
		}
		if state == initialized {
			return (after == initialized || after == 0) && writes == beforeWrites && declaration == beforeDeclaration
		}
		return beforeDeclaration == nil && (after == absent || after == initialized || after == 0)
	}
	walk = func(list []statements.Statement, state uint8) (uint8, bool) {
		if !enter() {
			return 0, false
		}
		defer leave()
		for _, st := range list {
			if sourceProofNil(st) || activeStatements[st] || !nativeProofWork(work, 1) {
				return 0, false
			}
			remaining--
			if remaining < 0 {
				return 0, false
			}
			activeStatements[st] = true
			if assign, ok := st.(*statements.AssignStatement); ok && assign.ArrayMember == nil {
				matched, known := same(assign.LeftValue)
				if !known {
					return 0, false
				}
				if matched {
					if assign.IsDeclare && assign.JavaValue == nil && !assign.IsFirst {
						if declaration != nil || state != absent {
							return 0, false
						}
						declaration, state = assign, blank
					} else {
						if assign.IsDeclare || assign.IsFirst || declaration == nil || state != blank || sourceProofNil(assign.JavaValue) || !value(assign.JavaValue, state) {
							return 0, false
						}
						if store != nil && !store(assign) {
							return 0, false
						}
						writes++
						state = initialized
					}
					delete(activeStatements, st)
					continue
				}
			}
			roots, children, known := nativeSourceNameChildren(st)
			if custom, ok := st.(*statements.CustomStatement); ok {
				// A compatibility ThrownValue annotation does not seal an
				// opaque callback's effects or abrupt completion behavior.
				operand, sealed := custom.SourceThrowOperand()
				roots, children, known = []values.JavaValue{operand}, nil, sealed
				if captureStatement != nil && loopDepth > 0 && custom.SourceTransferOnly() {
					roots, children, known = nil, nil, true
				}
			}
			if !known {
				return 0, false
			}
			for _, root := range roots {
				if !value(root, state) {
					return 0, false
				}
			}
			if captureStatement != nil && captureStatement(st) {
				captures++
				if captures != 1 || state != initialized {
					return 0, false
				}
			}
			switch x := st.(type) {
			case *statements.IfStatement:
				left, lk := walk(x.IfBody, state)
				right, rk := walk(x.ElseBody, state)
				if !lk || !rk {
					return 0, false
				}
				state = left | right
			case *statements.ReturnStatement:
				state = 0 // Abrupt predecessors do not reach the next join.
			case *statements.CustomStatement:
				if _, sealed := x.SourceThrowOperand(); !sealed {
					if captureStatement == nil || loopDepth == 0 || !x.SourceTransferOnly() {
						return 0, false // Unproved transfers cannot close a join.
					}
				}
				state = 0
			case *statements.SynchronizedStatement:
				var valid bool
				state, valid = walk(x.Body, state)
				if !valid {
					return 0, false
				}
			case *statements.WhileStatement:
				if !loopBody(x.Body, state) {
					return 0, false
				}
			case *statements.DoWhileStatement:
				if !loopBody(x.Body, state) {
					return 0, false
				}
			case *statements.ForStatement:
				for _, header := range []statements.Statement{x.InitVar, x.Condition} {
					if !sourceProofNil(header) {
						var valid bool
						state, valid = walk([]statements.Statement{header}, state)
						if !valid {
							return 0, false
						}
					}
				}
				if !loopBody(x.SubStatements, state) {
					return 0, false
				}
				if !sourceProofNil(x.EndExp) {
					after, valid := walk([]statements.Statement{x.EndExp}, state)
					if !valid || after != state {
						return 0, false
					}
				}
			default:
				// Switch and exception edges need their own flow transfer
				// model. They cannot certify this new multi-write declaration.
				if len(children) != 0 {
					return 0, false
				}
			}
			delete(activeStatements, st)
		}
		return state, true
	}
	_, known := walk(body, absent)
	return declaration, known && declaration != nil && writes >= 1 && captures == 1
}
