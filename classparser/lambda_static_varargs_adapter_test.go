package javaclassparser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialStaticVarargsPackKeepsOriginalParameterPorts(t *testing.T) {
	compiler, _ := t04Tools(t)
	const source = `import java.util.function.*;
class StaticPack {
 static Object[] pack(Object... values){return values;}
 static Object[] fixed(long marker,Object... values){return values;}
 static int primitive(int... values){return values.length;}
 static void consume(Object... values){}
 static BiFunction<String,Integer,Object[]> pair(){return StaticPack::pack;}
 static BiFunction<Long,String,Object[]> wide(){return StaticPack::fixed;}
 static Supplier<Object[]> empty(){return StaticPack::pack;}
 static IntBinaryOperator ints(){return StaticPack::primitive;}
 static BiConsumer<String,Integer> discard(){return StaticPack::consume;}
}`
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, source, "StaticPack", "none")
	for _, scenario := range []string{"pair", "wide", "empty", "ints", "discard", "reordered ports", "repeated port", "wrong pack length", "wrong array store", "not original synthetic"} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["StaticPack.class"]))
			if err != nil {
				t.Fatal(err)
			}
			factory := scenario
			if strings.Contains(scenario, " ") {
				factory = "pair"
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range object.Methods {
				if name, _ := sourceBridgeUTF8(object, m.NameIndex); strings.HasPrefix(name, "lambda$"+factory+"$") {
					if method != nil {
						t.Fatal("ambiguous original adapter")
					}
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("missing original static adapter")
			}
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(object.ConstantPool, index) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("original adapter did not decode")
			}
			switch scenario {
			case "reordered ports":
				for _, instruction := range constructorMotionOps(decoder) {
					if instruction.Instr.OpCode == core.OP_ALOAD_0 {
						code.Code[instruction.CurrentOffset] = core.OP_ALOAD_1
					} else if instruction.Instr.OpCode == core.OP_ALOAD_1 {
						code.Code[instruction.CurrentOffset] = core.OP_ALOAD_0
					}
				}
			case "repeated port":
				for _, instruction := range constructorMotionOps(decoder) {
					if instruction.Instr.OpCode == core.OP_ALOAD_1 {
						code.Code[instruction.CurrentOffset] = core.OP_ALOAD_0
					}
				}
			case "wrong pack length":
				if code.Code[0] != core.OP_ICONST_2 {
					t.Fatal("pair adapter does not start with its actual two-element allocation")
				}
				code.Code[0] = core.OP_ICONST_1
			case "wrong array store":
				for _, instruction := range constructorMotionOps(decoder) {
					if instruction.Instr.OpCode == core.OP_AASTORE {
						code.Code[instruction.CurrentOffset] = core.OP_IASTORE
					}
				}
			case "not original synthetic":
				method.AccessFlags &^= 0x1000
			}
			shape, known := NewClassObjectDumper(object).originalVarargsAdapterShape(method)
			want := scenario == factory
			if known != want || known && (shape.opcode != core.OP_INVOKESTATIC || shape.method != method || shape.code != code) {
				t.Fatalf("original static ports known=%v want=%v shape=%+v", known, want, shape)
			}
			if known && scenario == "empty" && (len(shape.parameters) != 0 || shape.packCount != 0) {
				t.Fatal("invented a receiver or argument for zero-arity adapter")
			}
		})
	}
}
