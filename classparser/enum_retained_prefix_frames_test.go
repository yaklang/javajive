package javaclassparser

import (
	"bytes"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialEnumRetainedPrefixFramesUseAbsolutePCMap(t *testing.T) {
	positions := map[int]int{0: core.OP_NEW, 5: core.OP_NEW, 7: core.OP_INVOKESTATIC, 9: core.OP_NOP, 10: core.OP_RETURN, 63: core.OP_GOTO, 72: core.OP_RETURN}
	for _, row := range []struct {
		name        string
		input, want []byte
	}{
		{"prefix unchanged", []byte{0, 1, 5}, []byte{0, 1, 251, 0, 5}},
		{"replacement entry unchanged", []byte{0, 1, 7}, []byte{0, 1, 251, 0, 7}},
		{"suffix boundary shifted", []byte{0, 1, 10}, []byte{0, 1, 251, 0, 14}},
		{"mixed frames", []byte{0, 3, 5, 57, 8}, []byte{0, 3, 251, 0, 5, 251, 0, 61, 251, 0, 8}},
		{"retained NEW identity", []byte{0, 1, 127, 8, 0, 5}, []byte{0, 1, 247, 0, 67, 8, 0, 5}},
		{"frame in removed span", []byte{0, 1, 9}, nil},
		{"NEW not instruction", []byte{0, 1, 127, 8, 0, 4}, nil},
		{"wrong allocation opcode", []byte{0, 1, 127, 8, 0, 7}, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			before := bytes.Clone(row.input)
			got, ok := enumArrayRelocateFramesWithRetainedPrefix(nil, row.input, positions, 7, 10, 4, nil)
			if ok != (row.want != nil) || !bytes.Equal(got, row.want) || !bytes.Equal(before, row.input) {
				t.Fatalf("absolute frame map: supported=%v got=%v want=%v", ok, got, row.want)
			}
		})
	}
}
