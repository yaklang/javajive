package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A result view can suppress a varargs factory's return-target inference only
// when its existing source argument independently fixes a fixed-arity call.
// The original CHECKCAST is that boundary: changing the surrounding result
// context cannot change inference inside this explicit cast. Keep its exact
// target and PC; an array input or even <T>T producer Signature proves no
// identity or narrower runtime result. The caller supplies the result's same
// erasure view; this proof never changes the producer or any argument.
func erasedOriginalArrayFactoryResult(ctx *class_context.ClassContext, f *FunctionCallExpression, result string) bool {
	if ctx == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || f == nil || !f.IsStatic || f.Kind != InvokeStatic || f.IsSpecialInvoke ||
		!f.HasOriginPC || f.OriginPC < 0 || f.OriginPC > 65535 || len(f.Arguments) != 1 || f.FuncType == nil ||
		len(f.FuncType.ParamTypes) != 1 || f.FuncType.ReturnType == nil || !strings.HasPrefix(result, "L") {
		return false
	}
	ps, ret, err := callbinding.Descriptor(f.Descriptor)
	if err != nil || len(ps) != 1 || ps[0] != "[Ljava/lang/Object;" || ret != result ||
		bindingType(f.FuncType.ParamTypes[0]) != ps[0] || bindingType(f.FuncType.ReturnType) != ret {
		return false
	}
	owner := strings.ReplaceAll(f.ClassName, ".", "/")
	meta, known := ctx.InvocationMetadata(owner)
	if !known || meta.Name != owner || !meta.MembersComplete || !meta.ParentsComplete || meta.IsInterface || len(meta.Methods) > 4096 {
		return false
	}
	var signature string
	matches := 0
	for _, method := range meta.Methods {
		if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if method.Name == f.FunctionName && method.Desc == f.Descriptor {
			if !method.Static || !method.Varargs || method.Bridge || !method.Public {
				return false
			}
			matches++
			signature = method.Signature
		}
	}
	if matches != 1 || len(signature) == 0 || len(signature) > 8192 || strings.Contains(signature, "^") ||
		ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(signature))*130+1) != nil || ctx.Work.CheckAlloc(int64(len(signature))*256+128) != nil) {
		return false
	}
	// Platform declarations can be known only to InvocationMetadata. Keep this
	// evidence local to the erasure proof; publishing it into legacy generic
	// inference would change unrelated source arguments and captures.
	_, signatures, known := invocationSignatureEvidence(ctx, owner)
	if !known || signatures[class_context.MethodDescKey(f.FunctionName, f.Descriptor)] != signature {
		return false
	}
	erasure, throws, valid := types.EraseLexicalMethodSignatureWithThrows("", signature)
	if !valid || erasure != f.Descriptor || len(throws) != 0 {
		return false
	}
	formals, refs, valid := types.SignatureTypeVariableReferences(signature)
	bounds := erasedInvocationBounds(signature)
	if !valid || len(formals) != 1 || len(bounds) != 1 || bounds[formals[0]] != "Ljava/lang/Object;" {
		return false
	}
	if !strings.HasPrefix(signature, "<"+formals[0]+":Ljava/lang/Object;>(") {
		return false // Additional or dependent bounds need a separate inference proof.
	}
	for _, ref := range refs {
		if ref != formals[0] {
			return false
		}
	}
	_, params, produced := types.ParseMethodSignatureFull(signature, ctx)
	if len(params) != 1 || params[0] == nil || !params[0].IsArray() || params[0].ElementType() == nil ||
		bindingType(params[0].ElementType()) != "L"+formals[0]+";" || erasedMethodType(produced, bounds) != result {
		return false
	}
	if p, parameterized := types.AsParameterizedType(produced); !parameterized || len(p.TypeArgs) == 0 || !methodTypeMentionsFormal(produced, bounds) {
		return false
	}
	cast, ok := UnpackSoltValue(f.Arguments[0]).(*CastExpression)
	if !ok || cast == nil || cast.Value == nil || cast.TargetType == nil || !cast.TargetType.IsArray() || cast.OriginPC >= f.OriginPC {
		return false
	}
	_, physical, original := cast.OriginalCheckCastWitness(ctx)
	// Every reference array is applicable to this T[] formal as a fixed
	// array, including a primitive-array component (for example int[][]).
	// Primitive one-dimensional arrays cannot satisfy Object[].
	return original && strings.HasPrefix(physical, "[") && bindingType(cast.TargetType) == physical &&
		callbinding.Assignable(physical, ps[0], ctx.InvocationMetadata)
}
