package javaclassparser

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A physical source unit spells a non-static member superclass as one binary
// class name. Its fresh Signature can consequently omit the original owner's
// argument segments. Prove the complete original declaration path and compare
// the actual child arguments structurally, rather than comparing raw signature
// spelling or accepting every pair with the same erasure. No signature bound,
// wildcard, array dimension or type-variable role is inferred from a name.
func memberGenericSupertypeProjection(original, fresh string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if len(original) > 65535 || len(fresh) > 65535 || !nativeProofWork(work, int64(len(original)+len(fresh))+1) || work.CheckAlloc(int64(len(original)+len(fresh))*128) != nil {
		return false
	}
	if _, ok := types.SignatureClassReferences(original); !ok {
		return false
	}
	if _, ok := types.SignatureClassReferences(fresh); !ok {
		return false
	}
	if len(types.ClassFormalTypeParamNames(original)) != 0 || len(types.ClassFormalTypeParamNames(fresh)) != 0 {
		return false
	}
	oldSuper, oldInterfaces := types.ParseClassSignatureSupers(original)
	newSuper, newInterfaces := types.ParseClassSignatureSupers(fresh)
	left, right := append([]types.JavaType{oldSuper}, oldInterfaces...), append([]types.JavaType{newSuper}, newInterfaces...)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		// RawClassFQN is an erasure helper, not a ClassTypeSignature grammar
		// check. A T-reference shares its JavaClass representation, but may
		// never be a superclass or a superinterface in a class Signature.
		if _, variable := types.TypeVariableName(left[i]); variable {
			return false
		}
		if _, variable := types.TypeVariableName(right[i]); variable {
			return false
		}
		if _, ok := types.RawClassFQN(left[i]); !ok {
			return false
		}
		if _, ok := types.RawClassFQN(right[i]); !ok {
			return false
		}
	}
	nodes := 0
	var equal func(types.JavaType, types.JavaType, int) bool
	equal = func(a, b types.JavaType, depth int) bool {
		nodes++
		if a == nil || b == nil || depth > 64 || nodes > 4096 || !nativeProofWork(work, 1) {
			return false
		}
		// These trees were freshly parsed under the bounded grammar above.
		switch x := a.RawType().(type) {
		case *types.JavaArrayType:
			y, ok := b.RawType().(*types.JavaArrayType)
			return ok && x.Dimension == y.Dimension && equal(x.JavaType, y.JavaType, depth+1)
		case *types.JavaWildcardType:
			y, ok := b.RawType().(*types.JavaWildcardType)
			if !ok || x.Variant != y.Variant {
				return false
			}
			if x.Bound == nil || y.Bound == nil {
				return x.Bound == nil && y.Bound == nil
			}
			return equal(x.Bound, y.Bound, depth+1)
		case *types.JavaParameterizedType:
			y, ok := b.RawType().(*types.JavaParameterizedType)
			if !ok || x.RawClassName != y.RawClassName || len(x.TypeArgs) != len(y.TypeArgs) {
				return false
			}
			if len(x.OwnerSegments) == len(y.OwnerSegments) {
				for i, segment := range x.OwnerSegments {
					other := y.OwnerSegments[i]
					if segment.BinaryName != other.BinaryName || len(segment.TypeArgs) != len(other.TypeArgs) {
						return false
					}
					for j, arg := range segment.TypeArgs {
						if !equal(arg, other.TypeArgs[j], depth+1) {
							return false
						}
					}
				}
			} else {
				if len(x.OwnerSegments) < 2 || len(y.OwnerSegments) != 0 || resolve == nil {
					return false
				}
				for i, segment := range x.OwnerSegments {
					name := strings.ReplaceAll(segment.BinaryName, ".", "/")
					if !nativeProofWork(work, int64(len(name))+1) {
						return false
					}
					obj, known := resolve(name)
					if !known || obj == nil || obj.GetClassName() != name {
						return false
					}
					signature, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, work)
					if signature != "" {
						if _, grammar := types.SignatureClassReferences(signature); !grammar {
							return false
						}
					}
					if !valid || len(types.ClassFormalTypeParamNames(signature)) != len(segment.TypeArgs) {
						return false
					}
					if i > 0 {
						owner, _, flags, member := originalMemberOwner(obj)
						if !member || flags&StaticFlag != 0 || owner != strings.ReplaceAll(x.OwnerSegments[i-1].BinaryName, ".", "/") {
							return false
						}
					}
				}
			}
			for i, arg := range x.TypeArgs {
				if !equal(arg, y.TypeArgs[i], depth+1) {
					return false
				}
			}
			return true
		default:
			// Includes the parser's explicit formal-vs-class role, not only its
			// human-readable text (TT; and LT; are different identities).
			return reflect.DeepEqual(a.RawType(), b.RawType())
		}
	}
	for i, t := range left {
		if !equal(t, right[i], 0) {
			return false
		}
	}
	return true
}

// A fresh physical header may redeclare exactly the visible enclosing formals.
// Both metadata transactions use this same projection; neither authorizes an
// unrelated bound, superclass or owner argument change.
func memberScopeProjectedClassSignature(want *ClassObject, fresh string, scopes []types.LexicalTypeScope, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	original, valid := nativeMethodLocalOriginalSignature(want, want.Attributes, work)
	if !valid || len(types.ClassFormalTypeParamNames(original)) != 0 {
		return false
	}
	formals, ok := types.ProjectLexicalFormalSignature(scopes, types.ClassFormalTypeParamNames(fresh))
	if !ok {
		return false
	}
	tail := original
	if tail == "" {
		tail = "L" + want.GetSupperClassName() + ";"
		for _, index := range want.Interfaces {
			name, ok := sourceBridgeClassName(want, index)
			if !ok {
				return false
			}
			tail += "L" + name + ";"
		}
	}
	return strings.HasPrefix(fresh, formals) && memberGenericSupertypeProjection(tail, strings.TrimPrefix(fresh, formals), resolve, work)
}
