package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorBooleanLiteralRequiresOriginalCanonicalWord(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class WordBase{WordBase(boolean flag,int n){}}class WordOwner{class Part extends WordBase{Part(int n){super(true,n);}}}`, "none")
	for _, variant := range []string{"true", "false", "bipush true", "sipush false", "ldc true", "ldc false", "int two", "int minus one", "bipush two", "sipush negative", "ldc two", "unknown int", "wide literal", "bad literal bytes", "truncated bipush", "truncated sipush", "bad ldc constant", "bad ldc index", "long literal", "source cast without original instruction"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["WordOwner$Part.class"])
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			var desc string
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "<init>" {
					continue
				}
				desc, _ = sourceBridgeUTF8(obj, method.DescriptorIndex)
				for _, attr := range method.Attributes {
					if c, ok := attr.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("missing original Code")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			if len(ops) != 8 || ops[2].Instr.OpCode != core.OP_PUTFIELD || ops[4].Instr.OpCode != core.OP_ICONST_1 {
				t.Fatal("missing physical capture/boolean word")
			}
			literal := ops[4]
			instr := *literal.Instr
			literal.Instr = &instr
			want := false
			switch variant {
			case "true":
				want = true
			case "false":
				instr.OpCode = core.OP_ICONST_0
				want = true
			case "bipush true":
				instr.OpCode = core.OP_BIPUSH
				literal.Data = []byte{1}
				want = true
			case "sipush false":
				instr.OpCode = core.OP_SIPUSH
				literal.Data = []byte{0, 0}
				want = true
			case "ldc true", "ldc false", "ldc two", "bad ldc constant":
				value := int32(1)
				if variant == "ldc false" {
					value = 0
				}
				if variant == "ldc two" {
					value = 2
				}
				obj.ConstantPool = append(obj.ConstantPool, &ConstantIntegerInfo{Value: value})
				index := len(obj.ConstantPool)
				if index > 255 {
					t.Fatal("authored constant pool too large")
				}
				instr.OpCode = core.OP_LDC
				literal.Data = []byte{byte(index)}
				if variant == "bad ldc constant" {
					obj.ConstantPool[index-1] = &ConstantUtf8Info{Value: "1"}
				}
				want = variant == "ldc true" || variant == "ldc false"
			case "int two":
				instr.OpCode = core.OP_ICONST_2
			case "int minus one":
				instr.OpCode = core.OP_ICONST_M1
			case "bipush two":
				instr.OpCode = core.OP_BIPUSH
				literal.Data = []byte{2}
			case "sipush negative":
				instr.OpCode = core.OP_SIPUSH
				literal.Data = []byte{255, 255}
			case "unknown int":
				instr.OpCode = core.OP_ILOAD_2
			case "wide literal":
				literal.IsWide = true
			case "bad literal bytes":
				literal.Data = []byte{1}
			case "truncated bipush":
				instr.OpCode = core.OP_BIPUSH
				literal.Data = nil
			case "truncated sipush":
				instr.OpCode = core.OP_SIPUSH
				literal.Data = []byte{1}
			case "bad ldc index":
				instr.OpCode = core.OP_LDC
				literal.Data = []byte{0}
			case "long literal":
				instr.OpCode = core.OP_LCONST_1
			case "source cast without original instruction":
				ops = append(ops[:4], ops[5:]...)
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
			if got := next != 0 && call != nil; got != want {
				t.Fatalf("original boolean binding = %v, want %v", got, want)
			}
			if want && call.Description != "(ZI)V" {
				t.Fatal("original constructor descriptor must remain unchanged")
			}
		})
	}
}
