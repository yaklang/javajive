package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Exact normal exits are a control certificate, independent of printed assert
// text. Every alternative path, partial instruction target, and reached cycle
// must refuse even when the source predicate looks like a normal assertion.
func TestNativeAssertionPhysicalPacketExitBoundaryGuards(t *testing.T) {
	branch := func(pc, target int) *core.OpCode {
		d := target - pc
		return &core.OpCode{CurrentOffset: uint16(pc), Instr: &core.Instruction{OpCode: core.OP_IFNE}, Data: []byte{byte(d >> 8), byte(d)}}
	}
	for _, variant := range []string{"forward", "backward", "different exit", "internal cycle", "operand target", "missing continuation", "no terminal", "early terminal", "duplicate PC", "wide bad", "internal switch", "nil instruction", "invalid end", "too many", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			op := func(pc, kind int) *core.OpCode {
				return &core.OpCode{CurrentOffset: uint16(pc), Instr: &core.Instruction{OpCode: kind}}
			}
			ops := []*core.OpCode{op(0, core.OP_NOP), op(1, core.OP_GETSTATIC), branch(4, 14), op(7, core.OP_ICONST_1), branch(8, 14), op(11, core.OP_NOP), op(12, core.OP_NOP), op(13, core.OP_ATHROW), op(14, core.OP_RETURN)}
			// The standalone boundary test uses original instruction positions;
			// metadata/frame admission is a separate prerequisite in production.
			start, end, join, size := 1, 14, 14, 15
			var work *workbudget.Budget
			switch variant {
			case "backward":
				join = 0
				ops[2] = branch(4, 0)
				ops[4] = branch(8, 0)
			case "different exit":
				ops[4] = branch(8, 0)
			case "internal cycle":
				ops[4] = branch(8, 7)
			case "operand target":
				ops[2] = branch(4, 6)
			case "missing continuation":
				join = 15
			case "no terminal":
				ops[7] = op(13, core.OP_NOP)
			case "early terminal":
				ops[3] = op(7, core.OP_ATHROW)
			case "duplicate PC":
				ops = append(ops, op(14, core.OP_NOP))
			case "wide bad":
				ops[4].Instr.OpCode = core.OP_GOTO_W
			case "internal switch":
				ops[3].Instr.OpCode = core.OP_TABLESWITCH
			case "nil instruction":
				ops[3].Instr = nil
			case "invalid end":
				end = 16
			case "too many":
				ops = make([]*core.OpCode, 8193)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAssertionPacketExitsClosed(ops, start, end, join, size, work)
			if got != (variant == "forward" || variant == "backward") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}
