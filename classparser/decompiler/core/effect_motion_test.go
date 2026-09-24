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
	allocationOp := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 5, Target: []*OpCode{sourceOp}}
	return &Decompiler{
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{delegationOp: delegation},
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{allocationOp: nil, sourceOp: nil},
	}, sourceOp
}

func TestInlineImmediateStoreRequiresExactOpcodeIdentityAndHandlerDomain(t *testing.T) {
	typ := types.NewJavaClass("java.lang.Class")
	newCase := func(sourceOpcode, targetOpcode int) (*Decompiler, *Node, *Node, map[int]*OpCode, *values.JavaRef, values.JavaValue) {
		local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		field := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		value := values.TagEffects(values.NewJavaLiteral("resolved", typ), values.EffectCall)
		sourceOp := &OpCode{Instr: &Instruction{OpCode: sourceOpcode}, CurrentOffset: 10}
		targetOp := &OpCode{Instr: &Instruction{OpCode: targetOpcode}, CurrentOffset: 11}
		sourceOp.Target = []*OpCode{targetOp}
		source := NewNode(statements.NewAssignStatement(local, value, true))
		target := NewNode(statements.NewAssignStatement(field, local, false))
		source.Id, target.Id = 1, 2
		source.AddNext(target)
		d := &Decompiler{}
		return d, source, target, map[int]*OpCode{source.Id: sourceOp, target.Id: targetOp}, local, value
	}

	for _, pair := range []struct{ source, target int }{
		{OP_DUP, OP_PUTSTATIC},
	} {
		d, source, target, origins, local, value := newCase(pair.source, pair.target)
		if !d.canInlineImmediateStore(source, target, origins) {
			t.Fatalf("exact adjacent opcode pair %d->%d should pass the narrow proof", pair.source, pair.target)
		}
		if !d.canInlineValue(value, source, target, origins, local, nil) {
			t.Fatalf("exact adjacent opcode pair %d->%d should preserve the stored value", pair.source, pair.target)
		}
	}

	d, source, target, origins, _, _ := newCase(OP_ASTORE, OP_PUTSTATIC)
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("unrecognized producer/store pair must not use the special fold")
	}
	d, source, target, origins, _, _ = newCase(OP_CHECKCAST, OP_PUTFIELD)
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("CHECKCAST/PUTFIELD is outside the class-cache store proof")
	}
	d, source, target, origins, _, _ = newCase(OP_DUP, OP_PUTSTATIC)
	targetStmt := target.Statement.(*statements.AssignStatement)
	targetStmt.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("store of a different local must not consume this definition")
	}
	d, source, target, origins, _, _ = newCase(OP_DUP, OP_PUTSTATIC)
	middle := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))))
	middle.Id = 3
	source.Next = nil
	source.AddNext(middle)
	middle.AddNext(target)
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("a non-adjacent store must not use the special fold")
	}
	d, source, target, origins, _, _ = newCase(OP_DUP, OP_PUTSTATIC)
	sourceOp := origins[source.Id]
	targetOp := origins[target.Id]
	otherOp := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 10}
	sourceOp.Target = []*OpCode{otherOp, targetOp}
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("a producer with an alternate opcode successor must remain materialized")
	}
	d, source, target, origins, _, _ = newCase(OP_DUP, OP_PUTSTATIC)
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 11, HandlerPc: 30}}
	if d.canInlineImmediateStore(source, target, origins) {
		t.Fatal("a handler-domain boundary between producer and store must block the fold")
	}
}

func TestArraySpillOrderUsesUniqueBytecodeProducers(t *testing.T) {
	field := &values.JavaClassMember{Name: "example.Owner", Member: "FLAG", JavaType: types.NewJavaPrimer(types.JavaInteger)}
	producedField := &values.JavaClassMember{Name: "example.Owner", Member: "FLAG", Description: "I", JavaType: types.NewJavaPrimer(types.JavaInteger)}
	field.Description = "I"
	producer := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 4, stackProduced: []values.JavaValue{producedField}}
	allocation := &OpCode{Instr: &Instruction{OpCode: OP_NEWARRAY}, CurrentOffset: 6}
	producer.Target = []*OpCode{allocation}
	d := &Decompiler{
		opCodes:               []*OpCode{producer, allocation},
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, allocation: nil},
	}
	if !d.valueWasProducedBefore(field, allocation) {
		t.Fatal("a unique earlier GETSTATIC with an equivalent constant-pool field must prove the prefix evaluation order")
	}

	late := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 8, stackProduced: []values.JavaValue{field}}
	d.opCodes = []*OpCode{allocation, late}
	if d.valueWasProducedBefore(field, allocation) {
		t.Fatal("a field value produced after array allocation cannot count as an earlier prefix")
	}

	duplicate := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 2, stackProduced: []values.JavaValue{field}, Target: []*OpCode{producer}}
	producer.Target = []*OpCode{allocation}
	d.opCodes = []*OpCode{duplicate, producer, allocation}
	if d.valueWasProducedBefore(field, allocation) {
		t.Fatal("ambiguous repeated producers must fail closed")
	}

	d.opCodes = []*OpCode{producer, allocation}
	producer.stackProduced = []values.JavaValue{&values.JavaClassMember{
		Name: "other.Owner", Member: "FLAG", Description: "I", JavaType: types.NewJavaPrimer(types.JavaInteger),
	}}
	if d.valueWasProducedBefore(field, allocation) {
		t.Fatal("a different field identity must not prove the prefix evaluation order")
	}

	duplicate.Target = []*OpCode{allocation, producer}
	d.opCodes = []*OpCode{duplicate, producer, allocation}
	if d.valueWasProducedBefore(field, allocation) {
		t.Fatal("a branch between a prefix producer and allocation must fail closed")
	}
}

func TestCompletedConstructorArgumentCanPrecedeArraySpill(t *testing.T) {
	classType := types.NewJavaClass("example.Prefix")
	prefix := &values.NewExpression{JavaType: classType, OriginPC: 4, HasOriginPC: true}
	constructor := &values.FunctionCallExpression{FunctionName: "<init>", OriginPC: 8, Object: prefix}
	prefix.ConstructorCall = constructor
	allocation := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 4}
	dup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 5}
	init := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 8}
	between := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 12}
	arrayAllocation := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 16}
	allocation.Target = []*OpCode{dup}
	dup.Target = []*OpCode{init}
	init.Target = []*OpCode{between}
	between.Target = []*OpCode{arrayAllocation}
	d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{
		allocation: nil, dup: nil, init: nil, between: nil, arrayAllocation: nil,
	}, invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{init: constructor}}
	if !d.evaluationCompletedBefore(prefix, arrayAllocation) {
		t.Fatal("a constructed argument whose invokespecial completed before NEWARRAY must prove its order")
	}

	zeroArgPrefix := &values.NewExpression{JavaType: classType, OriginPC: 4, HasOriginPC: true}
	zeroArgInit := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESPECIAL}, CurrentOffset: 8}
	zeroArgInvoke := &values.FunctionCallExpression{FunctionName: "<init>", OriginPC: 8, Object: zeroArgPrefix}
	zeroArgAllocation := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 4}
	zeroArgDup := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 5}
	zeroArgBetween := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 12}
	zeroArgArray := &OpCode{Instr: &Instruction{OpCode: OP_ANEWARRAY}, CurrentOffset: 16}
	zeroArgAllocation.Target = []*OpCode{zeroArgDup}
	zeroArgDup.Target = []*OpCode{zeroArgInit}
	zeroArgInit.Target = []*OpCode{zeroArgBetween}
	zeroArgBetween.Target = []*OpCode{zeroArgArray}
	zeroArgD := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{
			zeroArgAllocation: nil, zeroArgDup: nil, zeroArgInit: nil, zeroArgBetween: nil, zeroArgArray: nil,
		},
		invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{zeroArgInit: zeroArgInvoke},
	}
	if !zeroArgD.evaluationCompletedBefore(zeroArgPrefix, zeroArgArray) {
		t.Fatal("a zero-argument constructor still needs its invokespecial proven before the later array allocation")
	}
	missingCallD := &Decompiler{opcodeToSimulateStack: zeroArgD.opcodeToSimulateStack}
	if missingCallD.evaluationCompletedBefore(zeroArgPrefix, zeroArgArray) {
		t.Fatal("NEW without its matching invokespecial must not count as a completed object argument")
	}

	branch := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 10}
	init.Target = []*OpCode{branch}
	branch.Target = []*OpCode{between, arrayAllocation}
	d.opcodeToSimulateStack[branch] = nil
	if d.evaluationCompletedBefore(prefix, arrayAllocation) {
		t.Fatal("a control-flow split after the constructor call must not count as a unique completed prefix")
	}

	init.Target = []*OpCode{between}
	branch.Target = nil
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 8, EndPc: 9, HandlerPc: 30}}
	if d.evaluationCompletedBefore(prefix, arrayAllocation) {
		t.Fatal("a handler-domain change after constructor completion must keep the prefix unproven")
	}
}

func TestDelegatingArraySpillRequiresCompleteLinearInitializer(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaChar))
	array := values.NewNewExpression(arrayType)
	array.Initializer = []values.JavaValue{values.NewJavaLiteral(32, types.NewJavaPrimer(types.JavaChar))}
	array.OriginPC, array.HasOriginPC = 6, true
	array.EvaluationEndPC, array.HasEvaluationEndPC = 17, true
	pcs := []uint16{6, 8, 10, 12, 14, 17}
	ops := make([]*OpCode, len(pcs))
	for i, pc := range pcs {
		opcode := OP_DUP
		if i == 0 {
			opcode = OP_NEWARRAY
		}
		if i == len(pcs)-1 {
			opcode = OP_CASTORE
		}
		ops[i] = &OpCode{Instr: &Instruction{OpCode: opcode}, CurrentOffset: pc}
		if i > 0 {
			ops[i-1].Target = []*OpCode{ops[i]}
		}
	}
	opcodeIndex := map[*OpCode]*StackSimulationImpl{}
	for _, op := range ops {
		opcodeIndex[op] = nil
	}
	d := &Decompiler{opCodes: ops, opcodeToSimulateStack: opcodeIndex}
	if !d.provesDelegatingArraySpillSpan(array, ops[1]) {
		t.Fatal("complete straight-line initializer spanning the DUP producer should pass")
	}
	array.HasEvaluationEndPC = false
	if d.provesDelegatingArraySpillSpan(array, ops[1]) {
		t.Fatal("initializer without an exact final-store origin must fail closed")
	}
	array.HasEvaluationEndPC = true
	ops[1].Target = []*OpCode{ops[2], ops[3]}
	if d.provesDelegatingArraySpillSpan(array, ops[1]) {
		t.Fatal("branching array initialization must fail closed")
	}
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
	outerAllocation := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 1}
	arrayAllocation := &OpCode{Instr: &Instruction{OpCode: OP_NEWARRAY}, CurrentOffset: 2}
	store := &OpCode{Instr: &Instruction{OpCode: OP_CASTORE}, CurrentOffset: 4}
	source := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 8, stackProduced: []values.JavaValue{ref}}
	outerAllocation.Target = []*OpCode{arrayAllocation}
	arrayAllocation.Target = []*OpCode{store}
	store.Target = []*OpCode{source}
	d := &Decompiler{
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{delegationOp: delegation},
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{outerAllocation: nil, arrayAllocation: nil, store: nil, source: nil},
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

func TestInlineDelegationDoesNotBypassConstructorArrayRewrite(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	ref.IsParam = true
	d, sourceOp := delegationDecompiler(ref, OP_DUP)
	array := &values.NewExpression{
		JavaType:    arrayType,
		Initializer: []values.JavaValue{values.NewJavaLiteral("value", types.NewJavaClass("java.lang.Object"))},
	}
	if d.canInlineDelegationValue(array, ref, sourceOp) {
		t.Fatal("initialized array delegation args must use the constructor-entry proof")
	}
}

func TestPrefixBeforeUsePreservesAllocationBarrier(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	newValue := &values.NewExpression{
		JavaType:    typ,
		OriginPC:    5,
		HasOriginPC: true,
		ConstructorCall: &values.FunctionCallExpression{
			FunctionName: "<init>",
			Arguments:    []values.JavaValue{ref},
			OriginPC:     15,
		},
	}
	source := &OpCode{Instr: &Instruction{OpCode: OP_DUP}, CurrentOffset: 10}
	prefix, count, known := prefixBeforeUse(&Decompiler{}, newValue, ref, source)
	if !known || count != 1 || prefix.Effects&(values.EffectAllocate|values.EffectThrow|values.EffectClassInit) == 0 {
		t.Fatal("an older expression offset without its NEW opcode must retain allocation effects")
	}

	allocation := &OpCode{Instr: &Instruction{OpCode: OP_NEW}, CurrentOffset: 5, Target: []*OpCode{source}}
	d := &Decompiler{opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{allocation: nil, source: nil}}
	prefix, count, known = prefixBeforeUse(d, newValue, ref, source)
	if !known || count != 1 || prefix.Effects&(values.EffectAllocate|values.EffectThrow|values.EffectClassInit) == 0 {
		t.Fatal("even a same-handler straight-line NEW does not justify moving source expressions across allocation")
	}

	branch := &OpCode{Instr: &Instruction{OpCode: OP_IFEQ}, CurrentOffset: 7, Target: []*OpCode{source}}
	allocation.Target = []*OpCode{branch, source}
	prefix, count, known = prefixBeforeUse(d, newValue, ref, source)
	if !known || count != 1 || prefix.Effects&(values.EffectAllocate|values.EffectThrow|values.EffectClassInit) == 0 {
		t.Fatal("a branched allocation path must retain its effect barrier")
	}
}

func TestInlineCheckcastAtExactInvocationOperand(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	call := &values.FunctionCallExpression{FunctionName: "iterator", Object: ref}
	checkcast := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEINTERFACE}, CurrentOffset: 13}
	merge := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 16}
	checkcast.Target = []*OpCode{invoke}
	invoke.Target = []*OpCode{merge}
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(input, typ, 10), true))
	target := NewNode(statements.NewExpressionStatement(call))
	source.Id, target.Id = 1, 2
	source.AddNext(target)
	origins := map[int]*OpCode{source.Id: checkcast, target.Id: merge}
	d := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{checkcast: nil, invoke: nil, merge: nil},
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{invoke: call},
	}
	cast := values.NewCastExpression(input, typ, 10)
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("cast directly consumed as one invocation receiver should retain its exact evaluation position")
	}
	otherCast := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 11, stackProduced: []values.JavaValue{ref}}
	d.opcodeToSimulateStack[otherCast] = nil
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a checkcast temp with alternate-arm producers must retain every definition")
	}
	delete(d.opcodeToSimulateStack, otherCast)
	ref.ResetVarType(types.NewParameterizedType("example.Value", []types.JavaType{types.NewJavaClass("java.lang.String")}))
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("raw checkcast must not replace a local whose generic source type is stronger")
	}
	ref.ResetVarType(typ)

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
	d.ExceptionTable = nil
	otherPred := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))))
	otherPred.AddNext(target)
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("an invocation folded into a graph merge must remain materialized")
	}
	target.Statement = statements.NewExpressionStatement(&values.TernaryExpression{
		Condition:  values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		TrueValue:  call,
		FalseValue: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)),
	})
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a unique branch-local invocation may consume its cast across the expression merge")
	}
}

func TestInlineCheckcastAcrossArgumentLoadsToBranchMerge(t *testing.T) {
	typ := types.NewJavaClass("example.AbstractReader")
	input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
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
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(input, typ, 20), true))
	target := NewNode(statements.NewExpressionStatement(call))
	source.Id, target.Id = 1, 2
	source.AddNext(target)
	origins := map[int]*OpCode{source.Id: producer, target.Id: leaf}
	d := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, argumentLoad: nil, invoke: nil, leaf: nil},
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{invoke: call},
	}
	cast := values.NewCastExpression(input, typ, 20)
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

func TestInlineCheckcastAtGotoMergeUsesExactStackValue(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	call := &values.FunctionCallExpression{FunctionName: "escape", Object: ref}
	producer := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
	invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 12}
	leaf := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 14, StackEntry: newStackItem(nil, call)}
	merge := &OpCode{Instr: &Instruction{OpCode: OP_NOP}, CurrentOffset: 20}
	other := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: 18}
	producer.Target = []*OpCode{invoke}
	invoke.Target = []*OpCode{leaf}
	leaf.Target = []*OpCode{merge}
	merge.Source = []*OpCode{leaf, other}
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(input, typ, 10), true))
	target := NewNode(nil)
	source.Id, target.Id = 1, 2
	source.AddNext(target)
	origins := map[int]*OpCode{source.Id: producer, target.Id: leaf}
	d := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, invoke: nil, leaf: nil},
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{invoke: call},
	}
	cast := values.NewCastExpression(input, typ, 10)
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a GOTO carrying the exact call result into a multi-source merge should retain the branch-local cast")
	}

	leaf.StackEntry = newStackItem(nil, values.NewJavaLiteral("other", typ))
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a branch merge must not accept a different value at the top of the operand stack")
	}
	leaf.StackEntry = newStackItem(nil, call)
	merge.Source = []*OpCode{leaf}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a single-predecessor transfer is not a branch merge")
	}
	merge.Source = []*OpCode{leaf, other}
	d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 16, HandlerPc: 30}}
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("the selected branch and merge must remain in the same handler domain")
	}
}

func TestInlineCheckcastAtInvocationConsumerInTernaryArm(t *testing.T) {
	typ := types.NewJavaClass("example.Value")
	input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	selected := &values.FunctionCallExpression{FunctionName: "escape", Arguments: []values.JavaValue{ref}}
	other := &values.FunctionCallExpression{FunctionName: "fallback", Arguments: []values.JavaValue{values.NewJavaLiteral("safe", typ)}}
	merged := &values.TernaryExpression{
		Condition:  values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		TrueValue:  selected,
		FalseValue: other,
	}
	consumer := &values.FunctionCallExpression{FunctionName: "print", Arguments: []values.JavaValue{merged}}
	producer := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 10, stackProduced: []values.JavaValue{ref}}
	escapeOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 12}
	printOp := &OpCode{Instr: &Instruction{OpCode: OP_INVOKEVIRTUAL}, CurrentOffset: 20}
	producer.Target = []*OpCode{escapeOp}
	escapeOp.Target = []*OpCode{printOp}
	source := NewNode(statements.NewAssignStatement(ref, values.NewCastExpression(input, typ, 10), true))
	target := NewNode(statements.NewExpressionStatement(consumer))
	otherPred := NewNode(statements.NewExpressionStatement(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))))
	source.Id, target.Id, otherPred.Id = 1, 2, 3
	source.AddNext(target)
	otherPred.AddNext(target)
	origins := map[int]*OpCode{source.Id: producer, target.Id: printOp}
	d := &Decompiler{
		opcodeToSimulateStack: map[*OpCode]*StackSimulationImpl{producer: nil, escapeOp: nil, printOp: nil},
		invokeFuncCall:        map[*OpCode]*values.FunctionCallExpression{escapeOp: selected, printOp: consumer},
	}
	cast := values.NewCastExpression(input, typ, 10)
	if !d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a single-use call inside one ternary arm may retain its checkcast at the exact invocation")
	}

	target.Statement = statements.NewExpressionStatement(&values.FunctionCallExpression{
		FunctionName: "print",
		Arguments:    []values.JavaValue{selected},
	})
	if d.canInlineCheckcastAtInvocation(cast, source, target, origins, ref) {
		t.Fatal("a call outside a ternary arm must not be moved across a control-flow merge")
	}
}

func TestCallOperandUseIgnoresNestedInvocationReceiver(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
	nested := &values.FunctionCallExpression{FunctionName: "escape", Arguments: []values.JavaValue{ref}}
	outer := &values.FunctionCallExpression{FunctionName: "replace", Object: nested}
	if uses, known := countCallOperandLocalUses(outer, ref); !known || uses != 0 {
		t.Fatalf("a local nested inside an earlier call receiver is not a direct operand use: uses=%d known=%t", uses, known)
	}
	direct := &values.FunctionCallExpression{FunctionName: "escape", Arguments: []values.JavaValue{ref}}
	if uses, known := countCallOperandLocalUses(direct, ref); !known || uses != 1 {
		t.Fatalf("the call directly receiving the local must be selected: uses=%d known=%t", uses, known)
	}
	duplicate := &values.FunctionCallExpression{FunctionName: "escape", Arguments: []values.JavaValue{ref, ref}}
	if uses, known := countCallOperandLocalUses(duplicate, ref); !known || uses != 2 {
		t.Fatalf("duplicate direct operands must remain ambiguous: uses=%d known=%t", uses, known)
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
