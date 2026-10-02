package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// integerDecisionView is a source view of an exact two-literal integer
// decision DAG. The selected word remains an int; this is not a Z sink and
// never turns noncanonical words into nonzero/low-bit truth values.
func integerDecisionView(root *TernaryExpression, ctx *class_context.ClassContext) (JavaValue, int, int, bool) {
	if root == nil || !boundedDecisionGraph(root, 8192, 256) {
		return nil, 0, 0, false
	}
	typ := root.Type()
	if typ == nil {
		return nil, 0, 0, false
	}
	p, ok := typ.RawType().(*types.JavaPrimer)
	if !ok || (p.Name != types.JavaInteger && p.Name != types.JavaBoolean) {
		return nil, 0, 0, false
	}
	words := []int{}
	seen := map[JavaValue]uint8{}
	nodes := 0
	var prove func(JavaValue, int) bool
	prove = func(v JavaValue, depth int) bool {
		if isNilJavaValue(v) || depth > 128 || seen[v] == 1 {
			return false
		}
		if seen[v] == 2 {
			return true
		}
		nodes++
		if nodes > 2048 {
			return false
		}
		seen[v] = 1
		switch x := v.(type) {
		case *SlotValue:
			if !prove(x.GetValue(), depth+1) {
				return false
			}
		case *TernaryExpression:
			if x.Condition == nil || !isBooleanTyped(x.Condition) || !prove(x.TrueValue, depth+1) || !prove(x.FalseValue, depth+1) {
				return false
			}
		case *JavaLiteral:
			if x.JavaType == nil {
				return false
			}
			t, ok := x.JavaType.RawType().(*types.JavaPrimer)
			if !ok || t.Name != types.JavaInteger {
				return false
			}
			word, ok := x.Data.(int)
			if !ok || int64(word) < -2147483648 || int64(word) > 2147483647 {
				return false
			}
			found := false
			for _, w := range words {
				found = found || w == word
			}
			if !found {
				words = append(words, word)
				if len(words) > 2 {
					return false
				}
			}
		default:
			return false
		}
		seen[v] = 2
		return true
	}
	if !prove(root, 0) || len(words) == 0 {
		return nil, 0, 0, false
	}
	if p.Name == types.JavaBoolean {
		for _, word := range words {
			if word != 0 && word != 1 {
				return nil, 0, 0, false
			}
		}
	}
	first, last := words[0], words[0]
	if len(words) == 2 {
		last = words[1]
	}
	// Canonical 0/1 uses the conventional Boolean projection. Other two words
	// keep their exact original values rather than being globally normalized.
	if len(words) == 2 && ((first == 0 && last == 1) || (first == 1 && last == 0)) {
		first, last = 1, 0
	}
	memo := map[JavaValue]JavaValue{}
	var project func(JavaValue) JavaValue
	project = func(v JavaValue) JavaValue {
		if out, ok := memo[v]; ok {
			return out
		}
		var out JavaValue
		switch x := v.(type) {
		case *SlotValue:
			out = project(x.GetValue())
		case *JavaLiteral:
			word := x.Data.(int)
			b := 0
			if word == first {
				b = 1
			}
			out = NewJavaLiteral(b, types.NewJavaPrimer(types.JavaBoolean))
		case *TernaryExpression:
			t := NewTernaryExpression(x.Condition, project(x.TrueValue), project(x.FalseValue))
			t.SetCachedType(types.NewJavaPrimer(types.JavaBoolean))
			out = t
		}
		memo[v] = out
		return out
	}
	return boolReduce(project(root), ctx), first, last, true
}

// Memoization bounds proof work, while this separate saturated recurrence
// bounds the expanded source formula. An arbitrary shared BDD need not have a
// small equivalent formula. Refuse it before allocating exponential strings.
func boundedDecisionSource(v JavaValue, limit int) bool {
	memo := map[JavaValue]int{}
	active := map[JavaValue]bool{}
	work := 0
	var size func(JavaValue, int) int
	size = func(v JavaValue, depth int) int {
		if isNilJavaValue(v) || depth > 256 || active[v] {
			return limit + 1
		}
		if n, ok := memo[v]; ok {
			return n
		}
		work++
		if work > 8192 {
			return limit + 1
		}
		active[v] = true
		defer delete(active, v)
		n := 1
		children, known := Children(v)
		if known {
			for _, child := range children {
				if child == nil {
					continue
				}
				m := size(child, depth+1)
				if m > limit-n {
					return limit + 1
				}
				n += m
			}
		}
		memo[v] = n
		return n
	}
	return size(v, 0) <= limit
}

func renderIntegerDecision(root *TernaryExpression, ctx *class_context.ClassContext) (string, bool) {
	pred, first, last, ok := integerDecisionView(root, ctx)
	if !ok {
		return "", false
	}
	if !boundedDecisionSource(pred, 65536) {
		return EmptySlotValuePlaceholder, true
	}
	if root.Type().RawType().(*types.JavaPrimer).Name == types.JavaBoolean {
		if first == last && first == 0 {
			pred = NewBinaryExpression(pred, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaBoolean)), LOGICAL_AND, types.NewJavaPrimer(types.JavaBoolean))
		}
		return pred.String(ctx), true
	}
	return fmt.Sprintf("(%s) ? (%d) : (%d)", pred.String(ctx), first, last), true
}

// A Z consumer may use this compact condition only for proven original 0/1
// leaves. Other exact integer pairs stay integer views and retain sink low-bit
// narrowing; no global nonzero normalization is introduced.
func canonicalIntegerDecisionCondition(v JavaValue) (JavaValue, bool) {
	if !boundedDecisionGraph(v, 8192, 256) {
		return nil, false
	}
	t, ok := UnpackSoltValue(v).(*TernaryExpression)
	if !ok || t == nil {
		return nil, false
	}
	pred, first, last, ok := integerDecisionView(t, nil)
	if !ok || (first != 0 && first != 1) || (last != 0 && last != 1) || !boundedDecisionSource(pred, 65536) {
		return nil, false
	}
	if first == last && first == 0 {
		pred = NewBinaryExpression(pred, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaBoolean)), LOGICAL_AND, types.NewJavaPrimer(types.JavaBoolean))
	}
	return pred, true
}

// Bound unique graph work separately from formula expansion, before recursive
// type/reduction helpers are entered. Shared diamonds are visited once.
func boundedDecisionGraph(v JavaValue, limit, maxDepth int) bool {
	state := map[JavaValue]uint8{}
	work, edges := 0, 0
	var visit func(JavaValue, int) bool
	visit = func(v JavaValue, depth int) bool {
		if isNilJavaValue(v) || depth > maxDepth || state[v] == 1 {
			return false
		}
		if state[v] == 2 {
			return true
		}
		work++
		if work > limit {
			return false
		}
		state[v] = 1
		if t, ok := v.(*TernaryExpression); ok && (isNilJavaValue(t.Condition) || isNilJavaValue(t.TrueValue) || isNilJavaValue(t.FalseValue)) {
			return false
		}
		children, known := Children(v)
		if known {
			for _, child := range children {
				if child == nil {
					continue
				}
				edges++
				if edges > limit*8 || !visit(child, depth+1) {
					return false
				}
			}
		}
		state[v] = 2
		return true
	}
	return visit(v, 0)
}
