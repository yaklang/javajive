package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// functionalFormalForErasure exposes Signature shape without using unsolved
// callee variables as source types. `Function<T,Stream<R>>` proves that a raw
// Stream result lost nested arguments even when R cannot yet be inferred. The
// ordinary functional target resolver must still reject that foreign R; this
// witness is used only to view an existing function through its exact erasure.
func (f *FunctionCallExpression) functionalFormalForErasure(i int, ctx *class_context.ClassContext) types.JavaType {
	if f == nil || ctx == nil || i < 0 || i >= len(f.Arguments) || f.Descriptor == "" {
		return nil
	}
	// collectingAndThen(Collector<T,A,R>,Function<R,RR>) fixes the finisher's
	// input R from the collector result. Only its nested shape is needed here;
	// a wildcard keeps the producer's unknown element variable out of source.
	if f.IsStatic && i == 1 && len(f.Arguments) == 2 && sameErasureClassName(f.ClassName, "java.util.stream.Collectors") &&
		f.FunctionName == "collectingAndThen" && f.Descriptor == "(Ljava/util/stream/Collector;Ljava/util/function/Function;)Ljava/util/stream/Collector;" {
		if result := collectorResultShape(f.Arguments[0]); result != nil {
			return types.NewParameterizedType("java.util.function.Function", []types.JavaType{result, &types.JavaWildcardType{}})
		}
		return nil
	}
	// These JDK declarations are outside the sibling metadata index. Match
	// owner, staticness and full descriptor before consulting their signatures.
	// No method variable from this table is printed or used to target a lambda.
	sig := ""
	if !f.IsStatic && i == 0 && len(f.Arguments) == 1 && f.FunctionName == "flatMap" {
		switch {
		case sameErasureClassName(f.ClassName, "java.util.stream.Stream") && f.Descriptor == "(Ljava/util/function/Function;)Ljava/util/stream/Stream;":
			sig = "<R:Ljava/lang/Object;>(Ljava/util/function/Function<-TT;+Ljava/util/stream/Stream<+TR;>;>;)Ljava/util/stream/Stream<TR;>;"
		case sameErasureClassName(f.ClassName, "java.util.Optional") && f.Descriptor == "(Ljava/util/function/Function;)Ljava/util/Optional;":
			sig = "<U:Ljava/lang/Object;>(Ljava/util/function/Function<-TT;Ljava/util/Optional<TU;>;>;)Ljava/util/Optional<TU;>;"
		}
	}
	if sig != "" {
		_, params, _ := types.ParseMethodSignatureFull(sig, ctx)
		if i < len(params) {
			return params[i]
		}
		return nil
	}
	if ctx.Getenv("JDEC_FUNCTIONAL_ERASURE_RESOLVE_OFF") != "" {
		return nil
	}
	params, _, _ := f.genericMethodSignature(ctx)
	if i >= len(params) {
		return nil
	}
	return params[i]
}

func collectorResultShape(value JavaValue) types.JavaType {
	if value == nil {
		return nil
	}
	if collector, ok := types.AsParameterizedType(value.Type()); ok &&
		sameErasureClassName(collector.RawClassName, "java.util.stream.Collector") && len(collector.TypeArgs) == 3 {
		return collector.TypeArgs[2]
	}
	// These zero-argument factories declare Collector<T,?,List<T>> and
	// Collector<T,?,Set<T>>. Their erased return descriptor alone is insufficient;
	// exact factory identity supplies the missing declaration. A raw local is
	// never followed back into a potentially obsolete definition.
	call, ok := UnpackSoltValue(value).(*FunctionCallExpression)
	if !ok || call == nil || !call.IsStatic || !sameErasureClassName(call.ClassName, "java.util.stream.Collectors") ||
		call.Descriptor != "()Ljava/util/stream/Collector;" || len(call.Arguments) != 0 {
		return nil
	}
	result := ""
	switch call.FunctionName {
	case "toList":
		result = "java.util.List"
	case "toSet":
		result = "java.util.Set"
	default:
		return nil
	}
	return types.NewParameterizedType(result, []types.JavaType{&types.JavaWildcardType{}})
}
