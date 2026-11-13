package ir

import (
	"math"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func TestDecodePackedSwitchCarriesFirstKey(t *testing.T) {
	// packed-switch v0, payload@4; return-void; payload: size 3, first_key 10.
	words := []uint16{
		0x002b, 0x0004, 0x0000,
		0x000e,
		0x0100, 0x0003, 0x000a, 0x0000,
		0x0003, 0x0000,
		0x0003, 0x0000,
		0x0003, 0x0000,
	}
	instructions, err := Decode(&model.Code{Instructions: words}, Context{Entry: "classes.dex", Class: "LC;", Method: "m"})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(instructions) != 2 {
		t.Fatalf("instructions = %d, want 2 (payload is data)", len(instructions))
	}
	switchInstruction := instructions[0]
	if switchInstruction.Opcode != 0x2b || switchInstruction.FirstKey != 10 {
		t.Fatalf("packed switch = %#v", switchInstruction)
	}
	if len(switchInstruction.Targets) != 3 || switchInstruction.Targets[0] != 3 || switchInstruction.Targets[2] != 3 {
		t.Fatalf("packed targets = %v", switchInstruction.Targets)
	}
}

func TestDecodeSparseSwitchSortsKeys(t *testing.T) {
	// sparse-switch v0, payload@4; return-void; payload: keys 100, 5 followed
	// by their targets 3 and 9.
	words := []uint16{
		0x002c, 0x0004, 0x0000,
		0x000e,
		0x0200, 0x0002,
		0x0064, 0x0000, 0x0005, 0x0000,
		0x0003, 0x0000, 0x0009, 0x0000,
	}
	instructions, err := Decode(&model.Code{Instructions: words}, Context{Entry: "classes.dex", Class: "LC;", Method: "m"})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(instructions) != 2 {
		t.Fatalf("instructions = %d, want 2", len(instructions))
	}
	switchInstruction := instructions[0]
	if len(switchInstruction.Keys) != 2 || switchInstruction.Keys[0] != 5 || switchInstruction.Keys[1] != 100 {
		t.Fatalf("sparse keys = %v, want ascending", switchInstruction.Keys)
	}
	// Sorting must keep every key attached to its own target.
	if switchInstruction.Targets[0] != 9 || switchInstruction.Targets[1] != 3 {
		t.Fatalf("sparse targets = %v, want the pair for each key", switchInstruction.Targets)
	}
}

func TestDecodeRejectsInvalidSignedPayloadTargets(t *testing.T) {
	tests := []struct {
		name  string
		words []uint16
		want  string
	}{
		{
			name:  "negative switch",
			words: []uint16{0x002b, 0xffff, 0xffff},
			want:  "switch payload target -1 is outside code units [0,3)",
		},
		{
			name:  "wrapped switch",
			words: []uint16{0x002b, 0x0000, 0x8000},
			want:  "switch payload target -2147483648 is outside code units [0,3)",
		},
		{
			name:  "final word switch",
			words: []uint16{0x002b, 0x0003, 0x0000, 0x0100},
			want:  "switch payload at 3 has a truncated header",
		},
		{
			name:  "negative fill array",
			words: []uint16{0x0026, 0xffff, 0xffff},
			want:  "fill-array-data payload target -1 is outside code units [0,3)",
		},
		{
			name:  "final word fill array",
			words: []uint16{0x0026, 0x0003, 0x0000, 0x0300},
			want:  "fill-array-data payload at 3 has a truncated header",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(&model.Code{Instructions: test.words}, Context{Entry: "classes.dex", Class: "LC;", Method: "m"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPayloadIndexingRejectsWrappedAndTruncatedRanges(t *testing.T) {
	if _, _, _, err := decodeSwitchPayload([]uint16{0x0100, 0}, int64(math.MaxUint32)+1, 0, false); err == nil || !strings.Contains(err.Error(), "outside code units") {
		t.Fatalf("decodeSwitchPayload() error = %v", err)
	}
	if _, _, err := decodeFillArrayPayload([]uint16{0x0300, 1, 0, 0}, int64(math.MaxUint32)+1); err == nil || !strings.Contains(err.Error(), "outside code units") {
		t.Fatalf("decodeFillArrayPayload() error = %v", err)
	}
	if _, _, _, err := decodeSwitchPayload([]uint16{0x0100, 1, 0, 0, 0}, 0, 0, false); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("decodeSwitchPayload() error = %v", err)
	}
	if _, _, err := decodeFillArrayPayload([]uint16{0x0300, 8, 1, 0}, 0); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("decodeFillArrayPayload() error = %v", err)
	}
}

func TestBuildPackedSwitchSuccessors(t *testing.T) {
	words := []uint16{
		0x002b, 0x0008, 0x0000, // packed-switch v0, payload@8
		0x000e,                         // default: return-void
		0x000e,                         // case target
		0x000e,                         // filler
		0x000e,                         // filler
		0x000e,                         // filler
		0x0100, 0x0002, 0x0000, 0x0000, // size 2, first_key 0
		0x0004, 0x0000, // case 0 -> offset 4
		0x0005, 0x0000, // case 1 -> offset 5
	}
	in := wideInput("V", nil, 1, 0, words)
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(method.Blocks) < 2 {
		t.Fatalf("blocks = %d", len(method.Blocks))
	}
	successors := method.Blocks[0].Successors
	if len(successors) != 3 {
		t.Fatalf("switch block successors = %v, want two cases and the fall-through", successors)
	}
}
