package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestCheckcastLaterExpressionsStayAboveCheckedOperand(t *testing.T) {
	tests := []struct {
		name       string
		ops        []byte
		descriptor string
		want       bool
	}{
		{"array read", []byte{OP_ALOAD_1, OP_ICONST_0, OP_IALOAD}, "(Ljava/lang/String;I)V", true},
		{"array length", []byte{OP_ALOAD_1, OP_ARRAYLENGTH}, "(Ljava/lang/String;I)V", true},
		{"wide values not words", []byte{OP_LLOAD_1, OP_LDC2_W, OP_LXOR, OP_ICONST_1, OP_LSHL}, "(Ljava/lang/String;J)V", true},
		{"division", []byte{OP_ICONST_1, OP_ILOAD_1, OP_IDIV}, "(Ljava/lang/String;I)V", true},
		{"conversion", []byte{OP_ILOAD_1, OP_I2L, OP_LNEG}, "(Ljava/lang/String;J)V", true},
		{"instance field", []byte{OP_ALOAD_1, OP_GETFIELD}, "(Ljava/lang/String;I)V", true},
		{"static field", []byte{OP_GETSTATIC}, "(Ljava/lang/String;I)V", true},
		{"two later args", []byte{OP_ALOAD_1, OP_ICONST_0, OP_IALOAD, OP_GETSTATIC}, "(Ljava/lang/String;II)V", true},
		{"array consumes cast", []byte{OP_ICONST_0, OP_IALOAD}, "(I)V", false},
		{"length consumes cast", []byte{OP_ARRAYLENGTH}, "(I)V", false},
		{"field consumes cast", []byte{OP_GETFIELD}, "(I)V", false},
		{"arithmetic consumes cast", []byte{OP_ICONST_0, OP_IADD}, "(I)V", false},
		{"conversion consumes cast", []byte{OP_I2L}, "(J)V", false},
		{"wide second operand missing", []byte{OP_LLOAD_1, OP_LXOR}, "(J)V", false},
		{"dup barrier", []byte{OP_ALOAD_1, OP_DUP}, "(Ljava/lang/String;II)V", false},
		{"store barrier", []byte{OP_ALOAD_1, OP_ASTORE_1}, "(Ljava/lang/String;)V", false},
		{"allocation barrier", []byte{OP_NEW}, "(Ljava/lang/String;Ljava/lang/Object;)V", false},
		{"branch barrier", []byte{OP_ALOAD_1, OP_IFNULL}, "(Ljava/lang/String;)V", false},
		{"discard barrier", []byte{OP_ALOAD_1, OP_POP}, "(Ljava/lang/String;)V", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typ, err := types.ParseMethodDescriptor(tc.descriptor)
			if err != nil {
				t.Fatal(err)
			}
			d := &Decompiler{constantPoolGetter: func(index int) values.JavaValue {
				if index == 1 {
					return values.NewJavaClassMember("Receiver", "consume", tc.descriptor, typ)
				}
				return values.NewJavaClassMember("Holder", "value", "I", types.NewJavaPrimer(types.JavaInteger))
			}, ConstantPoolLiteralGetter: func(int) values.JavaValue {
				return values.NewJavaLiteral(int64(4294967297), types.NewJavaPrimer(types.JavaLong))
			}}
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			previous := check
			for i, instr := range append(append([]byte{}, tc.ops...), OP_INVOKESTATIC) {
				op := &OpCode{Instr: &Instruction{OpCode: int(instr)}, CurrentOffset: uint16(4 + i*3), Source: []*OpCode{previous}}
				if instr == OP_INVOKESTATIC {
					op.Data = []byte{0, 1}
				} else if instr == OP_GETFIELD || instr == OP_GETSTATIC {
					op.Data = []byte{0, 2}
				} else if instr == OP_LDC2_W {
					op.Data = []byte{0, 3}
				}
				previous.Target = []*OpCode{op}
				previous = op
			}
			if got := d.canInlineCheckcastArgument(check); got != tc.want {
				t.Fatalf("region accepted=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestCheckcastLaterExpressionRejectsInvalidPoolAndEdges(t *testing.T) {
	for _, change := range []string{"valid", "pool method instead of field", "field void", "field malformed", "missing bytes", "unrepresented literal", "wrong literal width", "extra entry", "handler split", "back edge", "custom operation"} {
		t.Run(change, func(t *testing.T) {
			check := &OpCode{Instr: &Instruction{OpCode: OP_CHECKCAST}, CurrentOffset: 1}
			field := &OpCode{Instr: &Instruction{OpCode: OP_GETSTATIC}, CurrentOffset: 4, Data: []byte{0, 2}, Source: []*OpCode{check}}
			call := &OpCode{Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 7, Data: []byte{0, 1}, Source: []*OpCode{field}}
			check.Target = []*OpCode{field}
			field.Target = []*OpCode{call}
			desc := "I"
			ft := types.NewJavaPrimer(types.JavaInteger)
			ct, _ := types.ParseMethodDescriptor("(Ljava/lang/String;I)V")
			d := &Decompiler{}
			switch change {
			case "pool method instead of field":
				desc = "()I"
			case "field void":
				desc = "V"
			case "field malformed":
				desc = "bad"
			case "missing bytes":
				field.Data = nil
			case "unrepresented literal", "wrong literal width":
				field.Instr.OpCode = OP_LDC_W
				d.ConstantPoolLiteralGetter = func(int) values.JavaValue {
					if change == "unrepresented literal" {
						return values.NewJavaClassValue(types.NewJavaClass("java.lang.String"))
					}
					return values.NewJavaLiteral(int64(3), types.NewJavaPrimer(types.JavaLong))
				}
			case "extra entry":
				field.Source = append(field.Source, &OpCode{})
			case "handler split":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 4, HandlerPc: 20}}
			case "back edge":
				field.CurrentOffset = 0
			case "custom operation":
				field.IsCustom = true
			}
			d.constantPoolGetter = func(index int) values.JavaValue {
				if index == 1 {
					return values.NewJavaClassMember("Receiver", "consume", "(Ljava/lang/String;I)V", ct)
				}
				if change == "pool method instead of field" {
					t, _ := types.ParseMethodDescriptor(desc)
					return values.NewJavaClassMember("Holder", "get", desc, t)
				}
				return values.NewJavaClassMember("Holder", "field", desc, ft)
			}
			if got, want := d.canInlineCheckcastArgument(check), change == "valid"; got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
		})
	}
}
