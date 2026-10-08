package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A solved local can consume a bounded method result at a proper supertype,
// without relating the caller's Class<T> to the callee's bounded N. Prove the
// original result widening and declaration-owned method formal, then use the
// existing complete overload/erasure proof for its materialized inputs. This
// permission belongs to this local store, not an enclosing poly expression.
// No result CHECKCAST, receiver evaluation or original operand check changes.
func (f *FunctionCallExpression) PlanErasedWidenedLocalResult(ctx *class_context.ClassContext, target types.JavaType) (*FunctionCallExpression, bool) {
	if f == nil || ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || !f.HasOriginPC ||
		target == nil || target.IsArray() || ctx.Getenv("JDEC_ERASED_WIDENED_LOCAL_RESULT_OFF") != "" ||
		f.IsStatic && f.Kind != InvokeStatic || !f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface {
		return nil, false
	}
	if _, parameterized := types.AsParameterizedType(target); parameterized {
		return nil, false
	}
	name, named := types.RawClassFQN(target)
	if !named || ctx.IsTypeParam(name) {
		return nil, false
	}
	for _, sig := range []string{ctx.CurrentMethodSig, ctx.ClassSig} {
		for _, formal := range types.ClassFormalTypeParamNames(sig) {
			if formal == name {
				return nil, false
			}
		}
	}
	queries := 0
	bounded := *ctx
	bounded.InvocationMetadata = func(name string) (callbinding.Class, bool) {
		queries++
		if queries > 4096 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		meta, known := ctx.InvocationMetadata(name)
		if !known || meta.Name != name || len(meta.Signature) > 4096 || len(meta.Parents)+len(meta.Methods) > 4096 ||
			ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(meta.Parents)+len(meta.Methods))) != nil {
			return callbinding.Class{}, false
		}
		for _, method := range meta.Methods {
			if method.Name == f.FunctionName && method.Desc == f.Descriptor && (len(method.Signature) > 4096 ||
				ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(meta.Signature)+len(method.Signature))) != nil) {
				return callbinding.Class{}, false
			}
		}
		return meta, true
	}
	bounded.SiblingClassSig = func(name string) (string, map[string]string, bool) {
		cs, methods, known := ctx.SiblingClassSig(name)
		key := class_context.MethodDescKey(f.FunctionName, f.Descriptor)
		sig := methods[key]
		if !known || len(cs) > 4096 || len(methods) > 4096 || len(sig) > 4096 ||
			ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(cs)+len(sig))) != nil {
			return "", nil, false
		}
		return cs, map[string]string{key: sig}, true
	}
	_, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || !strings.HasPrefix(result, "L") || !callbinding.Assignable(result, bindingType(target), bounded.InvocationMetadata) {
		return nil, false
	}
	_, _, signature := erasedInvocationDeclaration(&bounded, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
	body, fixedThrows := invocationFixedThrowsBody(signature)
	if !fixedThrows || !strings.HasPrefix(body, "<") {
		return nil, false
	}
	directFormal := false
	for formal, bound := range erasedInvocationBounds(body) {
		// Distinguish TN; from a named class LN; with the same spelling.
		if strings.HasSuffix(body, ")T"+formal+";") && bound == result {
			directFormal = true
		}
	}
	if !directFormal {
		return nil, false
	}
	// Do not retarget nested generic factories, lambdas or anonymous creation
	// through a newly raw argument. Original casts on materialized operands stay
	// in place, including casts that can fail before later arguments evaluate.
	for _, arg := range f.Arguments {
		arg = UnpackSoltValue(arg)
		for depth := 0; ; depth++ {
			if depth > 32 || arg == nil {
				return nil, false
			}
			cast, ok := arg.(*CastExpression)
			if !ok {
				break
			}
			arg = UnpackSoltValue(cast.Value)
		}
		switch value := arg.(type) {
		case *JavaRef:
			if value.StackVar != nil || value.CustomValue != nil {
				return nil, false
			}
		case *RefMember, *JavaClassMember, *JavaClassValue, *JavaLiteral:
		default:
			return nil, false
		}
	}
	return f.planErasedMethodInputProof(&bounded, true)
}
