package values

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func renderGuarded(funcCtx *class_context.ClassContext) bool {
	if funcCtx == nil || funcCtx.Work == nil {
		return false
	}
	return funcCtx.Work.RenderGuarded()
}

func beginValueRender(funcCtx *class_context.ClassContext) error {
	if !renderGuarded(funcCtx) {
		return nil
	}
	if err := funcCtx.Work.Check(); err != nil {
		return err
	}
	return funcCtx.Work.Enter(workbudget.CounterASTDepth)
}

func endValueRender(funcCtx *class_context.ClassContext) {
	if !renderGuarded(funcCtx) {
		return
	}
	funcCtx.Work.Leave(workbudget.CounterASTDepth)
}

func renderRejected(funcCtx *class_context.ClassContext) bool {
	return funcCtx != nil && funcCtx.Work != nil && funcCtx.Work.Err() != nil
}

// literalExactOutputBytes is the exact emitted length of this node alone.
// It does not recurse into descendants (no quadratic uncharged walk).
func literalExactOutputBytes(j *JavaLiteral, funcCtx *class_context.ClassContext) int64 {
	if j == nil || j.JavaType == nil {
		return 0
	}
	typeStr := j.JavaType.String(funcCtx)
	switch typeStr {
	case types.NewJavaPrimer(types.JavaBoolean).String(funcCtx):
		if v, ok := j.Data.(int); ok && v == 0 {
			return 5 // false
		}
		return 4 // true
	case types.NewJavaPrimer(types.JavaLong).String(funcCtx):
		s := fmt.Sprint(j.Data)
		if s != "" && !hasLongSuffix(s) {
			s += "L"
		}
		return int64(len(s))
	case types.NewJavaPrimer(types.JavaChar).String(funcCtx):
		if u, ok := javaLiteralCharUnit(j); ok {
			return int64(JavaUnitCharLiteralLen(u))
		}
	}
	if typeStr == "java.lang.String" || typeStr == "String" {
		return int64(JavaStringLiteralOutputBytes(j.Data, j.Units))
	}
	if typeStr == types.NewJavaPrimer(types.JavaInteger).String(funcCtx) ||
		typeStr == types.NewJavaPrimer(types.JavaByte).String(funcCtx) ||
		typeStr == types.NewJavaPrimer(types.JavaShort).String(funcCtx) {
		return int64(len(fmt.Sprint(j.Data)))
	}
	return int64(len(fmt.Sprint(j.Data)))
}

func hasLongSuffix(s string) bool {
	return len(s) > 0 && (s[len(s)-1] == 'L' || s[len(s)-1] == 'l')
}

func finishExpressionRender(funcCtx *class_context.ClassContext, guard bool, out string) string {
	if !guard {
		return out
	}
	if renderRejected(funcCtx) {
		return ""
	}
	n := int64(len(out))
	if err := funcCtx.CheckAlloc(n); err != nil {
		return ""
	}
	if err := funcCtx.PreflightOutput(n); err != nil {
		return ""
	}
	return out
}
