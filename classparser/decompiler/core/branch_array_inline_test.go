package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestBranchArrayInlineRejectsAlternateEntry(t *testing.T) {
	allocation := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 1}
	fill := &OpCode{Instr: &Instruction{OpCode: OP_AASTORE}, CurrentOffset: 2}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 3}
	allocation.Target, fill.Source = []*OpCode{fill}, []*OpCode{allocation}
	fill.Target, invoke.Source = []*OpCode{invoke}, []*OpCode{fill}
	d := &Decompiler{}
	if !branchArraySinglePath(d, allocation, invoke) {
		t.Fatal("the array initializer and its invocation form one linear path")
	}
	other := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 4, Target: []*OpCode{invoke}}
	invoke.Source = append(invoke.Source, other)
	if branchArraySinglePath(d, allocation, invoke) {
		t.Fatal("an alternate edge into the invocation must keep the allocation in place")
	}
}

func TestBranchArrayInertElementProof(t *testing.T) {
	literal := values.NewJavaLiteral("x", types.NewJavaClass("java.lang.String"))
	ref := &values.JavaRef{}
	for _, tc := range []struct {
		name  string
		value values.JavaValue
		want  bool
	}{
		{"literal", literal, true}, {"local", ref, true}, {"wrapped-local", values.NewSlotValue(ref, types.NewJavaClass("sample.Item")), true},
		{"nil", nil, false}, {"typed-nil-ref", (*values.JavaRef)(nil), false}, {"typed-nil-literal", (*values.JavaLiteral)(nil), false},
		{"call", &values.FunctionCallExpression{}, false}, {"array-read", &values.JavaArrayMember{}, false},
		{"field-read", &values.RefMember{}, false}, {"new-array", &values.NewExpression{}, false},
		{"opaque-local", &values.JavaRef{CustomValue: &values.CustomValue{}}, false},
		{"captured-local", &values.JavaRef{StackVar: ref}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchArrayInertElement(tc.value); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
