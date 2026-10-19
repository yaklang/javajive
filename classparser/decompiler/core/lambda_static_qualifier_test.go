package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLambdaStaticQualifierRequiresOriginalAdjacentCaptureAndClassTarget(t *testing.T) {
	for _, variant := range []string{"original", "wide argument", "ordinary method", "not captured", "unproved parameter", "wrong slot", "foreign erasure", "discard pair", "effectful qualifier", "argument store", "wrong argument width", "join", "fork", "gap", "wide encoding", "handler entry", "instance call", "missing witness", "wrong PC", "interface", "incomplete members", "incomplete parents", "missing target", "instance target", "duplicate target", "work", "memory"} {
		t.Run(variant, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("fixture.Engine"))
			ref.IsParam = true
			if variant != "unproved parameter" {
				ref.MarkOriginalParameter(0)
			}
			if variant == "foreign erasure" {
				ref.ResetVarType(types.NewJavaClass("fixture.Other"))
			}
			argument := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaPrimer(types.JavaInteger))
			load := &OpCode{Instr: &Instruction{OpCode: OP_ALOAD_0}, CurrentOffset: 0}
			pop := &OpCode{Instr: &Instruction{OpCode: OP_POP}, CurrentOffset: 1, stackConsumed: []values.JavaValue{ref}}
			arg := &OpCode{Instr: &Instruction{OpCode: OP_ILOAD_1}, CurrentOffset: 2}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 3, Data: []byte{0, 1}}
			for i, op := range []*OpCode{load, pop, arg} {
				next := []*OpCode{pop, arg, invoke}[i]
				op.Target, next.Source = []*OpCode{next}, []*OpCode{op}
			}
			owner := values.NewJavaClassValue(types.NewJavaClass("fixture.Engine"))
			call := &values.FunctionCallExpression{ClassName: "fixture.Engine", FunctionName: "read", Descriptor: "(I)I", IsStatic: true, Kind: values.InvokeStatic, HasOriginPC: true, OriginPC: 3, Object: owner, Arguments: []values.JavaValue{argument}}
			metadata := callbinding.Class{Name: "fixture/Engine", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: "(I)I", Static: true}}}
			d := &Decompiler{LambdaCaptureOrderSlots: []int{0}, FunctionContext: &class_context.ClassContext{IsStatic: true}}
			d.FunctionContext.InvocationMetadata = func(name string) (callbinding.Class, bool) { return metadata, name == metadata.Name }
			switch variant {
			case "wide argument":
				arg.Instr.OpCode = OP_LLOAD_1
				call.Descriptor, metadata.Methods[0].Desc = "(J)I", "(J)I"
			case "ordinary method":
				d.LambdaCaptureOrderSlots = nil
			case "not captured":
				d.LambdaCaptureOrderSlots = []int{1}
			case "wrong slot":
				load.Instr.OpCode = OP_ALOAD_1
			case "discard pair":
				pop.Instr.OpCode = OP_POP2
			case "effectful qualifier":
				load.Instr.OpCode = OP_GETSTATIC
			case "argument store":
				arg.Instr.OpCode = OP_ISTORE_1
			case "wrong argument width":
				arg.Instr.OpCode = OP_LLOAD_1
			case "join":
				arg.Source = append(arg.Source, load)
			case "fork":
				pop.Target = append(pop.Target, invoke)
			case "gap":
				arg.CurrentOffset++
			case "wide encoding":
				arg.IsWide = true
			case "handler entry":
				d.ExceptionTable = []*ExceptionTableEntry{{HandlerPc: arg.CurrentOffset}}
			case "instance call":
				call.IsStatic = false
			case "missing witness":
				call.HasOriginPC = false
			case "wrong PC":
				call.OriginPC++
			case "interface":
				metadata.IsInterface = true
			case "incomplete members":
				metadata.MembersComplete = false
			case "incomplete parents":
				metadata.ParentsComplete = false
			case "missing target":
				metadata.Methods = nil
			case "instance target":
				metadata.Methods[0].Static = false
			case "duplicate target":
				metadata.Methods = append(metadata.Methods, metadata.Methods[0])
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1, MaxGraphEdges: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			d.retainLambdaStaticQualifier(invoke, call)
			want := variant == "original" || variant == "wide argument"
			if got := call.Object == ref; got != want {
				t.Fatalf("retained qualifier=%v want=%v", got, want)
			}
			if !want && call.Object != owner {
				t.Fatal("failed qualifier proof changed the original call")
			}
		})
	}
}
