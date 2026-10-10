package values

// Distinct simulator refs can represent the same complete original definition
// web. Equal public IDs/UIDs alone cannot certify that relation. The web solver
// seals its actual members before STORE statements are constructed.
type originalLocalWeb struct {
	members map[*JavaRef]string
}

func MarkOriginalLocalWeb(refs []*JavaRef) {
	if len(refs) < 2 || len(refs) > 64 {
		return
	}
	first := refs[0]
	if first == nil || first.Id == nil || first.VarUid == "" {
		return
	}
	// Type propagation can revisit this already solved cohort. Retain its
	// original token instead of allocating a new map on every solver round.
	if first.originalLocalWeb != nil && len(first.originalLocalWeb.members) == len(refs) {
		retained := true
		seen := map[*JavaRef]bool{}
		for _, ref := range refs {
			retained = retained && !seen[ref] && first.SameOriginalLocalWeb(ref)
			seen[ref] = true
		}
		if retained {
			return
		}
	}
	w := &originalLocalWeb{members: map[*JavaRef]string{}}
	for _, ref := range refs {
		if ref == nil || ref.Id != first.Id || ref.VarUid != first.VarUid || ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil {
			return
		}
		if _, duplicate := w.members[ref]; duplicate {
			return
		}
		w.members[ref] = ref.VarUid
	}
	for _, ref := range refs {
		ref.originalLocalWeb = w
	}
}

func (ref *JavaRef) SameOriginalLocalWeb(other *JavaRef) bool {
	if ref == nil || other == nil || ref.Id == nil || ref.Id != other.Id || ref.originalLocalWeb == nil || ref.originalLocalWeb != other.originalLocalWeb {
		return false
	}
	w := ref.originalLocalWeb
	for _, r := range []*JavaRef{ref, other} {
		uid, member := w.members[r]
		if !member || uid == "" || uid != r.VarUid || r.IsParam || r.IsThis || r.StackVar != nil || r.CustomValue != nil {
			return false
		}
	}
	return true
}
