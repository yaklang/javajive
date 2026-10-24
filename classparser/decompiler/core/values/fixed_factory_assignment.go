package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// FixedFactoryAssignmentView retains an erased assignment boundary after a
// fixed array producer is reconnected into nested static factories. In Java,
// the destination's invariant generic arguments can otherwise flow backwards
// into the calls, although the JVM recorded only erased types. A same-erasure
// result view has no runtime narrowing check. Admit it only for a closed,
// non-poly tree with complete singleton invocation families: no lambda, diamond
// constructor, receiver dispatch, bare method-variable result or new overload
// can acquire a different meaning when destination inference is removed.
func FixedFactoryAssignmentView(value JavaValue, target types.JavaType, ctx *class_context.ClassContext) (JavaValue, bool) {
	if ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || target == nil || target.IsArray() {
		return value, false
	}
	if _, parameterized := types.AsParameterizedType(target); !parameterized {
		return value, false
	}
	root, ok := UnpackSoltValue(value).(*FunctionCallExpression)
	if !ok {
		return value, false
	}
	_, result, err := callbinding.Descriptor(root.Descriptor)
	if err != nil || result != bindingType(target) || !strings.HasPrefix(result, "L") {
		return value, false
	}
	budget := 2048
	var closed func(JavaValue, int) bool
	closed = func(value JavaValue, depth int) bool {
		budget--
		if budget < 0 || depth > 32 || isNilJavaValue(value) {
			return false
		}
		if value == JavaNull {
			return true
		}
		switch v := UnpackSoltValue(value).(type) {
		case *JavaLiteral, *JavaClassValue, *JavaClassMember:
			return true
		case *CastExpression:
			return v.TargetType != nil && closed(v.Value, depth+1)
		case *NewExpression:
			if v.JavaType == nil || !v.JavaType.IsArray() || v.ConstructorCall != nil {
				return false
			}
			children, known := Children(v)
			if !known {
				return false
			}
			for _, child := range children {
				if !closed(child, depth+1) {
					return false
				}
			}
			return true
		case *FunctionCallExpression:
			if !v.IsStatic || v.Kind != InvokeStatic || v.IsSpecialInvoke || !v.HasOriginPC || v.FunctionName == "<init>" {
				return false
			}
			params, ret, err := callbinding.Descriptor(v.Descriptor)
			if err != nil || len(params) != len(v.Arguments) {
				return false
			}
			effects, refs := InspectValue(v.Object)
			if effects != 0 || len(refs) != 0 {
				return false
			}
			family, err := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(v.ClassName, ".", "/"), Name: v.FunctionName, Desc: v.Descriptor}, ctx.InvocationMetadata)
			if err != nil || !family.Complete || family.Target == nil || !family.Target.Static || family.Target.Bridge || len(family.Methods) != 1 {
				return false
			}
			if family.Target.Generic {
				owner, _, sig := erasedInvocationDeclaration(ctx, strings.ReplaceAll(v.ClassName, ".", "/"), v.FunctionName, v.Descriptor)
				sig, fixed := invocationFixedThrowsBody(sig)
				_, arguments, returns := types.ParseMethodSignatureFull(sig, ctx)
				if owner == "" || !fixed || returns == nil || len(arguments) != len(params) || len(types.MethodFormalTypeParamNames(sig)) == 0 {
					return false
				}
				// A bare T (or T[]) can introduce a caller-side cast or a different
				// array runtime type. A declared parameterized class keeps its erasure.
				if _, parameterized := types.AsParameterizedType(returns); !parameterized || bindingType(returns) != ret {
					return false
				}
			}
			for i, arg := range v.Arguments {
				if !closed(arg, depth+1) {
					return false
				}
				if arg != JavaNull && (arg.Type() == nil || !callbinding.Assignable(bindingType(arg.Type()), params[i], ctx.InvocationMetadata)) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}
	if !closed(value, 0) {
		return value, false
	}
	raw, err := types.ParseDescriptor(result)
	if err != nil {
		return value, false
	}
	return &CastExpression{Value: value, TargetType: raw, Binding: true, OriginPC: root.OriginPC}, true
}
