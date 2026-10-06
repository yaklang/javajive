package statements

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type DoWhileStatement struct {
	Label          string
	ConditionValue values.JavaValue
	Body           []Statement
}

// ReplaceVar implements Statement.
func (w *DoWhileStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	w.ConditionValue.ReplaceVar(oldId, newId)
	for _, st := range w.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewDoWhileStatement(condition values.JavaValue, body []Statement) *DoWhileStatement {
	return &DoWhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}

func (w *DoWhileStatement) String(funcCtx *class_context.ClassContext) string {
	// Update order was proved from original stack loads in the opcode analyser.
	// Rendering must not convert a preceding local update into a post-update
	// based only on the shape of the following loop guard.
	normalizedBody := w.Body
	// Branch polarity belongs to the structured condition and its arms.
	// Comparison spelling alone cannot determine which arm exits the loop.
	parts := make([]string, 0, len(normalizedBody))
	for _, statement := range normalizedBody {
		parts = append(parts, statement.String(funcCtx))
	}
	// Opaque compatibility leaves do not carry their own separators. Keep
	// the statement boundaries used by declaration/reference recovery.
	body := strings.Join(parts, "\n")
	s := fmt.Sprintf("do{\n%s\n}while(%s)", body, w.ConditionValue.String(funcCtx))
	if w.Label != "" {
		return fmt.Sprintf("%s: %s", w.Label, s)
	}
	return s
}

type WhileStatement struct {
	ConditionValue values.JavaValue
	Body           []Statement
}

// ReplaceVar implements Statement.
func (w *WhileStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	w.ConditionValue.ReplaceVar(oldId, newId)
	for _, st := range w.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewWhileStatement(condition values.JavaValue, body []Statement) *WhileStatement {
	return &WhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}
func (w *WhileStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("while(%s) {\n%s\n}", w.ConditionValue.String(funcCtx), StatementsString(w.Body, funcCtx))
}

type TryCatchStatement struct {
	Exception   []*values.JavaRef
	TryBody     []Statement
	CatchBodies [][]Statement
	// Handlers preserves raw exception-table evidence in CatchBodies order.
	// A typed Throwable catch is distinct from catch_type == 0 (finally).
	Handlers []CatchHandler
	// EntryInitializers retains decoded, private predecessor assignments. A
	// resource-finally proof needs a real null initialization, not a printed name
	// or an assumption about an arbitrary local's value on entry.
	EntryInitializers []*AssignStatement
}

type CatchHandler struct {
	EntryPC         int
	CatchAll        bool
	ProtectedRanges [][2]int // half-open bytecode ranges
}

// ReplaceVar implements Statement.
func (w *TryCatchStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, exception := range w.Exception {
		exception.ReplaceVar(oldId, newId)
	}
	for _, body := range w.TryBody {
		body.ReplaceVar(oldId, newId)
	}
	// A handler can read a local defined before the try. The same identity
	// rebinding must reach every handler, including nested catch bodies; a
	// coincidentally equal temporary name is not a binding.
	for _, handler := range w.CatchBodies {
		for _, body := range handler {
			body.ReplaceVar(oldId, newId)
		}
	}
}

func NewTryCatchStatement(body1 []Statement, body2 [][]Statement) *TryCatchStatement {
	return &TryCatchStatement{
		TryBody:     body1,
		CatchBodies: body2,
	}
}
func (w *TryCatchStatement) String(funcCtx *class_context.ClassContext) string {
	bodies := []string{}
	for _, body := range w.CatchBodies {
		bodies = append(bodies, StatementsString(body, funcCtx))
	}
	s := fmt.Sprintf("try{\n%s\n}", StatementsString(w.TryBody, funcCtx))
	for _, body := range bodies {
		s += fmt.Sprintf("catch{\n%s\n}", body)
	}
	return s
}
