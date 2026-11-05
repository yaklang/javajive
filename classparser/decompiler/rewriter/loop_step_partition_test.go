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

func TestSharedLoopStepRequiresCompleteOriginalEdges(t *testing.T) {
	for _, change := range []string{"increment", "decrement", "stride", "same handler", "irreducible", "unknown handlers", "handler bound", "invalid handler", "different handler", "missing step origin", "bad step pc", "missing edge origin", "bad edge pc", "foreign entry", "hidden step", "encoded step", "catch step", "try step", "protected step", "hidden predecessor", "encoded predecessor", "catch predecessor", "stale reverse edge", "missing reverse edge", "duplicate reverse edge", "duplicate forward edge", "hidden incoming edge", "encoded incoming edge", "other successor", "head update", "wrong loop wrapper", "declaration", "effectful amount", "different local", "wide local", "source bound", "loop bound", "live bound"} {
		t.Run(change, func(t *testing.T) {
			word := types.NewJavaPrimer(types.JavaInteger)
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, word)
			amount := values.NewJavaLiteral(1, word)
			expr := values.NewBinaryExpression(local, amount, values.INC, word)
			nodes := make([]*core.Node, 7)
			for i := range nodes {
				nodes[i] = core.NewNode(&statements.MiddleStatement{})
				nodes[i].Id, nodes[i].OriginPC, nodes[i].HasOriginPC = i, i*10, true
			}
			root, loop, fork, left, right, bypass, tail := nodes[0], nodes[1], nodes[2], nodes[3], nodes[4], nodes[5], nodes[6]
			loop.Statement = statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
			tail.Statement = expr
			root.AddNext(loop)
			loop.AddNext(fork)
			for _, from := range []*core.Node{left, right, bypass} {
				fork.AddNext(from)
				from.AddNext(tail)
			}
			tail.AddNext(loop)
			m := NewRootStatementManager(root)
			m.currentNodeId = 100
			m.WhileNode = []*core.Node{loop}
			m.LoopRegionReducible, m.originalHandlersKnown = true, true
			switch change {
			case "decrement":
				expr.Op = values.DEC
			case "stride":
				expr.Op = values.ADD
				amount.Data = 3
				tail.Statement = statements.NewAssignStatement(local, expr, false)
			case "same handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 20, EndPc: 70, HandlerPc: 90, CatchType: 1}}
			case "irreducible":
				m.LoopRegionReducible = false
			case "unknown handlers":
				m.originalHandlersKnown = false
			case "handler bound":
				m.originalHandlers = make([]core.HandlerRange, 513)
			case "invalid handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 60, EndPc: 20, HandlerPc: 90}}
			case "different handler":
				m.originalHandlers = []core.HandlerRange{{StartPc: 60, EndPc: 70, HandlerPc: 90}}
			case "missing step origin":
				tail.HasOriginPC = false
			case "bad step pc":
				tail.OriginPC = 65536
			case "missing edge origin":
				right.HasOriginPC = false
			case "bad edge pc":
				right.OriginPC = -1
			case "foreign entry":
				root.AddNext(left)
			case "hidden step":
				tail.HideNext = root
			case "encoded step":
				tail.EncodedJumps = map[*core.Node]bool{loop: true}
			case "catch step":
				tail.IsCatchStart = true
			case "try step":
				tail.IsTryCatch = true
			case "protected step":
				tail.HasProtectedRange = true
			case "hidden predecessor":
				left.HideNext = root
			case "encoded predecessor":
				left.EncodedJumps = map[*core.Node]bool{tail: true}
			case "catch predecessor":
				left.IsCatchStart = true
			case "stale reverse edge":
				tail.Source = append(tail.Source, root)
			case "missing reverse edge":
				tail.Source = tail.Source[:2]
			case "duplicate reverse edge":
				tail.Source = append(tail.Source, left)
			case "duplicate forward edge":
				left.Next = append(left.Next, tail)
			case "hidden incoming edge":
				fork.HideNext = tail
			case "encoded incoming edge":
				fork.EncodedJumps = map[*core.Node]bool{tail: true}
			case "other successor":
				tail.AddNext(root)
			case "head update":
				loop.RemoveNext(fork)
				loop.AddNext(tail)
			case "wrong loop wrapper":
				loop.Statement = &statements.MiddleStatement{}
			case "declaration":
				tail.Statement = statements.NewAssignStatement(local, expr, true)
			case "effectful amount":
				expr.Op = values.ADD
				expr.Values[1] = &values.FunctionCallExpression{}
				tail.Statement = statements.NewAssignStatement(local, expr, false)
			case "different local":
				expr.Op = values.ADD
				tail.Statement = statements.NewAssignStatement(values.NewJavaRef(coreutils.NewRootVariableId(), nil, word), expr, false)
			case "wide local":
				local.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "source bound":
				for i := 0; i < 129; i++ {
					n := core.NewNode(&statements.MiddleStatement{})
					n.Id, n.OriginPC, n.HasOriginPC = 1000+i, 30, true
					fork.AddNext(n)
					n.AddNext(tail)
				}
			case "loop bound":
				m.WhileNode = make([]*core.Node, 65)
				m.WhileNode[0] = loop
			case "live bound":
				for i := 0; i < 4100; i++ {
					n := core.NewNode(&statements.MiddleStatement{})
					n.Id = 1000 + i
					root.AddNext(n)
				}
			}
			m.DominatorMap = GenerateDominatorTree(root)
			before := map[*core.Node][]*core.Node{}
			for _, n := range WalkNodeToList(root) {
				before[n] = slices.Clone(n.Next)
			}
			partitionSharedLoopSteps(m)
			want := change == "increment" || change == "decrement" || change == "stride" || change == "same handler"
			if !want {
				for n, next := range before {
					if !slices.Equal(n.Next, next) {
						t.Fatal("rejected proof changed original edges")
					}
				}
				return
			}
			if len(tail.Next) != 0 || len(tail.Source) != 0 {
				t.Fatal("shared update not retired")
			}
			for _, from := range []*core.Node{left, right, bypass} {
				n := from.Next[0]
				if n == tail || n.OriginPC != tail.OriginPC || !n.HasOriginPC || len(n.Source) != 1 || n.Source[0] != from || len(n.Next) != 1 || n.Next[0] != loop {
					t.Fatal("lost update evaluation or target")
				}
				var copied *values.JavaExpression
				if assign, ok := n.Statement.(*statements.AssignStatement); ok {
					if assign.LeftValue != local {
						t.Fatal("new binding")
					}
					copied, _ = assign.JavaValue.(*values.JavaExpression)
				} else {
					copied, _ = n.Statement.(*values.JavaExpression)
				}
				if copied == nil || copied == expr || copied.Op != expr.Op || copied.Values[0] != local || copied.Values[1] != amount {
					t.Fatal("changed update identity or displacement")
				}
			}
			count := m.currentNodeId
			partitionSharedLoopSteps(m)
			if m.currentNodeId != count {
				t.Fatal("not idempotent")
			}
		})
	}
}
