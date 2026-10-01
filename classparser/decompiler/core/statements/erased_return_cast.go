package statements

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
)

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
		return values.IsNullLiteral(values.UnpackSoltValue(v)) || values.ErasedFactoryReturn(ctx, v, ret)
	}
	if !prove(value, 0) {
		return "", ""
	}
	return ft.ReturnType.String(ctx), ctx.ShortTypeName(strings.ReplaceAll(raw, "/", "."))
}
