package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

type terminalOperandRenderProbe struct {
	values.JavaValue
	renders int
}

func (p *terminalOperandRenderProbe) String(*class_context.ClassContext) string {
	p.renders++
	return "new RuntimeException(new Object(){int hidden(){return 3;}}.toString())"
}

// A terminal query is structural analysis, not source generation. An operand
// renderer can contain nested methods and expensive generic-binding proofs.
func TestBuiltinThrowClassificationDoesNotRenderOperand(t *testing.T) {
	for _, variant := range []string{"terminal", "head", "switch completion", "shared loop exit", "null operand", "opaque throw", "opaque expression"} {
		t.Run(variant, func(t *testing.T) {
			operand := &terminalOperandRenderProbe{JavaValue: values.JavaNull}
			throw := statements.NewThrowStatement(operand)
			node := core.NewNode(throw)
			switch variant {
			case "terminal":
				if !isMethodTerminal(node) {
					t.Fatal("builtin throw lost abrupt completion")
				}
			case "head":
				if renderHead(throw) != "throw" {
					t.Fatal("builtin throw lost keyword")
				}
			case "switch completion":
				if !isTerminatorStatement(throw) {
					t.Fatal("builtin throw lost switch termination")
				}
			case "shared loop exit":
				left := core.NewNode(&statements.ConditionStatement{})
				right := core.NewNode(&statements.MiddleStatement{})
				join := core.NewNode(&statements.ReturnStatement{})
				left.AddNext(node)
				left.AddNext(join)
				right.AddNext(join)
				if commonLoopExit([]*core.Node{left, right}) != join {
					t.Fatal("original throw arm changed common-exit coverage")
				}
			case "null operand":
				if !isMethodTerminal(core.NewNode(statements.NewThrowStatement(values.JavaNull))) {
					t.Fatal("throw null lost abrupt completion")
				}
			case "opaque throw", "opaque expression":
				calls, text, want := 0, "throw hiddenEffect()", "throw"
				if variant == "opaque expression" {
					text, want = "new Object()", "new"
				}
				opaque := statements.NewCustomStatement(func(*class_context.ClassContext) string { calls++; return text }, nil)
				opaque.Name, opaque.ThrownValue = "throw", operand
				if renderHead(opaque) != want || calls != 1 {
					t.Fatal("opaque compatibility changed or annotation manufactured a builtin throw")
				}
			}
			if operand.renders != 0 || throw.ThrownValue != operand {
				t.Fatal("classification rendered or replaced the original operand")
			}
			// Actual source emission still renders the current operand exactly once.
			if render := throw.String(&class_context.ClassContext{}); render != "throw "+operand.String(nil) || operand.renders != 2 {
				t.Fatal("actual source rendering changed")
			}
		})
	}
}
