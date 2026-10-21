package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"slices"
)

// FinallyRegion is a rendering view. The original tree remains intact for
// variable rebinding, declaration placement and all existing child visitors.
type FinallyRegion struct {
	// SourcePrologue belongs only to RecoverCatchAllFinallyForSource. It
	// initializes private pending-Throwable state or declares an existing
	// local whose normal continuation has left the protected lexical scope.
	SourcePrologue []statements.Statement
	// SourceEpilogue is the original normal continuation exposed only after
	// an enclosing finally view removed its own duplicated cleanup. It is
	// rendered after this finally, without changing the canonical typed tree.
	SourceEpilogue []statements.Statement
	TryBody        []statements.Statement
	Exceptions     []*values.JavaRef
	CatchBodies    [][]statements.Statement
	Cleanup        []statements.Statement
	// Continuation is evaluated only after cleanup completes normally.
	Continuation *statements.CustomStatement
}

// RecoverCatchAllFinally recognizes javac's duplicated cleanup using raw
// exception-table coverage and typed invoke identities. A sibling catch-all
// does not protect preceding catch bodies; emitting it as one silently loses
// cleanup on rethrows. Conversely, normal cleanup copies are outside the
// protected intervals, so leaving them in the try catches cleanup failures
// incorrectly. Both changes must be proved together, before any tree changes.
// Unknown control flow, opaque effects and region gaps fail closed.
func RecoverCatchAllFinally(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
	if resource, ok := recoverTypedResourceFinally(tr); ok {
		return resource, true
	}
	if resource, ok := recoverPrimaryResourceFinally(tr); ok {
		return resource, true
	}
	if tr == nil || len(tr.Exception) == 0 || len(tr.Exception) > 32 || len(tr.Handlers) != len(tr.Exception) || len(tr.CatchBodies) != len(tr.Exception) {
		return nil, false
	}
	last := len(tr.Handlers) - 1
	h := tr.Handlers[last]
	if !h.CatchAll || len(h.ProtectedRanges) == 0 || len(h.ProtectedRanges) > 32 {
		return nil, false
	}
	var rows []core.HandlerRange
	for i, handler := range tr.Handlers {
		if i != last && (handler.CatchAll || !finallyContains(h.ProtectedRanges, handler.EntryPC)) {
			return nil, false
		}
		if len(handler.ProtectedRanges) == 0 || len(handler.ProtectedRanges) > 32 {
			return nil, false
		}
		for _, row := range handler.ProtectedRanges {
			if row[0] < 0 || row[0] >= row[1] || row[1] > 65535 {
				return nil, false
			}
			rows = append(rows, core.HandlerRange{StartPc: uint16(row[0]), EndPc: uint16(row[1])})
		}
	}
	body := finallyWithoutEnd(tr.CatchBodies[last])
	if len(body) == 0 || len(body) > 9 || tr.Exception[last] == nil {
		return nil, false
	}
	cleanup, ok := finallyHandlerCleanup(body, tr.Exception[last], h.ProtectedRanges)
	if !ok || len(cleanup) == 0 {
		return nil, false
	}
	excluded := append([]*values.JavaRef{}, tr.Exception...)
	queue := append([]statements.Statement{}, tr.TryBody...)
	for _, b := range tr.CatchBodies[:last] {
		queue = append(queue, b...)
	}
	scope := append([]statements.Statement(nil), queue...)
	scope = append(scope, tr.CatchBodies[last]...)
	seen := map[statements.Statement]bool{}
	for len(queue) > 0 && len(seen) < 256 {
		st := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if st == nil || seen[st] {
			return nil, false
		}
		seen[st] = true
		switch x := st.(type) {
		case *statements.AssignStatement:
			if x == nil {
				return nil, false
			}
			// Only declarations belong to the try's lexical scope. An outer
			// local may be updated by the protected body and read by cleanup;
			// each copy reads its latest value at the exit. Excluding every
			// reassignment prevents correct finally recovery and catches a
			// cleanup failure a second time in the emitted catch-all.
			if ref, ok := plainTryValue(x.LeftValue).(*values.JavaRef); ok && (x.IsDeclare || x.IsFirst) {
				excluded = append(excluded, ref)
			}
		case *statements.IfStatement:
			if x == nil {
				return nil, false
			}
			queue = append(queue, x.IfBody...)
			queue = append(queue, x.ElseBody...)
		}
	}
	if len(queue) != 0 {
		return nil, false
	}
	if !sameFinallyCleanupBlock(rows, cleanup, cleanup, excluded, scope) {
		return nil, false
	}
	p := finallyProof{cleanup: cleanup, rows: rows, excluded: excluded, scope: scope, remaining: 256, allowContinuation: true}
	// The try arm is protected by every typed catch, whereas catch bodies are
	// protected only by the raw catch-all. Preserve that distinction for folded
	// calls and casts, not just the PC of their enclosing statement.
	tryCovered := func(pc int) bool {
		for _, handler := range tr.Handlers {
			if !finallyContains(handler.ProtectedRanges, pc) {
				return false
			}
		}
		return true
	}
	tryBody, _, ok := p.block(tr.TryBody, true, tryCovered)
	if !ok {
		return nil, false
	}
	result := &FinallyRegion{TryBody: tryBody, Exceptions: tr.Exception[:last], Cleanup: cleanup}
	for _, b := range tr.CatchBodies[:last] {
		rewritten, _, ok := p.block(b, true, func(pc int) bool { return finallyContains(h.ProtectedRanges, pc) })
		if !ok {
			return nil, false
		}
		result.CatchBodies = append(result.CatchBodies, rewritten)
	}
	if p.continuation != nil {
		if p.normalWithoutContinuation {
			return nil, false
		}
		result.Continuation = p.continuation
	}
	return result, true
}

func finallyContains(rows [][2]int, pc int) bool {
	for _, row := range rows {
		if pc >= row[0] && pc < row[1] {
			return true
		}
	}
	return false
}

func finallyWithoutEnd(body []statements.Statement) []statements.Statement {
	for len(body) > 0 {
		end, ok := body[len(body)-1].(*statements.MiddleStatement)
		if !ok || end == nil || end.Flag != "end" || end.Data != nil {
			break
		}
		body = body[:len(body)-1]
	}
	return body
}

type finallyProof struct {
	cleanup      []statements.Statement
	rows         []core.HandlerRange
	excluded     []*values.JavaRef
	scope        []statements.Statement
	remaining    int
	loopDepth    int
	loopLabels   []string
	cleanupMatch func(statements.Statement, statements.Statement) bool
	// Only a caller that renders Continuation may remove it from a child.
	// Resource-tail queries also use this proof and do not own that exit.
	allowContinuation bool
	continuation      *statements.CustomStatement
	// Every normal completion must carry the same original continuation.
	// A different arm falling through cannot acquire that arm's throw.
	normalWithoutContinuation bool
}

func (p *finallyProof) block(input []statements.Statement, mustExit bool, covered func(int) bool) ([]statements.Statement, bool, bool) {
	body := finallyWithoutEnd(input)
	end := len(body)
	var ret *statements.ReturnStatement
	liftedContinuation := false
	if end > 0 && len(p.cleanup) > 0 {
		if normalized, lifted := finallyConditionalReturn(body[end-1], p.cleanup[len(p.cleanup)-1], p.rows, p.excluded); lifted != nil {
			body = append([]statements.Statement{}, body...)
			body[end-1] = normalized
			ret = lifted
		}
	}

	if p.allowContinuation && end > 0 && ret == nil && len(p.cleanup) > 0 {
		normalized, after := finallyFlatThrowContinuation(body, p.cleanup, p.rows, p.excluded)
		if after == nil {
			if last, continuation := finallyConditionalThrowContinuation(body[end-1], p.cleanup[len(p.cleanup)-1], p.rows, p.excluded); continuation != nil {
				normalized = append([]statements.Statement{}, body...)
				normalized[end-1], after = last, continuation
			}
		}
		if after != nil {
			if p.continuation != nil && (p.continuation.OriginPC != after.OriginPC || p.continuation.ThrownValue != after.ThrownValue) {
				return nil, false, false
			}
			p.continuation = after
			liftedContinuation = true
			body = normalized
			end = len(body)
		}
	}
	if end > 0 {
		terminal, _ := body[end-1].(*statements.ReturnStatement)
		if terminal != nil {
			ret = terminal
			if !ret.HasOriginPC || covered(ret.OriginPC) || (ret.JavaValue != nil && !finallyPureLocal(ret.JavaValue)) {
				return nil, false, false
			}
			end--
		}
	}
	if len(p.cleanup) > 0 && end >= len(p.cleanup) {
		candidates := body[end-len(p.cleanup) : end]
		matched := sameFinallyCleanupBlock(p.rows, candidates, p.cleanup, p.excluded, p.scope)
		if p.cleanupMatch != nil {
			matched = true
			for i, st := range p.cleanup {
				if !p.cleanupMatch(candidates[i], st) {
					matched = false
					break
				}
			}
		}
		if matched {
			if !mustExit && ret == nil {
				return nil, false, false // cleanup cannot precede a continuation
			}
			// A return exits this region even when nested in a loop whose
			// other paths keep executing inside it. Its inert operand and
			// original cleanup copy were proved above. A loop break/back edge
			// alone cannot acquire this permission.
			prefix, exits, ok := p.block(body[:end-len(p.cleanup)], false, covered)
			if !ok || exits {
				return nil, false, false
			}
			if ret != nil {
				prefix = append(prefix, ret)
			}
			if ret == nil && !liftedContinuation {
				p.normalWithoutContinuation = true
			}
			return prefix, ret != nil, true
		}
	}
	if ret != nil {
		return nil, false, false // normal exit without its proven cleanup copy
	}
	var out []statements.Statement
	exits := false
	normalCleaned := false
	for i, st := range body {
		p.remaining--
		if p.remaining < 0 || st == nil || exits {
			return nil, false, false
		}
		switch x := st.(type) {
		case *values.JavaExpression:
			if x == nil || (x.Op != values.INC && x.Op != values.DEC) || !finallyCoveredValue(x, covered) {
				return nil, false, false
			}
		case *statements.SourceAssertionStatement:
			condition, call, pc, known := x.SourceAssertionProtocol()
			if !known || !covered(pc) || !finallyCoveredValue(condition, covered) || !finallyCoveredValue(call, covered) {
				return nil, false, false
			}
		case *statements.ExpressionStatement:
			if x == nil || !finallyCoveredValue(x.Expression, covered) {
				return nil, false, false
			}
		case *statements.AssignStatement:
			if !finallyCoveredAssignment(x, covered) {
				return nil, false, false
			}
		case *statements.CustomStatement:
			if kind, label, proved := x.SourceTransferProtocol(); proved && p.loopDepth > 0 && (kind == "break" || kind == "continue") && (label == "" || slices.Contains(p.loopLabels, label)) {
				out = append(out, st)
				exits = true
				continue
			}
			if x != nil && p.loopDepth > 0 && ((x.Name == "break" || x.Name == "continue") || ((x.LoopTransferKind == "break" || x.LoopTransferKind == "continue") && x.LoopTargetLabel != "" && slices.Contains(p.loopLabels, x.LoopTargetLabel))) && x.ThrownValue == nil {
				out = append(out, st)
				exits = true
				continue
			}
			if x == nil || x.ThrownValue == nil || !x.HasOriginPC || !covered(x.OriginPC) || !finallyCoveredValue(x.ThrownValue, covered) {
				return nil, false, false
			}
			exits = true
		case *statements.IfStatement:
			if x == nil || !finallyCoveredValue(x.Condition, covered) {
				return nil, false, false
			}
			clone := *x
			var a, b, ok bool
			needExit := mustExit && i == len(body)-1
			clone.IfBody, a, ok = p.block(x.IfBody, needExit, covered)
			if !ok {
				return nil, false, false
			}
			clone.ElseBody, b, ok = p.block(x.ElseBody, needExit, covered)
			if !ok {
				return nil, false, false
			}
			exits = a && b
			normalCleaned = needExit
			st = &clone
		case *statements.DoWhileStatement:
			if x == nil || !finallyCoveredValue(x.ConditionValue, covered) {
				return nil, false, false
			}
			clone := *x
			p.loopDepth++
			if x.Label != "" {
				p.loopLabels = append(p.loopLabels, x.Label)
			}
			var ok bool
			clone.Body, _, ok = p.block(x.Body, false, covered)
			p.loopDepth--
			if x.Label != "" {
				p.loopLabels = p.loopLabels[:len(p.loopLabels)-1]
			}
			if !ok {
				return nil, false, false
			}
			st = &clone
		case *statements.SwitchStatement:
			if !p.internalSwitchCovered(x, covered) {
				return nil, false, false
			}
		case *statements.SynchronizedStatement:
			pc, known := x.OriginalMonitorEnterPC()
			if !known || !covered(pc) || !finallyCoveredValue(x.Argument, covered) {
				return nil, false, false
			}
			clone := *x
			var valid bool
			clone.Body, exits, valid = p.block(x.Body, false, covered)
			if !valid {
				return nil, false, false
			}
			st = &clone
		case *statements.TryCatchStatement:
			if x == nil || len(x.Exception) == 0 || len(x.Exception) != len(x.CatchBodies) || len(x.Handlers) != len(x.Exception) {
				return nil, false, false
			}
			clone := *x
			needExit := mustExit && i == len(body)-1
			var normalExit, ok bool
			clone.TryBody, normalExit, ok = p.block(x.TryBody, needExit, covered)
			if !ok {
				return nil, false, false
			}
			clone.CatchBodies = make([][]statements.Statement, len(x.CatchBodies))
			allExit := normalExit
			for j, handler := range x.Handlers {
				if !covered(handler.EntryPC) {
					return nil, false, false
				}
				var handlerExit bool
				clone.CatchBodies[j], handlerExit, ok = p.block(x.CatchBodies[j], needExit, covered)
				if !ok {
					return nil, false, false
				}
				allExit = allExit && handlerExit
			}
			exits = allExit
			// Each nested arm has independently proved either an abrupt
			// exit or its normal cleanup copy. Normal completion is not an
			// abrupt transfer, but still discharges this enclosing proof.
			normalCleaned = needExit
			st = &clone
		default:
			return nil, false, false
		}
		out = append(out, st)
	}
	return out, exits, !mustExit || exits || normalCleaned
}

func finallyPureLocal(v values.JavaValue) bool {
	v = plainTryValue(v)
	if v == values.JavaNull || values.IsNullLiteral(v) {
		return true
	}
	switch v := v.(type) {
	case *values.JavaRef:
		return v != nil && v.Id != nil && v.CustomValue == nil && v.StackVar == nil
	case *values.JavaLiteral:
		return v != nil
	}
	return false
}

func finallyCoveredValue(root values.JavaValue, covered func(int) bool) bool {
	active := map[values.JavaValue]bool{}
	proved := map[values.JavaValue]bool{}
	remaining := 512
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) (valid bool) {
		if v == nil || (reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil()) || active[v] {
			return false
		}
		// The covered predicate is fixed for this complete proof. A shared
		// immutable DAG node needs one domain check, irrespective of the number
		// of paths which reach it. This memoizes evidence, never evaluation:
		// the expression and its original conditional effects remain unchanged.
		if proved[v] {
			return true
		}
		remaining--
		if remaining < 0 {
			return false
		}
		active[v] = true
		defer func() {
			delete(active, v)
			if valid {
				proved[v] = true
			}
		}()
		var children []values.JavaValue
		if v == values.JavaNull || values.IsNullLiteral(v) {
			return true
		}
		switch x := v.(type) {
		case *values.JavaRef, *values.JavaLiteral:
			return finallyPureLocal(v)
		case *values.JavaClassValue:
			// An actual ldc Class can fail during resolution. A symbolic owner
			// used only for invocation/type metadata has no evaluation PC and
			// cannot serve as evidence for moving an evaluated class literal.
			return x != nil && x.JavaType != nil && x.HasOriginPC && x.OriginPC >= 0 && covered(x.OriginPC)
		case *values.SlotValue:
			if x == nil {
				return false
			}
			children = []values.JavaValue{x.GetValue()}
		case *values.FunctionCallExpression:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) || x.Kind >= values.InvokeDynamic || x.Descriptor == "" || x.ClassName == "" || x.FunctionName == "" || (x.Kind == values.InvokeStatic) != x.IsStatic {
				return false
			}
			children = append(children, x.Arguments...)
			if x.IsStatic {
				// Reuse the exact static-qualifier proof without requiring a
				// void return or an unprotected PC.
				if x.Object != nil && !finallyStaticQualifier(x) {
					return false
				}
			} else {
				children = append(children, x.Object)
			}
		case *values.JavaArrayMember:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) {
				return false
			}
			children = []values.JavaValue{x.Object, x.Index}
		case *values.ArrayLengthExpression:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) {
				return false
			}
			children = []values.JavaValue{x.Array}
		case *values.NewExpression:
			if x == nil {
				return false
			}
			if x.IsArray() {
				if !x.HasOriginPC || !covered(x.OriginPC) || x.HasEvaluationEndPC && !covered(x.EvaluationEndPC) || len(x.Initializer) > 0 && !x.HasEvaluationEndPC {
					return false
				}
				if x.HasEvaluationEndPC {
					if x.OriginPC < 0 || x.EvaluationEndPC < x.OriginPC || x.EvaluationEndPC > 65535 {
						return false
					}
					// Endpoints alone cannot prove the handler ownership of
					// folded stores across a gap between protected intervals.
					for pc := x.OriginPC; pc <= x.EvaluationEndPC; pc++ {
						if !covered(pc) {
							return false
						}
					}
				}
				children = append(append([]values.JavaValue{}, x.Length...), x.Initializer...)
				break
			}
			ctor := x.ConstructorCall
			if !x.HasOriginPC || !covered(x.OriginPC) || x.IsArray() || ctor == nil || !ctor.HasOriginPC || !covered(ctor.OriginPC) || ctor.Kind != values.InvokeSpecial || ctor.FunctionName != "<init>" {
				return false
			}
			children = ctor.Arguments // ctor.Object points back to this allocation
		case *values.CastExpression:
			if x == nil || x.TargetType == nil || !covered(x.OriginPC) {
				return false
			}
			children = []values.JavaValue{x.Value}
		case *values.JavaExpression:
			if x == nil {
				return false
			}
			if x.Op == values.SUB && len(x.Values) == 1 {
				// JVM numeric negation is total, including MIN_VALUE, signed
				// zero and NaN. Its operand may still throw at its original PC.
				// Do not import this fact into boxed/unboxing source views.
				return visit(x.Values[0]) && finallyUnaryNumericNegationTypes(x)
			}
			if (x.Op == values.Not && len(x.Values) != 1) || (x.Op != values.Not && len(x.Values) != 2) {
				return false
			}
			switch x.Op {
			case values.Not, values.EQ, values.NEQ, values.LT, values.LTE, values.GT, values.GTE, values.LOGICAL_AND, values.LOGICAL_OR:
				children = x.Values
			case values.ADD, values.SUB, values.MUL, values.AND, values.OR, values.XOR, values.SHL, values.SHR, values.USHR, values.INC, values.DEC:
				for _, operand := range x.Values {
					if operand == nil || operand.Type() == nil {
						return false
					}
					primitive, ok := operand.Type().RawType().(*types.JavaPrimer)
					if !ok {
						return false
					}
					switch primitive.Name {
					case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
					default:
						return false
					}
				}
				if x.Op == values.INC || x.Op == values.DEC {
					ref, ok := plainTryValue(x.Values[0]).(*values.JavaRef)
					if !ok || ref == nil || !finallyPureLocal(ref) {
						return false
					}
				}
				children = x.Values
			default:
				return false
			}
		case *values.TernaryExpression:
			if x == nil || x.Condition == nil || x.TrueValue == nil || x.FalseValue == nil {
				return false
			}
			// The selection itself does not throw. Every evaluation on both
			// arms must retain its own original protected-domain witness.
			children = []values.JavaValue{x.Condition, x.TrueValue, x.FalseValue}
		case *values.AssignmentExpression:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) || !finallyPureLocal(x.Target) {
				return false
			}
			children = []values.JavaValue{x.Value}
		case *values.RefMember:
			if x == nil || !x.HasOriginPC || !covered(x.OriginPC) || x.Member == "" || x.JavaType == nil {
				return false
			}
			children = []values.JavaValue{x.Object}
		case *values.JavaClassMember:
			return x != nil && x.HasOriginPC && covered(x.OriginPC) && x.Name != "" && x.Member != "" && x.Description != ""
		case *values.CustomValue:
			if operand, _, pc, original := x.OriginalInstanceOfSource(); original {
				if !covered(pc) || !finallyInstanceOfReferenceOperand(operand) {
					return false
				}
				children = []values.JavaValue{operand}
				break
			}
			if pc, captures, original := x.OriginalLambdaFactoryEvaluation(); original {
				if !covered(pc) {
					return false
				}
				children = captures
				break
			}
			if operand, _, _, original := x.SourcePrimitiveConversion(); original {
				if !totalPrimitiveConversionOperand(operand) {
					return false
				}
				children = []values.JavaValue{operand}
				break
			}
			if x == nil || x.Flag != "instanceof" || !x.CapturesKnown || len(x.Captures) != 1 || !x.HasOriginPC || !covered(x.OriginPC) {
				return false
			}
			children = x.Captures
		default:
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	return visit(root)
}

// Match only bounded cleanup syntax with stable operands and original PCs
// outside every protected interval. Names are field/invoke identities from the
// IR; rendered text and library-specific patterns never establish equivalence.
func sameFinallyCleanup(rows []core.HandlerRange, a, b statements.Statement, excluded []*values.JavaRef, depth int) bool {
	p := finallyCleanupComparison{remaining: 2048}
	return p.statement(rows, a, b, excluded, depth)
}

type finallyCleanupComparison struct {
	remaining int
}

func (p *finallyCleanupComparison) statement(rows []core.HandlerRange, a, b statements.Statement, excluded []*values.JavaRef, depth int) bool {
	p.remaining--
	if p.remaining < 0 || depth > 16 || a == nil || b == nil {
		return false
	}
	outside := func(pc int) bool {
		for _, row := range rows {
			if pc >= int(row.StartPc) && pc < int(row.EndPc) {
				return false
			}
		}
		return true
	}
	switch x := a.(type) {
	case *statements.ExpressionStatement:
		y, ok := b.(*statements.ExpressionStatement)
		return ok && x != nil && y != nil && (sameUnprotectedTryCall(rows, x.Expression, y.Expression, excluded...) || sameFinallyEffectValue(rows, x.Expression, y.Expression, excluded, depth))
	case *statements.AssignStatement:
		y, ok := b.(*statements.AssignStatement)
		if !ok || x == nil || y == nil || x.IsDeclare || y.IsDeclare || x.ArrayMember != nil || y.ArrayMember != nil || !x.HasOriginPC || !y.HasOriginPC || !outside(x.OriginPC) || !outside(y.OriginPC) {
			return false
		}
		lhs, lok := plainTryValue(x.LeftValue).(*values.RefMember)
		rhs, rok := plainTryValue(y.LeftValue).(*values.RefMember)
		if static, ok := plainTryValue(x.LeftValue).(*values.JavaClassMember); ok {
			other, ok := plainTryValue(y.LeftValue).(*values.JavaClassMember)
			return ok && static != nil && other != nil && static.Name != "" && static.Name == other.Name && static.Member != "" && static.Member == other.Member && static.Description != "" && static.Description == other.Description && static.JavaType != nil && other.JavaType != nil && reflect.DeepEqual(static.JavaType.RawType(), other.JavaType.RawType()) && sameFinallyEffectValue(rows, x.JavaValue, y.JavaValue, excluded, depth)
		}
		if !lok || !rok || lhs == nil || rhs == nil || lhs.Member == "" || lhs.Member != rhs.Member || lhs.JavaType == nil || rhs.JavaType == nil || !reflect.DeepEqual(lhs.JavaType.RawType(), rhs.JavaType.RawType()) {
			return false
		}
		self, sok := plainTryValue(lhs.Object).(*values.JavaRef)
		return sok && self != nil && self.IsThis && sameTryLocal(rhs.Object, self) && sameFinallyStableValue(x.JavaValue, y.JavaValue, excluded, 0)
	case *statements.IfStatement:
		y, ok := b.(*statements.IfStatement)
		if !ok || x == nil || y == nil || !(sameFinallyStableValue(x.Condition, y.Condition, excluded, 0) || sameFinallyEffectValue(rows, x.Condition, y.Condition, excluded, depth)) {
			return false
		}
		strip := func(body []statements.Statement) []statements.Statement {
			body = finallyWithoutEnd(body)
			if len(body) > 0 {
				if ret, ok := body[len(body)-1].(*statements.ReturnStatement); ok && ret != nil && ret.JavaValue == nil && ret.HasOriginPC && outside(ret.OriginPC) {
					return body[:len(body)-1]
				}
			}
			return body
		}
		xa, xb, ya, yb := strip(x.IfBody), strip(x.ElseBody), strip(y.IfBody), strip(y.ElseBody)
		if len(xa) != len(ya) || len(xb) != len(yb) || len(xa)+len(xb) > 16 {
			return false
		}
		for i, st := range xa {
			if !p.statement(rows, st, ya[i], excluded, depth+1) {
				return false
			}
		}
		for i, st := range xb {
			if !p.statement(rows, st, yb[i], excluded, depth+1) {
				return false
			}
		}
		return true
	case *statements.SynchronizedStatement:
		y, ok := b.(*statements.SynchronizedStatement)
		if !ok || x == nil || y == nil {
			return false
		}
		xp, xknown := x.OriginalMonitorEnterPC()
		yp, yknown := y.OriginalMonitorEnterPC()
		if !xknown || !yknown || !outside(xp) || !outside(yp) || !sameFinallyEffectValue(rows, x.Argument, y.Argument, excluded, depth) || len(x.Body) != len(y.Body) || len(x.Body) > 16 {
			return false
		}
		for i, st := range x.Body {
			if !p.statement(rows, st, y.Body[i], excluded, depth+1) {
				return false
			}
		}
		return true
	case *statements.TryCatchStatement:
		y, ok := b.(*statements.TryCatchStatement)
		return ok && p.nested(rows, x, y, excluded, depth+1)
	case *statements.CustomStatement:
		y, ok := b.(*statements.CustomStatement)
		return ok && x != nil && y != nil && x.ThrownValue != nil && y.ThrownValue != nil && x.HasOriginPC && y.HasOriginPC && outside(x.OriginPC) && outside(y.OriginPC) && sameFinallyEffectValue(rows, x.ThrownValue, y.ThrownValue, excluded, depth)
	}
	return false
}
func sameFinallyStableValue(a, b values.JavaValue, excluded []*values.JavaRef, depth int) bool {
	if depth > 16 {
		return false
	}
	a, b = plainTryValue(a), plainTryValue(b)
	if a == nil || b == nil {
		return false
	}
	if a == values.JavaNull || b == values.JavaNull {
		return a == values.JavaNull && b == values.JavaNull
	}
	for _, ref := range excluded {
		if sameTryLocal(a, ref) || sameTryLocal(b, ref) {
			return false
		}
	}
	switch x := a.(type) {
	case *values.JavaRef:
		return sameTryLocal(b, x)
	case *values.JavaLiteral:
		y, ok := b.(*values.JavaLiteral)
		return ok && x != nil && y != nil && reflect.DeepEqual(x.Data, y.Data) && x.Type() != nil && y.Type() != nil && reflect.DeepEqual(x.Type().RawType(), y.Type().RawType())
	case *values.JavaExpression:
		y, ok := b.(*values.JavaExpression)
		if !ok || x == nil || y == nil || x.Op != y.Op || len(x.Values) != len(y.Values) {
			return false
		}
		switch x.Op {
		case values.Not:
			if !sameFinallyBooleanNotShape(x, y) {
				return false
			}
		case values.EQ, values.NEQ:
			if len(x.Values) != 2 {
				return false
			}
		default:
			return false
		}
		for i, v := range x.Values {
			if !sameFinallyStableValue(v, y.Values[i], excluded, depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

// Boolean negation is a total, effect-free operator. Its operands still require
// the same binding/effect-domain proof; numeric nonzero tests, different arity
// or a coincidentally rendered "!" are not evidence of the same cleanup guard.
func sameFinallyBooleanNotShape(x, y *values.JavaExpression) bool {
	if x == nil || y == nil || x.Op != values.Not || y.Op != values.Not || len(x.Values) != 1 || len(y.Values) != 1 {
		return false
	}
	boolean := func(v values.JavaValue) bool {
		v = plainTryValue(v)
		if v == nil || v.Type() == nil {
			return false
		}
		p, ok := v.Type().RawType().(*types.JavaPrimer)
		return ok && p != nil && p.Name == types.JavaBoolean
	}
	return boolean(x) && boolean(y) && boolean(x.Values[0]) && boolean(y.Values[0])
}

// JLS unary numeric promotion must match the emitted source type. The JVM's
// total NEG instruction cannot justify an implicit reference unboxing, nor can
// a source-only local type projection borrow the underlying word's proof.
func finallyUnaryNumericNegationTypes(x *values.JavaExpression) bool {
	if x == nil || x.Op != values.SUB || len(x.Values) != 1 || x.Values[0] == nil || x.Typ == nil {
		return false
	}
	operand := plainTryValue(x.Values[0])
	if operand == nil {
		return false
	}
	var operandType types.JavaType
	switch source := operand.(type) {
	case *values.FunctionCallExpression:
		// A primitive return uses its declared call type. Do not ask a
		// generic receiver callback to infer it, or dereference missing
		// metadata in an incomplete source packet.
		if source.FuncType != nil && source.FuncType.ReturnType != nil {
			declared, err := types.ParseMethodDescriptor(source.Descriptor)
			if err == nil && declared != nil {
				abi, known := declared.RawType().(*types.JavaFuncType)
				if known && abi != nil && abi.ReturnType != nil && reflect.DeepEqual(abi.ReturnType.RawType(), source.FuncType.ReturnType.RawType()) {
					operandType = source.FuncType.ReturnType
				}
			}
		}
	case *values.CustomValue:
		if _, target, _, original := source.SourcePrimitiveConversion(); original {
			operandType = types.NewJavaPrimer(target)
		}
	default:
		operandType = operand.Type()
	}
	if operandType == nil {
		return false
	}
	p, ok := operandType.RawType().(*types.JavaPrimer)
	result, resultOK := x.Typ.RawType().(*types.JavaPrimer)
	if !ok || p == nil || !resultOK || result == nil {
		return false
	}
	if ref, local := operand.(*values.JavaRef); local && ref.WebDeclType != nil && !reflect.DeepEqual(ref.WebDeclType.RawType(), p) {
		return false
	}
	promoted := p.Name
	switch promoted {
	case types.JavaByte, types.JavaShort, types.JavaChar:
		promoted = types.JavaInteger
	case types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
	default:
		return false
	}
	return result.Name == promoted
}
