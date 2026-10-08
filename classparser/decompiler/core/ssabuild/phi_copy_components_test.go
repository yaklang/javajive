package ssabuild

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

// Equation models isolate recursive copies. Valid bytecode, frame joins and
// edge-selected numerical observations are exercised by the separate loop MVP.
func TestPhiCopyComponentsRequireOneExternalIdentity(t *testing.T) {
	for _, variant := range []string{"one external", "two equal-typed externals", "no external", "undefined equation", "budget"} {
		t.Run(variant, func(t *testing.T) {
			fn := &Function{EdgeStates: map[methodir.EdgeID]EdgeState{}}
			p := []Origin{}
			for i := 0; i < 4; i++ {
				b := BlockFrame{ID: methodir.BlockID(i), First: uint16(i), In: frametransfer.NewFrame(1)}
				p = append(p, Origin{Kind: OriginPhi, PC: b.First, Slot: 0, Aux: i})
				b.InOrig = []Origin{p[i]}
				fn.Blocks = append(fn.Blocks, b)
			}
			input := Origin{Kind: OriginParam, Slot: 0}
			equations := [][]Origin{{input, p[1], p[2]}, {p[0], p[2]}, {p[0], p[1]}, {p[0], p[2]}}
			if variant == "two equal-typed externals" {
				equations[1] = append(equations[1], Origin{Kind: OriginParam, Slot: 1})
			}
			if variant == "no external" {
				equations[0] = equations[0][1:]
			}
			if variant == "undefined equation" {
				equations[1] = append(equations[1], Origin{Kind: OriginPhi, PC: 99})
			}
			for i, operands := range equations {
				phi := Phi{Block: methodir.BlockID(i), Slot: SlotKey{Local: true}, Type: frametransfer.T(frametransfer.Int)}
				for j, o := range operands {
					phi.Operands = append(phi.Operands, PhiOperand{Edge: methodir.EdgeID{From: methodir.InstrID(j), To: methodir.InstrID(i)}, Origin: o})
				}
				fn.Phis = append(fn.Phis, phi)
			}
			fn.Instructions = []InstructionValues{{PC: 9, Uses: append([]Origin(nil), p...)}}
			var counter WorkCounter
			if variant == "budget" {
				counter = &LimitCounter{Max: 10}
			}
			err := simplifyPhiCopies(fn, counter)
			if variant == "no external" || variant == "undefined equation" || variant == "budget" {
				if err == nil {
					t.Fatal("unproved or over-budget equation graph accepted")
				}
				if variant == "budget" && !strings.Contains(err.Error(), "analysis_budget_exceeded") {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if variant == "one external" {
				if len(fn.Phis) != 0 {
					t.Fatal("recursive copies retained despite one incoming value")
				}
				for _, o := range fn.Instructions[0].Uses {
					if o != input {
						t.Fatal("copy cycle did not preserve its external value")
					}
				}
			} else if len(fn.Phis) != 4 {
				t.Fatal("equal types conflated two original value identities")
			}
		})
	}
}
