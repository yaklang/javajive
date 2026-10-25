package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A non-generic, unambiguous consumer gives its argument a concrete erased
// result context. A fresh fluent producer can use that context to retain raw
// JVM input ports without inventing narrower checks for inferred type arguments.
// The final widening pins the consumer selection; the producer proof preserves
// all original argument/receiver evaluations and every existing CHECKCAST.
func (f *FunctionCallExpression) erasedConcreteArgumentResultView(i int, ctx *class_context.ClassContext) JavaValue {
	if f == nil || ctx == nil || f.bindingPlanned || !f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || f.IsSpecialInvoke || f.FunctionName == "<init>" ||
		(f.Kind != InvokeStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface) || f.IsStatic != (f.Kind == InvokeStatic) || i < 0 || i >= len(f.Arguments) {
		return nil
	}
	params, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(params) != len(f.Arguments) || f.FuncType == nil || len(f.FuncType.ParamTypes) != len(params) || bindingType(f.FuncType.ReturnType) != result || !strings.HasPrefix(params[i], "L") {
		return nil
	}
	for j, p := range params {
		if bindingType(f.FuncType.ParamTypes[j]) != p {
			return nil
		}
	}
	producer, known := UnpackSoltValue(f.Arguments[i]).(*FunctionCallExpression)
	if !known || producer == nil || len(producer.Arguments) != 0 {
		return nil
	}
	_, ret, e := callbinding.Descriptor(producer.Descriptor)
	if e != nil {
		return nil
	}
	proof, known := boundedErasedResultContext(ctx)
	if !known {
		return nil
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	meta, known := proof.InvocationMetadata(owner)
	if !known || meta.Name != owner || !meta.MembersComplete || !meta.ParentsComplete || !erasedResultUniqueTarget(meta, f) {
		return nil
	}
	family, e := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: f.FunctionName, Desc: f.Descriptor}, proof.InvocationMetadata)
	if e != nil || !family.Complete || family.Target == nil || family.Target.Generic || family.Target.Bridge || family.Target.Varargs || family.Target.Static != f.IsStatic || family.Target.Signature != "" {
		return nil
	}
	for _, m := range family.Methods {
		ps, _, e := callbinding.Descriptor(m.Desc)
		if e != nil || m.Varargs || len(ps) == len(params) && m.Desc != f.Descriptor {
			return nil
		}
	}
	if !callbinding.Assignable(ret, params[i], proof.InvocationMetadata) {
		return nil
	}
	planned, ok := producer.planErasedZeroArgumentResult(proof)
	if !ok {
		return nil
	}
	remaining := 128
	if !erasedClassOnlyProjection(producer, planned, proof, &remaining) {
		return nil
	}
	target, e := types.ParseDescriptor(params[i])
	if e != nil {
		return nil
	}
	return &CastExpression{Value: planned, TargetType: target, Binding: true, OriginPC: f.OriginPC}
}
