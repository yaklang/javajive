package values

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestArrayLengthRetainsDependencyAndThrowBarrier(t *testing.T) {
	ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
	ref.Id.SetName("before")
	length := &ArrayLengthExpression{Array: ref, OriginPC: 8, HasOriginPC: true}
	children, known := Children(length)
	effects, refs := InspectValue(length)
	if !known || len(children) != 1 || children[0] != ref || !refs[ref] || effects != EffectReadMemory|EffectThrow || MayFold(length) || IsPure(length) {
		t.Fatalf("lost array dependency or null-check barrier: children=%v known=%v effects=%v refs=%v", children, known, effects, refs)
	}
	newID := utils.NewRootVariableId()
	newID.SetName("after")
	length.ReplaceVar(ref.Id, newID)
	if got := length.String(nil); got != "after.length" || length.OriginPC != 8 || !length.HasOriginPC || length.Type().String(nil) != "int" {
		t.Fatalf("replacement must retain origin and int result: %q", got)
	}
	var absent *ArrayLengthExpression
	if children, known := Children(absent); !known || len(children) != 0 || !IsPure(absent) {
		t.Fatal("typed nil must be an empty dependency")
	}
	if got := absent.String(nil); got != "" {
		t.Fatalf("typed nil rendered %q", got)
	}
}

func TestArrayLengthReceiverPrecedence(t *testing.T) {
	makeRef := func(name string) *JavaRef {
		ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
		ref.Id.SetName(name)
		return ref
	}
	first, second := makeRef("first"), makeRef("second")
	for _, tc := range []struct {
		array JavaValue
		want  string
	}{
		{first, "first.length"},
		{NewAssignmentExpression(first, second, 4, nil), "(first = second).length"},
		{&TernaryExpression{Condition: NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), TrueValue: first, FalseValue: second}, "((true) ? (first) : (second)).length"},
	} {
		if got := (&ArrayLengthExpression{Array: tc.array}).String(nil); got != tc.want {
			t.Fatalf("receiver %T: got %q want %q", tc.array, got, tc.want)
		}
	}
}

func TestArrayLengthChecksBudgetBeforeParentAllocation(t *testing.T) {
	for _, limit := range []int64{6, 16, 17} {
		calls := 0
		operand := NewStreamingCustomValue(func(_ *class_context.ClassContext, out *workbudget.Writer) error {
			calls++
			return out.WriteString("abcdefghij")
		}, nil)
		ctx := &class_context.ClassContext{Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: limit})}
		got := (&ArrayLengthExpression{Array: operand}).String(ctx)
		if limit < 17 {
			if got != "" || !workbudget.Is(ctx.Work.Err()) {
				t.Fatalf("over-budget parent emitted %q, error=%v", got, ctx.Work.Err())
			}
			if limit == 6 && calls != 0 {
				t.Fatal("visited operand after parent suffix already exceeded output budget")
			}
		} else if got != "abcdefghij.length" || ctx.Work.Err() != nil || calls != 1 {
			t.Fatalf("exact output boundary rejected: got=%q calls=%d error=%v", got, calls, ctx.Work.Err())
		}
	}
	ctx := &class_context.ClassContext{Work: workbudget.New(context.Background(), workbudget.Limits{MaxASTDepth: 4})}
	cycle := &ArrayLengthExpression{}
	cycle.Array = cycle
	if got := cycle.String(ctx); got != "" || !workbudget.Is(ctx.Work.Err()) {
		t.Fatal("cyclic operand did not stop at the depth budget")
	}
}
