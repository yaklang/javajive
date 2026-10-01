package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// A synthetic lambda is decoded with the implementation's erased parameters.
// Capturing a Consumer<? super E> must not constrain its Object payload to E:
// that would invent a check before the helper's own effects. Restore raw input
// views at the exact method descriptor, retaining every original CHECKCAST.
// Class-dependent results additionally require an explicit erased use-site view.
func (f *FunctionCallExpression) planErasedMethodInput(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	return f.planErasedMethodInputProof(ctx, false)
}
func (f *FunctionCallExpression) planErasedMethodInputProof(ctx *class_context.ClassContext, allowGenericResult bool) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(!f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface) {
		return nil, false
	}
	ps, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) == 0 || len(ps) != len(f.Arguments) {
		return nil, false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	declaring, cs, sig := erasedInvocationDeclaration(ctx, owner, f.FunctionName, f.Descriptor)
	sig, fixedThrows := invocationFixedThrowsBody(sig)
	bounds := erasedInvocationBounds(sig)
	if declaring == "" || !fixedThrows || (len(bounds) == 0 && (f.IsStatic || !allowGenericResult || len(erasedInvocationBounds(cs)) == 0)) {
		return nil, false
	}
	if len(bounds) == 0 {
		// Existing functional argument lowering preserves result inference and
		// the receiver chain; do not replace that narrower proved adaptation.
		if ctx.Getenv("JDEC_FUNCTIONAL_ERASURE_RESOLVE_OFF") != "" || ctx.Getenv("JDEC_GENERIC_PARAM_RECV_METHOD_OFF") != "" {
			return nil, false
		}
		for i, arg := range f.Arguments {
			if f.nestedGenericErasureArgCast(i, arg, ctx) != "" {
				return nil, false
			}
		}
	}
	_, params, ret := types.ParseMethodSignatureFull(sig, ctx)
	if len(params) != len(ps) || ret == nil {
		return nil, false
	}
	eraseReceiver := false
	if !f.IsStatic {
		classBounds := erasedInvocationBounds(cs)
		// Method formals shadow same-named class formals. Merge only the unshadowed
		// bounds, exactly as the Signature's lexical scope does.
		for name := range bounds {
			delete(classBounds, name)
		}
		classDependent := methodTypeMentionsFormal(ret, classBounds)
		for _, param := range params {
			classDependent = classDependent || methodTypeMentionsFormal(param, classBounds)
		}
		if classDependent {
			if !allowGenericResult || f.Object == nil || !callbinding.Assignable(bindingType(f.Object.Type()), "L"+declaring+";", ctx.InvocationMetadata) {
				return nil, false
			}
			eraseReceiver = true
			if bounds == nil {
				bounds = map[string]string{}
			}
			for name, bound := range classBounds {
				bounds[name] = bound
			}
		}
	}

	// A bare method-variable result needs a separate use-site check proof.
	if raw, ok := types.RawClassFQN(ret); ok && bounds[raw] != "" && !allowGenericResult {
		return nil, false
	}
	if (!allowGenericResult && methodTypeMentionsFormal(ret, bounds)) || erasedMethodType(ret, bounds) != result {
		return nil, false
	}
	family, e := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	if e != nil || !family.Complete || family.Target == nil || family.Target.Bridge || family.Target.Static != f.IsStatic {
		return nil, false
	}
	if family.Target.Varargs {
		last := len(ps) - 1
		if last < 0 || !strings.HasPrefix(ps[last], "[") || f.Arguments[last] == nil || bindingType(f.Arguments[last].Type()) != ps[last] {
			return nil, false
		}
	}
	// A static call keeps its original owner and access; only argument views
	// change. A receiver view to a different declaration requires public access.
	if !family.Target.Public && !f.IsStatic && owner != strings.ReplaceAll(ctx.ClassName, ".", "/") {
		return nil, false
	}
	for _, m := range family.Methods {
		p, r, e := callbinding.Descriptor(m.Desc)
		if m.Varargs && (len(p) == 0 || !strings.HasPrefix(p[len(p)-1], "[")) {
			return nil, false
		}
		// Raw arguments do not establish most-specific ordering between two
		// generic declarations of equal arity. Keep their original inference.
		applicable := len(p) == len(ps)
		for i := range p {
			if i >= len(ps) || !callbinding.Assignable(ps[i], p[i], ctx.InvocationMetadata) {
				applicable = false
				break
			}
		}
		if e != nil || ((m.Varargs || m.Generic) && m.Desc != f.Descriptor && applicable) || (!m.Bridge && strings.Join(p, "") == strings.Join(ps, "") && r != result) {
			return nil, false
		}
	}
	conflict := false
	for i, arg := range f.Arguments {
		if arg == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) || arg.Type() == nil || erasedMethodType(params[i], bounds) != ps[i] {
			return nil, false
		}
		if !callbinding.Assignable(erasedInvocationArgumentType(arg, ps[i]), ps[i], ctx.InvocationMetadata) {
			return nil, false
		}
		if _, nested := types.AsParameterizedType(params[i]); nested && methodTypeMentionsFormal(params[i], bounds) {
			if _, actual := types.AsParameterizedType(arg.Type()); actual {
				conflict = true
			}
			if inner, ok := UnpackSoltValue(arg).(*FunctionCallExpression); ok {
				_, sourceRet, _ := inner.genericMethodSignature(ctx)
				if sourceRet == nil {
					_, _, innerSig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(inner.ClassName, ".", "/"), inner.FunctionName, inner.Descriptor)
					_, _, sourceRet = types.ParseMethodSignatureFull(innerSig, ctx)
				}
				if _, ok := types.AsParameterizedType(sourceRet); ok {
					conflict = true
				}
			}
		}
	}
	if !conflict {
		return nil, false
	}
	out := f.Clone()
	if eraseReceiver {
		meta, known := ctx.InvocationMetadata(declaring)
		if !known || !meta.Public {
			return nil, false
		}
		out.Object = &CastExpression{Value: f.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(declaring, "/", ".")), Binding: true, OriginPC: f.OriginPC}
	}
	for i, p := range ps {
		if callbinding.Reference(p) {
			typ, _ := types.ParseDescriptor(p)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: typ, Binding: true, OriginPC: f.OriginPC}
		}
	}
	out.bindingPlanned = true
	return out, true
}

// Fixed checked exceptions do not depend on inference. Retain their declaration
// and handlers; a type-variable throws clause needs a separate proof.
func invocationFixedThrowsBody(sig string) (string, bool) {
	parts := strings.Split(sig, "^")
	for _, token := range parts[1:] {
		if !strings.HasPrefix(token, "L") {
			return "", false
		}
		_, result, err := callbinding.Descriptor("()" + token)
		if err != nil || result != token {
			return "", false
		}
	}
	return parts[0], true
}

func erasedMethodType(t types.JavaType, bounds map[string]string) string {
	if t == nil {
		return ""
	}
	if t.IsArray() {
		s := erasedMethodType(t.ElementType(), bounds)
		if s == "" {
			return ""
		}
		return "[" + s
	}
	if raw, ok := types.RawClassFQN(t); ok {
		if b := bounds[raw]; b != "" {
			return b
		}
	}
	return bindingType(t)
}
func methodTypeMentionsFormal(t types.JavaType, bounds map[string]string) bool {
	if t == nil {
		return false
	}
	if t.IsArray() {
		return methodTypeMentionsFormal(t.ElementType(), bounds)
	}
	if w, ok := t.(*types.JavaWildcardType); ok {
		return methodTypeMentionsFormal(w.Bound, bounds)
	}
	if p, ok := types.AsParameterizedType(t); ok {
		for _, a := range p.TypeArgs {
			if methodTypeMentionsFormal(a, bounds) {
				return true
			}
		}
		return false
	}
	raw, ok := types.RawClassFQN(t)
	return ok && bounds[raw] != ""
}

// ErasedFactoryReturn proves that a generic factory has exactly one binding,
// no deferred/poly inputs and a fixed reference result erasure. Suppressing
// return target inference therefore cannot select a different overload or add
// a payload check inside a lambda. A raw result view preserves the JVM result.
func ErasedFactoryReturn(ctx *class_context.ClassContext, v JavaValue, result string) bool {
	v = UnpackSoltValue(v)
	f, ok := v.(*FunctionCallExpression)
	if !ok || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || f.IsSpecialInvoke {
		return false
	}
	ps, r, e := callbinding.Descriptor(f.Descriptor)
	if e != nil || r != result || !callbinding.Reference(r) || len(ps) != len(f.Arguments) {
		return false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	family, e := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	if e != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || family.Target.Varargs || family.Target.Bridge {
		return false
	}
	_, cs, sig := erasedInvocationDeclaration(ctx, owner, f.FunctionName, f.Descriptor)
	if (!f.IsStatic && len(types.ClassFormalTypeParamNames(cs)) != 0) || strings.Contains(sig, "^") {
		return false
	}
	bounds := erasedInvocationBounds(sig)
	_, params, ret := types.ParseMethodSignatureFull(sig, ctx)
	if len(bounds) == 0 || len(params) != len(ps) || erasedMethodType(ret, bounds) != result {
		return false
	}
	if _, ok := types.AsParameterizedType(ret); !ok {
		return false
	}
	for i, arg := range f.Arguments {
		if arg == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) || arg.Type() == nil || erasedMethodType(params[i], bounds) != ps[i] || !callbinding.Assignable(erasedInvocationArgumentType(arg, ps[i]), ps[i], ctx.InvocationMetadata) {
			return false
		}
		// Calls/conditionals can themselves be poly expressions. Fail closed rather
		// than propagating a changed inference context through a nested invocation.
		actual := UnpackSoltValue(arg)
		if c, ok := actual.(*CastExpression); ok {
			actual = UnpackSoltValue(c.Value)
		}
		switch value := actual.(type) {
		case *FunctionCallExpression:
			// A complete, fixed zero-argument declaration has no input
			// inference for an outer target context to change. Class-formal
			// substitution is fixed by its receiver, not the assignment target.
			if value.Descriptor == "" || len(value.Arguments) != 0 || value.IsSpecialInvoke {
				return false
			}
			owner := strings.ReplaceAll(value.ClassName, ".", "/")
			family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: value.FunctionName, Desc: value.Descriptor}, ctx.InvocationMetadata)
			_, _, signature := erasedInvocationDeclaration(ctx, owner, value.FunctionName, value.Descriptor)
			if err != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || (family.Target.Generic && signature == "") || family.Target.Bridge || family.Target.Varargs || strings.HasPrefix(signature, "<") {
				return false
			}
		case *TernaryExpression, *CustomValue:
			return false
		}
	}
	return true
}

// Return lowering supplies the exact result erasure, and adds its unchecked
// generic view outside the call. This permission never flows into arguments.
func (f *FunctionCallExpression) PlanErasedMethodReturn(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	if ctx == nil {
		return nil, false
	}
	ft, ok := ctx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return nil, false
	}
	_, ret, e := callbinding.Descriptor(ctx.CurrentMethodDesc)
	_, result, e2 := callbinding.Descriptor(f.Descriptor)
	if e != nil || e2 != nil || ret != result {
		return nil, false
	}
	_, parameterized := types.AsParameterizedType(ft.ReturnType)
	formal, named := types.RawClassFQN(ft.ReturnType)
	// A synthetic lambda may intentionally return the raw container erasure.
	// Only a declaration returning that same parameterized head can license
	// this view. Bare method-variable results (Callable<T>, for example) still
	// need their distinct payload/checked-use proof.
	rawContainer := false
	if ctx.InvocationMetadata != nil && named && !parameterized && !ctx.IsTypeParam(formal) && result != "Ljava/lang/Object;" && bindingType(ft.ReturnType) == result {
		_, _, sig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
		_, _, declared := types.ParseMethodSignatureFull(sig, ctx)
		if p, ok := types.AsParameterizedType(declared); ok {
			rawContainer = "L"+strings.ReplaceAll(p.RawClassName, ".", "/")+";" == result
		}
	}
	if !parameterized && !(named && ctx.IsTypeParam(formal)) && !rawContainer && !f.erasedObjectPayloadReturn(ctx, ft.ReturnType) {
		return nil, false
	}
	return f.planErasedMethodInputProof(ctx, true)
}

// An existing CHECKCAST is an explicit use-site adaptation. Erase method-input
// inference inside it while leaving that original check outside, at its PC.
func (f *FunctionCallExpression) PlanErasedCheckedMethodInput(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	_, result, e := callbinding.Descriptor(f.Descriptor)
	if e != nil || !callbinding.Reference(result) {
		return nil, false
	}
	return f.planErasedMethodInputProof(ctx, true)
}

// Only a bare erased payload input licenses suppressing inference on an Object
// return. A generic Callable<T> with no payload input must retain T inference:
// erasing its sole nested argument would break the enclosing lambda's target.
func (f *FunctionCallExpression) erasedObjectPayloadReturn(ctx *class_context.ClassContext, ret types.JavaType) bool {
	if bindingType(ret) != "Ljava/lang/Object;" {
		return false
	}
	_, _, sig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
	bounds := erasedInvocationBounds(sig)
	_, params, _ := types.ParseMethodSignatureFull(sig, ctx)
	if len(params) != len(f.Arguments) {
		return false
	}
	for i, param := range params {
		name, bare := types.RawClassFQN(param)
		if bare && bounds[name] == "Ljava/lang/Object;" && f.Arguments[i] != nil && bindingType(f.Arguments[i].Type()) == "Ljava/lang/Object;" {
			return true
		}
	}
	return false
}
