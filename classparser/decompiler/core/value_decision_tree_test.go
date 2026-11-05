package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestClosedValueDecisionTreeRequiresUniqueConditionsAndClosedLeaves(t *testing.T) {
	for _, kind := range []string{"original", "shared condition", "cycle", "unknown terminal", "unreachable condition", "unused terminal", "nil condition", "nil terminal", "condition terminal alias", "duplicate condition owner", "nil opcode", "cancelled", "scan budget", "walk budget", "allocation budget", "too many conditions"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaInteger)
			a, b, c, d := values.NewJavaLiteral(1, typ), values.NewJavaLiteral(2, typ), values.NewJavaLiteral(3, typ), values.NewJavaLiteral(4, typ)
			built := map[*OpCode]*values.TernaryExpression{}
			node := func(yes, no values.JavaValue) *values.TernaryExpression {
				n := values.NewTernaryExpression(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean)), yes, no)
				built[&OpCode{Id: len(built)}] = n
				return n
			}
			tail := node(c, d)
			right := node(tail, d)
			left := node(a, b)
			root := node(left, right)
			leaves := map[*OpCode]values.JavaValue{&OpCode{}: a, &OpCode{}: b, &OpCode{}: c, &OpCode{}: d}
			var budget *workbudget.Budget
			switch kind {
			case "shared condition":
				left.FalseValue = tail
			case "cycle":
				tail.TrueValue = root
			case "unknown terminal":
				tail.TrueValue = values.NewJavaLiteral(5, typ)
			case "unreachable condition":
				node(a, d)
			case "unused terminal":
				leaves[&OpCode{}] = values.NewJavaLiteral(5, typ)
			case "nil condition":
				tail.Condition = nil
			case "nil terminal":
				leaves[&OpCode{}] = nil
			case "condition terminal alias":
				leaves[&OpCode{}] = tail
			case "duplicate condition owner":
				built[&OpCode{}] = tail
			case "nil opcode":
				built[nil] = tail
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				budget = workbudget.New(ctx, workbudget.Limits{})
			case "scan budget":
				budget = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "walk budget":
				budget = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 9})
			case "allocation budget":
				budget = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "too many conditions":
				for len(built) <= 512 {
					node(a, b)
				}
			}
			priorTrue, priorFalse, priorCondition := root.TrueValue, root.FalseValue, root.Condition
			if got := closedValueDecisionTree(root, built, leaves, budget); got != (kind == "original") {
				t.Fatalf("accepted=%v", got)
			}
			if root.TrueValue != priorTrue || root.FalseValue != priorFalse || root.Condition != priorCondition {
				t.Fatal("certificate changed original operands")
			}
		})
	}
}
