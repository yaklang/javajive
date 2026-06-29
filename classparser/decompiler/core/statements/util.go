package statements

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

func StatementsString(statements []Statement, funcCtx *class_context.ClassContext) string {
	var res string
	for _, statement := range statements {
		res += statement.String(funcCtx)
	}
	return res
}
