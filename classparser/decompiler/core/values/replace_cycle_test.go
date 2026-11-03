package values

import (
	"testing"
	"time"
)

func TestReplaceVarCyclicTernaryTerminates(t *testing.T) {
	ternary := &TernaryExpression{}
	call := &FunctionCallExpression{Arguments: []JavaValue{ternary}}
	ternary.TrueValue = ternary
	ternary.FalseValue = call
	call.Object = ternary
	done := make(chan struct{})
	go func() {
		ternary.ReplaceVar(nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cyclic ReplaceVar did not finish")
	}

	head := &TernaryExpression{}
	cursor := head
	for i := 0; i < 1000; i++ {
		next := &TernaryExpression{}
		cursor.TrueValue = next
		cursor = next
	}
	done = make(chan struct{})
	go func() {
		head.ReplaceVar(nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("long ternary chain did not finish")
	}
}
