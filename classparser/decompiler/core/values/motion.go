package values

// Access separates mutable local dependencies from observable heap/call effects.
// A local read may be pure while still being illegal to move across its write.
type Access struct {
	Effects       Effects
	Reads, Writes map[*JavaRef]bool
	Handlers      []int
}

func SameLocal(a, b *JavaRef) bool {
	return a != nil && b != nil && (a == b || a.VarUid != "" && a.VarUid == b.VarUid)
}
func InspectAccess(value JavaValue) Access {
	effects, reads := InspectValue(value)
	out := Access{Effects: effects, Reads: reads, Writes: map[*JavaRef]bool{}}
	seen := map[JavaValue]bool{}
	heapWrite := false
	var visit func(JavaValue)
	visit = func(v JavaValue) {
		if v == nil || seen[v] {
			return
		}
		seen[v] = true
		var target JavaValue
		switch x := v.(type) {
		case *AssignmentExpression:
			target = x.Target
		case *JavaExpression:
			switch x.Op {
			case "++", "--", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>=":
				if len(x.Values) > 0 {
					target = x.Values[0]
				}
			}
		case *FunctionCallExpression:
			heapWrite = true
		case *EffectTag:
			heapWrite = heapWrite || x.Extra&EffectWriteMemory != 0
		}
		if target != nil {
			if ref, ok := UnpackSoltValue(target).(*JavaRef); ok && ref != nil {
				out.Writes[ref] = true
			} else {
				heapWrite = true
			}
		}
		children, known := Children(v)
		if !known {
			heapWrite = true
		}
		for _, c := range children {
			visit(c)
		}
	}
	visit(value)
	if len(out.Writes) > 0 && !heapWrite {
		out.Effects &^= EffectWriteMemory
	}
	return out
}
func accessOverlap(a, b map[*JavaRef]bool) bool {
	for x := range a {
		for y := range b {
			if SameLocal(x, y) {
				return true
			}
		}
	}
	return false
}

// CanSwap is a sufficient conservative proof: effects and handler boundaries
// keep their original position; local RAW, WAR and WAW dependencies also order.
func CanSwap(a, b Access) bool {
	if a.Effects != 0 || b.Effects != 0 || len(a.Handlers) != len(b.Handlers) {
		return false
	}
	for i := range a.Handlers {
		if a.Handlers[i] != b.Handlers[i] {
			return false
		}
	}
	return !accessOverlap(a.Writes, b.Reads) && !accessOverlap(b.Writes, a.Reads) && !accessOverlap(a.Writes, b.Writes)
}

// CanMoveAcrossPrefix allows an operation to be nested into an adjacent use
// when the preceding operand evaluations have no effects or conflicting local
// access. The enclosing call/store still occurs after the operation.
func CanMoveAcrossPrefix(operation, prefix Access) bool {
	if prefix.Effects != 0 || len(prefix.Writes) != 0 || len(operation.Handlers) != len(prefix.Handlers) {
		return false
	}
	for i := range operation.Handlers {
		if operation.Handlers[i] != prefix.Handlers[i] {
			return false
		}
	}
	return !accessOverlap(operation.Writes, prefix.Reads)
}
