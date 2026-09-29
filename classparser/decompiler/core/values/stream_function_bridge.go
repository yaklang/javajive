package values

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

const streamMapDescriptor = "(Ljava/util/function/Function;)Ljava/util/stream/Stream;"

// A materialized getter may need Function<Optional<String>,String> so its
// method reference type-checks. An upstream mapper can still expose raw
// Optional because its nested arguments were erased. Bridge only that missing
// parameterization, preserving the function object and result type. Casting
// through raw Function changes no JVM checkcast or invocation descriptor;
// wrapping the getter in a lambda with a String cast would change behavior.
func (f *FunctionCallExpression) streamFunctionInputBridge(i int) types.JavaType {
	if f.IsStatic || !sameErasureClassName(f.ClassName, "java.util.stream.Stream") || f.FunctionName != "map" || f.Descriptor != streamMapDescriptor || i != 0 || len(f.Arguments) != 1 {
		return nil
	}
	// Poly expressions must keep their original target. This bridge only views
	// an already-created function through the same erased interface.
	arg, ok := UnpackSoltValue(f.Arguments[0]).(*JavaRef)
	if !ok {
		return nil
	}
	fn, ok := types.AsParameterizedType(arg.Type())
	if !ok || fn.RawClassName != "java.util.function.Function" || len(fn.TypeArgs) != 2 {
		return nil
	}
	if _, wildcard := fn.TypeArgs[0].(*types.JavaWildcardType); wildcard {
		return nil
	}
	input, ok := types.AsParameterizedType(fn.TypeArgs[0])
	if !ok {
		return nil
	}
	element := streamElementWitness(f.Object)
	if element == nil {
		return nil
	}
	if _, wildcard := element.(*types.JavaWildcardType); wildcard {
		return nil
	}
	raw, ok := element.RawType().(*types.JavaClass)
	if !ok || element.IsArray() || !sameErasureClassName(raw.Name, input.RawClassName) {
		return nil
	}
	return types.NewParameterizedType(fn.RawClassName, []types.JavaType{element, fn.TypeArgs[1]})
}

// Recover only exact Stream declaration facts: map's result is its Function's
// result, and filter preserves the element. Other calls supply no evidence.
func streamElementWitness(value JavaValue) types.JavaType {
	seen := map[*FunctionCallExpression]bool{}
	for value != nil {
		value = UnpackSoltValue(value)
		if value == nil {
			return nil
		}
		if call, ok := value.(*FunctionCallExpression); ok && !call.IsStatic && sameErasureClassName(call.ClassName, "java.util.stream.Stream") {
			if seen[call] {
				return nil
			}
			seen[call] = true
			if call.FunctionName == "map" && call.Descriptor == streamMapDescriptor && len(call.Arguments) == 1 {
				fn, ok := types.AsParameterizedType(call.Arguments[0].Type())
				if ok && fn.RawClassName == "java.util.function.Function" && len(fn.TypeArgs) == 2 {
					return fn.TypeArgs[1]
				}
				return nil
			}
			if call.FunctionName == "filter" && call.Descriptor == "(Ljava/util/function/Predicate;)Ljava/util/stream/Stream;" {
				value = call.Object
				continue
			}
			return nil
		}
		if stream, ok := types.AsParameterizedType(value.Type()); ok && stream.RawClassName == "java.util.stream.Stream" && len(stream.TypeArgs) == 1 {
			return stream.TypeArgs[0]
		}
		return nil
	}
	return nil
}
