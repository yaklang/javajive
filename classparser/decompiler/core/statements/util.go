package statements

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func StatementsString(statements []Statement, funcCtx *class_context.ClassContext) string {
	var res strings.Builder
	for _, statement := range statements {
		res.WriteString(statement.String(funcCtx))
	}
	return res.String()
}
