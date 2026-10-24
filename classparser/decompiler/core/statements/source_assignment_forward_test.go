package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"testing"
)

func TestSourceAssignmentForwardRetainsLiveIRAndBinding(t *testing.T) {
	old, newID := utils.NewRootVariableId(), utils.NewRootVariableId()
	old.Next()
	newID.Next()
	typ := types.NewJavaPrimer(types.JavaInteger)
	left := values.NewJavaRef(old, nil, typ)
	assignment := &AssignStatement{LeftValue: left, JavaValue: values.NewJavaLiteral(7, typ)}
	forward := NewSourceAssignmentForward(assignment)
	if reflect.DeepEqual(forward, NewSourceAssignmentForward(assignment)) {
		t.Fatal("distinct physical forwarding sites became structurally interchangeable")
	}
	if got, ok := forward.SourceAssignmentForward(); !ok || got != assignment {
		t.Fatal("original assignment identity lost")
	}
	forward.StringFunc = func(*class_context.ClassContext) string { t.Fatal("opaque renderer used"); return "throw hidden()" }
	forward.replaceVar = func(*utils.VariableId, *utils.VariableId) { t.Fatal("opaque rename used") }
	if _, ok := forward.SourceThrowOperand(); ok || forward.HasSourceTransfer() || forward.RetargetSourceTransfer("break", "") {
		t.Fatal("assignment gained control transfer permission")
	}
	ctx := &class_context.ClassContext{}
	if forward.String(ctx) != assignment.String(ctx) {
		t.Fatal("forwarded source differs")
	}
	assignment.JavaValue = values.NewJavaLiteral(23, typ)
	if forward.String(ctx) != assignment.String(ctx) {
		t.Fatal("cached premature constructor text")
	}
	forward.ReplaceVar(old, newID)
	if left.Id != newID {
		t.Fatal("late variable binding was not forwarded")
	}
	if NewSourceAssignmentForward(nil) != nil {
		t.Fatal("missing assignment admitted")
	}
	var absent *CustomStatement
	if _, ok := absent.SourceAssignmentForward(); ok {
		t.Fatal("nil admitted")
	}
	opaque := NewCustomStatement(func(*class_context.ClassContext) string { return "var0 = 7" }, nil)
	opaque.Info = assignment
	if _, ok := opaque.SourceAssignmentForward(); ok {
		t.Fatal("public annotation created sealed role")
	}
}

func TestSourceStatementRolesAreDisjoint(t *testing.T) {
	assignment := &AssignStatement{LeftValue: values.JavaNull, JavaValue: values.JavaNull}
	for _, role := range []string{"assignment with throw", "assignment with transfer", "transfer with throw"} {
		t.Run(role, func(t *testing.T) {
			var leaf *CustomStatement
			switch role {
			case "assignment with throw":
				leaf = NewSourceAssignmentForward(assignment)
				leaf.sourceThrow = true
				leaf.ThrownValue = values.JavaNull
			case "assignment with transfer":
				leaf = NewSourceAssignmentForward(assignment)
				leaf.sourceTransferKind = "break"
			case "transfer with throw":
				leaf = NewSourceTransferStatement("continue", "")
				leaf.sourceThrow = true
				leaf.ThrownValue = values.JavaNull
			}
			if _, ok := leaf.SourceAssignmentForward(); ok {
				t.Fatal("contradictory assignment role accepted")
			}
			if _, ok := leaf.SourceThrowOperand(); ok {
				t.Fatal("contradictory throw role accepted")
			}
			if leaf.SourceTransferOnly() {
				t.Fatal("contradictory transfer role accepted")
			}
		})
	}
}
