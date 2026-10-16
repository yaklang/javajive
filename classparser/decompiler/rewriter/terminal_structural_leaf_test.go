package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestTerminalQueriesDoNotRenderSealedOperands(t *testing.T) {
	for _, mode := range []string{"throw", "assignment", "break", "continue", "labelled break", "labelled continue"} {
		t.Run(mode, func(t *testing.T) {
			probe := &terminalOperandRenderProbe{JavaValue: values.JavaNull}
			var leaf *statements.CustomStatement
			switch mode {
			case "throw":
				leaf = statements.NewThrowStatement(probe)
			case "assignment":
				leaf = statements.NewSourceAssignmentForward(&statements.AssignStatement{LeftValue: probe, JavaValue: values.JavaNull})
			case "break":
				leaf = statements.NewSourceTransferStatement("break", "")
			case "continue":
				leaf = statements.NewSourceTransferStatement("continue", "")
			case "labelled break":
				leaf = statements.NewSourceTransferStatement("break", "OUTER")
			case "labelled continue":
				leaf = statements.NewSourceTransferStatement("continue", "OUTER")
			}
			leaf.StringFunc = func(*class_context.ClassContext) string {
				t.Fatal("sealed query used opaque fallback")
				return "throw hidden()"
			}
			terminal := mode != "assignment"
			hasBreak := mode == "break" || mode == "labelled break"
			for i := 0; i < 1000; i++ {
				if statementIsTerminal(leaf) != terminal || isTerminatorStatement(leaf) != terminal || statementSubtreeHasBreak(leaf) != hasBreak || isBreakStatement(leaf) != (mode == "break") {
					t.Fatalf("wrong structural role: %s", mode)
				}
			}
			if probe.renders != 0 {
				t.Fatal("control query rendered the original operand")
			}
			tail := statements.NewReturnStatement(values.JavaNull)
			got := PruneUnreachableStatements([]statements.Statement{leaf, tail})
			if (len(got) == 1) != terminal {
				t.Fatal("normal continuation changed")
			}
			if probe.renders != 0 {
				t.Fatal("pruning rendered an operand")
			}
		})
	}
}
