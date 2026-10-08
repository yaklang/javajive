package javaclassparser

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Hand-encoded JVMS frames use distinct CP operands and uninitialized offsets.
// Every strict prefix is invalid. No frame expansion or semantic decompiler is
// used as the reference for which encoded words are class dependencies.
func TestNativeStackMapClassDependenciesPreserveEveryEncodedClassWord(t *testing.T) {
	for _, scenario := range []struct {
		name string
		data []byte
		want []uint16
	}{
		{"empty", []byte{0, 0}, nil},
		{"same", []byte{0, 2, 0, 63}, nil},
		{"same locals one stack", []byte{0, 2, 64, 7, 0, 19, 127, 8, 0, 29}, []uint16{19}},
		{"extended one stack", []byte{0, 1, 247, 0, 12, 7, 0, 37}, []uint16{37}},
		{"chop and extended same", []byte{0, 4, 248, 0, 0, 249, 0, 0, 250, 0, 0, 251, 0, 0}, nil},
		{"append one", []byte{0, 1, 252, 0, 0, 7, 0, 41}, []uint16{41}},
		{"append two", []byte{0, 1, 253, 0, 0, 8, 0, 43, 7, 0, 47}, []uint16{47}},
		{"append three", []byte{0, 1, 254, 0, 0, 7, 0, 53, 4, 7, 0, 59}, []uint16{53, 59}},
		{"full", []byte{0, 1, 255, 0, 0, 0, 3, 7, 0, 61, 8, 0, 67, 2, 0, 2, 5, 7, 0, 71}, []uint16{61, 71}},
		{"all primitive words", []byte{0, 1, 255, 0, 0, 0, 7, 0, 1, 2, 3, 4, 5, 6, 0, 0}, nil},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var got []uint16
			if !nativeStackMapClassDependencies(scenario.data, func(index uint16) bool { got = append(got, index); return true }, nil) || !slices.Equal(got, scenario.want) {
				t.Fatalf("encoded class words %v want %v", got, scenario.want)
			}
			for length := 0; length < len(scenario.data); length++ {
				if nativeStackMapClassDependencies(scenario.data[:length], func(uint16) bool { return true }, nil) {
					t.Fatalf("truncated table accepted at %d/%d", length, len(scenario.data))
				}
			}
			if nativeStackMapClassDependencies(append(append([]byte(nil), scenario.data...), 0), func(uint16) bool { return true }, nil) {
				t.Fatal("trailing byte accepted")
			}
		})
	}
	for frame := 128; frame < 247; frame++ {
		t.Run(fmt.Sprintf("reserved frame %d", frame), func(t *testing.T) {
			if nativeStackMapClassDependencies([]byte{0, 1, byte(frame)}, func(uint16) bool { return true }, nil) {
				t.Fatal("reserved frame accepted")
			}
		})
	}
	for tag := 9; tag <= 255; tag++ {
		if nativeStackMapClassDependencies([]byte{0, 1, 64, byte(tag), 0, 0}, func(uint16) bool { return true }, nil) {
			t.Fatalf("invalid verification type accepted: %d", tag)
		}
	}
	if nativeStackMapClassDependencies([]byte{0, 2, 251, 255, 255, 0}, func(uint16) bool { return true }, nil) {
		t.Fatal("frame offset overflow accepted")
	}
	data := []byte{0, 1, 64, 7, 0, 1}
	if nativeStackMapClassDependencies(data, func(uint16) bool { return false }, nil) || nativeStackMapClassDependencies(data, nil, nil) {
		t.Fatal("unproved class operand accepted")
	}
	if nativeStackMapClassDependencies(data, func(uint16) bool { return true }, workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})) {
		t.Fatal("scan budget bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if nativeStackMapClassDependencies(data, func(uint16) bool { return true }, workbudget.New(ctx, workbudget.Limits{})) {
		t.Fatal("cancellation bypassed")
	}
}
