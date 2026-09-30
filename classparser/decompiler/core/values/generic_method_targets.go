package values

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// bindMethodTypeArguments solves only structurally matching generic witnesses.
// It does not guess subtyping or flatten nested invariant wildcards. The method
// variables are a separate namespace from the caller's variables; repeated
// occurrences must agree. Equal wildcard bounds are useful witnesses for
// Function<? super T,? extends R> wrappers without capturing caller wildcards
// inside a second wildcard.
func bindMethodTypeArguments(pattern, actual types.JavaType, variables map[string]bool, bindings map[string]types.JavaType) bool {
	if pattern == nil || actual == nil {
		return false
	}
	if w, ok := pattern.(*types.JavaWildcardType); ok {
		if w == nil || w.Bound == nil {
			return false
		}
		if a, ok := actual.(*types.JavaWildcardType); ok {
			if a == nil || a.Variant != w.Variant || a.Bound == nil {
				return false
			}
			actual = a.Bound
		}
		return bindMethodTypeArguments(w.Bound, actual, variables, bindings)
	}
	if p, ok := pattern.RawType().(*types.JavaClass); ok && p != nil && variables[p.Name] {
		if _, wildcard := actual.(*types.JavaWildcardType); wildcard {
			return false
		}
		if old := bindings[p.Name]; old != nil {
			return reflect.DeepEqual(old.RawType(), actual.RawType())
		}
		bindings[p.Name] = actual.Copy()
		return true
	}
	if p, ok := types.AsParameterizedType(pattern); ok {
		a, ok := types.AsParameterizedType(actual)
		if !ok || !sameErasureClassName(p.RawClassName, a.RawClassName) || len(p.TypeArgs) != len(a.TypeArgs) {
			return false
		}
		for i := range p.TypeArgs {
			if !bindMethodTypeArguments(p.TypeArgs[i], a.TypeArgs[i], variables, bindings) {
				return false
			}
		}
		return true
	}
	if pattern.IsArray() && actual.IsArray() {
		return bindMethodTypeArguments(pattern.ElementType(), actual.ElementType(), variables, bindings)
	}
	return reflect.DeepEqual(pattern.RawType(), actual.RawType())
}

func (f *FunctionCallExpression) genericMethodSignature(ctx *class_context.ClassContext) ([]types.JavaType, types.JavaType, []string) {
	if f == nil || ctx == nil || f.Descriptor == "" {
		return nil, nil, nil
	}
	// The stable JDK factory signatures are absent from a jar's sibling index.
	// Exact descriptors distinguish the optional executor overload.
	if f.IsStatic && sameErasureClassName(f.ClassName, "java.util.concurrent.CompletableFuture") && f.FunctionName == "supplyAsync" &&
		(f.Descriptor == "(Ljava/util/function/Supplier;)Ljava/util/concurrent/CompletableFuture;" ||
			f.Descriptor == "(Ljava/util/function/Supplier;Ljava/util/concurrent/Executor;)Ljava/util/concurrent/CompletableFuture;") {
		v := types.NewJavaClass("$factoryResult")
		params := []types.JavaType{types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{v})}
		if len(f.Arguments) == 2 {
			params = append(params, types.NewJavaClass("java.util.concurrent.Executor"))
		}
		return params, types.NewParameterizedType("java.util.concurrent.CompletableFuture", []types.JavaType{v}), []string{"$factoryResult"}
	}
	if f.IsStatic {
		sig := ""
		if sameErasureClassName(f.ClassName, ctx.ClassName) {
			sig = ctx.MethodSignatureByDesc(f.FunctionName, f.Descriptor)
		}
		if sig == "" && ctx.SiblingClassSig != nil {
			_, methods, _ := ctx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/"))
			sig = methods[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]
		}
		if sig != "" {
			_, params, ret := types.ParseMethodSignatureFull(sig, ctx)
			return params, ret, types.MethodFormalTypeParamNames(sig)
		}
		return nil, nil, nil
	}
	if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref.IsThis {
		if sig := ctx.MethodSignatureByDesc(f.FunctionName, f.Descriptor); sig != "" && f.isCurrentClass(ctx) {
			_, params, ret := types.ParseMethodSignatureFull(sig, ctx)
			return params, ret, types.MethodFormalTypeParamNames(sig)
		}
	}
	// Inspect the declaring owner before recursively solving a receiver chain.
	// Most calls (StringBuilder.append, for example) have no method Signature.
	// Traversing the same inner chain once for a speculative generic wrapper
	// and again for ordinary receiver recovery causes exponential work.
	if ctx.SiblingClassSig == nil {
		return nil, nil, nil
	}
	ownerSig, ownerMethods, known := ctx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/"))
	if !known {
		return nil, nil, nil
	}
	if sig, declared := ownerMethods[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]; declared {
		if sig == "" {
			return nil, nil, nil
		}
		if len(types.ClassFormalTypeParamNames(ownerSig)) == 0 {
			_, params, ret := types.ParseMethodSignatureFull(sig, ctx)
			return params, ret, types.MethodFormalTypeParamNames(sig)
		}
	}
	raw, args := f.receiverParamTypeArgs(ctx)
	if raw == "" && f.Object != nil && ctx.SiblingClassSig != nil {
		// A non-generic owner can still declare generic methods. Its exact
		// descriptor is sufficient; a raw GENERIC owner must remain erased.
		if owner, ok := types.RawClassFQN(f.Object.Type()); ok {
			if sig, _, known := ctx.SiblingClassSig(strings.ReplaceAll(owner, ".", "/")); known && len(types.ClassFormalTypeParamNames(sig)) == 0 {
				raw = owner
			}
		}
	}
	if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref.IsThis {
		raw = ctx.ClassName
		for _, name := range types.ClassFormalTypeParamNames(ctx.ClassSig) {
			args = append(args, types.NewJavaClass(name))
		}
	}
	return types.ResolveInstantiatedSignatureExact(ctx, ctx.SiblingClassSig, raw, args, f.FunctionName, f.Descriptor, len(f.Arguments))
}

func methodVariableSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// RetainFunctionalReturnSignature records a factory's complete declaration for
// a use-site view. It does not refine the local declaration or the SAM result:
// callers may deliberately use a raw interface with polluted payloads.
// Only an exact, denotable declaration whose erasure matches the invoke return
// is evidence. Foreign callee variables remain unresolved even when a caller
// variable has the same spelling. No producer is chased through a local .Val.
func (f *FunctionCallExpression) RetainFunctionalReturnSignature(ctx *class_context.ClassContext) {
	if f == nil || ctx == nil || f.FuncType == nil || f.FuncType.ReturnType == nil ||
		ctx.Getenv("JDEC_FUNCTIONAL_RETURN_SIGNATURE_OFF") != "" {
		return
	}
	raw, known := types.RawClassFQN(f.FuncType.ReturnType)
	if !known || !strings.HasPrefix(raw, "java.util.function.") {
		return
	}
	descriptor, err := types.ParseMethodDescriptor(f.Descriptor)
	if err != nil || descriptor.FunctionType() == nil || len(descriptor.FunctionType().ParamTypes) != len(f.Arguments) {
		return
	}
	descriptorRaw, known := types.RawClassFQN(descriptor.FunctionType().ReturnType)
	if !known || !sameErasureClassName(descriptorRaw, raw) {
		return
	}
	params, ret, formals := f.genericMethodSignature(ctx)
	pt, parameterized := types.AsParameterizedType(ret)
	if len(params) != len(f.Arguments) || !parameterized || !sameErasureClassName(pt.RawClassName, raw) || javaTypeMentionsNames(ret, formals) ||
		!sourceDenotableJavaType(ret, ctx) {
		return
	}
	if f.IsStatic && ctx.SiblingClassSig != nil {
		ownerSig, _, _ := ctx.SiblingClassSig(strings.ReplaceAll(f.ClassName, ".", "/"))
		if javaTypeMentionsNames(ret, types.ClassFormalTypeParamNames(ownerSig)) {
			return
		}
	}
	for _, method := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
		for _, class := range ctx.ClassTypeParams {
			if method == class && javaTypeMentionsNames(ret, []string{class}) {
				return
			}
		}
	}
	for _, arg := range pt.TypeArgs {
		if !accessibleOverloadBound(arg, ctx) {
			return
		}
	}
	f.SourceReturnType = ret.Copy()
}

// inferredGenericMethodReturn instantiates a generic wrapper's return using
// matching parameterized arguments. Erased arguments cannot supply evidence.
func (f *FunctionCallExpression) inferredGenericMethodReturn(ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || ctx.Getenv("JDEC_GENERIC_METHOD_RETURN_WITNESS_OFF") != "" {
		return nil
	}
	params, ret, formals := f.genericMethodSignature(ctx)
	if ret == nil || len(formals) == 0 || len(params) != len(f.Arguments) {
		return nil
	}
	bindings := map[string]types.JavaType{}
	variables := methodVariableSet(formals)
	for i, p := range params {
		if !javaTypeMentionsNames(p, formals) {
			continue
		}
		if !bindMethodTypeArguments(p, f.Arguments[i].Type(), variables, bindings) {
			return nil
		}
	}
	for _, name := range formals {
		if javaTypeMentionsNames(ret, []string{name}) && bindings[name] == nil {
			return nil
		}
	}
	result := types.SubstituteTypeVars(ret, bindings)
	if !sourceDenotableJavaType(result, ctx) {
		return nil
	}
	return result
}

// FunctionalReturnArgumentTargets propagates an invariant result target back
// through a generic factory to its functional arguments. For example, returning
// CompletableFuture<Map<K,V>> from supplyAsync fixes U=Map<K,V>, hence the
// materialized supplier's target. Only variables confined to functional
// arguments are eligible, so an unrelated value argument cannot be coerced to
// satisfy the return context. The caller further requires poly definitions.
func (f *FunctionCallExpression) FunctionalReturnArgumentTargets(expected types.JavaType, ctx *class_context.ClassContext) map[int]types.JavaType {
	if ctx == nil || ctx.Getenv("JDEC_POLY_FACTORY_TARGET_OFF") != "" || !sourceDenotableJavaType(expected, ctx) {
		return nil
	}
	params, ret, formals := f.genericMethodSignature(ctx)
	if ret == nil || len(formals) == 0 || len(params) != len(f.Arguments) {
		return nil
	}
	bindings := map[string]types.JavaType{}
	if !bindMethodTypeArguments(ret, expected, methodVariableSet(formals), bindings) {
		return nil
	}
	out := map[int]types.JavaType{}
	for i, p := range params {
		if !javaTypeMentionsNames(p, formals) {
			continue
		}
		pt, ok := types.AsParameterizedType(p)
		if !ok || !strings.HasPrefix(pt.RawClassName, "java.util.function.") {
			return nil
		}
		for _, name := range formals {
			if javaTypeMentionsNames(p, []string{name}) && bindings[name] == nil {
				return nil
			}
		}
		target := types.SubstituteTypeVars(p, bindings)
		if !sourceDenotableJavaType(target, ctx) || f.FuncType == nil || i >= len(f.FuncType.ParamTypes) {
			return nil
		}
		descriptorRaw, _ := types.RawClassFQN(f.FuncType.ParamTypes[i])
		if !sameErasureClassName(descriptorRaw, pt.RawClassName) {
			return nil
		}
		out[i] = target
	}
	return out
}
