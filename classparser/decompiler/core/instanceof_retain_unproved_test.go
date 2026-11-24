package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestInstanceOfInvalidIdentifierRetainUnprovedFallback(t *testing.T) {
	targetClass := values.NewJavaClassValue(types.NewJavaClass("coil.util.-Lifecycles"))
	code := []byte{OP_INSTANCEOF, 0, 1, OP_RETURN}

	run := func(retain bool) (string, error) {
		d := NewDecompiler(code, func(int) values.JavaValue { return targetClass })
		ctx := &class_context.ClassContext{
			ClassName:            "example.Caller",
			RetainUnprovedSource: retain,
		}
		d.FunctionContext = ctx
		if err := d.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		sim := NewStackSimulation(NewEmptyStackEntry(), map[int]*values.JavaRef{}, utils.NewRootVariableId())
		operand := values.NewJavaRef(&utils.VariableId{Name: "x"}, nil, types.NewJavaClass("java.lang.Object"))
		sim.Push(operand)
		var instOp *OpCode
		for _, op := range d.Opcodes() {
			if op.Instr.OpCode == OP_INSTANCEOF {
				instOp = op
				break
			}
		}
		if instOp == nil {
			t.Fatal("missing OP_INSTANCEOF")
		}
		if err := d.calcOpcodeStackInfo(sim, instOp); err != nil {
			return "", err
		}
		res := sim.Pop().(values.JavaValue)
		return res.String(ctx), nil
	}

	// Strict mode must reject non-Java-identifier instanceof targets.
	if _, err := run(false); err == nil || !strings.Contains(err.Error(), "invalid original instanceof") {
		t.Fatalf("strict mode error = %v, want invalid original instanceof", err)
	}

	// RetainUnprovedSource (Compatibility recovery) must render safe instanceof fallback.
	got, err := run(true)
	if err != nil {
		t.Fatalf("RetainUnprovedSource failed: %v", err)
	}
	if !strings.Contains(got, "instanceof") || !strings.Contains(got, "Lifecycles") {
		t.Fatalf("unexpected fallback rendering %q", got)
	}
}
