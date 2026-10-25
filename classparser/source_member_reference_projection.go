package javaclassparser

import (
	"reflect"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A physical member with no own formals can redeclare its nearest enclosing
// formals; an own-formal member retains its own ordered arguments. Prove the
// actual source declaration and each original ownership edge before comparing
// the relocated arguments; equal erasure alone never establishes this relation.
func memberReferenceSignatureProjection(original, fresh, descriptor string, scopes []types.LexicalTypeScope, projected func(string) ([]string, bool), resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if projected == nil || resolve == nil || len(original)+len(fresh) > 131070 || !nativeProofWork(work, int64(len(original)+len(fresh))+1) || work.CheckAlloc(int64(len(original)+len(fresh))*128) != nil {
		return false
	}
	oldErased, _, oldOK := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, original)
	newErased, _, newOK := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, fresh)
	if !oldOK || !newOK || oldErased != descriptor || newErased != descriptor {
		return false
	}
	left, right := strings.IndexByte(original, '('), strings.IndexByte(fresh, '(')
	if left < 0 || right < 0 || original[:left] != fresh[:right] {
		return false // Method binder declarations and bound order remain exact.
	}
	oldArgs, oldResult, oldThrows := types.ParseMethodSignatureWithThrows(original[left:])
	newArgs, newResult, newThrows := types.ParseMethodSignatureWithThrows(fresh[right:])
	if len(oldArgs) != len(newArgs) || len(oldThrows) != len(newThrows) {
		return false
	}
	oldTypes := append(append(oldArgs, oldResult), oldThrows...)
	newTypes := append(append(newArgs, newResult), newThrows...)
	nodes, changes := 0, 0
	var equal func(types.JavaType, types.JavaType, int) bool
	equal = func(a, b types.JavaType, depth int) bool {
		nodes++
		if a == nil || b == nil || depth > 64 || nodes > 4096 || !nativeProofWork(work, 1) {
			return false
		}
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
			if !ok || x.RawClassName != y.RawClassName {
				return false
			}
			args := x.TypeArgs
			if len(x.OwnerSegments) != len(y.OwnerSegments) {
				// TypeArgs also supports legacy flattened consumers and can retain
				// the nearest owner's arguments. Only the original final segment
				// describes this member's own declaration arguments.
				if len(x.OwnerSegments) < 2 || len(y.OwnerSegments) != 0 {
					return false
				}
				names, known := projected(strings.ReplaceAll(x.RawClassName, ".", "/"))
				if !known || len(names) == 0 || len(names) > 128 {
					return false
				}
				var nearest []string
				for i, segment := range x.OwnerSegments {
					binary := strings.ReplaceAll(segment.BinaryName, ".", "/")
					obj, known := resolve(binary)
					if !known || obj == nil || obj.GetClassName() != binary || !nativeProofWork(work, 1) {
						return false
					}
					sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, work)
					formals := types.ClassFormalTypeParamNames(sig)
					if !valid || len(formals) != len(segment.TypeArgs) {
						return false
					}
					if sig != "" {
						if _, grammar := types.SignatureClassReferences(sig); !grammar {
							return false
						}
					}
					if i > 0 {
						owner, _, flags, member := originalMemberOwner(obj)
						if !member || flags&StaticFlag != 0 || owner != strings.ReplaceAll(x.OwnerSegments[i-1].BinaryName, ".", "/") {
							return false
						}
					}
					if len(formals) > 0 {
						nearest, args = formals, segment.TypeArgs
					}
				}
				if !slices.Equal(nearest, names) {
					return false
				}
				changes++
			} else {
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
			}
			if len(args) != len(y.TypeArgs) {
				return false
			}
			for i, arg := range args {
				if !equal(arg, y.TypeArgs[i], depth+1) {
					return false
				}
			}
			return true
		default:
			return reflect.DeepEqual(a.RawType(), b.RawType())
		}
	}
	for i, old := range oldTypes {
		if !equal(old, newTypes[i], 0) {
			return false
		}
	}
	return changes > 0
}
