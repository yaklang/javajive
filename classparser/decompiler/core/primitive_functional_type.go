package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

// Primitive-specialized SAMs still have reference type parameters. Erasing
// those parameters turns ToLongFunction<Buffer>.applyAsLong(Buffer) into
// applyAsLong(Object), invalidating both explicit lambda parameter types and
// members in an implicit lambda body. Bootstrap instantiatedMethodType is the
// source of these types; caller names and lambda body text are not evidence.
func inferPrimitiveFunctionalType(name string, method *types.JavaFuncType) types.JavaType {
	if method == nil {
		return nil
	}
	// R denotes one reference type argument; I/J/D/V are fixed JVM primitives.
	contracts := map[string]string{
		"java.util.function.ToIntFunction": "RI", "java.util.function.ToLongFunction": "RJ", "java.util.function.ToDoubleFunction": "RD",
		"java.util.function.ToIntBiFunction": "RRI", "java.util.function.ToLongBiFunction": "RRJ", "java.util.function.ToDoubleBiFunction": "RRD",
		"java.util.function.IntFunction": "IR", "java.util.function.LongFunction": "JR", "java.util.function.DoubleFunction": "DR",
		"java.util.function.ObjIntConsumer": "RIV", "java.util.function.ObjLongConsumer": "RJV", "java.util.function.ObjDoubleConsumer": "RDV",
	}
	contract, ok := contracts[name]
	if !ok || len(method.ParamTypes)+1 != len(contract) {
		return nil
	}
	positions := append(append([]types.JavaType(nil), method.ParamTypes...), method.ReturnType)
	var args []types.JavaType
	for i, typ := range positions {
		if typ == nil {
			return nil
		}
		primitive, isPrimitive := typ.RawType().(*types.JavaPrimer)
		if contract[i] == 'R' {
			if isPrimitive {
				return nil
			}
			args = append(args, typ)
			continue
		}
		want := map[byte]string{'I': types.JavaInteger, 'J': types.JavaLong, 'D': types.JavaDouble, 'V': types.JavaVoid}[contract[i]]
		if !isPrimitive || primitive.Name != want {
			return nil
		}
	}
	return types.NewParameterizedType(name, args)
}
