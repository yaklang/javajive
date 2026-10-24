package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLambdaCaptureSnapshotsKeepDescriptorWithoutChangingProducer(t *testing.T) {
	for _, variant := range []string{"widening", "same erasure generic", "array widening", "unrelated", "unknown hierarchy", "no factory witness", "changed factory capture", "wrong PC", "method reference"} {
		t.Run(variant, func(t *testing.T) {
			produced := types.JavaType(types.NewJavaClass("example.Narrow"))
			expected := types.JavaType(types.NewJavaClass("example.Base"))
			if variant == "array widening" {
				produced = types.NewJavaArrayType(types.NewJavaClass("example.Narrow"))
				expected = types.NewJavaArrayType(types.NewJavaClass("example.Base"))
			}
			if variant == "same erasure generic" {
				produced = types.NewParameterizedType("example.Base", []types.JavaType{types.NewJavaClass("java.lang.String")})
			}
			if variant == "unrelated" {
				expected = types.NewJavaClass("example.Other")
			}
			provider := func(name string) (callbinding.Class, bool) {
				if variant == "unknown hierarchy" {
					return callbinding.Class{}, false
				}
				parents := map[string][]string{"example/Narrow": {"example/Base"}, "example/Base": {"java/lang/Object"}, "example/Other": {"java/lang/Object"}, "java/lang/Object": {}}
				p, ok := parents[name]
				return callbinding.Class{Name: name, Parents: p, ParentsComplete: ok}, ok
			}
			ctx := &class_context.ClassContext{InvocationMetadata: provider}
			d := &Decompiler{FunctionContext: ctx}
			op := &OpCode{Instr: InstrInfos[OP_INVOKEDYNAMIC], CurrentOffset: 42}
			d.opCodes = []*OpCode{op}
			source := values.NewJavaRef(utils.NewRootVariableId(), nil, produced)
			seed := values.NewSlotValue(source, produced)
			sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId().Next())
			args, err := d.snapshotDynamicOperands(op, sim, []values.JavaValue{seed}, []types.JavaType{expected})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := args[0].(*values.JavaRef)
			lambda := &values.CustomValue{Flag: "lambda", HasOriginPC: true, OriginPC: 42, CapturesKnown: true, Captures: []values.JavaValue{snapshot}, IsMethodRef: variant == "method reference"}
			if variant != "no factory witness" {
				lambda.MarkOriginalLambdaFactory(42)
			}
			if variant == "changed factory capture" {
				lambda.Captures[0] = source
			}
			if variant == "wrong PC" {
				op.CurrentOffset = 43
			}
			d.markLambdaCaptureDescriptorSnapshots(op, lambda)
			wantWidening := variant == "widening" || variant == "array widening"
			if d.refreshReferenceOperandSnapshotTypes() != wantWidening {
				t.Fatal("capture declaration conversion was not independently proved", variant)
			}
			want := produced
			if wantWidening {
				want = expected
			}
			if got, _ := values.SourceTypeErasure(snapshot.Type(), ctx); got != func() string { s, _ := values.SourceTypeErasure(want, ctx); return s }() {
				t.Fatal("capture declaration has wrong descriptor", variant)
			}
			if source.Type().String(ctx) != produced.String(ctx) || snapshot.Val != seed || d.evaluationSnapshots[op][0].Value != seed || snapshot.Id == source.Id {
				t.Fatal("capture declaration view changed original producer identity/value")
			}
			if variant == "same erasure generic" && snapshot.Type().String(ctx) != produced.String(ctx) {
				t.Fatal("equal erasure lost the solved generic source declaration")
			}
			if d.refreshReferenceOperandSnapshotTypes() {
				t.Fatal("capture type fixed point oscillated")
			}
		})
	}
}
