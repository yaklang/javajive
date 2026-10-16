package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestEmptySlotAssignmentUsesStructuralIdentity(t *testing.T) {
	object := types.NewJavaClass("java.lang.Object")
	rendered := 0
	custom := values.NewCustomValue(func(*class_context.ClassContext) string { rendered++; panic("render is not evidence") }, func() types.JavaType { return object })
	allocation := &values.NewExpression{JavaType: object, ArgumentsGetter: func() string { rendered++; return "callback()" }}
	empty := values.NewSlotValue(nil, object)
	deep := values.JavaValue(empty)
	for i := 0; i < 80; i++ {
		deep = values.NewSlotValue(deep, nil)
	}
	tests := []struct {
		name  string
		value values.JavaValue
		empty bool
	}{
		{"absent", nil, true}, {"empty", empty, true}, {"forwarded", values.NewSlotValue(empty, nil), true},
		{"literal marker", values.NewJavaLiteral(values.EmptySlotValuePlaceholder, types.NewJavaClass("java.lang.String")), false},
		{"formatter", custom, false}, {"constructor callback", allocation, false},
		{"bounded", deep, false}, {"typed nil", (*values.SlotValue)(nil), false},
		{"reference", values.NewJavaRef(utils.NewRootVariableId(), empty, object), false},
		{"compound hole", &values.FunctionCallExpression{ClassName: "Owner", FunctionName: "effect", Arguments: []values.JavaValue{empty}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assignmentValueIsEmptySlot(tt.value); got != tt.empty {
				t.Fatalf("empty=%v want=%v", got, tt.empty)
			}
		})
	}
	if rendered != 0 {
		t.Fatalf("structural query invoked %d render callbacks", rendered)
	}
}

func TestEmptySlotPruningKeepsCompoundEffectsAndJavaStrings(t *testing.T) {
	typ := types.NewJavaClass("java.lang.String")
	empty := values.NewSlotValue(nil, typ)
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	literal := values.NewJavaLiteral(values.EmptySlotValuePlaceholder, typ)
	mt, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	call := &values.FunctionCallExpression{ClassName: "Owner", FunctionName: "effect", Arguments: []values.JavaValue{empty}, FuncType: mt.FunctionType(), IsStatic: true}
	dropped := statements.NewAssignStatement(ref, empty, false)
	declaration := statements.NewAssignStatement(ref, empty, true)
	declaration.IsDeclare = true
	kept := []statements.Statement{statements.NewAssignStatement(ref, literal, false), statements.NewAssignStatement(ref, call, false), declaration}
	body := append([]statements.Statement{dropped}, kept...)
	branch := &statements.IfStatement{IfBody: body, ElseBody: append([]statements.Statement(nil), body...)}
	list := []statements.Statement{branch}
	dropEmptySlotAssignments(&list)
	for _, got := range [][]statements.Statement{branch.IfBody, branch.ElseBody} {
		if len(got) != len(kept) {
			t.Fatalf("pruned observable/declaring assignment: %d", len(got))
		}
		for i, s := range got {
			if s != kept[i] {
				t.Fatalf("changed assignment identity at %d", i)
			}
		}
	}
}
