package core

import (
	"testing"
	"time"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestPolyFunctionalDefinitionCyclicTernaryTerminates(t *testing.T) {
	ternary := &values.TernaryExpression{}
	ternary.TrueValue = ternary
	ternary.FalseValue = ternary
	done := make(chan struct{})
	go func() {
		poly, ok := polyFunctionalDefinition(ternary)
		if poly || ok {
			t.Errorf("cyclic ternary = poly %v admissible %v, want false false", poly, ok)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cyclic ternary recursion did not finish")
	}
}
