package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// These packets set the expected frontier independently of the simulator's
// historical JavaRef.Val. The real immutable CFG supplies ordinary and
// instruction-input exception edges, not the printer's synthetic try anchors.
func TestNullCaptureProjectionRequiresAllOriginalReachingValues(t *testing.T) {
	for _, kind := range []string{"straight", "matching descriptor", "array target", "string target", "two null arms", "non-null arm", "entry arm", "exception null", "exception non-null", "wide overwrite", "source history only", "wrong load bytes", "wrong factory bytes", "custom load", "unowned operand", "changed seed", "cancel", "scans", "memory"} {
		t.Run(kind, func(t *testing.T) {
			code := []byte{OP_ACONST_NULL, OP_ASTORE_1, OP_ALOAD_1, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_ARETURN}
			handlers := []*ExceptionTableEntry{}
			switch kind {
			case "two null arms", "non-null arm", "entry arm":
				code = []byte{OP_ILOAD_0, OP_IFEQ, 0, 8, OP_ACONST_NULL, OP_ASTORE_1, OP_GOTO, 0, 5, OP_ACONST_NULL, OP_ASTORE_1, OP_ALOAD_1, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_ARETURN}
				if kind == "non-null arm" {
					code[9] = OP_ALOAD_2
				}
				if kind == "entry arm" {
					code[2], code[3] = 0, 10
				}
			case "exception null", "exception non-null":
				code = []byte{OP_ACONST_NULL, OP_ASTORE_1, OP_INVOKESTATIC, 0, 1, OP_GOTO, 0, 6, OP_ASTORE_2, OP_ACONST_NULL, OP_ASTORE_1, OP_ALOAD_1, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_ARETURN}
				if kind == "exception non-null" {
					code[9] = OP_ALOAD_0
				}
				handlers = []*ExceptionTableEntry{{StartPc: 2, EndPc: 5, HandlerPc: 8, CatchType: 1}}
			case "wide overwrite":
				code = []byte{OP_ACONST_NULL, OP_ASTORE_1, OP_LCONST_0, OP_LSTORE_0, OP_ALOAD_1, OP_INVOKEDYNAMIC, 0, 1, 0, 0, OP_ARETURN}
			}
			d := auditCFG(t, code)
			d.FunctionContext = &class_context.ClassContext{IsStatic: true}
			parsed, parseErr := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/Object;")
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			d.FunctionType = parsed.FunctionType()
			d.ExceptionTable = handlers
			g, err := d.buildSemanticCFG()
			if err != nil {
				t.Fatal(err)
			}
			d.semanticCFG = g
			object := types.NewJavaClass("java.lang.Object")
			target := types.NewJavaClass("probe.CaptureType")
			if kind == "array target" || kind == "string target" {
				desc := "([Ljava/lang/String;)V"
				if kind == "string target" {
					desc = "(Ljava/lang/String;)V"
				}
				parsed, e := types.ParseMethodDescriptor(desc)
				if e != nil {
					t.Fatal(e)
				}
				target = parsed.FunctionType().ParamTypes[0]
			}
			d.stackValueProducers = map[values.JavaValue]*OpCode{}
			var factory, load *OpCode
			for _, op := range d.opCodes {
				if op == nil || op.Instr == nil {
					continue
				}
				k := op.Instr.OpCode
				if k == OP_ACONST_NULL || isReferenceLoadOpcode(k) {
					seed := values.NewJavaRef(nil, nil, object.Copy())
					seed.Id = utils.NewRootVariableId()
					value := values.NewSlotValue(seed, object.Copy())
					op.stackProduced = []values.JavaValue{value}
					d.stackValueProducers[value] = op
				}
				if k == OP_INVOKEDYNAMIC {
					factory = op
				}
			}
			for i, op := range d.opCodes {
				if op == nil || op.Instr == nil || !isReferenceStoreOpcode(op.Instr.OpCode) || i == 0 {
					continue
				}
				prev := d.opCodes[i-1]
				if len(prev.stackProduced) == 1 {
					op.stackConsumed = []values.JavaValue{prev.stackProduced[0]}
				}
			}
			for i, op := range d.opCodes {
				if op == factory && i > 0 {
					load = d.opCodes[i-1]
				}
			}
			if factory == nil || load == nil || len(load.stackProduced) != 1 {
				t.Fatal("fixture packet")
			}
			source := load.stackProduced[0]
			if kind == "matching descriptor" {
				values.UnpackSoltValue(source).(*values.JavaRef).ResetVarType(target.Copy())
			}
			originalSource, _ := values.SourceTypeErasure(source.Type(), d.FunctionContext)
			ref := values.NewJavaRef(utils.NewRootVariableId(), source, object.Copy())
			pc := int(factory.CurrentOffset)
			if kind != "unowned operand" {
				ref.MarkOriginalDynamicOperand(pc, 0, source)
			}
			d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{factory: {{Ref: ref, Value: source, ExpectedType: target, OriginPC: pc, Operand: true, lambdaCapture: true}}}
			switch kind {
			case "source history only":
				delete(d.stackValueProducers, source)
				values.UnpackSoltValue(source).(*values.JavaRef).Val = values.JavaNull
			case "wrong load bytes":
				d.bytecodes[int(load.CurrentOffset)] = OP_ALOAD_2
			case "wrong factory bytes":
				factory.Data[3] = 1
			case "custom load":
				load.IsCustom = true
			case "changed seed":
				ref.Val = values.JavaNull
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "scans":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			d.refineNullLambdaCaptureOperands()
			view, declared, known := ref.OriginalDynamicOperandSourceView(source)
			want := kind == "straight" || kind == "matching descriptor" || kind == "array target" || kind == "string target" || kind == "two null arms" || kind == "exception null"
			if known != want {
				t.Fatalf("projection=%v expected=%v", known, want)
			}
			if !known {
				return
			}
			if !values.IsNullLiteral(view) || d.evaluationSnapshots[factory][0].Value != source || ref.Val != source {
				t.Fatal("logical operand/seed replaced")
			}
			d.refreshReferenceOperandSnapshotTypes()
			if actual, _ := values.SourceTypeErasure(ref.Type(), d.FunctionContext); actual != func() string { v, _ := values.SourceTypeErasure(declared, d.FunctionContext); return v }() {
				t.Fatal("capture descriptor lost", actual)
			}
			if actual, _ := values.SourceTypeErasure(source.Type(), d.FunctionContext); actual != originalSource {
				t.Fatal("source declaration changed", actual)
			}
		})
	}
}
