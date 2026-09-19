package values

// Effects describes observable operations, not a claim that an expression can
// be moved. Unknown expressions are opaque and conservatively non-movable.
type Effects uint16

const (
	EffectReadMemory Effects = 1 << iota
	EffectWriteMemory
	EffectCall
	EffectAllocate
	EffectThrow
	EffectSynchronize
	EffectOpaque
)

// Children exposes value dependencies without following a JavaRef back into its
// defining value (which would confuse a use with a definition or create cycles).
func Children(value JavaValue) ([]JavaValue, bool) {
	switch v := value.(type) {
	case *SlotValue:
		if v == nil {
			return nil, true
		}
		return []JavaValue{v.GetValue()}, true
	case *JavaRef, *JavaLiteral, *JavaClassValue, *JavaClassMember, javaNull:
		return nil, true
	case *JavaExpression:
		return v.Values, true
	case *JavaArrayMember:
		return []JavaValue{v.Object, v.Index}, true
	case *RefMember:
		return []JavaValue{v.Object}, true
	case *TernaryExpression:
		return []JavaValue{v.Condition, v.TrueValue, v.FalseValue}, true
	case *FunctionCallExpression:
		return append([]JavaValue{v.Object}, v.Arguments...), true
	case *NewExpression:
		out := append([]JavaValue{}, v.Length...)
		out = append(out, v.Initializer...)
		if v.ConstructorCall != nil {
			out = append(out, v.ConstructorCall.Arguments...)
		}
		return out, v.ArgumentsGetter == nil || v.ConstructorCall != nil
	case *CastExpression:
		return []JavaValue{v.Value}, true
	case *AssignmentExpression:
		return []JavaValue{v.Target, v.Value}, true
	case *CustomValue:
		if v.CapturesKnown && (v.Flag == "lambda" || v.Flag == "primitive_cast") {
			return v.Captures, true
		}
		return nil, false
	default:
		return nil, false
	}
}

// InspectValue walks all explicit dependencies once. RefUses are keyed by stable
// JavaRef identity, not local names or printed types.
func InspectValue(value JavaValue) (effect Effects, refs map[*JavaRef]bool) {
	refs = map[*JavaRef]bool{}
	seen := map[JavaValue]bool{}
	var visit func(JavaValue)
	visit = func(value JavaValue) {
		if value == nil || seen[value] {
			return
		}
		seen[value] = true
		switch v := value.(type) {
		case *JavaRef:
			if v != nil {
				refs[v] = true
			}
		case *JavaExpression:
			switch v.Op {
			case "++", "--", "=", "+=", "-=":
				effect |= EffectWriteMemory
			case "/", "%":
				effect |= EffectThrow
			}
		case *JavaArrayMember, *RefMember, *JavaClassMember:
			effect |= EffectReadMemory | EffectThrow
		case *FunctionCallExpression:
			effect |= EffectCall | EffectReadMemory | EffectWriteMemory | EffectThrow
		case *NewExpression:
			effect |= EffectAllocate | EffectThrow
		case *CastExpression:
			effect |= EffectThrow
		case *AssignmentExpression:
			effect |= EffectWriteMemory
		case *CustomValue:
			if v.CapturesKnown && v.Flag == "lambda" {
				effect |= EffectAllocate | EffectThrow
			}
		}
		children, known := Children(value)
		if !known {
			effect |= EffectOpaque
		}
		for _, child := range children {
			visit(child)
		}
	}
	visit(value)
	return
}
