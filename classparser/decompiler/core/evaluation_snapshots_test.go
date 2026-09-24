package core

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestDynamicOperandsEvaluateBeforeConcatConversion(t *testing.T) {
	typ := types.NewJavaClass("java.lang.Object")
	operand := func(name string) values.JavaValue {
		return values.NewCustomValue(func(*class_context.ClassContext) string { return name + "()" }, func() types.JavaType { return typ })
	}
	first, second := operand("first"), operand("second")
	d := &Decompiler{}
	sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())
	op := &OpCode{CurrentOffset: 42}
	args, err := d.snapshotDynamicOperands(op, sim, []values.JavaValue{second, first}, []types.JavaType{typ, typ})
	if err != nil {
		t.Fatal(err)
	}
	req := CallSiteRequest{Identity: IdentityMakeConcat, CallSiteName: "makeConcat", CallSiteDescriptor: "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/String;", DynamicArgs: args, OriginPC: 42, TargetSourceVersion: 17, ClassMajor: 61}
	res := DispatchInvokeDynamic(req, nil, nil, types.NewJavaPrimer(types.JavaString))
	if res.Status != "" {
		t.Fatal(res.Reason)
	}
	d.snapshotDynamicResult(op, sim, res.Value)
	snaps := d.evaluationSnapshots[op]
	if len(snaps) != 3 || snaps[0].Value != first || snaps[1].Value != second || snaps[2].Value != res.Value {
		t.Fatalf("lost evaluate-all/convert/combine order: %+v", snaps)
	}
	// Conversions are only in the final expression. The raw dynamic operands are
	// referenced exactly once, in order, by emitted assignments.
	text := ""
	ctx := &class_context.ClassContext{}
	for _, s := range snaps {
		if s.OriginPC != 42 {
			t.Fatal("origin lost")
		}
		text += statements.NewAssignStatement(s.Ref, s.Value, true).String(ctx) + ";\n"
	}
	if strings.Count(text, "first()") != 1 || strings.Count(text, "second()") != 1 || strings.Index(text, "first()") > strings.Index(text, "second()") {
		t.Fatalf("operand order/count changed: %s", text)
	}
	converted := res.Value.String(ctx)
	if strings.Contains(converted, "first()") || strings.Contains(converted, "second()") {
		t.Fatalf("conversion re-evaluates operand: %s", converted)
	}
}

func TestImmediateMethodRefInliningRequiresOneFinalArgumentConsumer(t *testing.T) {
	typ, err := types.ParseMethodDescriptor("(Ljava/lang/String;Ljava/util/function/Function;)V")
	if err != nil {
		t.Fatal(err)
	}
	member := values.NewJavaClassMember("Probe", "consume", "(Ljava/lang/String;Ljava/util/function/Function;)V", typ)
	d := NewDecompiler(nil, func(int) values.JavaValue { return member })
	d.FunctionContext.ClassName = "Probe"
	methodRef := values.NewCustomValue(
		func(*class_context.ClassContext) string { return "Probe::method" },
		func() types.JavaType { return types.NewJavaClass("java.util.function.Function") },
	)
	methodRef.IsMethodRef = true

	makePair := func(nextOpcode int, nextDescriptor string, nextPC uint16, sources int) (*OpCode, *OpCode) {
		d.ExceptionTable = nil
		indy := &OpCode{
			CurrentOffset: 10,
			Instr:         &Instruction{OpCode: OP_INVOKEDYNAMIC},
			Data:          []byte{0, 1, 0, 0},
		}
		cpMember := member
		if nextDescriptor != member.Description {
			parsed, parseErr := types.ParseMethodDescriptor(nextDescriptor)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			cpMember = values.NewJavaClassMember("Probe", "consume", nextDescriptor, parsed)
		}
		d.constantPoolGetter = func(int) values.JavaValue { return cpMember }
		next := &OpCode{
			CurrentOffset: nextPC,
			Instr:         &Instruction{OpCode: nextOpcode},
			Data:          []byte{0, 2},
			Source:        make([]*OpCode, sources),
		}
		if sources > 0 {
			next.Source[0] = indy
		}
		indy.Target = []*OpCode{next}
		return indy, next
	}

	indy, _ := makePair(OP_INVOKESTATIC, "(Ljava/lang/String;Ljava/util/function/Function;)V", 15, 1)
	if !d.canInlineImmediateMethodRef(indy, methodRef) {
		t.Fatal("direct, contiguous final invocation argument lost its poly target type")
	}

	for name, pair := range map[string]struct {
		opcode int
		desc   string
		pc     uint16
		src    int
	}{
		"later argument evaluation": {OP_INVOKESTATIC, "()V", 15, 1},
		"duplicate before use":      {OP_DUP, "(Ljava/lang/Object;)V", 15, 1},
		"nonadjacent consumer":      {OP_INVOKESTATIC, "(Ljava/lang/Object;)V", 16, 1},
		"merged consumer":           {OP_INVOKESTATIC, "(Ljava/lang/Object;)V", 15, 2},
	} {
		t.Run(name, func(t *testing.T) {
			op, _ := makePair(pair.opcode, pair.desc, pair.pc, pair.src)
			if d.canInlineImmediateMethodRef(op, methodRef) {
				t.Fatal("inlined a reference without a direct, unique final-argument consumer")
			}
		})
	}

	t.Run("handler coverage change", func(t *testing.T) {
		op, _ := makePair(OP_INVOKESTATIC, "(Ljava/lang/String;Ljava/util/function/Function;)V", 15, 1)
		d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 11, HandlerPc: 30}}
		if d.canInlineImmediateMethodRef(op, methodRef) {
			t.Fatal("inlined a method reference across a protected-region boundary")
		}
	})
	t.Run("external erased generic consumer", func(t *testing.T) {
		op, _ := makePair(OP_INVOKESTATIC, "(Ljava/lang/String;Ljava/util/function/Function;)V", 15, 1)
		external := values.NewJavaClassMember("java.util.stream.Stream", "consume", "(Ljava/lang/String;Ljava/util/function/Function;)V", typ)
		d.constantPoolGetter = func(int) values.JavaValue { return external }
		if d.canInlineImmediateMethodRef(op, methodRef) {
			t.Fatal("inlined across an external generic call's erased receiver type")
		}
	})
	t.Run("mismatched functional target", func(t *testing.T) {
		op, _ := makePair(OP_INVOKESTATIC, "(Ljava/lang/String;Ljava/lang/Object;)V", 15, 1)
		if d.canInlineImmediateMethodRef(op, methodRef) {
			t.Fatal("inlined a method reference into a non-matching argument type")
		}
	})
}

func TestCaptureSnapshotSurvivesLaterLocalMutation(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	d := &Decompiler{}
	sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId().Next())
	op := &OpCode{CurrentOffset: 7}
	args, err := d.snapshotDynamicOperands(op, sim, []values.JavaValue{local}, []types.JavaType{typ})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := args[0].(*values.JavaRef)
	if values.SameLocal(local, snapshot) {
		t.Fatal("capture aliases live local")
	}
	state := map[*values.JavaRef]int{local: 7}
	for _, s := range d.evaluationSnapshots[op] {
		state[s.Ref] = state[s.Value.(*values.JavaRef)]
	}
	state[local] = 99
	if state[snapshot] != 7 {
		t.Fatal("capture re-read mutated local")
	}
	if len(d.disFoldRef) != 1 || d.disFoldRef[0] != snapshot {
		t.Fatal("snapshot may fold back into mutable source")
	}
}
func TestSnapshotBudgetFailsBeforeMutation(t *testing.T) {
	d := &Decompiler{Work: workbudget.New(context.Background(), workbudget.Limits{MaxNodeCopies: 1})}
	sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())
	op := &OpCode{}
	typ := types.NewJavaPrimer(types.JavaInteger)
	if args, err := d.snapshotDynamicOperands(op, sim, []values.JavaValue{values.NewJavaLiteral(7, typ)}, []types.JavaType{typ}); err == nil || args != nil {
		t.Fatal("budget accepted")
	}
	if len(d.evaluationSnapshots) != 0 || len(d.disFoldRef) != 0 {
		t.Fatal("partial snapshot plan on budget failure")
	}
}

func TestDirectConcatRejectsUnmaterializedEffects(t *testing.T) {
	typ := types.NewJavaClass("java.lang.Object")
	call := &values.FunctionCallExpression{FunctionName: "next", FuncType: &types.JavaFuncType{ReturnType: typ}}
	req := CallSiteRequest{Identity: IdentityMakeConcat, CallSiteName: "makeConcat", CallSiteDescriptor: "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/String;", DynamicArgs: []values.JavaValue{call, call}, TargetSourceVersion: 17, ClassMajor: 61}
	res := DispatchInvokeDynamic(req, nil, nil, types.NewJavaPrimer(types.JavaString))
	if res.Status != "unsupported" || !strings.Contains(res.Reason, "snapshots") {
		t.Fatalf("unsafe adapter result: %+v", res)
	}
}
func TestAltMetafactoryRejectsUnprovedMarkersAndBridges(t *testing.T) {
	sam := t19SAM()
	typ := t19IntOpType()
	lit := func(n int) values.JavaValue { return values.NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger)) }
	req := CallSiteRequest{Identity: IdentityLambdaAltMetafactory, CallSiteName: "applyAsInt", CallSiteDescriptor: "()Ljava/util/function/IntUnaryOperator;", StaticArgs: []values.JavaValue{sam, t19Impl("T", "f", "(I)I", RefInvokeStatic), sam, lit(lambdaFlagMarkers), lit(1), lit(3)}, TargetSourceVersion: 8, ClassMajor: 52}
	res := DispatchInvokeDynamic(req, nil, nil, typ)
	if res.Status != "invalid_input" || !strings.Contains(res.Reason, "class constant") {
		t.Fatalf("bad marker accepted: %+v", res)
	}
	req.StaticArgs = []values.JavaValue{sam, t19Impl("T", "f", "(I)I", RefInvokeStatic), sam, lit(lambdaFlagBridges), lit(1), sam}
	res = DispatchInvokeDynamic(req, nil, nil, typ)
	if res.Status != "unsupported" || !strings.Contains(res.Reason, "bridge") {
		t.Fatalf("bridge silently ignored: %+v", res)
	}
}
