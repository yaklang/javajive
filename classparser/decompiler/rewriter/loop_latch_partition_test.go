package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSharedLoopLatchPartitionsOnlyCompleteStableEdges(t *testing.T) {
	for _, variant := range []string{"complete", "reference condition", "reverse polarity", "same handler", "unknown handlers", "changed prefix handler", "changed guard handler", "invalid handler", "oversized handlers", "missing tail origin", "missing prefix origin", "missing source origin", "foreign entry", "second backedge", "stale backedge", "top test", "hidden", "encoded", "catch", "try", "declaration", "allocation", "dynamic call", "effectful condition", "cyclic condition", "operand limit", "edge limit", "irreducible"} {
		t.Run(variant, func(t *testing.T) {
			word := types.NewJavaPrimer(types.JavaInteger)
			boolean := types.NewJavaPrimer(types.JavaBoolean)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, word)
			condition := &statements.ConditionStatement{Condition: values.NewBinaryExpression(local, values.NewJavaLiteral(0, word), values.GTE, boolean)}
			call := &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, ClassName: "sample.Stack", FunctionName: "pop", Descriptor: "()I", FuncType: &types.JavaFuncType{ReturnType: word}, OriginPC: 60, HasOriginPC: true}
			assignment := statements.NewAssignStatement(local, call, false)
			assignment.OriginPC, assignment.HasOriginPC = 60, true
			nodes := make([]*core.Node, 9)
			for i := range nodes {
				nodes[i] = core.NewNode(&statements.MiddleStatement{})
				nodes[i].Id, nodes[i].OriginPC, nodes[i].HasOriginPC = i, i*10, true
			}
			root, loop, fork, left, right, bypass, prefix, tail, exit := nodes[0], nodes[1], nodes[2], nodes[3], nodes[4], nodes[5], nodes[6], nodes[7], nodes[8]
			loop.Statement = statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolean), nil)
			tail.Statement = condition
			prefix.Statement = assignment
			root.AddNext(loop)
			loop.AddNext(fork)
			fork.AddNext(left)
			fork.AddNext(right)
			fork.AddNext(bypass)
			left.AddNext(prefix)
			right.AddNext(prefix)
			bypass.AddNext(tail)
			prefix.AddNext(tail)
			tail.AddNext(exit)
			tail.AddNext(loop)
			tail.TrueNode = func() *core.Node { return tail.Next[1] }
			tail.FalseNode = func() *core.Node { return tail.Next[0] }
			m := NewRootStatementManager(root)
			m.currentNodeId = 100
			m.WhileNode = []*core.Node{loop}
			m.IfNodes = []*core.Node{tail}
			m.LoopRegionReducible, m.originalHandlersKnown = true, true
			want := variant == "complete" || variant == "reference condition" || variant == "reverse polarity" || variant == "same handler"
			switch variant {
			case "reference condition":
				local.ResetVarType(types.NewJavaClass("java.lang.Object"))
				call.Descriptor = "()Ljava/lang/Object;"
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Object")
				condition.Condition = values.NewBinaryExpression(local, values.JavaNull, values.NEQ, boolean)
			case "reverse polarity":
				tail.Next[0], tail.Next[1] = tail.Next[1], tail.Next[0]
			case "same handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 20, EndPc: 80, HandlerPc: 90}}
			case "unknown handlers":
				m.originalHandlersKnown = false
			case "changed prefix handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 60, EndPc: 80, HandlerPc: 90}}
			case "changed guard handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 70, EndPc: 80, HandlerPc: 90}}
			case "invalid handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 70, EndPc: 60, HandlerPc: 90}}
			case "oversized handlers":
				m.originalHandlers = make([]core.HandlerRange, 513)
			case "missing tail origin":
				tail.HasOriginPC = false
			case "missing prefix origin":
				prefix.HasOriginPC = false
			case "missing source origin":
				right.HasOriginPC = false
			case "foreign entry":
				root.AddNext(right)
			case "second backedge":
				bypass.AddNext(loop)
			case "stale backedge":
				loop.Source = append(loop.Source, right)
			case "top test":
				loop.RemoveNext(fork)
				loop.AddNext(tail)
			case "hidden":
				bypass.HideNext = exit
			case "encoded":
				bypass.EncodedJumps = map[*core.Node]bool{tail: true}
			case "catch":
				prefix.IsCatchStart = true
			case "try":
				tail.IsTryCatch = true
			case "declaration":
				assignment.IsDeclare = true
			case "allocation":
				assignment.JavaValue = &values.NewExpression{}
			case "dynamic call":
				call.Kind = values.InvokeDynamic
			case "effectful condition":
				condition.Condition = call
			case "cyclic condition":
				x := condition.Condition.(*values.JavaExpression)
				x.Values[0] = x
			case "operand limit":
				for i := 0; i < 18; i++ {
					condition.Condition = values.NewUnaryExpression(condition.Condition, values.Not, boolean)
				}
			case "edge limit":
				for i := 0; i < 130; i++ {
					n := core.NewNode(&statements.MiddleStatement{})
					n.HasOriginPC, n.OriginPC = true, 40
					fork.AddNext(n)
					n.AddNext(prefix)
				}
			case "irreducible":
				m.LoopRegionReducible = false
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := map[*core.Node][]*core.Node{}
			for _, n := range WalkNodeToList(root) {
				before[n] = slices.Clone(n.Next)
			}
			partitionSharedLoopLatches(m)
			if !want {
				for n, next := range before {
					if !slices.Equal(n.Next, next) {
						t.Fatal("rejected proof changed an edge")
					}
				}
				return
			}
			if len(tail.Next) != 0 || len(prefix.Next) != 0 || len(m.IfNodes) != 3 {
				t.Fatal("shared tail not retired atomically")
			}
			for _, from := range []*core.Node{left, right, bypass} {
				n := from.Next[0]
				if from != bypass {
					step, ok := n.Statement.(*statements.AssignStatement)
					if !ok || step == assignment || step.JavaValue != call || step.LeftValue != local || step.OriginPC != assignment.OriginPC {
						t.Fatal("changed original update identity")
					}
					n = n.Next[0]
				}
				c, ok := n.Statement.(*statements.ConditionStatement)
				if !ok || c == condition || len(n.Source) != 1 || len(n.Next) != 2 {
					t.Fatal("not a private test")
				}
				yes, no := ifBranchNodes(n)
				if variant == "reverse polarity" {
					if yes != exit || no != loop {
						t.Fatal("reversed polarity lost")
					}
				} else if yes != loop || no != exit {
					t.Fatal("polarity lost")
				}
			}
			count := len(m.IfNodes)
			partitionSharedLoopLatches(m)
			if len(m.IfNodes) != count {
				t.Fatal("second pass duplicated evaluations")
			}
		})
	}
}
