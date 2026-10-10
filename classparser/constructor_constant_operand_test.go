package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestConstructorMotionLiteralKeepsWidthsAndRejectsLinkage(t *testing.T) {
	pool := []ConstantInfo{&ConstantIntegerInfo{Value: 1000}, &ConstantLongInfo{Value: 7}, &ConstantFloatInfo{Value: 1.5}, &ConstantDoubleInfo{Value: -1.25}, &ConstantStringInfo{StringIndex: 6}, &ConstantUtf8Info{Value: "text"}, &ConstantClassInfo{NameIndex: 6}, &ConstantStringInfo{StringIndex: 99}, (*ConstantIntegerInfo)(nil), &ConstantStringInfo{StringIndex: 10}, &ConstantStringInfo{StringIndex: 7}}
	obj := &ClassObject{ConstantPool: pool}
	for _, tc := range []struct {
		name string
		code []byte
		want string
	}{
		{"null", []byte{core.OP_ACONST_NULL}, "null"},
		{"integer", []byte{core.OP_ICONST_M1}, "I"},
		{"byte", []byte{core.OP_BIPUSH, 128}, "I"},
		{"short", []byte{core.OP_SIPUSH, 128, 0}, "I"},
		{"long small", []byte{core.OP_LCONST_1}, "J"},
		{"float small", []byte{core.OP_FCONST_2}, "F"},
		{"double small", []byte{core.OP_DCONST_1}, "D"},
		{"integer pool", []byte{core.OP_LDC, 1}, "I"},
		{"integer wide index", []byte{core.OP_LDC_W, 0, 1}, "I"},
		{"long pool", []byte{core.OP_LDC2_W, 0, 2}, "J"},
		{"float pool", []byte{core.OP_LDC, 3}, "F"},
		{"double pool", []byte{core.OP_LDC2_W, 0, 4}, "D"},
		{"string", []byte{core.OP_LDC, 5}, "Ljava/lang/String;"},
		{"zero index", []byte{core.OP_LDC, 0}, ""},
		{"outside pool", []byte{core.OP_LDC_W, 127, 255}, ""},
		{"wide by narrow instruction", []byte{core.OP_LDC, 2}, ""},
		{"narrow by wide instruction", []byte{core.OP_LDC2_W, 0, 1}, ""},
		{"class linkage", []byte{core.OP_LDC, 7}, ""},
		{"malformed string", []byte{core.OP_LDC, 8}, ""},
		{"typed nil", []byte{core.OP_LDC, 9}, ""},
		{"cyclic string entry", []byte{core.OP_LDC, 10}, ""},
		{"nonUTF8 string entry", []byte{core.OP_LDC, 11}, ""},
		{"arbitrary UTF8", []byte{core.OP_LDC, 6}, ""},
		{"receiver", []byte{core.OP_ALOAD_0}, ""},
		{"getstatic effect", []byte{core.OP_GETSTATIC, 0, 1}, ""},
		{"call effect", []byte{core.OP_INVOKESTATIC, 0, 1}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := core.NewDecompiler(append(tc.code, core.OP_RETURN), func(int) values.JavaValue { return nil })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			op := decoder.OpcodeByPC(0)
			got, proved := constructorMotionLiteral(obj, op)
			if proved != (tc.want != "") || proved && got != tc.want {
				t.Fatalf("%q/%v want %q", got, proved, tc.want)
			}
			malformed := *op
			malformed.Data = append(append([]byte(nil), op.Data...), 0)
			if _, proved := constructorMotionLiteral(obj, &malformed); proved {
				t.Fatal("invalid operand length accepted")
			}
		})
	}
	if _, ok := constructorMotionLiteral(obj, nil); ok {
		t.Fatal("missing opcode accepted")
	}
	if _, ok := constructorMotionLiteral(nil, &core.OpCode{}); ok {
		t.Fatal("missing class accepted")
	}
}
