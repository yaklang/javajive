package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorScalarPacketCategories(t *testing.T) {
	for _, tc := range []struct {
		opcode         int
		inputs, result string
	}{
		{core.OP_IADD, "II", "I"}, {core.OP_LDIV, "JJ", "J"}, {core.OP_FMUL, "FF", "F"}, {core.OP_DREM, "DD", "D"},
		{core.OP_INEG, "I", "I"}, {core.OP_DNEG, "D", "D"},
		{core.OP_ISHL, "II", "I"}, {core.OP_LSHR, "JI", "J"}, {core.OP_LUSHR, "JI", "J"}, {core.OP_LXOR, "JJ", "J"},
		{core.OP_I2L, "I", "J"}, {core.OP_L2I, "J", "I"}, {core.OP_F2D, "F", "D"}, {core.OP_D2F, "D", "F"},
		{core.OP_LCMP, "JJ", "I"}, {core.OP_FCMPL, "FF", "I"}, {core.OP_FCMPG, "FF", "I"}, {core.OP_DCMPL, "DD", "I"}, {core.OP_DCMPG, "DD", "I"},
	} {
		t.Run(core.InstrInfos[tc.opcode].Name, func(t *testing.T) {
			inputs, result, ok := constructorScalarPacketContract(tc.opcode)
			if !ok || inputs != tc.inputs || result != tc.result {
				t.Fatalf("scalar contract=%q/%q/%v want=%q/%q", inputs, result, ok, tc.inputs, tc.result)
			}
			args := make([]string, len(tc.inputs))
			for i := range args {
				args[i] = tc.inputs[i : i+1]
			}
			if !constructorScalarPacketOperands(args, inputs) {
				t.Fatal("original exact categories refused")
			}
			for _, wrong := range []string{"", "Z", "null", "Ljava/lang/Integer;", "@allocation:3", "Ibad", "[I"} {
				changed := append([]string(nil), args...)
				changed[len(changed)-1] = wrong
				if constructorScalarPacketOperands(changed, inputs) {
					t.Fatalf("operand %q borrowed a scalar category", wrong)
				}
			}
			if constructorScalarPacketOperands(args[1:], inputs) {
				t.Fatal("underflow borrowed a scalar category")
			}
		})
	}
	for _, opcode := range []int{core.OP_I2B, core.OP_I2C, core.OP_I2S, core.OP_GETFIELD, core.OP_ARRAYLENGTH, core.OP_INVOKESTATIC, core.OP_IFEQ, core.OP_NEW, core.OP_DUP, core.OP_ATHROW, core.OP_RETURN} {
		if _, _, ok := constructorScalarPacketContract(opcode); ok {
			t.Fatalf("non-scalar opcode %d borrowed scalar contract", opcode)
		}
	}
	if !constructorScalarPacketOperands([]string{"B", "C"}, "II") || !constructorScalarPacketOperands([]string{"J", "S"}, "JI") || constructorScalarPacketOperands([]string{"J", "J"}, "JI") || constructorScalarPacketOperands([]string{"I", "I"}, "JI") {
		t.Fatal("logical narrow words or long shift count lost their original computational category")
	}
}

func TestAdversarialConstructorScalarPacketOriginalOperations(t *testing.T) {
	files := nativeCompileDebugClasses(t, scalarPacketFixture, "none")
	for _, variant := range []string{"original", "changed view opcode", "changed view operands", "wide view", "different original int operation", "wrong original wide operation", "receiver operand", "missing operand", "long shift count", "small stack", "static constructor", "duplicate Code"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["ScalarOwner$Part.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			var constructor *MemberInfo
			var desc string
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "<init>" {
					continue
				}
				constructor = method
				desc, _ = sourceBridgeUTF8(obj, method.DescriptorIndex)
				for _, a := range method.Attributes {
					if original, ok := a.(*CodeAttribute); ok {
						code = original
					}
				}
			}
			parse := func() []*core.OpCode {
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if decoder.ParseOpcode() != nil {
					t.Fatal("cannot decode original scalar fixture")
				}
				return constructorMotionOps(decoder)
			}
			ops := parse()
			xor, shift := -1, -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_IXOR {
					xor = i
				}
				if op.Instr.OpCode == core.OP_IUSHR {
					shift = i
				}
			}
			if xor < 0 || shift < 0 || code == nil || constructor == nil {
				t.Fatal("missing original scalar operations")
			}
			want := variant == "original" || variant == "different original int operation"
			switch variant {
			case "changed view opcode":
				instruction := *ops[xor].Instr
				instruction.OpCode = core.OP_IADD
				ops[xor].Instr = &instruction
			case "changed view operands":
				ops[xor].Data = []byte{0}
			case "wide view":
				ops[xor].IsWide = true
			case "different original int operation", "wrong original wide operation":
				opcode := core.OP_IADD
				if !want {
					opcode = core.OP_LXOR
				}
				code.Code[ops[xor].CurrentOffset] = byte(opcode)
				ops = parse()
			case "receiver operand", "missing operand":
				opcode := core.OP_ALOAD_0
				if variant == "missing operand" {
					opcode = core.OP_NOP
				}
				code.Code[ops[4].CurrentOffset] = byte(opcode)
				ops = parse()
			case "long shift count":
				code.Code[ops[shift-1].CurrentOffset] = byte(core.OP_LCONST_1)
				ops = parse()
			case "small stack":
				code.MaxStack = 2
			case "static constructor":
				constructor.AccessFlags |= 8
			case "duplicate Code":
				constructor.Attributes = append(constructor.Attributes, code)
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			next, member := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
			if got := next > 0 && member != nil; got != want {
				t.Fatalf("scalar packet closed=%v want=%v", got, want)
			}
			if member != nil && (member.Name != "ScalarTarget" || member.Description != "(I)V") {
				t.Fatal("original constructor binding changed")
			}
		})
	}
}
