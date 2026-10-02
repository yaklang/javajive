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
	TryBody     []statements.Statement
	Exceptions  []*values.JavaRef
	CatchBodies [][]statements.Statement
	Cleanup     []statements.Statement
}

// RecoverCatchAllFinally recognizes javac's duplicated cleanup using raw
// exception-table coverage and typed invoke identities. A sibling catch-all
// does not protect preceding catch bodies; emitting it as one silently loses
// cleanup on rethrows. Conversely, normal cleanup copies are outside the
// protected intervals, so leaving them in the try catches cleanup failures
// incorrectly. Both changes must be proved together, before any tree changes.
// Unknown control flow, opaque effects and region gaps fail closed.
func RecoverCatchAllFinally(tr *statements.TryCatchStatement) (*FinallyRegion, bool) {
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
	if len(body) < 2 || len(body) > 9 || tr.Exception[last] == nil {
		return nil, false
	}
	cleanup, ok := finallyHandlerCleanup(body, tr.Exception[last], h.ProtectedRanges)
	if !ok {
		return nil, false
	}
	excluded := append([]*values.JavaRef{}, tr.Exception...)
	queue := append([]statements.Statement{}, tr.TryBody...)
	for _, b := range tr.CatchBodies[:last] {
		queue = append(queue, b...)
	}
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
			if ref, ok := plainTryValue(x.LeftValue).(*values.JavaRef); ok {
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
	for _, st := range cleanup {
		if !sameFinallyCleanup(rows, st, st, excluded, 0) {
			return nil, false
		}
	}
	p := finallyProof{cleanup: cleanup, rows: rows, excluded: excluded, remaining: 256}
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
	remaining    int
	loopDepth    int
	loopLabels   []string
	cleanupMatch func(statements.Statement, statements.Statement) bool
}

func (p *finallyProof) block(input []statements.Statement, mustExit bool, covered func(int) bool) ([]statements.Statement, bool, bool) {
	body := finallyWithoutEnd(input)
	end := len(body)
	var ret *statements.ReturnStatement
	if end > 0 && len(p.cleanup) > 0 {
		if normalized, lifted := finallyConditionalReturn(body[end-1], p.cleanup[len(p.cleanup)-1], p.rows, p.excluded); lifted != nil {
			body = append([]statements.Statement{}, body...)
			body[end-1] = normalized
			ret = lifted
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
		matched := true
		for i, st := range p.cleanup {
			candidate := body[end-len(p.cleanup)+i]
			match := sameFinallyCleanup(p.rows, candidate, st, p.excluded, 0)
			if p.cleanupMatch != nil {
				match = p.cleanupMatch(candidate, st)
			}
			if !match {
				matched = false
				break
			}
		}
		if matched {
			if !mustExit {
				return nil, false, false // cleanup cannot precede a continuation
			}
			prefix, exits, ok := p.block(body[:end-len(p.cleanup)], false, covered)
			if !ok || exits {
				return nil, false, false
			}
			if ret != nil {
				prefix = append(prefix, ret)
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
		case *statements.ExpressionStatement:
			if x == nil || !finallyCoveredValue(x.Expression, covered) {
				return nil, false, false
			}
		case *statements.AssignStatement:
			if !finallyCoveredAssignment(x, covered) {
				return nil, false, false
			}
		case *statements.CustomStatement:
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
	remaining := 512
	var visit func(values.JavaValue) bool
	visit = func(v values.JavaValue) bool {
		remaining--
		if v == nil || remaining < 0 || active[v] {
			return false
		}
		active[v] = true
		defer delete(active, v)
		var children []values.JavaValue
		if v == values.JavaNull || values.IsNullLiteral(v) {
			return true
		}
		switch x := v.(type) {
		case *values.JavaRef, *values.JavaLiteral:
			return finallyPureLocal(v)
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
	if depth > 16 || a == nil || b == nil {
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
			if !sameFinallyCleanup(rows, st, ya[i], excluded, depth+1) {
				return false
			}
		}
		for i, st := range xb {
			if !sameFinallyCleanup(rows, st, yb[i], excluded, depth+1) {
				return false
			}
		}
		return true
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
