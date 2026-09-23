package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func guardedCallInlineFixture() (*Decompiler, *[]statements.Statement, *values.FunctionCallExpression, *values.TernaryExpression) {
	typ := types.NewJavaClass("java.lang.String")
	receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.ref.WeakReference"))
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, typ.Copy())
	result := values.NewJavaRef(utils.NewRootVariableId(), nil, typ.Copy())
	call := &values.FunctionCallExpression{
		Object:       receiver,
		ClassName:    "java.lang.ref.WeakReference",
		FunctionName: "get",
		Descriptor:   "()Ljava/lang/Object;",
		Kind:         values.InvokeVirtual,
		OriginPC:     30,
		FuncType:     types.NewJavaFuncType("()Ljava/lang/Object;", nil, typ.Copy()),
	}
	condition := &values.JavaExpression{Values: []values.JavaValue{receiver, values.JavaNull}, Op: values.EQ}
	ternary := &values.TernaryExpression{
		Condition:       condition,
		ConditionFromOp: 7,
		TrueValue:       values.JavaNull,
		FalseValue:      temp,
	}
	producer := &statements.AssignStatement{LeftValue: temp, JavaValue: call, OriginPC: 33, HasOriginPC: true}
	consumer := &statements.AssignStatement{LeftValue: result, JavaValue: ternary, OriginPC: 40, HasOriginPC: true}
	root := []statements.Statement{producer, consumer}
	d := NewDecompiler(nil, nil)
	d.opcodeIndexToOffset[7] = 20
	return d, &root, call, ternary
}

func TestInlineGuardedCallTempMovesCallIntoNonNullArm(t *testing.T) {
	d, root, call, ternary := guardedCallInlineFixture()
	if changed := d.InlineGuardedCallTemps(root); changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	if len(*root) != 1 {
		t.Fatalf("statements = %d, want only guarded assignment", len(*root))
	}
	if ternary.FalseValue != call {
		t.Fatalf("non-null arm = %T, want original call", ternary.FalseValue)
	}
}

func TestInlineGuardedCallTempRejectsUnsafeCases(t *testing.T) {
	t.Run("later temporary use", func(t *testing.T) {
		d, root, _, _ := guardedCallInlineFixture()
		temp := (*root)[0].(*statements.AssignStatement).LeftValue
		*root = append(*root, &statements.ReturnStatement{JavaValue: temp})
		if changed := d.InlineGuardedCallTemps(root); changed != 0 {
			t.Fatalf("changed = %d, want 0 when temporary has another use", changed)
		}
	})

	t.Run("opaque later expression", func(t *testing.T) {
		d, root, _, _ := guardedCallInlineFixture()
		opaque := values.NewCustomValue(nil, nil)
		*root = append(*root, &statements.ExpressionStatement{Expression: opaque})
		if changed := d.InlineGuardedCallTemps(root); changed != 0 {
			t.Fatalf("changed = %d, want 0 when another expression has hidden dependencies", changed)
		}
	})

	t.Run("different handler coverage", func(t *testing.T) {
		d, root, _, _ := guardedCallInlineFixture()
		d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 30, EndPc: 35, HandlerPc: 40}}
		if changed := d.InlineGuardedCallTemps(root); changed != 0 {
			t.Fatalf("changed = %d, want 0 across exception-handler boundary", changed)
		}
	})

	t.Run("eager call before null test", func(t *testing.T) {
		d, root, call, _ := guardedCallInlineFixture()
		call.OriginPC = 10
		if changed := d.InlineGuardedCallTemps(root); changed != 0 {
			t.Fatalf("changed = %d, want 0 when invoke precedes guard", changed)
		}
	})
}
