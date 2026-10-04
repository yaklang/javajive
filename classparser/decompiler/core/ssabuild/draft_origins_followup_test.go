// Copy to classparser/decompiler/core/ssabuild/draft_origins_followup_test.go.
package ssabuild

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"testing"
)

func TestDraftFollowupNegationChangesOrigin(t *testing.T) {
	f := frametransfer.Frame{Stack: []frametransfer.Type{frametransfer.T(frametransfer.Int)}}
	before := []Origin{{Kind: OriginParam, Slot: 0}}
	out, _, err := transferOrigins(before, f, f, methodir.Instr{PC: 7, Opcode: core.OP_INEG})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Key() == before[0].Key() {
		t.Fatal("ineg retained input identity merely because types match")
	}
}
func TestDraftFollowupSameTypeSwapOrigins(t *testing.T) {
	f := frametransfer.Frame{Stack: []frametransfer.Type{frametransfer.T(frametransfer.Int), frametransfer.T(frametransfer.Int)}}
	before := []Origin{{Kind: OriginParam, Slot: 0}, {Kind: OriginParam, Slot: 1}}
	out, _, err := transferOrigins(before, f, f, methodir.Instr{PC: 8, Opcode: core.OP_SWAP})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Key() != before[1].Key() || out[1].Key() != before[0].Key() {
		t.Fatal("same-type swap lost value permutation")
	}
}
