package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func delegationDecompiler(ref *values.JavaRef, sourceOpcode int) (*Decompiler, *OpCode) {
	formatType := types.NewJavaClass("example.Format")
	formatCall := &values.FunctionCallExpression{FunctionName: "<init>", Arguments: []values.JavaValue{ref}}
	formatValue := &values.NewExpression{JavaType: formatType, ConstructorCall: formatCall, OriginPC: 5, HasOriginPC: true}
	delegation := &values.FunctionCallExpression{
		FunctionName: "<init>",
		Object:       values.NewJavaRef(utils.NewRootVariableId(), nil, formatType),
		Arguments:    []values.JavaValue{formatValue},
	}
	delegation.Object.(*values.JavaRef).IsThis = true
	delegationOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 20}
	sourceOp := &OpCode{Instr: &Instruction{OpCode: sourceOpcode}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
	return &Decompiler{invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{delegationOp: delegation}}, sourceOp
}

func TestInlineConstructorDelegationValueAtExactNestedUse(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	ref.IsParam = true
	d, sourceOp := delegationDecompiler(ref, OP_DUP)
	target := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))))
	value := values.NewJavaLiteral("value", typ)
	if !d.canInlineValue(value, nil, target, nil, ref, sourceOp) {
		t.Fatal("stack-produced value used once under this(...) should remain at its argument position")
	}

	d, sourceOp = delegationDecompiler(ref, OP_DUP)
	effectful := values.TagEffects(values.NewJavaLiteral("effect", typ), values.EffectCall)
	if !d.canInlineDelegationValue(effectful, ref, sourceOp) {
		t.Fatal("spill computed after the nested NEW should keep its original evaluation order")
	}
	for _, call := range d.invokeFuncCall {
		call.Arguments[0].(*values.NewExpression).OriginPC = 15
	}
	if d.canInlineDelegationValue(effectful, ref, sourceOp) {
		t.Fatal("spill cannot move before a nested NEW that originally followed it")
	}

	d, sourceOp = delegationDecompiler(ref, OP_ALOAD_1)
	if !d.canInlineValue(ref, nil, target, nil, ref, sourceOp) {
		t.Fatal("pure constructor parameter load should be usable in this(...) argument")
	}
}

func TestInlineCtorSpillAfterCompletedArrayPrefix(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaChar))
	valueType := types.NewJavaClass("example.Format")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	ref.IsParam = true
	array := &values.NewExpression{
		JavaType:           arrayType,
		OriginPC:           2,
		HasOriginPC:        true,
		Initializer:        []values.JavaValue{values.NewJavaLiteral(32, types.NewJavaPrimer(types.JavaChar))},
		EvaluationEndPC:    4,
		HasEvaluationEndPC: true,
	}
	formatCall := &values.FunctionCallExpression{FunctionName: "<init>", Arguments: []values.JavaValue{array, ref}, OriginPC: 12}
	formatValue := &values.NewExpression{JavaType: valueType, ConstructorCall: formatCall, OriginPC: 1, HasOriginPC: true}
	delegation := &values.FunctionCallExpression{
		FunctionName: "<init>",
		Object:       values.NewJavaRef(utils.NewRootVariableId(), nil, valueType),
		Arguments:    []values.JavaValue{formatValue},
	}
	delegation.Object.(*values.JavaRef).IsThis = true
	delegationOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 20}
	store := &OpCode{Instr: &Instruction{OpCode: OP_CASTORE}, CurrentOffset: 4}
	source := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 8, stackProduced: []values.JavaValue{ref}}
	store.Target = []*OpCode{source}
	d := &Decompiler{
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{delegationOp: delegation},
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{store: nil, source: nil},
	}
	effectful := values.TagEffects(values.NewJavaLiteral("spill", arrayType), values.EffectCall)
	if !d.canInlineDelegationValue(effectful, ref, source) {
		t.Fatal("a fully completed earlier array argument should remain before the inlined spill")
	}

	array.EvaluationEndPC = 9
	if d.canInlineDelegationValue(effectful, ref, source) {
		t.Fatal("array stores after the spill cannot be moved before it")
	}
	array.EvaluationEndPC = 4
	store.Target = []*OpCode{{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 5}, source}
	if d.canInlineDelegationValue(effectful, ref, source) {
		t.Fatal("a branch between the last array store and spill invalidates source order")
	}
	store.Target = []*OpCode{source}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 6, HandlerPc: 30}}
	if d.canInlineDelegationValue(effectful, ref, source) {
		t.Fatal("a handler-domain change between the array store and spill invalidates the proof")
	}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 5, EndPc: 7, HandlerPc: 30}}
	middle := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 6, Target: []*OpCode{source}}
	store.Target = []*OpCode{middle}
	d.opcodeToSimulateStack[middle] = nil
	if d.canInlineDelegationValue(effectful, ref, source) {
		t.Fatal("a handler boundary inside the source-order path invalidates the proof even if endpoints match")
	}
}

func TestInlineConstructorDelegationValueRejectsAmbiguousOrMovedUses(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	d, sourceOp := delegationDecompiler(ref, OP_DUP)
	call := d.invokeFuncCall[func() *OpCode {
		for op := range d.invokeFuncCall {
			return op
		}
		return nil
	}()]
	call.Arguments = append(call.Arguments, ref)
	if d.canInlineDelegationValue(values.NewNewExpression(typ), ref, sourceOp) {
		t.Fatal("same local used twice under delegation must not be folded")
	}

	d, sourceOp = delegationDecompiler(ref, OP_DUP)
	for op, call := range d.invokeFuncCall {
		call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		op.CurrentOffset = 20
	}
	if d.canInlineDelegationValue(values.NewNewExpression(typ), ref, sourceOp) {
		t.Fatal("ordinary constructor call is not this/super delegation")
	}

	d, sourceOp = delegationDecompiler(ref, OP_DUP)
	for _, call := range d.invokeFuncCall {
		call.Arguments = append([]values.JavaValue{values.TagEffects(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), values.EffectCall)}, call.Arguments...)
	}
	if d.canInlineDelegationValue(values.TagEffects(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), values.EffectCall), ref, sourceOp) {
		t.Fatal("effectful spill cannot move after an earlier constructor argument")
	}

	d, sourceOp = delegationDecompiler(ref, OP_DUP)
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 15, HandlerPc: 30}}
	if d.canInlineDelegationValue(values.NewNewExpression(typ), ref, sourceOp) {
		t.Fatal("moving across a handler boundary must stay blocked")
	}

	d, sourceOp = delegationDecompiler(ref, OP_ALOAD_1)
	if d.canInlineDelegationValue(values.NewNewExpression(typ), ref, sourceOp) {
		t.Fatal("effectful value cannot be justified by a parameter load")
	}
}

func TestInlineCheckcastAtExactInvocationOperand(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	call := &values.FunctionCallExpression{FunctionName: "iterator", Object: ref}
	checkcast := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEINTERFACE}, CurrentOffset: 13}
	merge := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 16}
	checkcast.Target = []*OpCode{invoke}
	invoke.Target = []*OpCode{merge}
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(values.NewJavaLiteral("x", typ), typ, 10), true))
	target := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))))
	source.Id, target.Id = 1, 2
	source.AddNext(target)
	origins := map[int]*OpCode{source.Id: checkcast, target.Id: merge}
	d := &Decompiler{invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{invoke: call}}
	cast := values.NewCastExpression(values.NewJavaLiteral("x", typ), typ, 10)
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("cast directly consumed as one invocation receiver should retain its exact evaluation position")
	}

	call.Arguments = []values.JavaValue{ref}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("receiver and argument duplicate uses must remain materialized")
	}
	call.Arguments = nil

	branch := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 15}
	invoke.Target = []*OpCode{branch}
	branch.Target = []*OpCode{merge}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("invocation result crossing a conditional edge must remain materialized")
	}

	invoke.Target = []*OpCode{merge}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 12, HandlerPc: 30}}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("cast and invocation in different handler domains must remain materialized")
	}
}

func TestInlineCheckcastAcrossArgumentLoadsToBranchMerge(t *testing.T) {
	typ := types.NewJavaClass("example.AbstractReader")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	call := &values.FunctionCallExpression{
		FunctionName: "decodeNode",
		Object:       ref,
		Arguments:    []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))},
		OriginPC:     24,
	}
	leaf := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 27}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 24, Target: []*OpCode{leaf}}
	argumentLoad := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_1}, CurrentOffset: 23, Target: []*OpCode{invoke}}
	producer := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 20, Target: []*OpCode{argumentLoad}, stackProduced: []values.JavaValue{ref}}
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(values.NewJavaLiteral("reader", typ), typ, 20), true))
	target := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))))
	source.Id, target.Id = 1, 2
	origins := map[int]*OpCode{source.Id: producer, target.Id: leaf}
	d := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, argumentLoad: nil, invoke: nil, leaf: nil},
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{invoke: call},
	}
	cast := values.NewCastExpression(values.NewJavaLiteral("reader", typ), typ, 20)
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("cast receiver followed by straight-line argument loads should fold at its branch-local invocation")
	}

	branch := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 22, Target: []*OpCode{invoke, leaf}}
	producer.Target = []*OpCode{branch}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a branch between the cast and invocation must remain materialized")
	}
	producer.Target = []*OpCode{argumentLoad}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 23, EndPc: 24, HandlerPc: 40}}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("handler change between the cast and invocation must remain materialized")
	}
	d.ExceptionTable = nil
	call.Arguments = append(call.Arguments, ref)
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("duplicate invocation use of the cast temp must remain materialized")
	}
}

func TestInlineEffectfulCastAtExactMergeLeaf(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	leaf := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 13}
	producer := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, Target: []*OpCode{leaf}, stackProduced: []values.JavaValue{ref}}
	lookup := &values.FunctionCallExpression{FunctionName: "lookup", Object: ref}
	cast := &values.CastExpression{Value: lookup, TargetType: typ, OriginPC: 10}
	d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, leaf: nil}}
	if !d.canInlineEffectfulCastAtMergeLeaf(ref, cast, leaf) {
		t.Fatal("effectful cast immediately feeding the selected arm's goto should retain its branch position")
	}

	wrongLeaf := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 14}
	if d.canInlineEffectfulCastAtMergeLeaf(ref, cast, wrongLeaf) {
		t.Fatal("cast cannot be moved to a different branch leaf")
	}
	producer.Target = []*OpCode{leaf, wrongLeaf}
	if d.canInlineEffectfulCastAtMergeLeaf(ref, cast, leaf) {
		t.Fatal("ambiguous cast producer edge must fail closed")
	}
	producer.Target = []*OpCode{leaf}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 12, HandlerPc: 20}}
	if d.canInlineEffectfulCastAtMergeLeaf(ref, cast, leaf) {
		t.Fatal("cast cannot cross an exception-handler boundary")
	}
	d.ExceptionTable = nil
	producer.stackProduced = nil
	if d.canInlineEffectfulCastAtMergeLeaf(ref, cast, leaf) {
		t.Fatal("producer must be tied to the exact folded local")
	}
	producer.stackProduced = []values.JavaValue{ref}
	cast.Value = values.TagEffects(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), values.EffectWriteMemory)
	if d.canInlineEffectfulCastAtMergeLeaf(ref, cast, leaf) {
		t.Fatal("untracked local writes are not safe to replay at the merge leaf")
	}
}

func TestInlineMotionChecksEntirePath(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	x := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	saved := values.NewJavaRef(utils.NewRootVariableId().Next(), x, typ)
	source := NewNode(statements.NewAssignStatement(saved, x, true))
	source.Id = 0
	middle := NewNode(statements.NewAssignStatement(x, values.NewJavaLiteral(99, typ), false))
	middle.Id = 1
	target := NewNode(statements.NewReturnStatement(saved))
	target.Id = 2
	source.AddNext(middle)
	middle.AddNext(target)
	origins := map[int]*OpCode{0: {CurrentOffset: 0}, 1: {CurrentOffset: 1}, 2: {CurrentOffset: 2}}
	d := &Decompiler{}
	if d.canInlineValue(x, source, target, origins, saved, origins[0]) {
		t.Fatal("saved read crossed write of original local")
	}
	middle.Statement = statements.NewAssignStatement(values.NewJavaRef(utils.NewRootVariableId().Next().Next(), nil, typ), values.NewJavaLiteral(99, typ), false)
	if !d.canInlineValue(x, source, target, origins, saved, origins[0]) {
		t.Fatal("pure independent linear motion rejected")
	}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 4}}
	if d.canInlineValue(x, source, target, origins, saved, origins[0]) {
		t.Fatal("read crossed handler scope")
	}
	d.ExceptionTable = nil
	middle.AddNext(source)
	if d.canInlineValue(x, source, target, origins, saved, origins[0]) {
		t.Fatal("ambiguous/cyclic path accepted")
	}
}

func TestAdjacentCallInliningRespectsEvaluatedPrefix(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	saved := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	effect := values.TagEffects(values.NewJavaLiteral(1, typ), values.EffectCall)
	source := NewNode(statements.NewAssignStatement(saved, effect, true))
	source.Id = 0
	call := &values.FunctionCallExpression{Arguments: []values.JavaValue{values.NewJavaLiteral(0, typ), saved}}
	target := NewNode(statements.NewExpressionStatement(call))
	target.Id = 1
	source.AddNext(target)
	d := &Decompiler{}
	origins := map[int]*OpCode{0: {CurrentOffset: 0}, 1: {CurrentOffset: 1}}
	if !d.canInlineValue(effect, source, target, origins, saved, origins[0]) {
		t.Fatal("adjacent use with pure prefix rejected")
	}
	call.Arguments[0] = values.TagEffects(values.NewJavaLiteral(0, typ), values.EffectCall)
	if d.canInlineValue(effect, source, target, origins, saved, origins[0]) {
		t.Fatal("reordered two calls")
	}
	call.Arguments = []values.JavaValue{saved, saved}
	if d.canInlineValue(effect, source, target, origins, saved, origins[0]) {
		t.Fatal("duplicated effectful value")
	}
}
