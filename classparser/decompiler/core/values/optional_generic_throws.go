package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Optional.orElseThrow declares its exception independently from the element:
// <X extends Throwable> T orElseThrow(Supplier<? extends X>) throws X.
// A raw receiver erases X to Throwable before javac sees the supplier. A view
// through Optional<?> retains X without guessing T. Its erasure is exactly the
// existing receiver, so neither evaluation order nor any JVM cast changes.
func (f *FunctionCallExpression) optionalGenericThrowsReceiver(ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || ctx.Getenv("JDEC_OPTIONAL_GENERIC_THROWS_OFF") != "" || f.IsStatic || f.IsSpecialInvoke ||
		f.FunctionName != "orElseThrow" || !sameErasureClassName(f.ClassName, "java.util.Optional") ||
		f.Descriptor != "(Ljava/util/function/Supplier;)Ljava/lang/Object;" || len(f.Arguments) != 1 || f.Object == nil {
		return nil
	}
	if _, literal := UnpackSoltValue(f.Object).(*JavaClassValue); literal {
		return nil
	}
	typ := f.Object.Type()
	if _, parameterized := types.AsParameterizedType(typ); parameterized {
		return nil
	}
	raw, known := types.RawClassFQN(typ)
	if !known || !sameErasureClassName(raw, "java.util.Optional") {
		return nil
	}
	return types.NewParameterizedType("java.util.Optional", []types.JavaType{&types.JavaWildcardType{}})
}

// OptionalGenericThrowsSupplierTarget accepts declaration evidence for a raw
// supplier only at this erased invocation. It does not constrain get() uses of
// the same object. The core caller must independently prove the reaching factory
// definition; a JavaRef.Val is not such proof.
func (f *FunctionCallExpression) OptionalGenericThrowsSupplierTarget(source types.JavaType, ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || ctx.Getenv("JDEC_OPTIONAL_GENERIC_THROWS_OFF") != "" || f.IsStatic || f.IsSpecialInvoke || f.Object == nil ||
		f.FunctionName != "orElseThrow" || !sameErasureClassName(f.ClassName, "java.util.Optional") ||
		f.Descriptor != "(Ljava/util/function/Supplier;)Ljava/lang/Object;" || len(f.Arguments) != 1 || f.Arguments[0] == nil {
		return nil
	}
	if _, literal := UnpackSoltValue(f.Object).(*JavaClassValue); literal {
		return nil
	}
	owner, known := types.RawClassFQN(f.Object.Type())
	if !known || !sameErasureClassName(owner, "java.util.Optional") {
		return nil
	}
	actual := f.Arguments[0].Type()
	if _, parameterized := types.AsParameterizedType(actual); parameterized {
		return nil
	}
	raw, known := types.RawClassFQN(actual)
	pt, parameterized := types.AsParameterizedType(source)
	if !known || !sameErasureClassName(raw, "java.util.function.Supplier") || !parameterized ||
		!sameErasureClassName(pt.RawClassName, raw) || len(pt.TypeArgs) != 1 {
		return nil
	}
	bound := pt.TypeArgs[0]
	if wildcard, ok := bound.(*types.JavaWildcardType); ok {
		if wildcard.Variant != "extends" || wildcard.Bound == nil {
			return nil
		}
		bound = wildcard.Bound
	}
	exception, known := types.RawClassFQN(bound)
	if !known || !types.IsReferenceSubtypeBridged(exception, "java.lang.Throwable", ctx.SiblingSuperTypes) {
		return nil
	}
	return source.Copy()
}
