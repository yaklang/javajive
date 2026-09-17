package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// CastExpression and AssignmentExpression retain dependencies and origin PCs,
// unlike string-producing CustomValue closures.
type CastExpression struct {
	Value      JavaValue
	TargetType types.JavaType
	OriginPC   int
}

func (c *CastExpression) Type() types.JavaType { return c.TargetType }
func (c *CastExpression) String(ctx *class_context.ClassContext) string {
	return fmt.Sprintf("((%s)(%s))", c.TargetType.String(ctx), c.Value.String(ctx))
}
func (c *CastExpression) ReplaceVar(old, new *utils.VariableId) { c.Value.ReplaceVar(old, new) }

type AssignmentExpression struct {
	Target   JavaValue
	Value    JavaValue
	OriginPC int
	Render   func(*class_context.ClassContext) string
}

func (a *AssignmentExpression) Type() types.JavaType { return a.Value.Type() }
func (a *AssignmentExpression) String(ctx *class_context.ClassContext) string {
	if a.Render != nil {
		return a.Render(ctx)
	}
	return fmt.Sprintf("%s = %s", a.Target.String(ctx), a.Value.String(ctx))
}
func (a *AssignmentExpression) ReplaceVar(old, new *utils.VariableId) {
	a.Target.ReplaceVar(old, new)
	a.Value.ReplaceVar(old, new)
}
