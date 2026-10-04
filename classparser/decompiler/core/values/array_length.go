package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// ArrayLengthExpression retains the operand and bytecode origin of ARRAYLENGTH.
// The read may throw on null; explicit dependencies are not permission to move it.
type ArrayLengthExpression struct {
	Array       JavaValue
	OriginPC    int
	HasOriginPC bool
}

func (a *ArrayLengthExpression) Type() types.JavaType {
	return types.NewJavaPrimer(types.JavaInteger)
}

func (a *ArrayLengthExpression) ReplaceVar(old, new *utils.VariableId) {
	if a != nil && !isNilJavaValue(a.Array) {
		a.Array.ReplaceVar(old, new)
	}
}

func (a *ArrayLengthExpression) String(ctx *class_context.ClassContext) string {
	if a == nil || isNilJavaValue(a.Array) {
		return ""
	}
	if err := beginValueRender(ctx); err != nil {
		return ""
	}
	defer endValueRender(ctx)
	// Check both before visiting a child and before constructing the parent.
	// Unlike fmt.Sprintf, this never allocates an over-budget parent fragment.
	if err := ctx.PreflightOutput(int64(len(".length"))); err != nil {
		return ""
	}
	operand := a.Array.String(ctx)
	if renderRejected(ctx) {
		return ""
	}
	parenthesize := false
	switch UnpackSoltValue(a.Array).(type) {
	case *AssignmentExpression, *TernaryExpression, *JavaExpression:
		parenthesize = true
	}
	size := int64(len(operand)) + int64(len(".length"))
	if parenthesize {
		size += 2
	}
	if err := ctx.PreflightOutput(size); err != nil {
		return ""
	}
	if parenthesize {
		return "(" + operand + ").length"
	}
	return operand + ".length"
}

var _ JavaValue = (*ArrayLengthExpression)(nil)
