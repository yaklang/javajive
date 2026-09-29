package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// invariantGenericArgumentBridge restores an erased raw cast at a void call
// when the exact generic declaration proves a fixed invariant argument conflict
// (Metadata<K,V> -> Metadata<Boolean,U>). This cast does no JVM work: both sides
// have the invocation descriptor's erasure. It must not be inferred from a
// callee's method-variable spelling, nor applied to a poly expression or a
// value-returning call where erasure would change result inference.
func (f *FunctionCallExpression) invariantGenericArgumentBridge(i int, ctx *class_context.ClassContext) string {
	if f == nil || ctx == nil || ctx.Getenv("JDEC_INVARIANT_ARGUMENT_BRIDGE_OFF") != "" || f.FuncType == nil || f.FuncType.ReturnType == nil || f.FuncType.ReturnType.String(ctx) != "void" || i < 0 || i >= len(f.Arguments) || i >= len(f.FuncType.ParamTypes) {
		return ""
	}
	arg := f.Arguments[i]
	if arg == nil || isWitnessLambdaArg(UnpackSoltValue(arg)) {
		return ""
	}
	actual, ok := types.AsParameterizedType(arg.Type())
	if !ok {
		return ""
	}
	params, _, formals := f.genericMethodSignature(ctx)
	if i >= len(params) {
		return ""
	}
	formal, ok := types.AsParameterizedType(params[i])
	if !ok || !sameErasureClassName(formal.RawClassName, actual.RawClassName) || len(formal.TypeArgs) != len(actual.TypeArgs) {
		return ""
	}
	raw, ok := types.RawClassFQN(f.FuncType.ParamTypes[i])
	if !ok || !sameErasureClassName(raw, formal.RawClassName) {
		return ""
	}
	variables := methodVariableSet(formals)
	var conflict func(types.JavaType, types.JavaType) bool
	conflict = func(p, a types.JavaType) bool {
		if p == nil || a == nil {
			return false
		}
		if pc, ok := p.RawType().(*types.JavaClass); ok {
			ac, ok := a.RawType().(*types.JavaClass)
			return ok && !variables[pc.Name] && strings.Contains(pc.Name, ".") && !sameErasureClassName(pc.Name, ac.Name)
		}
		pp, pok := types.AsParameterizedType(p)
		ap, aok := types.AsParameterizedType(a)
		if !pok || !aok || !sameErasureClassName(pp.RawClassName, ap.RawClassName) || len(pp.TypeArgs) != len(ap.TypeArgs) {
			return false
		}
		for j := range pp.TypeArgs {
			if conflict(pp.TypeArgs[j], ap.TypeArgs[j]) {
				return true
			}
		}
		return false
	}
	for j := range formal.TypeArgs {
		if conflict(formal.TypeArgs[j], actual.TypeArgs[j]) {
			return types.NewJavaClass(raw).String(ctx)
		}
	}
	return ""
}
