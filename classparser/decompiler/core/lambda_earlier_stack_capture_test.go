package core

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestEarlierLambdaStackCaptureRetainsOnlySealedUnchangedParameters(t *testing.T) {
	for _, kind := range []string{"original", "proxy", "instance", "instance wide", "missing scope", "bad capture order", "changed parameter type", "parameter overwrite", "unsealed parameter", "changed entry order", "missing producer", "disconnected producer", "changed load packet", "wide view", "changed factory bytes", "changed factory capture", "work", "copies", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			code := []byte{OP_ALOAD_0, OP_ALOAD_1, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_INVOKESTATIC, 0, 2, OP_ARETURN}
			instance := kind == "instance" || kind == "instance wide"
			if instance {
				code[0], code[1] = OP_ALOAD_1, OP_ALOAD_2
			}
			if kind == "instance wide" {
				code[0], code[1] = OP_LLOAD_1, OP_ALOAD_3
			}
			d := NewDecompiler(bytes.Clone(code), func(int) values.JavaValue { return nil })
			d.FunctionContext.ClassName = "p.Subject"
			d.FunctionContext.FunctionName = "outer"
			d.FunctionContext.IsStatic = true
			parsed, err := types.ParseMethodDescriptor("(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/Object;")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "instance wide" {
				parsed, err = types.ParseMethodDescriptor("(JLjava/lang/String;)Ljava/lang/Object;")
				if err != nil {
					t.Fatal(err)
				}
			}
			d.FunctionType = parsed.FunctionType()
			if instance {
				d.FunctionContext.IsStatic = false
				d.FunctionType.ParamTypes = append([]types.JavaType{types.NewJavaClass("p.Subject")}, d.FunctionType.ParamTypes...)
			}
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			// ParseOpcode decodes bytes; the actual source pipeline also
			// constructs the original control-flow edges before proving
			// producer dominance. The unit fixture must do the same.
			if err := d.ScanJmp(); err != nil {
				t.Fatal(err)
			}
			var producer, factory *OpCode
			for _, op := range d.opCodes {
				if op.Instr.OpCode == int(code[0]) {
					producer = op
				}
				if op.Instr.OpCode == OP_INVOKEDYNAMIC {
					factory = op
				}
			}
			if producer == nil || factory == nil {
				t.Fatal("original instruction inventory")
			}
			key := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
			key.IsParam = true
			if kind == "instance wide" {
				key.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			}
			keySlot := 0
			if instance {
				keySlot = 1
			}
			key.MarkOriginalParameter(keySlot)
			late := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
			late.IsParam = true
			lateSlot := 1
			if instance {
				lateSlot = 2
			}
			if kind == "instance wide" {
				lateSlot = 3
			}
			late.MarkOriginalParameter(lateSlot)
			if kind == "unsealed parameter" {
				key = values.NewJavaRef(utils.NewRootVariableId(), nil, key.Type())
				key.IsParam = true
			}
			d.Params = []values.JavaValue{key, late}
			d.LambdaCaptureOrderSlots = []int{keySlot, lateSlot}
			if instance {
				receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Subject"))
				receiver.IsParam = true
				receiver.IsThis = true
				receiver.MarkOriginalReceiver()
				d.Params = append([]values.JavaValue{receiver}, d.Params...)
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), late, late.Type().Copy())
			snapshot.MarkOriginalDynamicOperand(2, 0, late)
			calls := 0
			lambda := values.NewCustomValue(func(*class_context.ClassContext) string { calls++; return "unused" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Supplier") })
			lambda.Flag = "lambda"
			lambda.HasOriginPC = true
			lambda.OriginPC = 2
			lambda.CapturesKnown = true
			lambda.Captures = []values.JavaValue{snapshot}
			lambda.InstantiatedMtdDesc = "()Ljava/lang/Object;"
			lambda.MarkOriginalLambdaFactory(2)
			d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{factory: {{Ref: snapshot, Value: late, Operand: true, OriginPC: 2}}}
			d.stackValueProducers = map[values.JavaValue]*OpCode{key: producer}
			sim := &StackSimulationImpl{stackEntry: NewEmptyStackEntry(), currentVarId: utils.NewRootVariableId(), varTable: map[int]*values.JavaRef{}}
			sim.Push(key)
			before := sim.stackEntry
			var use StackSimulation = sim
			if kind == "proxy" {
				use = NewStackSimulationProxy(sim, sim.Push, sim.Pop)
			}
			switch kind {
			case "missing scope":
				d.LambdaCaptureOrderSlots = nil
			case "bad capture order":
				d.LambdaCaptureOrderSlots = []int{1, 0}
			case "changed parameter type":
				key.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "parameter overwrite":
				d.opCodes = append(d.opCodes, &OpCode{Instr: &Instruction{OpCode: OP_ASTORE_0}})
			case "changed entry order":
				d.Params = []values.JavaValue{late, key}
			case "missing producer":
				delete(d.stackValueProducers, key)
			case "disconnected producer":
				factory.Source = nil
			case "changed load packet":
				producer.Data = []byte{0}
			case "wide view":
				producer.IsWide = true
			case "changed factory bytes":
				d.bytecodes[2] = OP_NOP
			case "changed factory capture":
				lambda.Captures[0] = key
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "copies":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxNodeCopies: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.preserveEarlierLambdaCaptureOperands(factory, use, lambda)
			want := kind == "original" || kind == "proxy" || instance
			if (len(d.evaluationSnapshots[factory]) == 2) != want || calls != 0 || before.value != key {
				t.Fatalf("copy=%v callbacks=%d", len(d.evaluationSnapshots[factory]) == 2, calls)
			}
			if !want {
				if sim.stackEntry != before || sim.Peek() != key || len(d.disFoldRef) != 0 {
					t.Fatal("unproved packet published partial state")
				}
				return
			}
			copy := d.evaluationSnapshots[factory][0]
			if copy.Ref == nil || copy.Value != key || copy.Ref != sim.Peek() || copy.Operand || copy.OriginPC != 2 || d.evaluationSnapshots[factory][1].Ref != snapshot || sim.stackEntry == before {
				t.Fatal("original earlier value/factory order or private stack lost")
			}
			if pc, index, known := snapshot.OriginalPendingDynamicOperand(late); !known || pc != 2 || index != 0 {
				t.Fatal("changed the factory's own captured operand")
			}
			if !bytes.Equal(d.bytecodes, code) {
				t.Fatal("copy changed original Code")
			}
		})
	}
}

func TestEarlierLambdaOriginalInstructionRequiresWholePacket(t *testing.T) {
	for _, kind := range []string{"load", "factory", "truncated", "extra load data", "changed operand", "changed opcode", "wide", "custom", "foreign PC"} {
		t.Run(kind, func(t *testing.T) {
			d := NewDecompiler([]byte{OP_ALOAD, 1, OP_INVOKEDYNAMIC, 0, 2, 0, 0}, nil)
			op := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD}, Data: []byte{1}}
			switch kind {
			case "factory":
				op = &OpCode{CurrentOffset: 2, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}, Data: []byte{0, 2, 0, 0}}
			case "truncated":
				op.Data = nil
			case "extra load data":
				op.Data = []byte{1, 0}
			case "changed operand":
				op.Data[0] = 0
			case "changed opcode":
				op.Instr.OpCode = OP_ILOAD
			case "wide":
				op.IsWide = true
			case "custom":
				op.IsCustom = true
			case "foreign PC":
				op.CurrentOffset = 100
			}
			if got := d.lambdaEarlierOriginalInstruction(op); got != (kind == "load" || kind == "factory") {
				t.Fatalf("original packet=%v", got)
			}
		})
	}
}
