package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeTerminalQueriesDoNotRenderSealedOperands(t *testing.T) {
	for _, role := range []string{"throw", "assignment", "break", "continue", "outer break", "outer continue"} {
		t.Run(role, func(t *testing.T) {
			renders := 0
			probe := values.NewCustomValue(func(*class_context.ClassContext) string { renders++; return "effect()" }, func() types.JavaType { return types.NewJavaClass("java.lang.RuntimeException") })
			var leaf *statements.CustomStatement
			switch role {
			case "throw":
				leaf = statements.NewThrowStatement(probe)
			case "assignment":
				leaf = statements.NewSourceAssignmentForward(&statements.AssignStatement{LeftValue: probe, JavaValue: values.JavaNull})
			case "break":
				leaf = statements.NewSourceTransferStatement("break", "")
			case "continue":
				leaf = statements.NewSourceTransferStatement("continue", "")
			case "outer break":
				leaf = statements.NewSourceTransferStatement("break", "OUTER")
			case "outer continue":
				leaf = statements.NewSourceTransferStatement("continue", "OUTER")
			}
			leaf.StringFunc = func(*class_context.ClassContext) string {
				t.Fatal("sealed leaf used opaque fallback")
				return "throw hidden()"
			}
			for i := 0; i < 1000; i++ {
				if isUnconditionalTerminalStatement(leaf, nil) != (role != "assignment") {
					t.Fatal("terminal classification changed")
				}
				if loopBodyHasEscapingBreak([]statements.Statement{leaf}, "OUTER", true, nil) != (role == "break" || role == "outer break") {
					t.Fatal("loop target classification changed")
				}
				if loopBodyHasEscapingBreak([]statements.Statement{leaf}, "OUTER", false, nil) != (role == "outer break") {
					t.Fatal("nested break crossed its original owner")
				}
			}
			if renders != 0 {
				t.Fatal("structural control query rendered original operands")
			}
		})
	}
}
