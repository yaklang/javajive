package javaclassparser

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorLiteralPacketRejectsFreshPayloadChanges(t *testing.T) {
	fixture := strings.ReplaceAll(strings.ReplaceAll(constructorLiteralPacketFixture, "LABEL", `"a\ud800"`), "CODE", "1000009")
	_, fresh, a := constructorSuffixFamilyAtTargetRoundTrip(t, fixture, "none", []string{"SuffixOwner$Member"}, "36:constructor:literal:packet:oracle\n", 8, 16)
	for _, variant := range []string{"control", "integer word", "long word", "zero sign", "string unit"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			changed := variant == "control"
			for _, c := range obj.ConstantPool {
				switch x := c.(type) {
				case *ConstantIntegerInfo:
					if variant == "integer word" && x.Value == 1000009 {
						x.Value++
						changed = true
					}
				case *ConstantLongInfo:
					if variant == "long word" && x.Value == math.MinInt64 {
						x.Value++
						changed = true
					}
				case *ConstantDoubleInfo:
					if variant == "zero sign" && math.Float64bits(x.Value) == 1<<63 {
						x.Value = 0
						changed = true
					}
				case *ConstantUtf8Info:
					if variant == "string unit" {
						u := x.CodeUnits()
						if len(u) == 2 && u[0] == 'a' && u[1] == 0xd800 {
							x.SetUnits([]uint16{'a', 0xd801})
							changed = true
						}
					}
				}
			}
			if !changed {
				t.Fatal("mutation did not reach the fresh operand")
			}
			raw := obj.Bytes()
			before := bytes.Clone(raw)
			_, err := a.FinishClass("SuffixOwner$Member.class", raw)
			if (err == nil) != (variant == "control") {
				t.Fatalf("fresh constant validation: %v", err)
			}
			if !bytes.Equal(raw, before) {
				t.Fatal("rejected input mutated")
			}
		})
	}
}

func TestConstructorLiteralPacketSourceRequiresExactCategoryAndWord(t *testing.T) {
	for _, variant := range []string{"control", "different word", "float category", "wrong payload", "mixed role", "units", "null masquerade", "integer overflow"} {
		t.Run(variant, func(t *testing.T) {
			arg := constructorLoweringArgument{descriptor: "I", literal: &constructorLoweringLiteral{word: 42}}
			v := values.NewJavaLiteral(42, types.NewJavaPrimer(types.JavaInteger))
			switch variant {
			case "different word":
				v.Data = 43
			case "float category":
				v.JavaType = types.NewJavaPrimer(types.JavaFloat)
			case "wrong payload":
				v.Data = "42"
			case "mixed role":
				arg.slot = 1
			case "units":
				v.Units = []uint16{42}
			case "null masquerade":
				v = values.NewOriginalNullLiteral(2)
			case "integer overflow":
				v.Data = int64(1<<32) + 42
			}
			if constructorLoweringSourceArgument(arg, 2, v, nil, nil) != (variant == "control") {
				t.Fatal("stale source constant accepted")
			}
		})
	}
}

func TestConstructorLiteralPacketDecoderChecksWidthPoolAndBudget(t *testing.T) {
	for _, variant := range []string{"control", "ldc wide index", "wrong ldc category", "missing pool", "zero index", "wide opcode", "truncated index", "class constant", "bad string index", "bad string entry", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := &ClassObject{ConstantPool: []ConstantInfo{&ConstantStringInfo{StringIndex: 2}, &ConstantUtf8Info{Value: "a", Units: []uint16{'a', 0xd800}}}}
			op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_LDC}, Data: []byte{1}}
			var budget *workbudget.Budget
			switch variant {
			case "ldc wide index":
				op.Instr.OpCode = core.OP_LDC_W
				op.Data = []byte{0, 1}
			case "wrong ldc category":
				op.Instr.OpCode = core.OP_LDC2_W
				op.Data = []byte{0, 1}
			case "missing pool":
				obj.ConstantPool = nil
			case "zero index":
				op.Data[0] = 0
			case "wide opcode":
				op.IsWide = true
			case "truncated index":
				op.Instr.OpCode = core.OP_LDC_W
			case "class constant":
				obj.ConstantPool[0] = &ConstantClassInfo{NameIndex: 2}
			case "bad string index":
				obj.ConstantPool[0].(*ConstantStringInfo).StringIndex = 3
			case "bad string entry":
				obj.ConstantPool[1] = &ConstantIntegerInfo{Value: 2}
			case "budget":
				budget = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				budget = workbudget.New(ctx, workbudget.Limits{})
			}
			_, desc, ok := constructorLoweringReadLiteral(obj, op, budget)
			want := variant == "control" || variant == "ldc wide index"
			if ok != want || ok && desc != "Ljava/lang/String;" {
				t.Fatalf("literal packet decode=%v descriptor=%s", ok, desc)
			}
		})
	}
}
