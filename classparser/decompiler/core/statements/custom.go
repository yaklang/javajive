package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type CustomStatement struct {
	Name string
	Info any
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
	v.replaceVar(oldId, newId)
}

func (v *CustomStatement) String(funcCtx *class_context.ClassContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomStatement(stringFun func(funcCtx *class_context.ClassContext) string, replaceVar func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomStatement {
	return &CustomStatement{
		StringFunc: stringFun,
		replaceVar: replaceVar,
	}
}
