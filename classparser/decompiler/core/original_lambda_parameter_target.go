package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

var errFixedLambdaParameterTarget = errors.New("original lambda parameter target has no source representation")

// An explicit source SAM formal has a fixed physical implementation type.
// Preserve its original instantiated functional target independently of a
// later invariant Function<T,T>/Comparator<T> consumer. The outer raw view has
// the same original FI erasure; it neither changes entry checks nor evaluates
// a different factory. A body-source flag alone cannot establish that target.
func retainOriginalLambdaParameterTarget(d *Decompiler, value *values.CustomValue, source LambdaBodySource, raw types.JavaType, impl *values.JavaClassMember, static []values.JavaValue, captures int) (*values.CustomValue, error) {
	if !source.FixedParameterTypes {
		return value, nil
	}
	if d == nil || d.FunctionContext == nil || value == nil || impl == nil || impl.RefKind != RefInvokeStatic || captures < 0 || captures > 64 || len(static) < 3 || d.blockPartialFunctionalTarget {
		return nil, fmt.Errorf("%w: fixed formals require the original instantiated target", errFixedLambdaParameterTarget)
	}
	physical, known := directSamTokens(impl.Description, false)
	if !known || len(physical) < captures+1 {
		return nil, fmt.Errorf("%w: missing original implementation parameter words", errFixedLambdaParameterTarget)
	}
	sam := "(" + strings.Join(physical[captures:len(physical)-1], "") + ")" + physical[len(physical)-1]
	if t19MethodTypeDesc(static[2]) != sam {
		return nil, fmt.Errorf("%w: instantiated target changes original SAM parameter words", errFixedLambdaParameterTarget)
	}
	name, known := types.RawClassFQN(raw)
	if !known {
		return nil, fmt.Errorf("%w: fixed formals have no original functional-interface erasure", errFixedLambdaParameterTarget)
	}
	erasure := types.NewJavaClass(name)
	target := inferDeclaredLambdaTarget(d, erasure, static[0], static[2])
	if target == nil {
		if !fixedRawLambdaParameterTarget(d, name, sam, t19MethodTypeDesc(static[0])) {
			return nil, fmt.Errorf("%w: fixed formals have an unresolved functional declaration", errFixedLambdaParameterTarget)
		}
		target = erasure
	}
	selected, typed := types.RawClassFQN(target)
	if !typed || strings.ReplaceAll(selected, ".", "/") != strings.ReplaceAll(name, ".", "/") {
		return nil, fmt.Errorf("%w: fixed target changes the original interface erasure", errFixedLambdaParameterTarget)
	}
	constrained := retainErasedFunctionalValue(value, erasure, target)
	constrained.RequireLambdaParameterNames()
	return constrained, nil
}

// A raw target needs no invented type arguments when its declared SAM erasure
// already equals the physical implementation. Use the original complete
// interface/member table; referenced descriptor words alone are insufficient.
func fixedRawLambdaParameterTarget(d *Decompiler, owner, implementation, sam string) bool {
	if d == nil || d.FunctionContext == nil || d.FunctionContext.InvocationMetadata == nil || implementation != sam {
		return false
	}
	name := strings.ReplaceAll(owner, ".", "/")
	metadata, known := d.FunctionContext.InvocationMetadata(name)
	if !known || strings.ReplaceAll(metadata.Name, ".", "/") != name || !metadata.Public || !metadata.IsInterface || !metadata.MembersComplete || !metadata.ParentsComplete {
		return false
	}
	found := 0
	for _, method := range metadata.Methods {
		if method.Name == d.InvokeDynamicName && method.Desc == sam && method.Public && !method.Static && !method.Bridge && !strings.HasPrefix(method.Signature, "<") {
			found++
		}
	}
	return found == 1
}
