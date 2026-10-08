package statements

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A field's Signature survives in Java source while PUTFIELD consumes only its
// descriptor erasure. A fixed instance result in a conditional can inherit
// inferred receiver arguments which no longer match that Signature. Restore
// the raw result view only after proving every leaf widens to the actual field
// descriptor. The outer view leaves selection, receivers, operands and original
// payload checks intact. It cannot retarget a generic method or a bare lambda.
func conditionalGenericAssignmentBridge(ctx *class_context.ClassContext, left, value values.JavaValue) string {
	if ctx == nil || left == nil || value == nil || ctx.InvocationMetadata == nil || ctx.Getenv("JDEC_CONDITIONAL_GENERIC_ASSIGNMENT_OFF") != "" {
		return ""
	}
	if _, ok := values.UnpackSoltValue(value).(*values.TernaryExpression); !ok {
		return ""
	}
	target := sameClassFieldGenericType(ctx, left)
	pt, ok := types.AsParameterizedType(target)
	if !ok || target.IsArray() || len(pt.TypeArgs) == 0 || strings.Contains(target.String(ctx), "?") {
		return ""
	}
	raw, ok := types.RawClassFQN(left.Type())
	if !ok || raw != pt.RawClassName || ctx.IsTypeParam(raw) {
		return ""
	}
	descriptor := "L" + strings.ReplaceAll(raw, ".", "/") + ";"
	// Bound both expression visits and original hierarchy/member queries. Budget
	// refusal publishes no partial adaptation, and uses the request's ledger.
	queries, nodes := 0, 0
	bounded := *ctx
	bounded.InvocationMetadata = func(name string) (callbinding.Class, bool) {
		queries++
		if queries > 4096 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		meta, known := ctx.InvocationMetadata(name)
		if !known || meta.Name != name || !meta.ParentsComplete {
			return callbinding.Class{}, false
		}
		if len(meta.Parents)+len(meta.Methods) > 4096 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(meta.Parents)+len(meta.Methods))) != nil {
			return callbinding.Class{}, false
		}
		return meta, known
	}
	hiddenGenericResult := false
	var prove func(values.JavaValue, int) bool
	prove = func(v values.JavaValue, depth int) bool {
		nodes++
		if v == nil || depth > 32 || nodes > 256 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		v = values.UnpackSoltValue(v)
		if ternary, ok := v.(*values.TernaryExpression); ok {
			return ternary.Condition != nil && prove(ternary.TrueValue, depth+1) && prove(ternary.FalseValue, depth+1)
		}
		if values.IsNullLiteral(v) {
			return true
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			// Inferred receiver arguments may give the source call a proper
			// subtype. Its physical result, not that recovered type, is the
			// original PUTFIELD operand. The fixed declaration proof also
			// binds a class formal's original erasure before any adaptation.
			_, result, err := callbinding.Descriptor(call.Descriptor)
			if err != nil || !callbinding.Assignable(result, descriptor, bounded.InvocationMetadata) || !values.ErasedFixedInstanceResult(&bounded, call, result) {
				return false
			}
			hiddenGenericResult = true
			return true
		}
		raw, ok := types.RawClassFQN(values.TernaryArmRValueType(v))
		if !ok || !callbinding.Assignable("L"+strings.ReplaceAll(raw, ".", "/")+";", descriptor, bounded.InvocationMetadata) {
			return false
		}
		switch leaf := v.(type) {
		case *values.JavaRef:
			return leaf.StackVar == nil && leaf.CustomValue == nil
		case *values.RefMember, *values.JavaClassMember:
			return true
		default:
			return false
		}
	}
	if !prove(value, 0) || !hiddenGenericResult {
		return ""
	}
	return types.NewJavaClass(raw).String(ctx)
}
