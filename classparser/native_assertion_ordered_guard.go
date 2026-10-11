package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Structuring can merge an enclosing conditional with the compiler's assertion
// guard. Split only ordered short-circuit AND at the original flag occurrence:
// prefix && !disabled && failure becomes if(prefix) { assert !failure; }.
// Prefix remains evaluated in disabled mode, and neither suffix nor message
// moves before the flag. OR, eager connectives and casts are indivisible.
// This is only an operand partition; the caller must still close all original
// reads, allocation/call/throw identities and the common false continuation.
func nativeAssertionOrderedGuard(v values.JavaValue, owner string, work *workbudget.Budget, depth int) (*values.JavaClassMember, values.JavaValue, values.JavaValue, bool) {
	remaining := 8192
	active := map[*values.JavaExpression]bool{}
	invalid := false
	var split func(values.JavaValue, int) (*values.JavaClassMember, values.JavaValue, values.JavaValue, bool)
	split = func(v values.JavaValue, depth int) (*values.JavaClassMember, values.JavaValue, values.JavaValue, bool) {
		remaining--
		if depth <= 0 || remaining < 0 || sourceProofNil(v) || !nativeProofWork(work, 1) {
			invalid = true
			return nil, nil, nil, false
		}
		raw, ok := nativeMemberEnclosingUnpack(v, work)
		if !ok {
			invalid = true
			return nil, nil, nil, false
		}
		exp, ok := raw.(*values.JavaExpression)
		if !ok {
			return nil, nil, nil, false
		}
		if active[exp] {
			invalid = true
			return nil, nil, nil, false
		}
		active[exp] = true
		defer delete(active, exp)
		if exp.Op == values.Not {
			field, failure, ok := nativeAssertionFailureGuard(raw, owner, work, 1)
			return field, nil, failure, ok
		}
		if exp.Op != values.LOGICAL_AND || len(exp.Values) != 2 {
			return nil, nil, nil, false
		}
		if sourceProofNil(exp.Values[0]) || sourceProofNil(exp.Values[1]) {
			invalid = true
			return nil, nil, nil, false
		}
		boolean := types.NewJavaPrimer(types.JavaBoolean)
		field, prefix, failure, found := split(exp.Values[0], depth-1)
		if invalid {
			return nil, nil, nil, false
		}
		if found {
			return field, prefix, values.NewBinaryExpression(failure, exp.Values[1], values.LOGICAL_AND, boolean), true
		}
		field, prefix, failure, found = split(exp.Values[1], depth-1)
		if invalid || !found {
			return nil, nil, nil, false
		}
		if prefix == nil {
			prefix = exp.Values[0]
		} else {
			prefix = values.NewBinaryExpression(exp.Values[0], prefix, values.LOGICAL_AND, boolean)
		}
		return field, prefix, failure, true
	}
	field, prefix, failure, found := split(v, depth)
	return field, prefix, failure, found && !invalid
}
