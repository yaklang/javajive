package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type CustomValue struct {
	// Known captures describe lambda creation, without executing or inspecting
	// the deferred lambda body. Other custom expressions remain opaque.
	CapturesKnown  bool
	Captures       []JavaValue
	Flag           string
	NoOuterCapture bool
	// IsMethodRef distinguishes a method reference (`Type::method`, `receiver::method`, `Type::new`)
	// from an inlined lambda body (`(x) -> ...`). Both carry Flag=="lambda" (so receiver/call-site
	// functional-interface cast logic fires for both), but a method reference binds NATURALLY to a
	// raw SAM (it has no explicit parameter types to mismatch), whereas an explicitly-typed lambda
	// needs the cast to bind. A parameterized FI cast on a method reference is therefore unnecessary
	// and, when the target FI's SAM mentions nested wildcards (e.g. Stream.flatMap's
	// `Function<? super T, ? extends Stream<? extends R>>`), the cast pins a concrete parameterization
	// that defeats javac's poly inference ("method flatMap cannot be applied"). The cast helpers use
	// this flag to skip method references (fastjson2 ObjectReaderCreator.toFieldReaderArray
	// `flatMap(Collection::stream)`).
	IsMethodRef bool
	// InstantiatedMtdDesc carries a lambda/method-reference's invokedynamic instantiatedMethodType
	// descriptor (3rd LambdaMetafactory bootstrap arg), e.g. "(Ljava/lang/Throwable;[Ljava/lang/StackTraceElement;)V".
	// For a method reference passed to a constructor whose formal is a RAW functional interface (raw
	// BiConsumer.accept(Object,Object)), the bare method ref fails to bind ("invalid method reference")
	// because the SAM arity erases to (Object,Object) while the impl method is (Throwable,StackTraceElement[]).
	// The source's `(BiConsumer<Throwable,StackTraceElement[]>) Type::method` cast -- recoverable from
	// this descriptor -- re-targets the SAM so the method ref binds. Set only on the bootstrap method-ref
	// branch; consumed by ctorRawFISAMMethodRefCast (renderArgAt). Empty/unused for lambdas and non-FI uses.
	InstantiatedMtdDesc string
	StringFunc          func(funcCtx *class_context.ClassContext) string
	WriteFunc           func(funcCtx *class_context.ClassContext, out *workbudget.Writer) error
	TypeFunc            func() types.JavaType
	ReplaceFunc         func(oldId *utils.VariableId, newId *utils.VariableId)
}

// ReplaceVar implements JavaValue.
func (v *CustomValue) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if v.ReplaceFunc != nil {
		v.ReplaceFunc(oldId, newId)
	}
}

func (v *CustomValue) Type() types.JavaType {
	return v.TypeFunc()
}
func (v *CustomValue) String(funcCtx *class_context.ClassContext) string {
	guard := renderGuarded(funcCtx)
	if guard {
		if err := beginValueRender(funcCtx); err != nil {
			return ""
		}
		defer endValueRender(funcCtx)
	}
	if v.StringFunc == nil && v.WriteFunc == nil {
		return ""
	}
	if guard {
		if err := funcCtx.Work.Check(); err != nil {
			return ""
		}
	}
	if guard && funcCtx.Work.HasOutputLimit() && v.WriteFunc == nil {
		funcCtx.Work.RejectUnboundedRender()
		return ""
	}
	if v.WriteFunc != nil {
		out := workbudget.NewWriter(nil)
		if guard {
			out = workbudget.NewWriter(funcCtx.Work)
			out.SetBase(funcCtx.OutputHeld)
		}
		if err := v.WriteFunc(funcCtx, out); err != nil {
			if funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.Err() == nil {
				funcCtx.Work.FailRender(err)
			}
			return ""
		}
		if guard && renderRejected(funcCtx) {
			return ""
		}
		return out.String()
	}
	s := v.StringFunc(funcCtx)
	if renderRejected(funcCtx) {
		return ""
	}
	if !guard {
		return s
	}
	n := int64(len(s))
	if err := funcCtx.CheckAlloc(n); err != nil {
		return ""
	}
	if err := funcCtx.PreflightOutput(n); err != nil {
		return ""
	}
	return s
}

// WithType returns a shallow copy with a replacement type function. Unlike
// reconstructing a CustomValue from StringFunc, it preserves a bounded writer
// and the capture/replace metadata.
func (v *CustomValue) WithType(typeFunc func() types.JavaType) *CustomValue {
	if v == nil {
		return nil
	}
	copy := *v
	copy.TypeFunc = typeFunc
	return &copy
}

// NewCustomValue is the compatibility constructor for an opaque string
// callback. It remains usable with unlimited output or AST-depth-only budgets,
// but an explicit output cap rejects it before callback execution. Renderers
// that can include class-file-controlled text should use NewStreamingCustomValue.
func NewCustomValue(stringFun func(funcCtx *class_context.ClassContext) string, typeFunc func() types.JavaType, replaceFunc ...func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomValue {
	var rf func(oldId *utils.VariableId, newId *utils.VariableId)
	if len(replaceFunc) > 0 {
		rf = replaceFunc[0]
	}
	return &CustomValue{
		StringFunc:  stringFun,
		TypeFunc:    typeFunc,
		ReplaceFunc: rf,
	}
}

// NewStreamingCustomValue creates a CustomValue whose output is charged before
// each append when request output limits are enabled. Prefer it for any
// callback that can incorporate class-file-controlled text or child values.
func NewStreamingCustomValue(writeFun func(funcCtx *class_context.ClassContext, out *workbudget.Writer) error, typeFunc func() types.JavaType, replaceFunc ...func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomValue {
	var rf func(oldId *utils.VariableId, newId *utils.VariableId)
	if len(replaceFunc) > 0 {
		rf = replaceFunc[0]
	}
	return &CustomValue{
		WriteFunc:   writeFun,
		TypeFunc:    typeFunc,
		ReplaceFunc: rf,
	}
}
