package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A class-formal array parameter has a source constraint even when its JVM
// descriptor is Object[]. Restore that constraint at this consumption edge,
// without narrowing the runtime array or erasing the receiver/result. The
// exact declaration owns the formal; equal spelling in another scope is not
// evidence. Method inference and inherited/foreign receiver substitutions
// remain with their dedicated solvers.
func (f *FunctionCallExpression) declaredArrayArgumentCast(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.IsStatic || f.IsStatic || f.FunctionName == "<init>" ||
		(f.Kind != InvokeVirtual && f.Kind != InvokeSpecial) || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 ||
		!sameErasureClassName(f.ClassName, ctx.ClassName) || i < 0 || i >= len(f.Arguments) || len(f.Arguments) > 128 {
		return ""
	}
	receiver, ok := UnpackSoltValue(f.Object).(*JavaRef)
	if !ok || receiver == nil || !receiver.IsThis || f.Arguments[i] == nil || f.Arguments[i].Type() == nil || !f.Arguments[i].Type().IsArray() {
		return ""
	}
	if _, original := receiver.OriginalReceiverSlot(); !original {
		return ""
	}
	if raw, known := types.RawClassFQN(receiver.Type()); !known || !sameErasureClassName(raw, f.ClassName) {
		return ""
	}
	ps, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != len(f.Arguments) || !strings.HasPrefix(ps[i], "[") {
		return ""
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	meta, known := ctx.InvocationMetadata(owner)
	if !known || meta.Name != owner || meta.IsInterface || meta.Signature != ctx.ClassSig || !meta.MembersComplete || !meta.ParentsComplete {
		return ""
	}
	var signature string
	for _, method := range meta.Methods {
		if method.Name == f.FunctionName && method.Desc == f.Descriptor {
			if signature != "" || method.Static || method.Bridge || method.Signature == "" {
				return ""
			}
			signature = method.Signature
		}
	}
	if signature == "" || len(signature) > 8192 || len(ctx.ClassSig) > 8192 ||
		ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature)+len(ctx.ClassSig))+1) != nil {
		return ""
	}
	own, refs, valid := types.SignatureTypeVariableReferences(signature)
	if !valid || len(own) != 0 {
		return ""
	}
	classFormals := map[string]bool{}
	for _, formal := range types.ClassFormalTypeParamNames(ctx.ClassSig) {
		classFormals[formal] = true
	}
	for _, formal := range refs {
		if !classFormals[formal] || !ctx.IsTypeParam(formal) || ctx.RawEraseTypeVar(formal) {
			return ""
		}
		for _, shadow := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
			if formal == shadow {
				return ""
			}
		}
	}
	_, params, ret := types.ParseMethodSignatureFull(signature, ctx)
	if len(params) != len(ps) || ret == nil || params[i] == nil || !params[i].IsArray() {
		return ""
	}
	element := params[i]
	for rank := 0; element.IsArray(); rank++ {
		if rank >= 8 {
			return ""
		}
		element = element.ElementType()
		if element == nil {
			return ""
		}
	}
	formal, valid := types.RawClassFQN(element)
	if !valid || !classFormals[formal] {
		return ""
	}
	// Check the entire selected Signature against its descriptor. No additional
	// runtime check may be hidden in the source-only array view, including a
	// dependent or intersection bound with a different first erasure.
	for j, param := range params {
		if erased, ok := SourceTypeErasure(param, ctx); !ok || erased != ps[j] {
			return ""
		}
	}
	if erased, ok := SourceTypeErasure(ret, ctx); !ok || erased != result {
		return ""
	}
	if !sourceDenotableJavaType(f.Arguments[i].Type(), ctx) {
		return ""
	}
	actual, known := SourceTypeErasure(f.Arguments[i].Type(), ctx)
	if !known || !callbinding.Assignable(actual, ps[i], ctx.InvocationMetadata) {
		return ""
	}
	physical := declaredArrayOperandDescriptor(f.Arguments[i], ctx)
	if !strings.HasPrefix(physical, "[") || !callbinding.Assignable(physical, ps[i], ctx.InvocationMetadata) {
		return ""
	}
	views, known := invocationSourceFormalViews(params[i], ctx)
	if !known {
		return ""
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Target == nil || family.Target.Static || family.Target.Bridge {
		return ""
	}
	if family.Target.Varargs && !strings.HasPrefix(ps[len(ps)-1], "[") {
		return ""
	}
	for _, rival := range family.Methods {
		if rival.Desc == f.Descriptor {
			continue
		}
		other, otherResult, err := callbinding.Descriptor(rival.Desc)
		if err != nil || len(other) != len(ps) {
			return ""
		}
		if strings.Join(other, "") == strings.Join(ps, "") {
			// A covariant ancestor declaration and its return bridge are one
			// source signature, not another argument overload. The selected
			// declaration must be owned here, and cannot have a second ordinary
			// same-parameter declaration here. This permission does not remove,
			// inline or certify any bridge body.
			if rival.Static || !family.Target.Public || !callbinding.Assignable(result, otherResult, ctx.InvocationMetadata) {
				return ""
			}
			for _, method := range meta.Methods {
				if method.Name == f.FunctionName && method.Desc == rival.Desc && !method.Bridge {
					return ""
				}
			}
			continue
		}
		if rival.Varargs || rival.Bridge {
			return ""
		}
		for j := range ps {
			if j != i && ps[j] != other[j] {
				return ""
			}
		}
		// A fixed rival either cannot accept any array view, or its concrete
		// formal is a supertype of the restored class-formal array. Generic
		// rivals that can accept it require inference and are not decided here.
		accepts := callbinding.Assignable(ps[i], other[i], ctx.InvocationMetadata)
		for _, view := range views {
			accepts = accepts || callbinding.Assignable(view, other[i], ctx.InvocationMetadata)
		}
		if accepts && rival.Generic {
			return ""
		}
	}
	return params[i].String(ctx)
}

// An inferred expression type is not the verifier's operand type. Read the
// original invoke descriptor, retained CHECKCAST target or descriptor-seeded
// parameter slot. A ref's initializer is not its current value; mutable local
// webs, field projections and other producers need their own physical proof.
func declaredArrayOperandDescriptor(value JavaValue, ctx *class_context.ClassContext) string {
	if ctx == nil {
		return ""
	}
	switch v := UnpackSoltValue(value).(type) {
	case *FunctionCallExpression:
		if v == nil || !v.HasOriginPC || v.OriginPC < 0 || v.OriginPC > 65535 || v.FunctionName == "<init>" {
			return ""
		}
		ps, result, err := callbinding.Descriptor(v.Descriptor)
		if err == nil && len(ps) == len(v.Arguments) {
			return result
		}
	case *CastExpression:
		if _, descriptor, original := v.OriginalCheckCastWitness(ctx); original {
			return descriptor
		}
	case *JavaRef:
		position, original := v.OriginalParameterSlot()
		ps, _, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
		if !original || err != nil {
			return ""
		}
		slot := 0
		if !ctx.IsStatic {
			slot = 1
		}
		for _, descriptor := range ps {
			if slot == position {
				return descriptor
			}
			slot++
			if descriptor == "J" || descriptor == "D" {
				slot++
			}
		}
	}
	return ""
}
