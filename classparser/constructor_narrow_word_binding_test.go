package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorNarrowWordRequiresPhysicalConversion(t *testing.T) {
	for _, subject := range []struct {
		source, descriptor string
		opcode             int
	}{{"byte", "B", core.OP_I2B}, {"short", "S", core.OP_I2S}, {"char", "C", core.OP_I2C}} {
		files := nativeCompileDebugClasses(t, strings.ReplaceAll(`class ConversionBase{ConversionBase(TYPE n){}}class ConversionOwner{class Part extends ConversionBase{Part(int n){super((TYPE)n);}}}`, "TYPE", subject.source), "none")
		for _, variant := range []string{"original", "missing conversion", "wrong conversion", "bad operand bytes", "wide conversion", "reference input", "long input", "boolean input", "missing operand", "duplicate conversion", "wrong parent descriptor"} {
			t.Run(subject.source+"/"+variant, func(t *testing.T) {
				obj, err := Parse(files["ConversionOwner$Part.class"])
				if err != nil {
					t.Fatal(err)
				}
				var code *CodeAttribute
				var desc string
				for _, m := range obj.Methods {
					name, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if name != "<init>" {
						continue
					}
					desc, _ = sourceBridgeUTF8(obj, m.DescriptorIndex)
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
				if code == nil {
					t.Fatal("missing original Code")
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := constructorMotionOps(d)
				if len(ops) != 8 || ops[5].Instr.OpCode != subject.opcode {
					t.Fatal("missing original narrowing")
				}
				conversion := ops[5]
				instruction := *conversion.Instr
				conversion.Instr = &instruction
				params, _, err := callbinding.Descriptor(desc)
				if err != nil {
					t.Fatal(err)
				}
				want := variant == "original" || variant == "duplicate conversion"
				switch variant {
				case "missing conversion":
					ops = append(ops[:5], ops[6:]...)
				case "wrong conversion":
					instruction.OpCode = core.OP_I2B
					if subject.opcode == core.OP_I2B {
						instruction.OpCode = core.OP_I2S
					}
				case "bad operand bytes":
					conversion.Data = []byte{0}
				case "wide conversion":
					conversion.IsWide = true
				case "reference input":
					load := *ops[4].Instr
					load.OpCode = core.OP_ALOAD_1
					ops[4].Instr = &load
				case "long input":
					load := *ops[4].Instr
					load.OpCode = core.OP_LCONST_1
					ops[4].Instr = &load
				case "boolean input":
					params[1] = "Z"
				case "missing operand":
					ops = append(ops[:4], ops[5:]...)
				case "duplicate conversion":
					ops = append(append(append([]*core.OpCode{}, ops[:6]...), conversion), ops[6:]...)
				case "wrong parent descriptor":
					member := constructorMotionMember(obj, ops[6], core.OP_INVOKESPECIAL)
					if member == nil {
						t.Fatal("missing original parent")
					}
					cp := obj.ConstantPool[core.Convert2bytesToInt(ops[6].Data)-1].(*ConstantMethodrefInfo)
					nat := obj.ConstantPool[cp.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					obj.ConstantPool[nat.DescriptorIndex-1] = &ConstantUtf8Info{Value: "(I)V"}
				}
				next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
				if got := next != 0 && call != nil; got != want {
					t.Fatalf("physical conversion binding=%v want=%v", got, want)
				}
				if want && call.Description != "("+subject.descriptor+")V" {
					t.Fatal("original target descriptor changed")
				}
			})
		}
	}
}
