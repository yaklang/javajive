package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorArrayLoadPacketComponents(t *testing.T) {
	for _, tc := range []struct {
		opcode        int
		array, result string
	}{
		{core.OP_IALOAD, "[I", "I"}, {core.OP_LALOAD, "[J", "J"}, {core.OP_FALOAD, "[F", "F"}, {core.OP_DALOAD, "[D", "D"},
		{core.OP_BALOAD, "[B", "B"}, {core.OP_BALOAD, "[Z", "Z"}, {core.OP_CALOAD, "[C", "C"}, {core.OP_SALOAD, "[S", "S"},
		{core.OP_AALOAD, "[Ljava/lang/Object;", "Ljava/lang/Object;"}, {core.OP_AALOAD, "[[I", "[I"}, {core.OP_AALOAD, "[[[Ljava/lang/String;", "[[Ljava/lang/String;"},
	} {
		t.Run(core.InstrInfos[tc.opcode].Name+tc.array, func(t *testing.T) {
			for _, index := range []string{"I", "B", "C", "S"} {
				got, ok := constructorArrayLoadPacketResult(tc.opcode, tc.array, index)
				if !ok || got != tc.result {
					t.Fatalf("load=%q/%v want=%q", got, ok, tc.result)
				}
			}
			for _, index := range []string{"Z", "J", "F", "D", "null", "Ljava/lang/Integer;", "Ibad", ""} {
				if _, ok := constructorArrayLoadPacketResult(tc.opcode, tc.array, index); ok {
					t.Fatalf("index %q borrowed computational I", index)
				}
			}
			for _, array := range []string{"null", "Ljava/lang/Object;", "@allocation:3", "[", "[V", "[Ibad", "[L;"} {
				if _, ok := constructorArrayLoadPacketResult(tc.opcode, array, "I"); ok {
					t.Fatalf("array %q borrowed component type", array)
				}
			}
		})
	}
	for _, tc := range []struct {
		opcode int
		array  string
	}{
		{core.OP_IALOAD, "[J"}, {core.OP_LALOAD, "[D"}, {core.OP_FALOAD, "[I"}, {core.OP_DALOAD, "[J"}, {core.OP_AALOAD, "[I"}, {core.OP_BALOAD, "[S"}, {core.OP_CALOAD, "[S"}, {core.OP_SALOAD, "[C"},
		{core.OP_IALOAD, "[[I"}, {core.OP_ARRAYLENGTH, "[I"}, {core.OP_IASTORE, "[I"}, {core.OP_AASTORE, "[Ljava/lang/Object;"},
	} {
		if _, ok := constructorArrayLoadPacketResult(tc.opcode, tc.array, "I"); ok {
			t.Fatalf("opcode %d borrowed array component %s", tc.opcode, tc.array)
		}
	}
}

func TestAdversarialConstructorArrayLoadPacketOriginalOperations(t *testing.T) {
	files := nativeCompileDebugClasses(t, arrayLoadPacketFixture, "none")
	for _, variant := range []string{"original", "changed decoded opcode", "changed decoded operands", "wide view", "wrong component instruction", "receiver as array", "receiver as index", "missing operand", "wrong component descriptor", "boolean index", "small stack", "small locals", "loaded word cannot be enclosing identity"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["ElementOwner$Part.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			var method *MemberInfo
			var desc string
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name != "<init>" {
					continue
				}
				method = m
				desc, _ = sourceBridgeUTF8(obj, m.DescriptorIndex)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			parse := func() []*core.OpCode {
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if d.ParseOpcode() != nil {
					t.Fatal("invalid fixture opcodes")
				}
				return constructorMotionOps(d)
			}
			ops := parse()
			load := -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_IALOAD {
					load = i
				}
			}
			if code == nil || method == nil || load < 2 {
				t.Fatal("missing physical original array load")
			}
			switch variant {
			case "changed decoded opcode":
				instruction := *ops[load].Instr
				instruction.OpCode = core.OP_LALOAD
				ops[load].Instr = &instruction
			case "changed decoded operands":
				ops[load].Data = []byte{0}
			case "wide view":
				ops[load].IsWide = true
			case "wrong component instruction":
				code.Code[ops[load].CurrentOffset] = byte(core.OP_CALOAD)
				ops = parse()
			case "receiver as array":
				code.Code[ops[load-2].CurrentOffset] = byte(core.OP_ALOAD_0)
				ops = parse()
			case "receiver as index":
				code.Code[ops[load-1].CurrentOffset] = byte(core.OP_ALOAD_0)
				ops = parse()
			case "missing operand":
				code.Code[ops[load-1].CurrentOffset] = byte(core.OP_NOP)
				ops = parse()
			case "wrong component descriptor":
				desc = strings.Replace(desc, "[I", "[J", 1)
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = desc
			case "boolean index":
				desc = strings.Replace(desc, "[II)", "[IZ)", 1)
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = desc
			case "small stack":
				code.MaxStack = 2
			case "small locals":
				code.MaxLocals = 3
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			var enclosing []int
			if variant == "loaded word cannot be enclosing identity" {
				enclosing = []int{1}
			}
			next, member := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil, enclosing...)
			if got, want := next > 0 && member != nil, variant == "original"; got != want {
				t.Fatalf("packet closed=%v want=%v", got, want)
			}
			if member != nil && (member.Name != "ElementTarget" || member.Description != "(I)V") {
				t.Fatal("original overload binding changed")
			}
		})
	}
}
