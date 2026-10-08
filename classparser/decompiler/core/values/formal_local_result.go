package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// SourceFormalLocalResult recovers a caller formal from the original generic
// invocation's invariant argument witnesses. The store's computational type
// remains its descriptor erasure. Only a first local declaration at that exact
// erasure may adopt this view; an explicit whole-web consumer takes precedence.
// No call, operand check, evaluation or mutable expression type is changed.
func (f *FunctionCallExpression) SourceFormalLocalResult(ctx *class_context.ClassContext, consumer types.JavaType) types.JavaType {
	if f == nil || ctx == nil || !f.HasOriginPC || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || consumer == nil ||
		ctx.Getenv("JDEC_SOURCE_FORMAL_LOCAL_RESULT_OFF") != "" ||
		f.IsStatic && f.Kind != InvokeStatic || !f.IsStatic && f.Kind != InvokeVirtual && f.Kind != InvokeInterface ||
		consumer.IsArray() || len(f.Arguments) > 255 || len(f.Descriptor)+len(ctx.ClassSig)+len(ctx.CurrentMethodSig) > 12288 {
		return nil
	}
	if _, parameterized := types.AsParameterizedType(consumer); parameterized {
		return nil
	}
	ps, result, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != len(f.Arguments) || !strings.HasPrefix(result, "L") || bindingType(consumer) != result {
		return nil
	}
	if ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(f.Arguments)+512)*64) != nil {
		return nil
	}
	bounded := *ctx
	queries := 0
	bounded.InvocationMetadata = func(n string) (callbinding.Class, bool) {
		queries++
		if queries > 256 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		meta, ok := ctx.InvocationMetadata(n)
		if !ok || meta.Name != n || len(meta.Methods)+len(meta.Parents) > 4096 || len(meta.Signature) > 4096 ||
			ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(meta.Methods)+len(meta.Parents)+len(meta.Signature))) != nil {
			return callbinding.Class{}, false
		}
		for _, m := range meta.Methods {
			if m.Name == f.FunctionName && m.Desc == f.Descriptor && len(m.Signature) > 4096 {
				return callbinding.Class{}, false
			}
		}
		return meta, true
	}
	bounded.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		queries++
		if queries > 256 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return "", nil, false
		}
		cs, methods, ok := ctx.SiblingClassSig(n)
		key := class_context.MethodDescKey(f.FunctionName, f.Descriptor)
		if !ok || len(cs)+len(methods[key]) > 8192 || len(methods) > 4096 ||
			ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(cs)+len(methods[key]))) != nil {
			return "", nil, false
		}
		return cs, map[string]string{key: methods[key]}, true
	}
	_, cs, sig := erasedInvocationDeclaration(&bounded, strings.ReplaceAll(f.ClassName, ".", "/"), f.FunctionName, f.Descriptor)
	if len(cs)+len(sig) > 8192 || !strings.HasPrefix(sig, "<") || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(cs)+len(sig))+1) != nil {
		return nil
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{cs}, sig)
	if !valid || erased != f.Descriptor {
		return nil
	}
	// Keep the structural solver on this exact original declaration, including
	// same-class calls; a separately cached arity/signature view is not evidence.
	if strings.ReplaceAll(f.ClassName, ".", "/") == strings.ReplaceAll(ctx.ClassName, ".", "/") {
		bounded.MethodSignaturesByDesc = map[string]string{class_context.MethodDescKey(f.FunctionName, f.Descriptor): sig}
	}
	// The existing structural solver recursively examines types. Precharge and
	// bound that graph first, rejecting cyclic, deep or owner-dependent views.
	nodes := 0
	active := map[types.JavaType]bool{}
	var shape func(types.JavaType, int) bool
	shape = func(typ types.JavaType, depth int) bool {
		nodes++
		if typ == nil || depth > 32 || nodes > 512 || active[typ] || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		active[typ] = true
		defer delete(active, typ)
		if typ.IsArray() {
			return shape(typ.ElementType(), depth+1)
		}
		if p, ok := types.AsParameterizedType(typ); ok {
			if len(p.OwnerSegments) > 1 || len(p.TypeArgs) > 255 {
				return false
			}
			for _, arg := range p.TypeArgs {
				if !shape(arg, depth+1) {
					return false
				}
			}
		} else if w, ok := typ.(*types.JavaWildcardType); ok {
			return w != nil && w.Bound != nil && shape(w.Bound, depth+1)
		}
		return true
	}
	for i, arg := range f.Arguments {
		if arg == nil {
			return nil
		}
		value := UnpackSoltValue(arg)
		for depth := 0; ; depth++ {
			if depth > 32 || value == nil {
				return nil
			}
			cast, ok := value.(*CastExpression)
			if !ok {
				break
			}
			value = UnpackSoltValue(cast.Value)
		}
		switch v := value.(type) {
		case *JavaRef:
			if v.StackVar != nil || v.CustomValue != nil {
				return nil
			}
		case *RefMember, *JavaClassMember, *JavaClassValue, *JavaLiteral:
		default:
			return nil
		}
		if !shape(arg.Type(), 0) {
			return nil
		}
		physical, known := SourceTypeErasure(arg.Type(), &bounded)
		if _, literal := value.(*JavaClassValue); literal {
			physical, known = "Ljava/lang/Class;", true
		}
		if !known || !callbinding.Assignable(physical, ps[i], bounded.InvocationMetadata) {
			return nil
		}
	}
	inferred := f.inferredGenericMethodReturn(&bounded)
	formal, known := types.RawClassFQN(inferred)
	if !known || !ctx.IsTypeParam(formal) {
		return nil
	}
	original, known := SourceTypeErasure(inferred, &bounded)
	if !known || original != result {
		return nil
	}
	return inferred
}
