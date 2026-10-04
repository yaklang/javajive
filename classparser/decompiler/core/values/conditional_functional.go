package values

// StandaloneFunctionalArms gives each lambda/method-reference arm its own
// instantiated target before a surrounding erased conversion is rendered.
// A cast around an entire conditional is NOT a target-typing context for its
// arms in Java. Casting the individual poly expressions preserves their SAM
// contracts and leaves condition order and chosen-arm evaluation unchanged.
// Work on copies: the same IR may also render in a proper assignment context.
func StandaloneFunctionalArms(value JavaValue) JavaValue {
	memo := map[*TernaryExpression]JavaValue{}
	var visit func(JavaValue, int) JavaValue
	visit = func(v JavaValue, depth int) JavaValue {
		if v == nil || depth > 64 {
			return v
		}
		switch x := UnpackSoltValue(v).(type) {
		case *CustomValue:
			if x != nil && x.Flag == "lambda" && x.Type() != nil {
				return &CastExpression{Value: x, TargetType: x.Type(), Binding: true}
			}
		case *TernaryExpression:
			if prior, ok := memo[x]; ok {
				return prior
			}
			memo[x] = v
			yes, no := visit(x.TrueValue, depth+1), visit(x.FalseValue, depth+1)
			if yes != x.TrueValue || no != x.FalseValue {
				result := NewTernaryExpression(x.Condition, yes, no)
				memo[x] = result
				return result
			}
		}
		return v
	}
	return visit(value, 0)
}
