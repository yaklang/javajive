package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The capturing class supplies a positively checked original compiler protocol
// and an actual factory/Code/PC source sink. No generic bootstrap dispatch or
// declaration flag supplies these capabilities by itself.
type SerializableReferenceSource struct {
	Metadata callbinding.Provider
	Target   callbinding.Witness
	Retain   func(*values.CustomValue) (*values.CustomValue, bool)
}

func t19SerializableReference(req CallSiteRequest, d *Decompiler, static []values.JavaValue, impl *values.JavaClassMember, result types.JavaType, packet SerializableReferenceSource) DispatchResult {
	refuse := func() DispatchResult {
		return unsupportedDispatch(req, FamilyLambda, DiagBootstrapUnknown, "serializable reference source requires its original target and deserializer/factory closure", result)
	}
	if d == nil || d.FunctionContext == nil || packet.Metadata == nil || packet.Retain == nil || impl == nil || impl.RefKind != RefInvokeStatic || len(req.DynamicArgs) != 0 || len(static) != 3 || t19MethodTypeDesc(static[2]) != impl.Description || packet.Target.Kind != callbinding.Static || NormalizeBootstrapOwner(packet.Target.Owner) != NormalizeBootstrapOwner(impl.Name) || packet.Target.Name != impl.Member || packet.Target.Desc != impl.Description || packet.Target.PC != req.OriginPC {
		return refuse()
	}
	// The ordinary public binding kernel receives only the capturing class's
	// proved package-access projection. Do not change another invocation's
	// metadata, or widen the shared parser context while building this value.
	context := *d.FunctionContext
	context.InvocationMetadata = packet.Metadata
	scoped := *d
	scoped.FunctionContext = &context
	scoped.blockPartialFunctionalTarget = false
	value, ok := methodRefCheckedPolyTarget(&scoped, req, static, impl, result, types.NewJavaClass("java.io.Serializable")).(*values.CustomValue)
	if !ok || value == nil || !value.IsMethodRef || !value.CapturesKnown || len(value.Captures) != 0 || value.WriteFunc == nil {
		return refuse()
	}
	value.OriginPC, value.HasOriginPC = req.OriginPC, true
	value.MarkOriginalLambdaFactory(req.OriginPC)
	retained, known := packet.Retain(value)
	if !known || retained == nil {
		return refuse()
	}
	return okDispatch(req, FamilyLambda, retained, values.EffectCall|values.EffectAllocate)
}
