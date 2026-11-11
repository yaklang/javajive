package ir

import (
	"fmt"
	"testing"
)

func TestBuildBlocksScaleFixture(t *testing.T) {
	instructions, codeUnits := branchScaleFixture(12000)
	blocks, err := buildBlocks(instructions, codeUnits, Context{}, nil)
	if err != nil {
		t.Fatalf("buildBlocks() error = %v", err)
	}
	if got, want := len(blocks), len(instructions); got != want {
		t.Fatalf("block count = %d, want %d", got, want)
	}
	for index := range blocks {
		if len(blocks[index].Instructions) != 1 || blocks[index].Instructions[0] != index {
			t.Fatalf("block %d instructions = %v", index, blocks[index].Instructions)
		}
	}
}

func BenchmarkBuildBlocksScale(b *testing.B) {
	for _, count := range []int{256, 1024, 4096, 16384} {
		b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
			instructions, codeUnits := branchScaleFixture(count)
			b.ReportAllocs()
			b.SetBytes(int64(len(instructions)))
			for i := 0; i < b.N; i++ {
				if _, err := buildBlocks(instructions, codeUnits, Context{}, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func branchScaleFixture(count int) ([]Instruction, uint32) {
	instructions := make([]Instruction, count)
	for index := range instructions {
		instructions[index] = Instruction{Offset: uint32(index), Opcode: 0x28, Name: "goto", Width: 1, Target: int64((index + 1) % count)}
	}
	instructions[count-1] = Instruction{Offset: uint32(count - 1), Opcode: 0x0e, Name: "return-void", Width: 1, Target: -1}
	return instructions, uint32(count)
}
