package callbinding

import "strings"

// Source lookup groups declarations by name and parameter types, not by JVM
// return descriptors. Normalize an overriding chain before the applicability
// phases: even a non-varargs override hides its ancestor's varargs declaration.
// This bounded proof handles public, non-generic instance overrides. Unrelated
// interface merges, package/protected inheritance and static hiding need their
// own proofs and are deliberately not inferred from the caller's access view.
func (q *variableArityQuery) sourceOverrideFamily(raw []variableArityCandidate, kind Kind, arity int) ([]variableArityCandidate, bool) {
	groups := map[string][]variableArityCandidate{}
	var keys []string
	for _, candidate := range raw {
		key := strings.Join(candidate.parameters, "")
		if !q.charge(int64(len(key)) + 1) {
			return nil, false
		}
		if _, found := groups[key]; !found {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], candidate)
	}
	var result []variableArityCandidate
	for _, key := range keys {
		group := groups[key]
		potential := false
		for _, candidate := range group {
			n := arity
			if kind == Static && !candidate.method.Static {
				n-- // TypeName::name also searches an unbound receiver.
			}
			potential = potential || n >= 0 && (len(candidate.parameters) == n || candidate.method.Varargs && len(candidate.parameters) > 0 && n >= len(candidate.parameters)-1)
		}
		if !potential {
			continue
		}
		for _, candidate := range group {
			m := candidate.method
			cl, known := q.provider(candidate.owner)
			if !q.charge(1) || !known || cl.Name != candidate.owner || !m.Public || m.Static != (kind == Static) || m.Generic || strings.HasPrefix(cl.Signature, "<") || kind == Static && cl.IsInterface {
				return nil, false
			}
			if m.Varargs && (len(candidate.parameters) == 0 || !strings.HasPrefix(candidate.parameters[len(candidate.parameters)-1], "[")) {
				return nil, false
			}
		}
		var ordinary []variableArityCandidate
		for _, candidate := range group {
			if !candidate.method.Bridge {
				ordinary = append(ordinary, candidate)
			}
		}
		for _, bridge := range group {
			if !bridge.method.Bridge {
				continue
			}
			if kind == Static || !bridge.method.AccessFlagsKnown || bridge.method.AccessFlags != 0x1041 || bridge.method.CovariantBridgeTarget == "" {
				return nil, false
			}
			var target *variableArityCandidate
			for i := range ordinary {
				c := &ordinary[i]
				if !q.charge(1) {
					return nil, false
				}
				if c.owner == bridge.owner && c.method.Desc == bridge.method.CovariantBridgeTarget {
					if target != nil {
						return nil, false
					}
					target = c
				}
			}
			if target == nil || !q.covariantResult(*target, bridge) {
				return nil, false
			}
			// A forwarding flag alone does not establish a compiler override
			// bridge. Reopen the erased ancestor declaration it implements.
			ancestor := false
			for _, parent := range ordinary {
				if !q.charge(1) {
					return nil, false
				}
				if parent.method.Desc == bridge.method.Desc && q.overrides(*target, parent) {
					ancestor = true
				}
			}
			if !ancestor {
				return nil, false
			}
		}
		winner := -1
		for i, candidate := range ordinary {
			dominates := true
			for j, other := range ordinary {
				if i != j && !q.overrides(candidate, other) {
					dominates = false
				}
			}
			if dominates {
				if winner >= 0 {
					return nil, false
				}
				winner = i
			}
		}
		if winner < 0 || !q.charge(1) {
			return nil, false
		}
		result = append(result, ordinary[winner])
	}
	return result, true
}

func (q *variableArityQuery) overrides(child, parent variableArityCandidate) bool {
	if !q.charge(1) || child.owner == parent.owner || child.method.Static || parent.method.Static || !child.method.AccessFlagsKnown || !parent.method.AccessFlagsKnown {
		return false
	}
	// ACC_PUBLIC must be original evidence; Public can mean package access.
	// A final parent cannot be overridden. Private/static flags cannot acquire
	// override permission through an inconsistent source accessibility view.
	if child.method.AccessFlags&0xf != 1 || parent.method.AccessFlags&0x1f != 1 {
		return false
	}
	ancestry, known := q.subtype("L"+child.owner+";", "L"+parent.owner+";")
	return known && ancestry && q.covariantResult(child, parent)
}

func (q *variableArityQuery) covariantResult(child, parent variableArityCandidate) bool {
	_, a, errA := Descriptor(child.method.Desc)
	_, b, errB := Descriptor(parent.method.Desc)
	if errA != nil || errB != nil || !q.charge(1) {
		return false
	}
	if a == b {
		return true
	}
	// Numeric widening is an invocation conversion, not return-type
	// substitutability for an override (JLS 8.4.5 and 8.4.8.3).
	if !Reference(a) || !Reference(b) {
		return false
	}
	value, known := q.subtype(a, b)
	return known && value
}
