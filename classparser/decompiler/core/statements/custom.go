package statements

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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

// ATHROW consumes Throwable regardless of a source throws type variable. A
// source cast to a formal erased to that SAME Throwable restores the declared
// view without a narrower runtime check. Never guess from a printed name or
// use this for IOException bounds, class formals or ambiguous throws clauses.
func erasedThrowableTypeVariableView(ctx *class_context.ClassContext, value values.JavaValue) (string, bool) {
	if ctx == nil || value == nil || value.Type() == nil || ctx.CurrentMethodSig == "" {
		return "", false
	}
	operand, ok := value.Type().RawType().(*types.JavaClass)
	if !ok || operand.Name != "java.lang.Throwable" {
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
	if !strings.HasSuffix(ctx.CurrentMethodSig, "^T"+formal.Name+";") || types.ClassFormalTypeParamErasures(ctx.CurrentMethodSig)[formal.Name] != "java.lang.Throwable" {
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
