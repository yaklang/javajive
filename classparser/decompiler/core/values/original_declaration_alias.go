package values

// Source naming can copy the actual declaration without introducing a new
// definition. Explicit enrollment preserves that identity; a shallow struct
// copy or matching public ID cannot manufacture membership in this cohort.
type originalDeclarationAliases struct {
	members map[*JavaRef]string
}

func (ref *JavaRef) CloneForSourceDeclaration() *JavaRef {
	if ref == nil {
		return nil
	}
	copy := *ref
	ref.enrollSourceDeclarationClone(&copy)
	if _, _, known := ref.OriginalLocalDeclaration(ref.Val); known && ref.originalDeclarationAliases != nil && len(ref.originalDeclarationAliases.members) < 64 {
		if uid, member := ref.originalDeclarationAliases.members[ref]; member && uid == ref.VarUid {
			ref.originalDeclarationAliases.members[&copy] = copy.VarUid
		}
	}
	return &copy
}

func (ref *JavaRef) SameOriginalLocalDeclaration(other *JavaRef) bool {
	if ref == nil || other == nil || ref.Id == nil || ref.Id != other.Id || ref.originalDeclarationAliases == nil || ref.originalDeclarationAliases != other.originalDeclarationAliases {
		return false
	}
	pc, slot, known := ref.OriginalLocalDeclaration(ref.Val)
	otherPC, otherSlot, otherKnown := other.OriginalLocalDeclaration(ref.Val)
	if !known || !otherKnown || pc != otherPC || slot != otherSlot {
		return false
	}
	for _, r := range []*JavaRef{ref, other} {
		uid, member := ref.originalDeclarationAliases.members[r]
		if !member || uid != r.VarUid {
			return false
		}
	}
	return true
}
