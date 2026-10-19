package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A blank local can be initialized on multiple mutually exclusive edges.
// Count writes along paths, rather than counting assignment syntax: every
// normal predecessor of the capture must be initialized exactly once. This
// proves source declaration stability only; original allocation, descriptor,
// capture field and lexical ownership are still checked independently.
func nativeCaptureJoinedDeclaration(body []statements.Statement, ref *values.JavaRef, allocation *values.NewExpression, work *workbudget.Budget) (*statements.AssignStatement, bool) {
	return nativeCaptureLocalDeclaration(body, ref, allocation, work, false)
}

// A snapshot needs a definitely assigned source word, not an effectively final
// source storage location. The caller copies that word into a fresh binder at
// the original allocation contour. Actual capture binding still uses the
// stricter effective-final proof on that fresh binder.
func nativeCaptureSnapshotDeclaration(body []statements.Statement, ref *values.JavaRef, allocation *values.NewExpression, work *workbudget.Budget) (*statements.AssignStatement, bool) {
	return nativeCaptureLocalDeclaration(body, ref, allocation, work, true)
}

func nativeCaptureLocalDeclaration(body []statements.Statement, ref *values.JavaRef, allocation *values.NewExpression, work *workbudget.Budget, snapshot bool) (*statements.AssignStatement, bool) {
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
	return nativeCaptureSourceFlow(body, ref, work, func(v values.JavaValue) bool { return v == allocation }, nil, nil, snapshot)
}

// The same definite-assignment transfer proves a blank local at either an
// original constructor capture or an original dynamic operand declaration.
// A lambda additionally checks each physical STORE's retained source witness.
func nativeCaptureJoinedSource(body []statements.Statement, ref *values.JavaRef, work *workbudget.Budget, captureValue func(values.JavaValue) bool, captureStatement func(statements.Statement) bool, store func(*statements.AssignStatement) bool) (*statements.AssignStatement, bool) {
	return nativeCaptureSourceFlow(body, ref, work, captureValue, captureStatement, store, false)
}

func nativeCaptureSourceFlow(body []statements.Statement, ref *values.JavaRef, work *workbudget.Budget, captureValue func(values.JavaValue) bool, captureStatement func(statements.Statement) bool, store func(*statements.AssignStatement) bool, snapshot bool) (*statements.AssignStatement, bool) {
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
	// A declaration scoped inside the loop is a fresh local each iteration.
	// Its initializer alternatives may join before the original allocation:
	// effective finality counts writes per iteration, not across iterations.
	// Reset that binder to absent at the loop boundary, so no use outside its
	// body can borrow its initialization. A blank outer declaration cannot be
	// repeatedly assigned and claim effective finality. A preinitialized outer
	// binder is stable for either checkpoint only when the complete loop body
	// leaves that same declaration initialized and performs no further writes.
	// This is a source binding proof; no allocation or producer is moved.
	loopBody := func(list []statements.Statement, state uint8) bool {
		if state != absent && state != initialized || captureStatement == nil && captureValue == nil {
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
			// An already initialized local remains assigned on both zero-trip
			// and iterative paths. Reassignments prevent direct capture, but
			// not a later pure snapshot of its current word. A declaration
			// introduced inside the loop never supplies an outer binder.
			return (after == initialized || after == 0) && (snapshot || writes == beforeWrites) && declaration == beforeDeclaration
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
			// A standalone local update completes before a later snapshot.
			// Preserve it verbatim and retain definite assignment. Updates
			// nested in operands still go through value(), which refuses them:
			// copying before NEW must not move a capture ahead of an update
			// performed while evaluating that allocation's arguments.
			if snapshot {
				var update *values.JavaExpression
				switch x := st.(type) {
				case *values.JavaExpression:
					update = x
				case *statements.ExpressionStatement:
					update, _ = x.Expression.(*values.JavaExpression)
				}
				if update != nil && (update.Op == values.INC || update.Op == values.DEC) && (len(update.Values) == 1 || len(update.Values) == 2) {
					if len(update.Values) == 2 {
						// The core's standalone IINC uses a binary postfix node
						// with an int-one marker. No hidden/effectful operand may
						// borrow that representation's otherwise pure local update.
						marker, ok := update.Values[1].(*values.JavaLiteral)
						if !ok || marker == nil || marker.Units != nil || marker.Type() == nil {
							return 0, false
						}
						word, integer := marker.Data.(int)
						primitive, ok := marker.Type().RawType().(*types.JavaPrimer)
						if !integer || word != 1 || !ok || primitive == nil || primitive.Name != types.JavaInteger {
							return 0, false
						}
					}
					matched, known := same(update.Values[0])
					if !known {
						return 0, false
					}
					if matched {
						if state != initialized || !value(update.Values[0], state) || ref.Type() == nil {
							return 0, false
						}
						primitive, numeric := ref.Type().RawType().(*types.JavaPrimer)
						if !numeric || primitive == nil {
							return 0, false
						}
						operand, bounded := nativeMemberEnclosingUnpack(update.Values[0], work)
						if !bounded || sourceProofNil(operand) || operand.Type() == nil {
							return 0, false
						}
						updatedType, numeric := operand.Type().RawType().(*types.JavaPrimer)
						if !numeric || updatedType == nil || updatedType.Name != primitive.Name {
							return 0, false // A reused physical slot is not a cross-category source binder.
						}
						switch primitive.Name {
						case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
						default:
							return 0, false // Boolean/reference storage has no primitive ++/-- operation.
						}
						writes++
						delete(activeStatements, st)
						continue
					}
				}
			}
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
					} else if (assign.IsDeclare || assign.IsFirst) && declaration == nil && state == absent && !sourceProofNil(assign.JavaValue) {
						// A singly initialized declaration has the same flow state
						// as a blank declaration followed by its sole assignment.
						// Read the initializer before making the binder available;
						// self reads/captures and later writes remain forbidden.
						if !value(assign.JavaValue, state) || store != nil && !store(assign) {
							return 0, false
						}
						declaration, state = assign, initialized
						writes++
					} else {
						if assign.IsDeclare || assign.IsFirst || declaration == nil || state != blank && !(snapshot && state == initialized) || sourceProofNil(assign.JavaValue) || !value(assign.JavaValue, state) {
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
				if (captureStatement != nil || captureValue != nil) && loopDepth > 0 && custom.SourceTransferOnly() {
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
					if captureStatement == nil && captureValue == nil || loopDepth == 0 || !x.SourceTransferOnly() {
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
			case *statements.TryCatchStatement:
				// A handler may be entered before or after any protected
				// assignment. Use the lattice's conservative upper bound,
				// never the try's normal exit state. This intentionally
				// refuses handler reads/writes of the captured binder until
				// a more precise exceptional reaching-definition proof exists.
				// A normally completing handler keeps that uncertainty; only
				// proved return/throw paths disappear from the following join.
				if len(x.CatchBodies) == 0 || len(x.CatchBodies) > 64 || len(x.Exception) != len(x.CatchBodies) || len(x.Handlers) != len(x.CatchBodies) || !nativeProofWork(work, int64(len(x.Handlers))) {
					return 0, false
				}
				for i, handler := range x.Handlers {
					caught := x.Exception[i]
					if caught == nil || caught.Id == nil || caught.Id == ref.Id || caught.IsParam || caught.IsThis || caught.CustomValue != nil || caught.StackVar != nil || handler.CatchAll || handler.EntryPC < 0 || handler.EntryPC > 65535 || len(handler.ProtectedRanges) == 0 || len(handler.ProtectedRanges) > 4096 || !nativeProofWork(work, int64(len(handler.ProtectedRanges))) {
						return 0, false
					}
					for _, span := range handler.ProtectedRanges {
						if span[0] < 0 || span[0] >= span[1] || span[1] > 65536 {
							return 0, false
						}
					}
				}
				// Catch-all/finally needs pending abrupt-completion states;
				// it cannot be interpreted as an ordinary typed catch here.
				beforeDeclaration := declaration
				normal, valid := walk(x.TryBody, state)
				if !valid {
					return 0, false
				}
				if beforeDeclaration == nil && declaration != nil {
					// A declaration introduced inside the protected block has
					// no source scope at the following outer checkpoint.
					normal = absent
				}
				for _, handler := range x.CatchBodies {
					caught, valid := walk(handler, absent|blank|initialized)
					if !valid {
						return 0, false
					}
					normal |= caught
				}
				state = normal
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
				// Switch edges need their own multi-entry transfer model.
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
