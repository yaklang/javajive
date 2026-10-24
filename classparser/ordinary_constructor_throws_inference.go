package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This is the throws-only subset of JLS 18.4/18.5.1. A method inference
// variable must occur exclusively in throws, with proper upper bounds among
// Object, Exception and Throwable. Consequently argument rendering (including
// descriptor casts) cannot constrain it. Argument-dependent inference belongs
// to a different source-binding proof, and is deliberately not approximated.
// A source throws formal itself must have a Throwable upper bound. This first
// domain excludes multiple/intersection and dependent bounds entirely.
// https://docs.oracle.com/javase/specs/jls/se8/html/jls-18.html#jls-18.4
func ordinaryConstructorThrowsOnly(provider callbinding.Provider, owner, descriptor string, scopes []string, work *workbudget.Budget) ([]string, bool) {
	if provider == nil || len(owner)+len(descriptor) > 8192 || len(scopes) > 32 || !nativeProofWork(work, 1) {
		return nil, false
	}
	cl, known := provider(owner)
	if !known || cl.Name != owner || !cl.MembersComplete || !cl.ParentsComplete || cl.IsInterface || len(cl.Methods) > 256 {
		return nil, false
	}
	var target callbinding.Method
	constructors := 0
	for _, m := range cl.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if m.Name == "<init>" {
			constructors++
			if m.Desc == descriptor {
				target = m
			}
		}
	}
	// This first domain needs no inference-sensitive overload exclusion. The
	// complete source member table (after an independently proved native port
	// projection, if any) contains exactly one constructor.
	if constructors != 1 || target.Desc != descriptor || !target.Generic || target.Signature == "" || target.Static || target.Bridge || target.Varargs || !target.ExceptionsKnown || len(target.Exceptions) > 32 || len(target.Signature) > 8192 || !nativeProofWork(work, int64(len(target.Signature))+1) {
		return nil, false
	}
	if work != nil && work.CheckAlloc(int64(len(target.Signature))*16+8192) != nil {
		return nil, false
	}
	erased, rawThrows, known := types.EraseLexicalOwnerMethodSignatureWithThrows(scopes, target.Signature)
	if !known || erased != descriptor || len(rawThrows) != len(target.Exceptions) {
		return nil, false
	}
	for i, raw := range rawThrows {
		if !nativeProofWork(work, 1) || raw != "L"+target.Exceptions[i]+";" {
			return nil, false
		}
	}
	bounds, _, known := types.FormalBoundConstraints(target.Signature)
	if !known || len(bounds) == 0 || len(bounds) > 32 {
		return nil, false
	}
	_, parameters, result, thrown := types.ParseMethodSignatureFullWithThrows(target.Signature, &class_context.ClassContext{})
	physical, ret, err := callbinding.Descriptor(descriptor)
	if err != nil || ret != "V" || result == nil || len(parameters) != len(physical) || len(thrown) != len(target.Exceptions) {
		return nil, false
	}
	remaining := 256
	var independent func(types.JavaType) bool
	independent = func(t types.JavaType) bool {
		remaining--
		if remaining < 0 || t == nil || !nativeProofWork(work, 1) {
			return false
		}
		if name, formal := types.TypeVariableName(t); formal {
			return bounds[name] == nil
		}
		if wildcard, ok := t.(*types.JavaWildcardType); ok {
			return wildcard.Bound == nil || independent(wildcard.Bound)
		}
		if t.IsArray() {
			return independent(t.ElementType())
		}
		if p, ok := types.AsParameterizedType(t); ok {
			for _, segment := range p.OwnerSegments {
				for _, arg := range segment.TypeArgs {
					if !independent(arg) {
						return false
					}
				}
			}
			for _, arg := range p.TypeArgs {
				if !independent(arg) {
					return false
				}
			}
		}
		return true
	}
	for _, parameter := range parameters {
		if !independent(parameter) {
			return nil, false
		}
	}
	if !independent(result) {
		return nil, false
	}
	used := map[string]bool{}
	for _, t := range thrown {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if name, formal := types.TypeVariableName(t); formal {
			if bounds[name] == nil {
				return nil, false // class formals are not method inference variables
			}
			used[name] = true
		}
	}
	for name, upper := range bounds {
		if !used[name] || len(upper) != 1 {
			return nil, false
		}
		for _, t := range upper {
			if !nativeProofWork(work, 1) || !constructorInvocationThrowsSimpleType(t, false) {
				return nil, false
			}
			raw, known := types.RawClassFQN(t)
			if !known || raw != "java.lang.Exception" && raw != "java.lang.Throwable" {
				return nil, false
			}
		}
	}
	q := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if !nativeProofWork(work, 1) {
			return callbinding.Class{}, false
		}
		return provider(name)
	})
	var exceptions []string
	for _, t := range thrown {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		name := "java/lang/RuntimeException"
		if _, formal := types.TypeVariableName(t); !formal {
			raw, known := types.RawClassFQN(t)
			if !known || !constructorInvocationThrowsSimpleType(t, false) {
				return nil, false
			}
			name = strings.ReplaceAll(raw, ".", "/")
		}
		if !q.assignable("L"+name+";", "Ljava/lang/Throwable;") {
			return nil, false
		}
		exceptions = append(exceptions, name)
	}
	return slices.Clone(exceptions), !q.exhausted
}
