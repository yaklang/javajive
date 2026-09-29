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
	// A descriptor does not prove dependent or intersection bounds. In
	// particular, Assert<T> erased to Assert cannot prove A extends Assert<A,T>.
	if len(types.ClassFormalTypeParamBounds(sig, nil)) != 0 {
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
			if !descriptorHasCompleteTypeArguments(ctx, actual) {
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

// Bootstrap descriptors erase both unresolved variables and nested generic
// arguments. Object and raw generic classes therefore cannot establish an
// invariant type argument: Consumer<Collection> is not Consumer<Collection<T>>.
// Require declaration evidence that a named class has no parameters. For JDK
// classes outside the resolver, only these fixed scalar declarations are known.
func descriptorHasCompleteTypeArguments(ctx *class_context.ClassContext, actual types.JavaType) bool {
	if actual == nil {
		return false
	}
	if actual.IsArray() {
		if _, primitive := actual.ElementType().RawType().(*types.JavaPrimer); primitive {
			return true
		}
		return descriptorHasCompleteTypeArguments(ctx, actual.ElementType())
	}
	name, ok := types.ClassFQNOf(actual)
	if !ok || name == "java.lang.Object" {
		return false
	}
	if ctx != nil && ctx.SiblingClassSig != nil {
		if sig, _, known := ctx.SiblingClassSig(strings.ReplaceAll(name, ".", "/")); known {
			return len(types.ClassFormalTypeParamNames(sig)) == 0
		}
	}
	switch name {
	case "java.lang.String", "java.lang.Boolean", "java.lang.Byte", "java.lang.Character", "java.lang.Short", "java.lang.Integer", "java.lang.Long", "java.lang.Float", "java.lang.Double":
		return true
	}
	return false
}
