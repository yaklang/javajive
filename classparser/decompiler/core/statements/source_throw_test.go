package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSourceThrowSealsCurrentOperandRenderingAndBinding(t *testing.T) {
	s := NewThrowStatement(values.JavaNull)
	s.StringFunc = func(*class_context.ClassContext) string { t.Fatal("opaque renderer used"); return "hiddenEffect()" }
	s.replaceVar = func(*utils.VariableId, *utils.VariableId) { t.Fatal("opaque binding callback used") }
	if value, ok := s.SourceThrowOperand(); !ok || value != values.JavaNull || s.String(nil) != "throw null" {
		t.Fatal("throw null lost its complete operand")
	}
	oldID, newID := utils.NewRootVariableId(), utils.NewRootVariableId()
	ref := values.NewJavaRef(oldID, nil, types.NewJavaClass("java.lang.RuntimeException"))
	s.ThrownValue = ref
	s.ReplaceVar(oldID, newID)
	if ref.Id != newID {
		t.Fatal("current operand was not rebound")
	}
	if value, ok := s.SourceThrowOperand(); !ok || value != ref || s.String(nil) != "throw "+ref.String(nil) {
		t.Fatal("analysis and rendering disagreed after operand replacement")
	}
}

func TestSourceThrowRefusesMissingOpaqueAndConflictingCertificates(t *testing.T) {
	opaque := NewCustomStatement(func(*class_context.ClassContext) string { return "throw hiddenEffect()" }, nil)
	opaque.ThrownValue = values.JavaNull
	conflict := NewThrowStatement(values.JavaNull)
	conflict.sourceTransferKind = "break"
	for _, s := range []*CustomStatement{nil, NewThrowStatement(nil), opaque, conflict} {
		if _, ok := s.SourceThrowOperand(); ok {
			t.Fatal("incomplete or conflicting statement gained a complete operand view")
		}
	}
	if opaque.String(nil) != "throw hiddenEffect()" {
		t.Fatal("legacy opaque rendering changed")
	}
}
