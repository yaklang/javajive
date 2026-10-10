package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A member enum regenerates assertions before its constant allocations. The
// status literal is the original outermost declaration, including static owners;
// class-name spelling or the enum's own assertion status cannot substitute it.
func nativeEnumAssertionInitialization(obj *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (*nativeMemberAssertion, bool) {
	if obj == nil {
		return nil, false
	}
	hasFlag := false
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		n, ok := sourceBridgeUTF8(obj, f.NameIndex)
		if !ok {
			return nil, false
		}
		hasFlag = hasFlag || n == nativeAssertionField
	}
	if !hasFlag {
		return nil, true
	}
	owner, known := nativeEnumAssertionStatusOwner(obj, resolve, work)
	if !known {
		return nil, false
	}
	return nativeMemberAssertionProofMode(obj, owner, work, true)
}

// Assertion status follows the entire original lexical nest, including static
// declarations. Constant bodies use their separately proved declaring enum here.
func nativeEnumAssertionStatusOwner(obj *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (string, bool) {
	if obj == nil || resolve == nil {
		return "", false
	}
	current := obj
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		n := current.GetClassName()
		if seen[n] || !nativeProofWork(work, 1) {
			return "", false
		}
		seen[n] = true
		owner, _, _, member := originalMemberOwner(current)
		if !member {
			if _, _, anonymous := originalAnonymousOwner(current); anonymous || !nativeMemberTopLevelEvidence(current, work) {
				return "", false
			}
			return n, true
		}
		parent, known := resolve(owner)
		if !known || parent == nil || parent.GetClassName() != owner {
			return "", false
		}
		current = parent
	}
	return "", false
}
