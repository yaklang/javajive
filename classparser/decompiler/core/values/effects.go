package values

// Effects describes observable operations, not a claim that an expression can
// be moved. Unknown expressions are opaque and conservatively non-movable.
// New bits are appended so existing iota values stay stable.
type Effects uint16

const (
	EffectReadMemory Effects = 1 << iota
	EffectWriteMemory
	EffectCall
	EffectAllocate
	EffectThrow
	EffectSynchronize
	EffectOpaque
	EffectClassInit
	EffectVolatile
)

// EffectMonitor is the monitor acquire/release bit. EffectSynchronize is the
// historical name; both refer to the same bit.
const EffectMonitor = EffectSynchronize

// BarrierEffects cannot be judged pure, copied, or deleted.
const BarrierEffects = EffectOpaque | EffectClassInit | EffectVolatile | EffectMonitor | EffectThrow | EffectCall | EffectWriteMemory | EffectAllocate

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
	case *JavaCompare:
		return []JavaValue{v.JavaValue1, v.JavaValue2}, true
	case *JavaArray:
		return []JavaValue{v.Length}, true
	case *EffectTag:
		if v == nil || v.Inner == nil {
			return nil, true
		}
		return []JavaValue{v.Inner}, true
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
		case *JavaArrayMember:
			effect |= EffectReadMemory | EffectThrow
		case *RefMember:
			effect |= EffectReadMemory | EffectThrow
		case *JavaClassMember:
			// Static field access is getstatic/putstatic: it may run <clinit>
			// and cannot be moved across class-init boundaries.
			effect |= EffectReadMemory | EffectThrow | EffectClassInit
		case *FunctionCallExpression:
			effect |= EffectCall | EffectReadMemory | EffectWriteMemory | EffectThrow
			if v != nil && (v.FunctionName == "<clinit>" || v.FunctionName == "<init>") {
				effect |= EffectClassInit
			}
		case *NewExpression:
			effect |= EffectAllocate | EffectThrow | EffectClassInit
		case *CastExpression:
			effect |= EffectThrow
		case *AssignmentExpression:
			effect |= EffectWriteMemory
		case *EffectTag:
			if v != nil {
				effect |= v.Extra
			}
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

// MayFold reports whether value may participate in an enabled fold. Opaque
// CustomValue, class-init, volatile, monitor, and unproven throws are barriers.
// Text similarity is never treated as purity.
func MayFold(value JavaValue) bool {
	if value == nil {
		return true
	}
	effect, _ := InspectValue(value)
	return mayFoldEffects(effect)
}

func mayFoldEffects(effect Effects) bool {
	if effect&EffectOpaque != 0 {
		return false
	}
	if effect&(EffectThrow|EffectMonitor|EffectVolatile|EffectClassInit) != 0 {
		return false
	}
	if effect&(EffectCall|EffectWriteMemory|EffectAllocate) != 0 {
		return false
	}
	return true
}

// IsPure reports whether InspectValue found no observable effects. Unknown and
// opaque values are never pure.
func IsPure(value JavaValue) bool {
	if value == nil {
		return true
	}
	effect, _ := InspectValue(value)
	return effect == 0
}

// EffectSummary names the conservative effect classes of value.
func EffectSummary(value JavaValue) string {
	effect, _ := InspectValue(value)
	return summarizeEffects(effect)
}

func summarizeEffects(effect Effects) string {
	if effect&EffectOpaque != 0 {
		return "unknown"
	}
	if effect == 0 {
		return "pure"
	}
	var parts []string
	if effect&EffectReadMemory != 0 {
		parts = append(parts, "read")
	}
	if effect&EffectWriteMemory != 0 {
		parts = append(parts, "write")
	}
	if effect&EffectCall != 0 {
		parts = append(parts, "call")
	}
	if effect&EffectAllocate != 0 {
		parts = append(parts, "allocate")
	}
	if effect&EffectThrow != 0 {
		parts = append(parts, "throw")
	}
	if effect&EffectClassInit != 0 {
		parts = append(parts, "init")
	}
	if effect&EffectMonitor != 0 {
		parts = append(parts, "monitor")
	}
	if effect&EffectVolatile != 0 {
		parts = append(parts, "volatile")
	}
	if len(parts) == 0 {
		return "unknown"
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "+" + parts[i]
	}
	return out
}
