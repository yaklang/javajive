package values

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

// NumericConditionalType implements primitive numeric conditional promotion.
// Unlike slot unification, this source expression operation never aliases or
// mutates either operand's type. Boolean and reference conditionals retain their
// separate inference rules. Boxed numeric types are deliberately not certified.
func NumericConditionalType(a, b JavaValue) (types.JavaType, bool) {
	graph := conditionalNumericGraph{memo: make(map[*TernaryExpression]string), active: make(map[*TernaryExpression]bool), remaining: 4096}
	ka, kb := graph.kind(a, 0), graph.kind(b, 0)
	if ka == "" || kb == "" {
		return nil, false
	}
	return types.NewJavaPrimer(conditionalNumericPromotion(ka, kb, a, b)), true
}

func conditionalNumericPromotion(ka, kb string, a, b JavaValue) string {
	kind := types.JavaInteger
	switch {
	case ka == kb:
		kind = ka
	case ka == types.JavaByte && kb == types.JavaShort || ka == types.JavaShort && kb == types.JavaByte:
		kind = types.JavaShort
	case kb == types.JavaInteger && conditionalIntFits(b, ka):
		kind = ka
	case ka == types.JavaInteger && conditionalIntFits(a, kb):
		kind = kb
	default:
		for _, wide := range []string{types.JavaDouble, types.JavaFloat, types.JavaLong} {
			if ka == wide || kb == wide {
				kind = wide
				break
			}
		}
	}
	return kind
}

type conditionalNumericGraph struct {
	memo      map[*TernaryExpression]string
	active    map[*TernaryExpression]bool
	remaining int
}

func (g *conditionalNumericGraph) kind(v JavaValue, depth int) string {
	if depth > 64 || g.remaining <= 0 {
		return ""
	}
	g.remaining--
	if isNilJavaValue(v) {
		return ""
	}
	// Slot cycles can occur during incomplete phi construction. Do not recurse
	// through them, or through mutable JavaRef producer links, for a type proof.
	for depth := 0; depth < 32; depth++ {
		slot, ok := v.(*SlotValue)
		if !ok {
			break
		}
		if slot == nil {
			return ""
		}
		if isNilJavaValue(slot.GetValue()) {
			if slot.TmpType == nil {
				return ""
			}
			p, ok := slot.TmpType.RawType().(*types.JavaPrimer)
			if !ok || p == nil {
				return ""
			}
			return conditionalPrimitiveKind(p.Name)
		}
		v = slot.GetValue()
		if depth == 31 {
			return ""
		}
	}
	if ternary, ok := v.(*TernaryExpression); ok {
		if ternary == nil || g.active[ternary] {
			return ""
		}
		if kind, seen := g.memo[ternary]; seen {
			return kind
		}
		g.active[ternary] = true
		ka, kb := g.kind(ternary.TrueValue, depth+1), g.kind(ternary.FalseValue, depth+1)
		delete(g.active, ternary)
		kind := ""
		if ka != "" && kb != "" {
			kind = conditionalNumericPromotion(ka, kb, ternary.TrueValue, ternary.FalseValue)
		}
		g.memo[ternary] = kind
		return kind
	}
	t := v.Type()
	if t == nil {
		return ""
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	if !ok || p == nil {
		return ""
	}
	return conditionalPrimitiveKind(p.Name)
}
func conditionalPrimitiveKind(kind string) string {
	switch kind {
	case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
		return kind
	}
	return ""
}
func conditionalIntFits(v JavaValue, kind string) bool {
	var low, high int64
	switch kind {
	case types.JavaByte:
		low, high = -128, 127
	case types.JavaShort:
		low, high = -32768, 32767
	case types.JavaChar:
		low, high = 0, 65535
	default:
		return false
	}
	for depth := 0; depth < 32; depth++ {
		slot, ok := v.(*SlotValue)
		if !ok {
			break
		}
		if slot == nil {
			return false
		}
		v = slot.GetValue()
		if depth == 31 {
			return false
		}
	}
	literal, ok := v.(*JavaLiteral)
	if !ok || literal == nil {
		return false
	}
	var word int64
	switch n := literal.Data.(type) {
	case int:
		word = int64(n)
	case int32:
		word = int64(n)
	default:
		return false
	}
	return word >= low && word <= high
}
