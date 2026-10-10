package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestImmediateDynamicSnapshotExposesItsOriginalSourceBinding(t *testing.T) {
	desc := "(Ljava/util/function/Function;)V"
	typ, e := types.ParseMethodDescriptor(desc)
	if e != nil {
		t.Fatal(e)
	}
	member := values.NewJavaClassMember("Probe", "consume", desc, typ)
	d := NewDecompiler(nil, func(int) values.JavaValue { return member })
	d.FunctionContext.ClassName = "Probe"
	op := &OpCode{CurrentOffset: 10, Instr: &Instruction{OpCode: OP_INVOKEDYNAMIC}, Data: []byte{0, 1, 0, 0}}
	next := &OpCode{CurrentOffset: 15, Instr: &Instruction{OpCode: OP_INVOKESTATIC}, Data: []byte{0, 2}, Source: []*OpCode{op}}
	op.Target = []*OpCode{next}
	source := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	source.Id.SetName("input")
	snapshot := values.NewJavaRef(utils.NewRootVariableId(), nil, source.Type().Copy())
	snapshot.Id.SetName("snapshot")
	value := values.NewCustomValue(func(ctx *class_context.ClassContext) string { return snapshot.String(ctx) + "::toString" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Function") })
	value.Flag = "lambda"
	value.IsMethodRef = true
	value.CapturesKnown = true
	value.Captures = []values.JavaValue{snapshot}
	d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{op: {{Ref: snapshot, Value: source, Operand: true}}}
	if !d.canInlineImmediateMethodRef(op, value) {
		t.Fatal("original direct pure capture refused")
	}
	got := d.snapshotDynamicResult(op, nil, value)
	children, known := values.Children(snapshot)
	if !known || len(children) != 1 || children[0] != source {
		t.Fatalf("capture source hidden behind an opaque forwarding closure: %v %v", children, known)
	}
	inner, known := values.Children(children[0])
	if !known || len(inner) != 0 {
		t.Fatal("a source local acquired definition dependencies")
	}
	ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{source.Id: "reserved$input"}}
	if snapshot.String(ctx) != "reserved$input" || got.String(ctx) == "" {
		t.Fatal("scoped source binding not retained")
	}
	before, _ := values.InspectValue(source)
	after, _ := values.InspectValue(snapshot)
	if before != after {
		t.Fatalf("copy acquired effects %v -> %v", before, after)
	}
	if _, present := d.evaluationSnapshots[op]; present {
		t.Fatal("eager snapshot retained at an already adjacent use")
	}
}
