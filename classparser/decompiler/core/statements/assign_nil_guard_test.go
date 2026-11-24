package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNewAssignStatementNilGuards(t *testing.T) {
	left := values.NewJavaRef(&utils.VariableId{Name: "v"}, nil, types.NewJavaClass("java.lang.String"))

	// Plain nil right-hand value must not panic.
	stmt := NewAssignStatement(left, nil, true)
	if stmt == nil || stmt.LeftValue != left || stmt.JavaValue != nil || !stmt.IsFirst {
		t.Fatalf("unexpected statement for nil value: %+v", stmt)
	}

	// Plain nil left-hand target must not panic.
	stmt = NewAssignStatement(nil, left, false)
	if stmt == nil || stmt.LeftValue != nil || stmt.JavaValue != left || stmt.IsFirst {
		t.Fatalf("unexpected statement for nil leftVal: %+v", stmt)
	}

	// Typed-nil *values.JavaRef passed directly or wrapped in SlotValue must not panic
	// (reproduces RichTextState nil pointer dereference at ref.Val).
	var typedNilRef *values.JavaRef
	stmt = NewAssignStatement(left, typedNilRef, false)
	if stmt == nil || stmt.LeftValue != left {
		t.Fatalf("unexpected statement for typed-nil JavaRef: %+v", stmt)
	}

	wrappedNil := values.NewSlotValue(typedNilRef, nil)
	stmt = NewAssignStatement(left, wrappedNil, false)
	if stmt == nil || stmt.LeftValue != left {
		t.Fatalf("unexpected statement for SlotValue(typed-nil JavaRef): %+v", stmt)
	}

	// Typed-nil *values.SlotValue must also be tolerated gracefully.
	var typedNilSlot *values.SlotValue
	stmt = NewAssignStatement(left, typedNilSlot, false)
	if stmt == nil || stmt.LeftValue != left {
		t.Fatalf("unexpected statement for typed-nil SlotValue: %+v", stmt)
	}
}
