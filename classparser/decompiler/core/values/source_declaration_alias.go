package values

// Source placement can copy an ordinary named binder whose historical Val
// is deliberately unknown. This relation certifies only the target identity;
// it is not an original value definition, STORE, parameter or type proof.
type sourceDeclarationAliases struct {
	members map[*JavaRef]string
}

func (ref *JavaRef) enrollSourceDeclarationClone(clone *JavaRef) {
	if ref == nil || clone == nil || ref.originalLocalDeclaration || ref.Id == nil || ref.VarUid == "" || ref.IsParam || ref.IsThis || ref.StackVar != nil || ref.CustomValue != nil {
		return
	}
	if ref.sourceDeclarationAliases == nil {
		ref.sourceDeclarationAliases = &sourceDeclarationAliases{members: map[*JavaRef]string{ref: ref.VarUid}}
	} else if !ref.SharesSourceDeclaration(ref) {
		return // A shallow copy cannot enroll itself through the copied token.
	}
	w := ref.sourceDeclarationAliases
	if len(w.members) >= 64 {
		return
	}
	clone.sourceDeclarationAliases = w
	w.members[clone] = clone.VarUid
}

// SharesSourceDeclaration reopens an explicit source-placement copy. Strong
// original value declarations retain their separate seed-sensitive protocol;
// this target-only relation cannot replace one after its value changed.
func (ref *JavaRef) SharesSourceDeclaration(other *JavaRef) bool {
	if ref == nil || other == nil || ref.Id == nil || ref.Id != other.Id || ref.originalLocalDeclaration || other.originalLocalDeclaration || ref.sourceDeclarationAliases == nil || ref.sourceDeclarationAliases != other.sourceDeclarationAliases {
		return false
	}
	w := ref.sourceDeclarationAliases
	for _, member := range []*JavaRef{ref, other} {
		uid, known := w.members[member]
		if !known || uid == "" || uid != member.VarUid || member.IsParam || member.IsThis || member.StackVar != nil || member.CustomValue != nil {
			return false
		}
	}
	return true
}
