package values

import (
	"github.com/yaklang/javajive/internal/workbudget"
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

type receiverTypeQueryResult struct {
	raw  string
	args []types.JavaType
}

// Each query observes one immutable IR/type state. Cache positive and negative
// receiver results only for this operation; reference webs can change types
// between operations, so persistent expression caches would be unsound.
type receiverTypeQuery struct {
	results map[*FunctionCallExpression]receiverTypeQueryResult
	active  map[*FunctionCallExpression]bool
}

func newReceiverTypeQuery() *receiverTypeQuery {
	return &receiverTypeQuery{results: map[*FunctionCallExpression]receiverTypeQueryResult{}, active: map[*FunctionCallExpression]bool{}}
}

func (f *FunctionCallExpression) genericMethodSignature(ctx *class_context.ClassContext) ([]types.JavaType, types.JavaType, []string) {
	return f.genericMethodSignatureQuery(ctx, newReceiverTypeQuery())
}

func (f *FunctionCallExpression) genericMethodSignatureQuery(ctx *class_context.ClassContext, query *receiverTypeQuery) ([]types.JavaType, types.JavaType, []string) {
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
		if sig == "" {
			_, methods, _ := invocationSignatureEvidence(ctx, strings.ReplaceAll(f.ClassName, ".", "/"))
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
	if ctx.SiblingClassSig == nil && ctx.InvocationMetadata == nil {
		return nil, nil, nil
	}
	ownerSig, ownerMethods, known := invocationSignatureEvidence(ctx, strings.ReplaceAll(f.ClassName, ".", "/"))
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
	raw, args := f.receiverParamTypeArgsQuery(ctx, query)
	if raw == "" && f.Object != nil {
		// A non-generic owner can still declare generic methods. Its exact
		// descriptor is sufficient; a raw GENERIC owner must remain erased.
		if owner, ok := types.RawClassFQN(f.Object.Type()); ok {
			if sig, _, known := invocationSignatureEvidence(ctx, strings.ReplaceAll(owner, ".", "/")); known && len(types.ClassFormalTypeParamNames(sig)) == 0 {
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
	evidence := func(n string) (string, map[string]string, bool) { return invocationSignatureEvidence(ctx, n) }
	return types.ResolveInstantiatedSignatureExact(ctx, evidence, raw, args, f.FunctionName, f.Descriptor, len(f.Arguments))
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
	if f.IsStatic {
		ownerSig, _, _ := invocationSignatureEvidence(ctx, strings.ReplaceAll(f.ClassName, ".", "/"))
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
// matching parameterized arguments. Erased words supply no binding, but may agree
// with an independently bound source cast having exactly the original erasure.
func (f *FunctionCallExpression) inferredGenericMethodReturn(ctx *class_context.ClassContext) types.JavaType {
	return f.inferredGenericMethodReturnQuery(ctx, newReceiverTypeQuery())
}

func (f *FunctionCallExpression) inferredGenericMethodReturnQuery(ctx *class_context.ClassContext, query *receiverTypeQuery) types.JavaType {
	if f == nil || ctx == nil || ctx.Getenv("JDEC_GENERIC_METHOD_RETURN_WITNESS_OFF") != "" {
		return nil
	}
	// Only a callee's own method variables can be solved by argument witnesses.
	// An exact original declaration without method formals cannot contribute
	// such a result, even when its declaring class has type parameters. Reject
	// that speculation before recursively resolving the receiver: a raw fluent
	// chain would otherwise solve each prefix here and again in receiver recovery,
	// giving exponential work while ultimately returning the same unknown result.
	if f.Descriptor != "" {
		_, methods, known := invocationSignatureEvidence(ctx, strings.ReplaceAll(f.ClassName, ".", "/"))
		if sig, declared := methods[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]; known && declared && len(types.MethodFormalTypeParamNames(sig)) == 0 {
			return nil
		}
	}
	params, ret, formals := f.genericMethodSignatureQuery(ctx, query)
	if ret == nil || len(formals) == 0 || len(params) != len(f.Arguments) {
		return nil
	}
	bindings := map[string]types.JavaType{}
	variables := methodVariableSet(formals)
	// Solve invariant container/array constraints before erased bare variables,
	// independently of their order in the selected original descriptor.
	for i, p := range params {
		if !javaTypeMentionsNames(p, formals) {
			continue
		}
		if _, bare := bareMethodVariable(p, variables); bare {
			continue
		}
		if f.Arguments[i] == nil || !bindMethodTypeArguments(p, f.Arguments[i].Type(), variables, bindings) {
			return nil
		}
	}
	for i, p := range params {
		name, bare := bareMethodVariable(p, variables)
		if !bare {
			continue
		}
		if f.Arguments[i] == nil {
			return nil
		}
		if !bindMethodTypeArguments(p, f.Arguments[i].Type(), variables, bindings) && !f.erasedCallerArgumentAgrees(i, bindings[name], ctx) {
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

func bareMethodVariable(t types.JavaType, variables map[string]bool) (string, bool) {
	if t == nil {
		return "", false
	}
	c, ok := t.RawType().(*types.JavaClass)
	if ok && c != nil && variables[c.Name] {
		return c.Name, true
	}
	return "", false
}

// Agreement with an already planned source cast, not new inference. The same
// caller variable must be selected by the renderer's original witnesses. Both
// argument and target erasures must equal the exact original invoke word, so
// no payload narrowing or new CHECKCAST is introduced. Callee and caller bound
// scopes remain separate; contradictory or missing declarations refuse proof.
func (f *FunctionCallExpression) erasedCallerArgumentAgrees(i int, bound types.JavaType, ctx *class_context.ClassContext) bool {
	if f == nil || ctx == nil || bound == nil || i < 0 || i >= len(f.Arguments) || f.Arguments[i] == nil {
		return false
	}
	caller, ok := bound.RawType().(*types.JavaClass)
	if !ok || caller == nil || !ctx.IsTypeParam(caller.Name) {
		return false
	}
	planned := f.genericMethodWitnessArgParamType(i, ctx)
	if planned == nil || !reflect.DeepEqual(planned.RawType(), bound.RawType()) {
		return false
	}
	owner, methods, known := invocationSignatureEvidence(ctx, strings.ReplaceAll(f.ClassName, ".", "/"))
	signature := methods[class_context.MethodDescKey(f.FunctionName, f.Descriptor)]
	if sameErasureClassName(f.ClassName, ctx.ClassName) {
		owner, signature, known = ctx.ClassSig, ctx.MethodSignatureByDesc(f.FunctionName, f.Descriptor), true
	}
	if !known || signature == "" || len(owner)+len(signature) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(owner)+len(signature))+1) != nil {
		return false
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{owner}, signature)
	if !valid || erased != f.Descriptor {
		return false
	}
	descriptor, err := types.ParseMethodDescriptor(f.Descriptor)
	if err != nil || descriptor.FunctionType() == nil || len(descriptor.FunctionType().ParamTypes) != len(f.Arguments) {
		return false
	}
	expected := bindingType(descriptor.FunctionType().ParamTypes[i])
	source, sourceOK := SourceTypeErasure(bound, ctx)
	actual, actualOK := SourceTypeErasure(f.Arguments[i].Type(), ctx)
	return expected != "" && sourceOK && actualOK && source == expected && actual == expected
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
