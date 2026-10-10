package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"strings"
)

// A conditional's descriptor type hides the generic type of a field arm.
// `flag ? new RawWrapper(field) : field` can therefore appear to be raw R,
// although source javac sees R<? super T> on the second arm. A declared R<T>
// return needs the source's erased unchecked conversion. Verify every leaf's
// erasure and require an actual wildcard-to-invariant conflict, then cast the
// whole result: selection and evaluation order are unchanged.
func conditionalGenericReturnBridge(ctx *class_context.ClassContext, value values.JavaValue) (string, string) {
	if ctx == nil || ctx.Getenv("JDEC_CONDITIONAL_GENERIC_RETURN_OFF") != "" {
		return "", ""
	}
	ft, ok := ctx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil {
		return "", ""
	}
	target, ok := types.AsParameterizedType(ft.ReturnType)
	if !ok {
		return "", ""
	}
	if _, ok := values.UnpackSoltValue(value).(*values.TernaryExpression); !ok {
		return "", ""
	}
	conflict := false
	var visit func(values.JavaValue, int) bool
	visit = func(v values.JavaValue, depth int) bool {
		if v == nil || depth > 64 {
			return false
		}
		v = values.UnpackSoltValue(v)
		if ternary, ok := v.(*values.TernaryExpression); ok {
			return visit(ternary.TrueValue, depth+1) && visit(ternary.FalseValue, depth+1)
		}
		if values.IsNullLiteral(v) {
			return true
		}
		typ := v.Type()
		if field := values.RecoverThisFieldInstantiatedType(ctx, v); field != nil {
			typ = field
		}
		raw, ok := types.RawClassFQN(typ)
		if !ok {
			return false
		}
		if raw != target.RawClassName && !types.IsReferenceSubtypeBridged(raw, target.RawClassName, ctx.SiblingSuperTypes) {
			return false
		}
		actual, ok := types.AsParameterizedType(typ)
		if ok && actual.RawClassName == target.RawClassName && len(actual.TypeArgs) == len(target.TypeArgs) {
			for i, a := range actual.TypeArgs {
				if _, wild := a.(*types.JavaWildcardType); !wild {
					continue
				}
				if _, wild := target.TypeArgs[i].(*types.JavaWildcardType); wild {
					continue
				}
				if !reflect.DeepEqual(a.RawType(), target.TypeArgs[i].RawType()) {
					conflict = true
				}
			}
		}
		return true
	}
	if !visit(value, 0) || !conflict {
		return "", ""
	}
	return ft.ReturnType.String(ctx), ctx.ShortTypeName(strings.ReplaceAll(target.RawClassName, "/", "."))
}
