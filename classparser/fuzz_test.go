package javaclassparser

import (
	"context"
	"os"
	"testing"
)

func FuzzDecompileWithLimitsNeverPanics(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xca, 0xfe, 0xba, 0xbe})
	f.Add(readValidationSeedForFuzz())
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		options := DecompileOptions{
			Mode:    Precision,
			Context: context.Background(),
			Limits: Limits{
				MaxInputBytes:  1 << 20,
				MaxReadBytes:   2 << 20,
				MaxParseItems:  1 << 16,
				MaxOutputBytes: 2 << 20,
				MaxDiagnostics: 64,
				MaxRequestWork: 4 << 20,
			},
			EnvSnapshot: map[string]string{},
		}
		first, firstErr := DecompileWithOptions(data, options)
		second, secondErr := DecompileWithOptions(data, options)
		if first.Status != second.Status || (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("nondeterministic status: %q/%v versus %q/%v", first.Status, firstErr, second.Status, secondErr)
		}
		if len(first.Source) > 2<<20 || len(first.Diagnostics) > 64 {
			t.Fatalf("unbounded result: source=%d diagnostics=%d", len(first.Source), len(first.Diagnostics))
		}
	})
}

func readValidationSeedForFuzz() []byte {
	data, _ := os.ReadFile(validationSeedClass)
	return data
}
