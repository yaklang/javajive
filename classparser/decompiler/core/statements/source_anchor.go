package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
)

// SourceAnchorStatement preserves a CFG/exception-table position that has no
// explicit Java statement. It carries no hidden render callback or Java local
// operand. This says nothing about the effects of its owning JVM instruction
// (which may be an implicit superclass constructor).
type SourceAnchorStatement struct{}

func NewSourceAnchorStatement() *SourceAnchorStatement                         { return &SourceAnchorStatement{} }
func (*SourceAnchorStatement) String(*class_context.ClassContext) string       { return "" }
func (*SourceAnchorStatement) ReplaceVar(*utils.VariableId, *utils.VariableId) {}
