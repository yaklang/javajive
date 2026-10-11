package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A class-formal method result may be narrower in source than its physical
// return descriptor. Preserve the consumer's original overload with a widening
// view only after the exact declaration, receiver instantiation, result erasure
// and competing overloads prove it. Receiver evaluation and dispatch stay put.
func (f *FunctionCallExpression) instantiatedMethodResultOverloadCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || i < 0 || i >= len(f.Arguments) {
		return ""
	}
	inner, ok := UnpackSoltValue(f.Arguments[i]).(*FunctionCallExpression)
	if !ok || inner == nil {
		return ""
	}
	params, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != len(f.Arguments) || !ErasedFixedInstanceResult(ctx, inner, params[i]) {
		return ""
	}
	receiver, ok := types.AsParameterizedType(recoverParameterizedFieldReceiver(ctx, inner.Object))
	if !ok || receiver == nil {
		return ""
	}
	owner, signature, method := erasedInvocationDeclaration(ctx, strings.ReplaceAll(inner.ClassName, ".", "/"), inner.FunctionName, inner.Descriptor)
	if owner == "" || strings.ReplaceAll(receiver.RawClassName, ".", "/") != owner || len(signature) > 65535 || len(method) > 65535 {
		return ""
	}
	if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature)+len(method))+1) != nil {
		return ""
	}
	formals := types.ClassFormalTypeParamNames(signature)
	if len(formals) != len(receiver.TypeArgs) {
		return ""
	}
	body, valid := invocationFixedThrowsBody(method)
	if !valid {
		return ""
	}
	for j, formal := range formals {
		if body != "()T"+formal+";" {
			continue
		}
		source := receiver.TypeArgs[j]
		if source == nil || types.IsWildcardType(source) || !sourceDenotableJavaType(source, ctx) {
			return ""
		}
		actual, known := SourceTypeErasure(source, ctx)
		if !known || !callbinding.Reference(actual) {
			return ""
		}
		views, known := invocationSourceFormalViews(source, ctx)
		if !known {
			return ""
		}
		return f.narrowSourceOverloadCast(i, actual, ctx, views...)
	}
	return ""
}

// A widening view of one slot must independently exclude every rival. Generic
// inference, varargs and rivals differing in other slots need the full solver.
func (f *FunctionCallExpression) narrowSourceOverloadCast(i int, actual string, ctx *class_context.ClassContext, sourceViews ...string) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || f.FunctionName == "<init>" || f.IsSpecialInvoke || f.Kind == InvokeSpecial || f.Kind == InvokeDynamic || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 {
		return ""
	}
	if f.IsStatic && f.Kind != InvokeStatic || !f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface {
		return ""
	}
	ps, _, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != len(f.Arguments) || i < 0 || i >= len(ps) || !callbinding.Reference(ps[i]) || !callbinding.Assignable(actual, ps[i], ctx.InvocationMetadata) {
		return ""
	}
	kind := callbinding.Virtual
	if f.IsStatic {
		kind = callbinding.Static
	} else if f.Kind == InvokeInterface {
		kind = callbinding.Interface
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(f.ClassName, ".", "/"), Name: f.FunctionName, Desc: f.Descriptor, Kind: kind}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Compete || family.Target == nil || family.Target.Generic || family.Target.Varargs || family.Target.Bridge || family.Target.Static != f.IsStatic {
		return ""
	}
	competing := false
	for _, method := range family.Methods {
		if method.Static != f.IsStatic || method.Desc == f.Descriptor {
			continue
		}
		other, _, err := callbinding.Descriptor(method.Desc)
		if err != nil || method.Generic || method.Varargs || method.Bridge || len(other) != len(ps) {
			return ""
		}
		// This one restored slot must exclude each rival; unrelated argument
		// differences and generic inference need the full invocation solver.
		for j := range ps {
			if j != i && ps[j] != other[j] {
				return ""
			}
		}
		if callbinding.Assignable(ps[i], other[i], ctx.InvocationMetadata) {
			return ""
		}
		competing = competing || callbinding.Assignable(actual, other[i], ctx.InvocationMetadata)
		for _, view := range sourceViews {
			competing = competing || callbinding.Assignable(view, other[i], ctx.InvocationMetadata)
		}
	}
	if !competing {
		return ""
	}
	target := f.witnessDescriptorParamType(i)
	if !accessibleOverloadBound(target, ctx) {
		return ""
	}
	return target.String(ctx)
}

// Source applicability depends on every bound of a lexical formal, including
// interfaces after its first JVM erasure bound. Bind nearest declaration first;
// a method formal shadows class/enclosing formals with the same spelling.
// Array source views lift every proved element bound through the original rank;
// concrete instantiations already supply their own descriptor view.
func invocationSourceFormalViews(source types.JavaType, ctx *class_context.ClassContext) ([]string, bool) {
	if source == nil || ctx == nil || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return nil, false
	}
	dimensions := 0
	for source.IsArray() {
		dimensions++
		if dimensions > 128 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return nil, false
		}
		source = source.ElementType()
		if source == nil {
			return nil, false
		}
	}
	name, formal := types.RawClassFQN(source)
	if !formal || !ctx.IsTypeParam(name) {
		return nil, true
	}
	if len(ctx.LexicalTypeParamSignatures) > 128 {
		return nil, false
	}
	scopes := append([]string{ctx.CurrentMethodSig, ctx.ClassSig}, ctx.LexicalTypeParamSignatures...)
	for _, signature := range scopes {
		if len(signature) > 65535 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature))+1) != nil {
			return nil, false
		}
		for _, declared := range types.ClassFormalTypeParamNames(signature) {
			if declared != name {
				continue
			}
			bounds, valid := types.FormalTypeBounds(signature)
			if !valid || len(bounds[name]) == 0 {
				return nil, false
			}
			var views []string
			for ordinal, bound := range bounds[name] {
				view, known := SourceTypeErasure(bound, ctx)
				if !known || !callbinding.Reference(view) {
					return nil, false
				}
				if ordinal > 0 {
					if ctx.InvocationMetadata == nil || !strings.HasPrefix(view, "L") {
						return nil, false
					}
					internal := view[1 : len(view)-1]
					declaration, available := ctx.InvocationMetadata(internal)
					if !available || declaration.Name != internal || !declaration.IsInterface {
						return nil, false
					}
				}
				views = append(views, strings.Repeat("[", dimensions)+view)
			}
			return views, true
		}
	}
	return nil, false
}
