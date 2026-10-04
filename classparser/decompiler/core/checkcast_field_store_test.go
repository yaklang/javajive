package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestImmediateCheckcastFieldStoreRequiresPrivateExceptionEdge(t *testing.T) {
	for _, v := range []string{"instance", "static", "array", "widening reference", "extra entry", "different predecessor", "handler boundary", "duplicate", "local store", "primitive field", "method descriptor", "missing field", "operand bytes", "backward edge", "two targets", "custom store", "catch store", "wrong cast", "primitive cast", "missing cast type"} {
		t.Run(v, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, Data: []byte{0, 2}, CurrentOffset: 1}
			store := &OpCode{Instr: &Instruction{OpCode: OP_PUTFIELD}, Data: []byte{0, 1}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{store}
			field := values.NewJavaClassMember("example/Owner", "value", "Ljava/lang/String;", types.NewJavaClass("java.lang.String"))
			cast := types.NewJavaClass("java.lang.String")
			d := &Decompiler{}
			d.constantPoolGetter = func(int) values.JavaValue { return field }
			switch v {
			case "static":
				store.Instr.OpCode = OP_PUTSTATIC
			case "array":
				field.Description = "[Ljava/lang/String;"
				cast = types.NewJavaArrayType(cast)
			case "widening reference":
				field.Description = "Ljava/lang/Object;"
			case "extra entry":
				store.Source = append(store.Source, &OpCode{})
			case "different predecessor":
				store.Source = []*OpCode{{}}
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "duplicate":
				store.Instr.OpCode = OP_DUP
			case "local store":
				store.Instr.OpCode = OP_ASTORE
			case "primitive field":
				field.Description = "I"
			case "method descriptor":
				field.Description = "()Ljava/lang/String;"
			case "missing field":
				d.constantPoolGetter = func(int) values.JavaValue { return nil }
			case "operand bytes":
				store.Data = nil
			case "backward edge":
				store.CurrentOffset = 0
			case "two targets":
				check.Target = append(check.Target, &OpCode{})
			case "custom store":
				store.IsCustom = true
			case "catch store":
				store.IsCatch = true
			case "wrong cast":
				check.Instr.OpCode = OP_ALOAD
			case "primitive cast":
				cast = types.NewJavaPrimer(types.JavaInteger)
			case "missing cast type":
				cast = nil
			}
			want := v == "instance" || v == "static" || v == "array" || v == "widening reference"
			if got := d.canInlineImmediateCheckcastFieldStore(check, cast); got != want {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
