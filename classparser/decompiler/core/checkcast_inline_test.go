package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestImmediateCheckcastArgumentProof(t *testing.T) {
	for _, change := range []string{"valid", "other source", "handler boundary", "dup", "constructor", "no arguments", "primitive", "invalid source"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			invoke := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, Data: []byte{0, 1}, CurrentOffset: 4, Source: []*OpCode{check}}
			check.Target = []*OpCode{invoke}
			descriptor, name := "(ILjava/lang/CharSequence;)Ljava/lang/String;", "join"
			d := &Decompiler{}
			switch change {
			case "other source":
				invoke.Source = append(invoke.Source, &OpCode{})
			case "handler boundary":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 8}}
			case "dup":
				invoke.Instr = &Instruction{OpCode: OP_DUP}
			case "constructor":
				name = "<init>"
			case "no arguments":
				descriptor = "()V"
			case "primitive":
				descriptor = "(I)V"
			case "invalid source":
				check.Instr = &Instruction{OpCode: OP_ALOAD}
			}
			typ, err := types.ParseMethodDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			d.constantPoolGetter = func(int) values.JavaValue { return values.NewJavaClassMember("Probe", name, descriptor, typ) }
			if got := d.canInlineImmediateCheckcastArgument(check); got != (change == "valid") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
