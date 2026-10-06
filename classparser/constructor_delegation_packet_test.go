package javaclassparser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorPacketJoinPreservesEveryPath(t *testing.T) {
	frame := func(desc string, origin int) *constructorMotionPacket {
		return &constructorMotionPacket{arguments: []string{desc}, origins: []int{origin}, allocations: map[string]string{}, arrays: map[int]string{}}
	}
	for _, tc := range []struct {
		name, a, b, wantType string
		originA, originB     int
		accepted, identity   bool
	}{
		{"unchanged outer", "Lexample/Outer;", "Lexample/Outer;", "Lexample/Outer;", 1, 1, true, true},
		{"changed outer", "Lexample/Outer;", "Lexample/Outer;", "Lexample/Outer;", 1, 2, true, false},
		{"unknown outer", "Lexample/Outer;", "Lexample/Outer;", "Lexample/Outer;", 1, -1, true, false},
		{"narrow computational words", "B", "C", "I", -1, -1, true, true},
		{"narrow and int", "S", "I", "I", -1, -1, true, true},
		{"long width", "J", "J", "J", -1, -1, true, true},
		{"mixed width", "I", "J", "", -1, -1, false, false},
		{"boolean cannot narrow int", "Z", "I", "", -1, -1, false, false},
		{"reference category", "Lexample/A;", "I", "", -1, -1, false, false},
		{"unknown reference LUB", "Lexample/A;", "Lexample/B;", "", -1, -1, false, false},
		{"uninitialized allocations differ", "@allocation:4", "@allocation:5", "", -1, -1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := frame(tc.a, tc.originA), frame(tc.b, tc.originB)
			out, ok := constructorMotionPacketJoin(a, b, 8, map[int]bool{})
			if ok != tc.accepted {
				t.Fatalf("join=%v want=%v", ok, tc.accepted)
			}
			if ok && (out.arguments[0] != tc.wantType || (out.origins[0] == tc.originA) != tc.identity) {
				t.Fatalf("joined type/origin=%v/%v", out.arguments, out.origins)
			}
			if a.arguments[0] != tc.a || b.arguments[0] != tc.b || a.origins[0] != tc.originA || b.origins[0] != tc.originB {
				t.Fatal("join changed an incoming packet")
			}
		})
	}
	for _, variant := range []string{"canonical on both", "unknown arm", "three incoming arms", "stack mismatch", "origin mismatch", "live allocation mismatch", "fresh array only on one arm"} {
		t.Run(variant, func(t *testing.T) {
			a, b := frame("I", -131077), frame("I", -131080)
			booleans := map[int]bool{-131077: true, -131080: true}
			want, canonical := true, true
			switch variant {
			case "unknown arm":
				b.origins[0] = -1
				canonical = false
			case "stack mismatch":
				b.arguments, b.origins = nil, nil
				want = false
			case "origin mismatch":
				b.origins = nil
				want = false
			case "live allocation mismatch":
				a.allocations["@allocation:3"] = "example/Other"
				want = false
			case "fresh array only on one arm":
				b.arrays[-1008] = "[Ljava/lang/Object;"
			}
			out, ok := constructorMotionPacketJoin(a, b, 9, booleans)
			if ok != want {
				t.Fatalf("join=%v want=%v", ok, want)
			}
			if !ok {
				return
			}
			if variant == "three incoming arms" {
				out, ok = constructorMotionPacketJoin(out, frame("I", -1), 9, booleans)
				canonical = false
			}
			if !ok || booleans[out.origins[0]] != canonical || len(out.arrays) != 0 {
				t.Fatal("joined fact is not true on every incoming edge")
			}
		})
	}
}

func TestAdversarialConstructorConditionalPacketOriginalEdges(t *testing.T) {
	files := nativeCompileDebugClasses(t, conditionalPacketFixture, "none")
	for _, variant := range []string{"original", "changed decoded edge", "omitted decoded arm", "duplicate decoded PC", "middle instruction target", "back edge", "early return", "receiver in argument arm", "extra operand arm", "small stack", "small locals", "changed descriptor"} {
		t.Run(variant, func(t *testing.T) {
			// Parsed Code can share the class input buffer. Every physical
			// mutation owns its bytes so the following guard starts unchanged.
			obj, err := Parse(append([]byte(nil), files["ChoiceOwner$Part.class"]...))
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
			parse := func() []*core.OpCode {
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if decoder.ParseOpcode() != nil {
					t.Fatal("malformed fixture instructions")
				}
				return constructorMotionOps(decoder)
			}
			ops := parse()
			branch, jump, arm := -1, -1, -1
			for i, op := range ops {
				if op.Instr.OpCode >= core.OP_IFEQ && op.Instr.OpCode <= core.OP_IFLE {
					branch = i
				}
				if op.Instr.OpCode == core.OP_GOTO {
					jump = i
				}
				if op.Instr.OpCode == core.OP_BIPUSH && arm < 0 {
					arm = i
				}
			}
			if code == nil || branch < 0 || jump < 0 || arm < 0 {
				t.Fatal("missing original branching constructor")
			}
			switch variant {
			case "changed decoded edge":
				ops[branch].Data = []byte{0, 1}
			case "omitted decoded arm":
				ops = append(ops[:arm], ops[arm+1:]...)
			case "duplicate decoded PC":
				ops[arm].CurrentOffset = ops[branch].CurrentOffset
			case "middle instruction target", "back edge":
				pc := int(ops[branch].CurrentOffset)
				delta := int16(1)
				if variant == "back edge" {
					delta = -1
				}
				binary.BigEndian.PutUint16(code.Code[pc+1:pc+3], uint16(delta))
				ops = parse()
			case "early return", "receiver in argument arm":
				pc := int(ops[arm].CurrentOffset)
				opcode := core.OP_RETURN
				if variant == "receiver in argument arm" {
					opcode = core.OP_ALOAD_0
				}
				code.Code[pc], code.Code[pc+1] = byte(opcode), byte(core.OP_NOP)
				ops = parse()
			case "extra operand arm":
				pc := int(ops[jump].CurrentOffset)
				code.Code[pc], code.Code[pc+1], code.Code[pc+2] = byte(core.OP_ICONST_0), byte(core.OP_NOP), byte(core.OP_NOP)
				ops = parse()
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "changed descriptor":
				desc = strings.ReplaceAll(desc, "I", "J")
			}
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			next, member := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
			if got, want := next > 0 && member != nil, variant == "original"; got != want {
				t.Fatalf("packet closed=%v want=%v", got, want)
			}
			if member != nil && (member.Name != "ChoiceTarget" || member.Description != "(I)V") {
				t.Fatal("original selected overload changed")
			}
		})
	}
}
