package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestReleasedMonitorLoopRequiresPairedStructuredTransfers(t *testing.T) {
	transfer := func(name string) *statements.CustomStatement {
		st := statements.NewCustomStatement(func(*class_context.ClassContext) string { return name }, func(*utils.VariableId, *utils.VariableId) {})
		st.Name = name
		return st
	}
	exit := func() statements.Statement { return &statements.MiddleStatement{Flag: "monitor_exit"} }
	for _, name := range []string{"break", "return", "continue held", "break held", "continue released", "dangling release", "effect after release", "opaque", "nested loop", "nested monitor", "double release"} {
		t.Run(name, func(t *testing.T) {
			body := []statements.Statement{exit(), transfer("break")}
			switch name {
			case "return":
				body[1] = statements.NewReturnStatement(nil)
			case "continue held":
				body = []statements.Statement{transfer("continue")}
			case "break held":
				body = body[1:]
			case "continue released":
				body[1] = transfer("continue")
			case "dangling release":
				body = body[:1]
			case "effect after release":
				body = append(body[:1], &statements.ExpressionStatement{}, transfer("break"))
			case "opaque":
				body[1] = transfer("unknown")
			case "nested loop":
				body = []statements.Statement{statements.NewDoWhileStatement(nil, body)}
			case "nested monitor":
				body = []statements.Statement{statements.NewSynchronizedStatement(nil, body)}
			case "double release":
				body = append(body[:1], exit(), body[1])
			}
			branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), body, nil)
			result, count, ok := stripReleasedLoopExits([]statements.Statement{branch}, false, 0)
			want := name == "break" || name == "return" || name == "continue held"
			if ok != want {
				t.Fatalf("proof=%v expected=%v", ok, want)
			}
			if ok {
				copy := result[0].(*statements.IfStatement)
				if copy == branch || len(branch.IfBody) != len(body) || len(copy.IfBody) != len(body)-count {
					t.Fatal("mutated shared input or lost transfer")
				}
			}
		})
	}
	if _, _, ok := stripReleasedLoopExits(nil, false, 33); ok {
		t.Fatal("unbounded traversal")
	}
	for _, cond := range []values.JavaValue{nil, values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))} {
		if isUnconditionalMonitorLoop(statements.NewDoWhileStatement(cond, nil)) {
			t.Fatal("implicit condition exit is not a released transfer")
		}
	}
	if !isUnconditionalMonitorLoop(statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)) {
		t.Fatal("missing literal true proof")
	}
}
