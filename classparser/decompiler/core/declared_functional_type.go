package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A jar-defined functional interface has the same bootstrap witnesses as a
// JDK Function. Match the exact SAM declaration (including inherited generic
// substitutions) to the instantiated descriptor. Without its parameterization
// a local lambda is targeted at Object and loses access to its input's members.
// Unsolved or contradictory class variables keep the original target; names or
// the rendered lambda body never supply type evidence.
func inferDeclaredFunctionalType(ctx *class_context.ClassContext, fi types.JavaType, name, samDescriptor, instantiatedDescriptor string) types.JavaType {
	if ctx == nil || ctx.SiblingClassSig == nil || fi == nil || name == "" {
		return nil
	}
	raw, ok := fi.RawType().(*types.JavaClass)
	if !ok {
		return nil
	}
	sig, _, known := ctx.SiblingClassSig(strings.ReplaceAll(raw.Name, ".", "/"))
	formals := types.ClassFormalTypeParamNames(sig)
	if !known || len(formals) == 0 {
		return nil
	}
	sam, err := types.ParseMethodDescriptor(samDescriptor)
	if err != nil || sam.FunctionType() == nil {
		return nil
	}
	inst, err := types.ParseMethodDescriptor(instantiatedDescriptor)
	if err != nil || inst.FunctionType() == nil || len(sam.FunctionType().ParamTypes) != len(inst.FunctionType().ParamTypes) {
		return nil
	}
	args := make([]types.JavaType, len(formals))
	variables := map[string]bool{}
	for i, variable := range formals {
		args[i] = types.NewJavaClass(variable)
		variables[variable] = true
	}
	params, ret, methodFormals := types.ResolveInstantiatedSignatureExact(ctx, ctx.SiblingClassSig, raw.Name, args, name, samDescriptor, len(sam.FunctionType().ParamTypes))
	if ret == nil || len(methodFormals) != 0 || len(params) != len(inst.FunctionType().ParamTypes) {
		return nil
	}
	bindings := map[string]types.JavaType{}
	var bind func(types.JavaType, types.JavaType) bool
	bind = func(pattern, actual types.JavaType) bool {
		if pattern == nil || actual == nil {
			return false
		}
		if variable, ok := pattern.RawType().(*types.JavaClass); ok && variables[variable.Name] {
			if _, primitive := actual.RawType().(*types.JavaPrimer); primitive {
				return false
			}
			if old := bindings[variable.Name]; old != nil {
				return reflect.DeepEqual(old.RawType(), actual.RawType())
			}
			bindings[variable.Name] = actual.Copy()
			return true
		}
		if pattern.IsArray() && actual.IsArray() {
			return bind(pattern.ElementType(), actual.ElementType())
		}
		// A descriptor cannot reveal arguments nested in List<T>, etc. Other
		// occurrences may solve T, but this erased position supplies no binding.
		return true
	}
	for i, pattern := range params {
		if !bind(pattern, inst.FunctionType().ParamTypes[i]) {
			return nil
		}
	}
	if !bind(ret, inst.FunctionType().ReturnType) {
		return nil
	}
	for i, variable := range formals {
		if bindings[variable] == nil {
			return nil
		}
		args[i] = bindings[variable]
	}
	return types.NewParameterizedType(raw.Name, args)
}
