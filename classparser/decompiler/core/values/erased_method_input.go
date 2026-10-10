package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"maps"
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

// A discarded invocation has no source result constraint or payload check.
// This permission belongs to its expression-statement consumer, not to the
// call's arguments or a parent assignment. Existing materialized functional
// values keep their own SAM checks under a descriptor-only argument view.
func (f *FunctionCallExpression) PlanErasedDiscardedMethodInput(ctx *class_context.ClassContext) (*FunctionCallExpression, bool) {
	return f.planErasedMethodInputProof(ctx, true)
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
	methodBounds := erasedInvocationBounds(sig)
	// Class bounds participate in receiver inference. They cannot borrow the
	// separate permission for a method's own overload-constraining variables.
	bounds := maps.Clone(methodBounds)
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
	var formalConstraints map[string][]types.JavaType
	var bareMethodParams map[int]string
	if len(methodBounds) > 0 {
		var valid bool
		formalConstraints, bareMethodParams, valid = types.FormalBoundConstraints(sig)
		if !valid {
			return nil, false
		}
	}
	// JVM erasure records only the first bound. Pinning that bound alone can
	// make a previously valid intersection-constrained invocation inapplicable.
	// Retain every constraint only when each view is accessible/nonthrowing and
	// the complete overload family cannot acquire another applicable target.
	intersections := map[int][]types.JavaType{}
	for i, name := range bareMethodParams {
		constraints := formalConstraints[name]
		if len(constraints) <= 1 {
			continue
		}
		if i >= len(f.Arguments) || f.Arguments[i] == nil || f.Arguments[i].Type() == nil {
			return nil, false
		}
		seen := map[string]bool{}
		for ordinal, bound := range constraints {
			raw, known := types.RawClassFQN(bound)
			descriptor := "L" + strings.ReplaceAll(raw, ".", "/") + ";"
			meta, available := ctx.InvocationMetadata(strings.ReplaceAll(raw, ".", "/"))
			if !known || !available || !meta.Public || ordinal > 0 && !meta.IsInterface || seen[descriptor] || ordinal == 0 && descriptor != ps[i] || !callbinding.Assignable(erasedInvocationArgumentType(f.Arguments[i], descriptor), descriptor, ctx.InvocationMetadata) {
				return nil, false
			}
			seen[descriptor] = true
			intersections[i] = append(intersections[i], types.NewJavaClass(raw))
		}
	}
	if family.Target.Varargs {
		last := len(ps) - 1
		if last < 0 || !strings.HasPrefix(ps[last], "[") || f.Arguments[last] == nil || bindingType(f.Arguments[last].Type()) != ps[last] {
			return nil, false
		}
	}
	// A static call keeps its original owner and access; only argument views
	// change. A receiver view to a different declaration requires public access.
	if !family.Target.Public && !f.IsStatic && (owner != strings.ReplaceAll(ctx.ClassName, ".", "/") || eraseReceiver && declaring != strings.ReplaceAll(ctx.ClassName, ".", "/")) {
		// JVM access through this class does not license a source cast to a
		// nonpublic member's declaring ancestor. In particular, cross-package
		// protected access requires a receiver of the accessing subtype, and
		// casting it to the ancestor destroys that qualification. Keep the
		// original receiver/inference unless this view has public access or
		// the member is declared on the caller itself.
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
		if m.Desc != f.Descriptor && len(intersections) != 0 && len(p) == len(ps) {
			intersectionApplicable := true
			for i, parameter := range p {
				fits := callbinding.Assignable(ps[i], parameter, ctx.InvocationMetadata)
				for _, constraint := range intersections[i] {
					fits = fits || callbinding.Assignable(bindingType(constraint), parameter, ctx.InvocationMetadata)
				}
				intersectionApplicable = intersectionApplicable && fits
			}
			if intersectionApplicable {
				return nil, false
			}
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
		// A bare bounded method variable also exposes a source overload
		// constraint. Its erased descriptor can select the generic operation
		// while a narrower source operand selects a wrapper (even itself).
		// Pin the descriptor only after the complete family and nonthrowing
		// conversions above have been proved; result-use permission is unchanged.
		if len(family.Methods) > 1 && bareMethodParams[i] != "" && erasedInvocationArgumentType(arg, ps[i]) != ps[i] {
			conflict = true
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
		receiverValue := f.Object
		if child, ok := UnpackSoltValue(receiverValue).(*FunctionCallExpression); ok {
			// The proved raw receiver use also consumes the producer at its
			// original result erasure. Propagate that use through fully proved
			// fluent edges before freezing it in a binding cast.
			if planned, ok := child.PlanErasedResultChain(ctx, "L"+owner+";"); ok {
				receiverValue = planned
			}
		}
		out.Object = &CastExpression{Value: receiverValue, TargetType: types.NewJavaClass(strings.ReplaceAll(declaring, "/", ".")), Binding: true, OriginPC: f.OriginPC}
	}
	for i, p := range ps {
		if callbinding.Reference(p) {
			typ, _ := types.ParseDescriptor(p)
			out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: typ, Binding: true, OriginPC: f.OriginPC, bindingIntersection: intersections[i]}
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

// ErasedFixedInstanceResult identifies a standalone, fixed zero-input result.
// Its generic arguments belong to the already evaluated receiver, never to
// method formals or deferred arguments. A consumer may use the original raw
// result erasure after separately proving a nonthrowing widening to its target.
func ErasedFixedInstanceResult(ctx *class_context.ClassContext, f *FunctionCallExpression, result string) bool {
	if ctx == nil || ctx.InvocationMetadata == nil || f == nil || f.IsStatic || f.IsSpecialInvoke ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || f.Object == nil || len(f.Arguments) != 0 || !f.HasOriginPC {
		return false
	}
	ps, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != 0 || ret != result || !strings.HasPrefix(ret, "L") {
		return false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || family.Target.Static || family.Target.Bridge || family.Target.Varargs {
		return false
	}
	_, cs, sig := erasedInvocationDeclaration(ctx, owner, f.FunctionName, f.Descriptor)
	body, fixedThrows := invocationFixedThrowsBody(sig)
	bounds := erasedInvocationBounds(cs)
	if !fixedThrows || !strings.HasPrefix(body, "()") || len(bounds) == 0 {
		return false
	}
	_, params, typ := types.ParseMethodSignatureFull(body, ctx)
	if len(params) != 0 || erasedMethodType(typ, bounds) != result {
		return false
	}
	if _, parameterized := types.AsParameterizedType(typ); parameterized {
		return methodTypeMentionsFormal(typ, bounds)
	}
	// TR; is a declaration-owned class formal, while LR; is a named
	// class even if a formal has the same spelling. Do not conflate them
	// through the legacy inferred type representation. Arrays and method
	// formals require separate use evidence and remain refused here.
	for formal, erasure := range bounds {
		if body == "()T"+formal+";" {
			return erasure == result
		}
	}
	return false
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
