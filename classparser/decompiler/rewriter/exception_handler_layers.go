package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// restoreExceptionHandlerLayers separates an enclosing exception-table region
// from the handlers it protects. Several enclosing types can have the same
// ranges; they belong to one outer layer, not a sequence of sibling catches
// after an inner multi-catch. Priority and actual caught-value bindings remain
// unchanged. Moving a throwing effect under an additional handler requires an
// original protected-PC witness, or a closed child guard that absorbs it.
func restoreExceptionHandlerLayers(tr *statements.TryCatchStatement) (*statements.TryCatchStatement, bool) {
	if tr == nil || len(tr.Exception) < 2 || len(tr.Exception) > 16 || len(tr.Exception) != len(tr.CatchBodies) || len(tr.Exception) != len(tr.Handlers) {
		return nil, false
	}
	for split := 1; split < len(tr.Handlers); split++ {
		ranges, valid := canonicalHandlerRanges(tr.Handlers[split].ProtectedRanges)
		if !valid {
			continue
		}
		covered := func(pc int) bool { return finallyContains(ranges, pc) }
		matched := true
		for _, outer := range tr.Handlers[split:] {
			other, ok := canonicalHandlerRanges(outer.ProtectedRanges)
			if !ok || !slices.Equal(ranges, other) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		for _, inner := range tr.Handlers[:split] {
			if !covered(inner.EntryPC) || len(inner.ProtectedRanges) == 0 {
				matched = false
				break
			}
			for _, interval := range inner.ProtectedRanges {
				if !handlerIntervalCovered(ranges, interval) {
					matched = false
					break
				}
			}
		}
		if !matched {
			continue
		}
		proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
		if !proof.block(tr.TryBody, covered, 0) {
			continue
		}
		for _, body := range tr.CatchBodies[:split] {
			if !proof.block(body, covered, 0) {
				matched = false
				break
			}
		}
		// Java sibling handlers do not catch one another. The proposed outer
		// layer must therefore not erase a raw protection edge between their
		// throwing effects; catch-entry ASTORE self-coverage alone is inert.
		for _, body := range tr.CatchBodies[split:] {
			if !proof.block(body, func(pc int) bool { return !covered(pc) }, 0) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		inner := *tr
		inner.Exception = slices.Clone(tr.Exception[:split])
		inner.CatchBodies = slices.Clone(tr.CatchBodies[:split])
		inner.Handlers = slices.Clone(tr.Handlers[:split])
		// Every recursive prefix has fewer handlers (at most sixteen), so
		// repeated enclosing layers terminate without revisiting a raw tree.
		if nested, ok := restoreExceptionHandlerLayers(&inner); ok {
			inner = *nested
		}
		outer := *tr
		outer.TryBody = []statements.Statement{&inner}
		outer.Exception = slices.Clone(tr.Exception[split:])
		outer.CatchBodies = slices.Clone(tr.CatchBodies[split:])
		outer.Handlers = slices.Clone(tr.Handlers[split:])
		return &outer, true
	}
	return nil, false
}

func canonicalHandlerRanges(input [][2]int) ([][2]int, bool) {
	if len(input) == 0 || len(input) > 64 {
		return nil, false
	}
	ranges := slices.Clone(input)
	for _, interval := range ranges {
		if interval[0] < 0 || interval[0] >= interval[1] || interval[1] > 65535 {
			return nil, false
		}
	}
	slices.SortFunc(ranges, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	result := ranges[:0]
	for _, interval := range ranges {
		if len(result) > 0 && interval[0] <= result[len(result)-1][1] {
			if interval[1] > result[len(result)-1][1] {
				result[len(result)-1][1] = interval[1]
			}
		} else {
			result = append(result, interval)
		}
	}
	return result, true
}

func handlerIntervalCovered(ranges [][2]int, interval [2]int) bool {
	if interval[0] < 0 || interval[0] >= interval[1] {
		return false
	}
	for _, outer := range ranges {
		if outer[0] <= interval[0] && interval[1] <= outer[1] {
			return true
		}
	}
	return false
}

type handlerLayerProof struct {
	remaining int
	active    map[statements.Statement]bool
}

func (p *handlerLayerProof) block(body []statements.Statement, covered func(int) bool, depth int) bool {
	if depth > 24 {
		return false
	}
	for _, statement := range body {
		p.remaining--
		if statement == nil || p.remaining < 0 || p.active[statement] {
			return false
		}
		p.active[statement] = true
		ok := p.statement(statement, covered, depth)
		delete(p.active, statement)
		if !ok {
			return false
		}
	}
	return true
}

func (p *handlerLayerProof) statement(statement statements.Statement, covered func(int) bool, depth int) bool {
	switch x := statement.(type) {
	case *statements.ReturnStatement:
		return x != nil && (x.JavaValue == nil || finallyCoveredValue(x.JavaValue, covered))
	case *statements.AssignStatement:
		if x == nil {
			return false
		}
		if local, ok := x.LeftValue.(*values.JavaRef); ok && x.ArrayMember == nil {
			return finallyPureLocal(local) && (x.JavaValue == nil || finallyCoveredValue(x.JavaValue, covered))
		}
		return finallyCoveredAssignment(x, covered)
	case *statements.ExpressionStatement:
		return x != nil && finallyCoveredValue(x.Expression, covered)
	case *statements.IfStatement:
		return x != nil && finallyCoveredValue(x.Condition, covered) && p.block(x.IfBody, covered, depth+1) && p.block(x.ElseBody, covered, depth+1)
	case *statements.TryCatchStatement:
		if x == nil || len(x.Exception) == 0 || len(x.Exception) != len(x.CatchBodies) || len(x.Exception) != len(x.Handlers) {
			return false
		}
		// An internally handled cleanup call cannot propagate to this added
		// outer layer. Require a real Throwable handler protecting every effect
		// and a non-throwing handler body, rather than assuming a printed try is
		// safe. This also preserves cleanup side effects and first-error aliases.
		if len(x.Exception) == 1 && x.Exception[0] != nil && x.Exception[0].Type() != nil {
			name := x.Exception[0].Type().String(&class_context.ClassContext{})
			if name == "Throwable" || name == "java.lang.Throwable" {
				rows, valid := canonicalHandlerRanges(x.Handlers[0].ProtectedRanges)
				if valid && p.block(x.CatchBodies[0], func(int) bool { return false }, depth+1) && p.block(x.TryBody, func(pc int) bool { return finallyContains(rows, pc) }, depth+1) {
					return true
				}
			}
		}
		if !p.block(x.TryBody, covered, depth+1) {
			return false
		}
		for _, handler := range x.CatchBodies {
			if !p.block(handler, covered, depth+1) {
				return false
			}
		}
		return true
	case *statements.CustomStatement:
		if x == nil {
			return false
		}
		if x.Name == "end" && x.ThrownValue == nil {
			return true
		}
		return x.ThrownValue != nil && x.HasOriginPC && covered(x.OriginPC) && finallyCoveredValue(x.ThrownValue, covered)
	case *statements.MiddleStatement:
		return x != nil && x.Flag == "end" && x.Data == nil
	}
	return false
}
