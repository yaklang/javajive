package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// ArrayLengthExpression retains the operand and bytecode origin of ARRAYLENGTH.
// The read may throw on null; explicit dependencies are not permission to move it.
type ArrayLengthExpression struct {
	Array             JavaValue
	ExpectedArrayType types.JavaType
	OriginPC          int
	HasOriginPC       bool
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
	var castType string
	var useReflect bool
	if at := a.Array.Type(); (at == nil || !at.IsArray()) && a.ExpectedArrayType != nil && a.ExpectedArrayType.IsArray() {
		castType = a.ExpectedArrayType.String(ctx)
	} else if at != nil && !at.IsArray() {
		if jc, ok := at.RawType().(*types.JavaClass); ok && jc != nil && jc.Name == "java.lang.Object" {
			useReflect = true
		}
	}
	// Check both before visiting a child and before constructing the parent.
	// Unlike fmt.Sprintf, this never allocates an over-budget parent fragment.
	const reflectPrefix = "java.lang.reflect.Array.getLength("
	var fixedLen int
	if useReflect {
		fixedLen = len(reflectPrefix) + 1
	} else if castType != "" {
		fixedLen = len("((") + len(castType) + len(")()).length")
	} else {
		fixedLen = len(".length")
	}
	if err := ctx.PreflightOutput(int64(fixedLen)); err != nil {
		return ""
	}
	operand := a.Array.String(ctx)
	if renderRejected(ctx) {
		return ""
	}
	if useReflect {
		if err := ctx.PreflightOutput(int64(len(operand)) + int64(fixedLen)); err != nil {
			return ""
		}
		return reflectPrefix + operand + ")"
	}
	if castType != "" {
		if err := ctx.PreflightOutput(int64(len(operand)) + int64(fixedLen)); err != nil {
			return ""
		}
		return "((" + castType + ")(" + operand + ")).length"
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
