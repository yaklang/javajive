package statements

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

type CustomStatement struct {
	sourceTransferKind string
	Name               string
	Info               any
	// Labeled transfers retain their target separately from the render closure.
	// A protected-region proof accepts them only while that target is enclosed.
	LoopTransferKind string
	LoopTargetLabel  string
	// ThrownValue retains ATHROW's dependency without changing its rendering.
	// Region proofs must not infer a thrown operand from an opaque closure.
	ThrownValue values.JavaValue
	OriginPC    int
	HasOriginPC bool
	StringFunc  func(funcCtx *class_context.ClassContext) string
	replaceVar  func(oldId *utils.VariableId, newId *utils.VariableId)
}

// ReplaceVar implements Statement.
func (v *CustomStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if v.sourceTransferKind != "" {
		return
	}
	v.replaceVar(oldId, newId)
}

func (v *CustomStatement) String(funcCtx *class_context.ClassContext) string {
	if v.sourceTransferKind != "" {
		if v.LoopTargetLabel == "" {
			return v.sourceTransferKind
		}
		return v.sourceTransferKind + " " + v.LoopTargetLabel
	}
	if name, ok := erasedThrowableTypeVariableView(funcCtx, v.ThrownValue); ok {
		return fmt.Sprintf("throw (%s) (%s)", name, v.ThrownValue.String(funcCtx))
	}
	return v.StringFunc(funcCtx)
}

// A throws formal has an erased JVM view. Restoring that formal is harmless
// only when its original first bound is exactly the operand's static erasure:
// the source cast adds no narrower runtime check. Original hierarchy evidence
// must also prove Throwable membership. A wider Throwable, subtype operand,
// free class formal or ambiguous throws list does not license this view.
func erasedThrowableTypeVariableView(ctx *class_context.ClassContext, value values.JavaValue) (string, bool) {
	if ctx == nil || value == nil || value.Type() == nil || ctx.CurrentMethodSig == "" {
		return "", false
	}
	operand, ok := value.Type().RawType().(*types.JavaClass)
	if !ok {
		return "", false
	}
	if len(ctx.CurrentMethodSig) > 4096 || ctx.Work != nil && (ctx.Work.Charge(workbudget.CounterGraphScans, int64(len(ctx.CurrentMethodSig))*130+1) != nil || ctx.Work.CheckAlloc(int64(len(ctx.CurrentMethodSig))*256) != nil) {
		return "", false
	}
	_, _, result, throws := types.ParseMethodSignatureFullWithThrows(ctx.CurrentMethodSig, ctx)
	if result == nil || len(throws) != 1 || throws[0] == nil {
		return "", false
	}
	formal, ok := throws[0].RawType().(*types.JavaClass)
	if !ok || !ctx.IsTypeParam(formal.Name) {
		return "", false
	}
	if !strings.HasSuffix(ctx.CurrentMethodSig, "^T"+formal.Name+";") || types.ClassFormalTypeParamErasures(ctx.CurrentMethodSig)[formal.Name] != operand.Name {
		return "", false
	}
	visited := 0
	provider := func(name string) (callbinding.Class, bool) {
		visited++
		if visited > 128 || ctx.InvocationMetadata == nil || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		c, known := ctx.InvocationMetadata(name)
		if !known || c.Name != name || len(c.Parents) > 64 {
			return callbinding.Class{}, false
		}
		if ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(c.Parents))*256) != nil {
			return callbinding.Class{}, false
		}
		return c, true
	}
	if !callbinding.Assignable("L"+strings.ReplaceAll(operand.Name, ".", "/")+";", "Ljava/lang/Throwable;", provider) || visited > 128 || ctx.Work != nil && ctx.Work.Err() != nil {
		return "", false
	}
	return formal.Name, true
}
func NewCustomStatement(stringFun func(funcCtx *class_context.ClassContext) string, replaceVar func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomStatement {
	return &CustomStatement{
		StringFunc: stringFun,
		replaceVar: replaceVar,
	}
}
