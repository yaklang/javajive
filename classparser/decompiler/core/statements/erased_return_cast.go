package statements

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

// A declared supertype return can consume a descriptor-exact factory result.
// Prove the reference widening from declared hierarchy metadata before using
// the existing chain planner at the producer's own erasure. The unchecked
// generic view remains outside that call; overload selection, operand order
// and any original operand checks stay unchanged. Unrelated/narrowing results
// and unavailable hierarchy evidence cannot license this adaptation.
func erasedWidenedReturnChain(ctx *class_context.ClassContext, call *values.FunctionCallExpression) (*values.FunctionCallExpression, bool) {
	if ctx == nil || call == nil || ctx.InvocationMetadata == nil {
		return nil, false
	}
	ft, ok := ctx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil {
		return nil, false
	}
	target, ok := types.AsParameterizedType(ft.ReturnType)
	if !ok || len(target.TypeArgs) == 0 {
		return nil, false
	}
	_, ret, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
	_, produced, producerErr := callbinding.Descriptor(call.Descriptor)
	if err != nil || producerErr != nil || ret == produced ||
		ret != "L"+strings.ReplaceAll(target.RawClassName, ".", "/")+";" ||
		!strings.HasPrefix(produced, "L") ||
		!callbinding.Assignable(produced, ret, ctx.InvocationMetadata) {
		return nil, false
	}
	return call.PlanErasedResultChain(ctx, produced)
}

// A cast already chosen by return lowering suppresses poly target inference.
// Bridge invariant generic types through that SAME return erasure. The extra
// raw cast cannot add a new runtime check or move the existing check, and this
// helper never introduces a cast to a previously uncast poly expression.
func renderExistingReturnCast(ctx *class_context.ClassContext, target, expr string) string {
	if ctx != nil {
		if ft, ok := ctx.FunctionType.(*types.JavaFuncType); ok && ft != nil && ft.ReturnType != nil && target == ft.ReturnType.String(ctx) {
			if p, ok := types.AsParameterizedType(ft.ReturnType); ok && len(p.TypeArgs) > 0 {
				_, ret, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
				raw := strings.ReplaceAll(p.RawClassName, ".", "/")
				if err == nil && ret == "L"+raw+";" {
					return fmt.Sprintf("return (%s) (%s) (%s)", target, ctx.ShortTypeName(strings.ReplaceAll(raw, "/", ".")), expr)
				}
			}
		}
	}
	return fmt.Sprintf("return (%s) (%s)", target, expr)
}

// The return descriptor supplies the raw view, not a guessed payload subtype.
func erasedFactoryReturnCast(ctx *class_context.ClassContext, value values.JavaValue) (string, string) {
	if ctx == nil {
		return "", ""
	}
	ft, ok := ctx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil {
		return "", ""
	}
	p, ok := types.AsParameterizedType(ft.ReturnType)
	if !ok || len(p.TypeArgs) == 0 {
		return "", ""
	}
	_, ret, e := callbinding.Descriptor(ctx.CurrentMethodDesc)
	raw := strings.ReplaceAll(p.RawClassName, ".", "/")
	if e != nil || ret != "L"+raw+";" {
		return "", ""
	}
	var prove func(values.JavaValue, int) bool
	prove = func(v values.JavaValue, depth int) bool {
		if depth > 32 {
			return false
		}
		if t, ok := values.UnpackSoltValue(v).(*values.TernaryExpression); ok {
			return prove(t.TrueValue, depth+1) && prove(t.FalseValue, depth+1)
		}
		if values.IsNullLiteral(values.UnpackSoltValue(v)) || values.ErasedFactoryReturn(ctx, v, ret) {
			return true
		}
		call, ok := values.UnpackSoltValue(v).(*values.FunctionCallExpression)
		if !ok || ctx.InvocationMetadata == nil {
			return false
		}
		_, produced, err := callbinding.Descriptor(call.Descriptor)
		// The new view is a proved widening at the final return edge. It
		// cannot check a payload, change an operand or retarget an inner call.
		return err == nil && callbinding.Assignable(produced, ret, ctx.InvocationMetadata) && values.ErasedZeroInputFactoryResult(ctx, call, produced)
	}
	if !prove(value, 0) {
		return "", ""
	}
	return ft.ReturnType.String(ctx), ctx.ShortTypeName(strings.ReplaceAll(raw, "/", "."))
}

// Zero arguments do not make an invocation a generic poly factory. A fixed
// declaration such as Container<?> make() cannot infer Container<T> from the
// enclosing return target. Use its exact static declaration and identical JVM
// result erasure to restore the unchecked source view. Genuine method-formal
// factories retain target inference; missing or ambiguous bindings fail closed.
// No call operand, dispatch tuple, origin or payload check is changed.
func fixedParameterizedFactoryReturn(ctx *class_context.ClassContext, call *values.FunctionCallExpression) bool {
	if ctx == nil || call == nil || ctx.InvocationMetadata == nil || ctx.SiblingClassSig == nil || !call.IsStatic || call.Kind != values.InvokeStatic || call.IsSpecialInvoke || len(call.Arguments) != 0 || !call.HasOriginPC {
		return false
	}
	ft, ok := ctx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil {
		return false
	}
	target, ok := types.AsParameterizedType(ft.ReturnType)
	if !ok || ft.ReturnType.IsArray() || len(target.TypeArgs) == 0 {
		return false
	}
	ps, produced, err := callbinding.Descriptor(call.Descriptor)
	_, ret, consumerErr := callbinding.Descriptor(ctx.CurrentMethodDesc)
	raw := "L" + strings.ReplaceAll(target.RawClassName, ".", "/") + ";"
	if err != nil || consumerErr != nil || len(ps) != 0 || produced != ret || ret != raw {
		return false
	}
	owner := strings.ReplaceAll(call.ClassName, ".", "/")
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: owner, Name: call.FunctionName, Desc: call.Descriptor, Kind: callbinding.Static}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || !family.Target.Static || family.Target.Bridge || family.Target.Varargs {
		return false
	}
	// Direct declaration evidence prevents an inherited name/arity match from
	// borrowing another producer's Signature. The descriptor remains part of key.
	meta, known := ctx.InvocationMetadata(owner)
	if !known || meta.Name != owner || !meta.MembersComplete {
		return false
	}
	matches := 0
	for _, method := range meta.Methods {
		if method.Name != call.FunctionName {
			continue
		}
		parameters, _, err := callbinding.Descriptor(method.Desc)
		if err != nil {
			return false
		}
		if len(parameters) == 0 {
			if method.Desc != call.Descriptor || !method.Static {
				return false
			}
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	_, signatures, known := ctx.SiblingClassSig(owner)
	if !known {
		return false
	}
	signature := signatures[class_context.MethodDescKey(call.FunctionName, call.Descriptor)]
	if !fixedFactoryResultSignature(signature) || len(types.MethodFormalTypeParamNames(signature)) != 0 || len(types.TypeVarRefsInMethodSig(signature)) != 0 {
		return false
	}
	_, parameters, source := types.ParseMethodSignatureFull(signature, ctx)
	declared, ok := types.AsParameterizedType(source)
	return ok && !source.IsArray() && len(parameters) == 0 && len(declared.TypeArgs) > 0 && strings.ReplaceAll(declared.RawClassName, ".", "/") == strings.ReplaceAll(target.RawClassName, ".", "/") && source.String(ctx) != ft.ReturnType.String(ctx)
}

// The legacy Signature parser accepts a valid prefix. Bound this proof to one
// complete reference result with no trailing token or throws-variable scope.
func fixedFactoryResultSignature(signature string) bool {
	if len(signature) > 4096 || !strings.HasPrefix(signature, "()L") {
		return false
	}
	depth := 0
	for i := 2; i < len(signature); i++ {
		switch signature[i] {
		case '<':
			depth++
			if depth > 32 {
				return false
			}
		case '>':
			depth--
			if depth < 0 {
				return false
			}
		case ';':
			if depth == 0 {
				return i == len(signature)-1
			}
		}
	}
	return false
}
