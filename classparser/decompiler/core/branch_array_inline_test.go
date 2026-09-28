package core

import "testing"

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
