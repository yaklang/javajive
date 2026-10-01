package javaclassparser

import (
	"maps"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A plain rethrow passes the very same exception object to the cleanup handler.
// Once those handlers are folded, the second handler's entry definition is bound
// to the surviving parameter. Retain that binding around its statements instead
// of globally renaming either identity (which may be shared with another scope).
type catchBoundStatement struct {
	statement statements.Statement
	from, to  *utils.VariableId
}

func (s *catchBoundStatement) ReplaceVar(old, next *utils.VariableId) {
	s.statement.ReplaceVar(old, next)
	if s.from == old {
		s.from = next
	}
	if s.to == old {
		s.to = next
	}
}

func (s *catchBoundStatement) String(ctx *class_context.ClassContext) string {
	if ctx == nil {
		ctx = &class_context.ClassContext{}
	}
	copy := *ctx
	return s.render(&copy, func() string { return s.statement.String(&copy) })
}

func (s *catchBoundStatement) render(ctx *class_context.ClassContext, render func() string) string {
	previous := ctx.LocalNames
	names := maps.Clone(previous)
	if names == nil {
		names = map[*utils.VariableId]string{}
	}
	name := s.to.String()
	if bound := previous[s.to]; bound != "" {
		name = bound
	}
	names[s.from] = name
	ctx.LocalNames = names
	defer func() { ctx.LocalNames = previous }()
	return render()
}

func bindCleanupParameter(body []statements.Statement, from, to *values.JavaRef) []statements.Statement {
	if from == nil || to == nil || from.Id == nil || to.Id == nil || from.Id == to.Id {
		return body
	}
	bound := make([]statements.Statement, len(body))
	for i, st := range body {
		bound[i] = &catchBoundStatement{statement: st, from: from.Id, to: to.Id}
	}
	return bound
}

// A matching source spelling is insufficient: slot reuse can give an unrelated
// exception exactly the same name. Follow explicit ATHROW and scoped bindings.
func plainCatchRethrowID(st statements.Statement) *utils.VariableId {
	if bound, ok := st.(*catchBoundStatement); ok {
		id := plainCatchRethrowID(bound.statement)
		if id == bound.from {
			return bound.to
		}
		return id
	}
	if thrown, ok := st.(*statements.CustomStatement); ok && thrown.ThrownValue != nil {
		if ref, ok := values.UnpackSoltValue(thrown.ThrownValue).(*values.JavaRef); ok && ref != nil && ref.CustomValue == nil && ref.StackVar == nil {
			return ref.Id
		}
	}
	return nil
}
