package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// PlanErasedResultChain restores a descriptor-exact argument tuple only where
// the consumer supplies the identical result erasure. It can cross fluent
// receiver edges whose producer result is exactly the next invoke owner.
// This proof does not infer an unavailable API Signature: a raw owner and
// exact descriptor argument types preserve erased lookup. Poly expressions,
// object allocations, narrowed operands and unrelated result contexts fail
// closed. Materialized functional values retain their own SAM entry checks.
func (f *FunctionCallExpression) PlanErasedResultChain(ctx *class_context.ClassContext, result string) (*FunctionCallExpression, bool) {
	return f.planErasedResultChain(ctx, result, 0)
}
func (f *FunctionCallExpression) planErasedResultChain(ctx *class_context.ClassContext, result string, depth int) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || depth > 32 || f.IsSpecialInvoke || f.FunctionName == "<init>" || f.Kind == InvokeDynamic || ctx.Getenv("JDEC_ERASED_RESULT_CHAIN_OFF") != "" {
		return nil, false
	}
	ps, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || ret != result || !callbinding.Reference(ret) || strings.HasPrefix(ret, "[") || len(ps) != len(f.Arguments) {
		return nil, false
	}
	out := f.Clone()
	changed := false
	owner := "L" + strings.ReplaceAll(f.ClassName, ".", "/") + ";"
	valid := !f.bindingPlanned
	hasParameterized := false
	for i, arg := range f.Arguments {
		if arg == nil || arg.Type() == nil {
			valid = false
			break
		}
		actual := UnpackSoltValue(arg)
		switch actual.(type) {
		case *JavaRef:
			ref := actual.(*JavaRef)
			if ref.StackVar != nil || ref.CustomValue != nil {
				valid = false
			}
		case *RefMember, *JavaClassMember, *JavaClassValue, *JavaLiteral:
		case *CastExpression:
			// An explicit parameterized target fixes the SAM context before
			// the containing invocation is erased. Keep this inner cast and
			// its checks; a bare lambda has no such independent target.
			cast := actual.(*CastExpression)
			_, pinned := types.AsParameterizedType(cast.TargetType)
			if cast.Value == nil || (!pinned && isWitnessLambdaArg(UnpackSoltValue(cast.Value))) {
				valid = false
			}
		case *NewExpression:
			// Packed arrays have an explicit component type and rank. The
			// exact descriptor comparison below excludes covariance casts;
			// changing the containing receiver cannot retarget an initializer.
			if !actual.(*NewExpression).IsArray() {
				valid = false
			}
		default:
			valid = false
		}
		if isWitnessLambdaArg(actual) || erasedInvocationArgumentType(arg, ps[i]) != ps[i] {
			valid = false
		}
		if _, ok := types.AsParameterizedType(arg.Type()); ok {
			hasParameterized = true
		}
	}
	if !f.IsStatic && (f.Object == nil || f.Object.Type() == nil || bindingType(f.Object.Type()) != owner) {
		valid = false
	}
	// A raw producer erases the next receiver's parameter types too. Never
	// cross a use with unpinned poly or narrowed operands merely because the child
	// alone is provable; every traversed call must have an exact fixed tuple.
	if !valid {
		return nil, false
	}
	if !f.IsStatic {
		if inner, ok := UnpackSoltValue(f.Object).(*FunctionCallExpression); ok {
			if child, ok := inner.planErasedResultChain(ctx, owner, depth+1); ok {
				out.Object = child
				changed = true
			}
		}
	}
	if valid && hasParameterized {
		if !f.IsStatic {
			out.Object = &CastExpression{Value: out.Object, TargetType: types.NewJavaClass(strings.ReplaceAll(f.ClassName, "/", ".")), Binding: true, OriginPC: f.OriginPC}
		}
		for i, p := range ps {
			if callbinding.Reference(p) {
				target, _ := types.ParseDescriptor(p)
				out.Arguments[i] = &CastExpression{Value: f.Arguments[i], TargetType: target, Binding: true, OriginPC: f.OriginPC}
			}
		}
		out.bindingPlanned = true
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// PlanErasedFormalResult uses the consumer's first bound, with lexical method
// shadowing, as its JVM erasure. Only an already widening result may use this
// route: it never introduces a narrower payload check or guesses a callee's
// unavailable generic Signature.
func (f *FunctionCallExpression) PlanErasedFormalResult(ctx *class_context.ClassContext, target types.JavaType) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || target == nil || target.IsArray() {
		return nil, false
	}
	name, ok := types.RawClassFQN(target)
	if !ok || !ctx.IsTypeParam(name) {
		return nil, false
	}
	bound := ""
	for _, sig := range []string{ctx.CurrentMethodSig, ctx.ClassSig} {
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				bound = erasedInvocationBounds(sig)[name]
				break
			}
		}
		if bound != "" {
			break
		}
		// A malformed/dependent method bound still shadows a class formal.
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				return nil, false
			}
		}
	}
	_, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || bound == "" || !callbinding.Assignable(ret, bound, ctx.InvocationMetadata) {
		return nil, false
	}
	return f.PlanErasedResultChain(ctx, ret)
}
