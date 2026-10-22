package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestRegeneratedStaticArrayRequiresClosedSourceOccurrence(t *testing.T) {
	for _, change := range []string{"canonical", "nop", "wrong owner", "wrong field", "wrong allocation", "wrong length", "wrong component", "wrong index", "wrong field read", "forged read", "wrong fill value", "different dup", "different sink", "extra original use", "extra source use", "duplicate sink", "ambiguous opcode", "live source gap", "unbound source store", "budget"} {
		t.Run(change, func(t *testing.T) {
			owner := "example.Choice"
			typ := types.NewJavaClass(owner)
			arrayType := types.NewJavaArrayType(typ)
			literal := func(n int) *values.JavaLiteral {
				return values.NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger))
			}
			array := values.NewNewArrayExpression(arrayType, literal(2))
			array.OriginPC, array.HasOriginPC, array.EvaluationEndPC, array.HasEvaluationEndPC = 1, true, 9, true
			ref := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType)
			other := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType)
			pool := map[int]values.JavaValue{1: values.NewJavaClassValue(typ)}
			for i, name := range []string{"LEFT", "RIGHT"} {
				member := values.NewJavaClassMember(owner, name, "Lexample/Choice;", typ)
				pool[i+2] = member
				element := values.NewJavaClassMember(owner, name, member.Description, typ)
				element.OriginPC, element.HasOriginPC = 4+4*i, true
				element.MarkOriginalFieldRead(member, element.OriginPC)
				array.Initializer = append(array.Initializer, element)
			}
			field := values.NewJavaClassMember(owner, "backing", "[Lexample/Choice;", arrayType)
			pool[4] = field
			allocation := op(OP_ANEWARRAY, 1)
			allocation.Data, allocation.stackProduced = []byte{0, 1}, []values.JavaValue{array}
			ops := []*OpCode{allocation}
			for i := 0; i < 2; i++ {
				dup, index, read, fill := op(OP_DUP, uint16(2+4*i)), op(OP_ICONST_0+i, uint16(3+4*i)), op(OP_GETSTATIC, uint16(4+4*i)), op(OP_AASTORE, uint16(5+4*i))
				if i == 0 {
					dup.stackConsumed, dup.stackProduced = []values.JavaValue{array}, []values.JavaValue{ref, ref}
				} else {
					dup.stackProduced = []values.JavaValue{ref}
				}
				read.Data = []byte{0, byte(i + 2)}
				fill.stackConsumed = []values.JavaValue{array.Initializer[i], literal(i), ref}
				ops = append(ops, dup, index, read, fill)
			}
			sink := op(OP_PUTSTATIC, 10)
			sink.Data, sink.stackConsumed = []byte{0, 4}, []values.JavaValue{ref}
			ops = append(ops, sink)
			producer := statements.NewAssignStatement(ref, array, true)
			assignment := statements.NewAssignStatement(field, ref, false)
			assignment.OriginPC, assignment.HasOriginPC = 10, true
			body := []statements.Statement{producer, assignment}
			d := &Decompiler{opCodes: ops, FunctionContext: &class_context.ClassContext{ClassName: owner, FunctionName: "<clinit>"}, constantPoolGetter: func(i int) values.JavaValue { return pool[i] }}
			d.opcodeToSimulateStack = make(map[*OpCode]*StackSimulationImpl)
			for _, instruction := range ops {
				d.opcodeToSimulateStack[instruction] = nil
			}
			packet := RegeneratedStaticArray{Owner: "example/Choice", Field: "backing", Descriptor: "[Lexample/Choice;", AllocationPC: 1, StorePC: 10, Constants: []string{"LEFT", "RIGHT"}}
			switch change {
			case "nop":
				d.opCodes = append(d.opCodes[:5:5], append([]*OpCode{op(OP_NOP, 5)}, d.opCodes[5:]...)...)
			case "wrong owner":
				packet.Owner = "elsewhere/Choice"
			case "wrong field":
				packet.Field = "unrelated"
			case "wrong allocation":
				array.OriginPC++
			case "wrong length":
				array.Length[0] = literal(3)
			case "wrong component":
				pool[1] = values.NewJavaClassValue(types.NewJavaClass("java.lang.Object"))
			case "wrong index":
				ops[6].Instr.OpCode = OP_ICONST_0
			case "wrong field read":
				ops[7].Data = []byte{0, 2}
			case "forged read":
				forged := values.NewJavaClassMember(owner, "RIGHT", "Lexample/Choice;", typ)
				forged.OriginPC, forged.HasOriginPC = 8, true
				array.Initializer[1], ops[8].stackConsumed[0] = forged, forged
			case "wrong fill value":
				ops[8].stackConsumed[0] = array.Initializer[0]
			case "different dup":
				ops[5].stackProduced[0] = other
			case "different sink":
				sink.stackConsumed[0] = other
			case "extra original use":
				extra := op(OP_ASTORE_0, 11)
				extra.stackConsumed = []values.JavaValue{ref}
				d.opCodes = append(d.opCodes, extra)
			case "extra source use":
				body = append(body, &statements.ReturnStatement{JavaValue: ref})
			case "duplicate sink":
				body = append(body, assignment)
			case "ambiguous opcode":
				d.opcodeToSimulateStack[op(OP_PUTSTATIC, 10)] = nil
			case "live source gap":
				body = []statements.Statement{producer, &statements.ReturnStatement{JavaValue: values.JavaNull}, assignment}
			case "unbound source store":
				assignment.HasOriginPC = false
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			before := append([]statements.Statement(nil), body...)
			want := change == "canonical" || change == "nop"
			if got := d.ProjectRegeneratedStaticArray(&body, packet); got != want {
				t.Fatalf("source occurrence projection=%v want=%v", got, want)
			}
			if want && len(body) != 0 {
				t.Fatal("owned allocation or field store leaked into source")
			}
			if !want {
				if len(body) != len(before) {
					t.Fatal("refused projection changed the source")
				}
				for i := range before {
					if body[i] != before[i] {
						t.Fatal("refused projection replaced a statement")
					}
				}
			}
		})
	}
}

func TestRegeneratedArrayIndexUsesEncodedSignedOperands(t *testing.T) {
	d := &Decompiler{}
	for _, tt := range []struct {
		name string
		code int
		data []byte
		want int
	}{{"negative iconst", OP_ICONST_M1, nil, -1}, {"iconst", OP_ICONST_5, nil, 5}, {"bipush", OP_BIPUSH, []byte{127}, 127}, {"negative bipush", OP_BIPUSH, []byte{255}, -1}, {"sipush", OP_SIPUSH, []byte{0, 128}, 128}, {"negative sipush", OP_SIPUSH, []byte{255, 255}, -1}} {
		t.Run(tt.name, func(t *testing.T) {
			instruction := op(tt.code, 1)
			instruction.Data = tt.data
			if !d.regeneratedArrayIndex(instruction, tt.want) || d.regeneratedArrayIndex(instruction, tt.want+1) {
				t.Fatal("encoded JVM integer changed")
			}
		})
	}
}
