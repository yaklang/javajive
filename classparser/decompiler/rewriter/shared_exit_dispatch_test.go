package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSharedExitContinuationPlan(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	localID := coreutils.NewRootVariableId()
	localID.SetName("var5")
	local := values.NewJavaRef(localID, nil, integer)
	stateID := coreutils.NewRootVariableId()
	stateID.SetName("jdecState0")
	state := values.NewJavaRef(stateID, nil, integer)
	exitID := coreutils.NewRootVariableId()
	exitID.SetName("jdecExit0")
	exitState := values.NewJavaRef(exitID, nil, integer)

	shared := core.NewNode(statements.NewReturnStatement(local))
	exit1 := core.NewNode(statements.NewAssignStatement(local, values.NewJavaLiteral(1, integer), false))
	exit2 := core.NewNode(statements.NewAssignStatement(local, values.NewJavaLiteral(2, integer), false))
	pre1 := core.NewNode(statements.NewMiddleStatement("", nil))
	pre2 := core.NewNode(statements.NewMiddleStatement("", nil))
	pre1.AddNext(exit1)
	pre2.AddNext(exit2)
	exit1.AddNext(shared)
	exit2.AddNext(shared)

	plan := planSharedExit([]*core.Node{exit1, exit2})
	if plan == nil || plan.exit != shared {
		t.Fatalf("shared exit plan = %#v, want shared return", plan)
	}
	if body := plan.prefixes[exit1]; len(body) != 1 || body[0] != exit1.Statement {
		t.Fatalf("exit1 prefix = %#v, want its state assignment", body)
	}

	// Detach only forward links: the member exit still targets exit1 so
	// dispatchCaseBody can inspect it, while the shared continuation no longer
	// sees the dead prefix node.
	detachExitChainTo(exit1, shared)
	if len(exit1.Next) != 0 {
		t.Fatalf("exit1 forward links remain: %d", len(exit1.Next))
	}
	if len(exit1.Source) != 1 || exit1.Source[0] != pre1 {
		t.Fatalf("exit1 source links changed: %#v", exit1.Source)
	}
	for _, source := range shared.Source {
		if source == exit1 {
			t.Fatal("shared continuation still lists detached exit1")
		}
	}

	member := core.NewNode(statements.NewAssignStatement(local, values.NewJavaLiteral(0, integer), false))
	member.AddNext(exit1)
	body, ok := dispatchCaseBody(member,
		map[*core.Node]bool{member: true},
		map[*core.Node]int{member: 0},
		state,
		map[*core.Node]int{exit1: 0},
		exitState,
		"LOOP_1",
		plan.prefixes,
	)
	if !ok {
		t.Fatal("dispatchCaseBody rejected a shared-exit prefix")
	}
	if len(body) != 3 || body[0] != member.Statement || body[1] != exit1.Statement {
		t.Fatalf("case body = %#v, want member + exit prefix + break", body)
	}
	if got := body[2].String(&class_context.ClassContext{}); got != "break LOOP_1" {
		t.Fatalf("case transfer = %q, want break LOOP_1", got)
	}
}

func TestSharedExitPlanRejectsIndependentTerminals(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	localID := coreutils.NewRootVariableId()
	localID.SetName("var5")
	local := values.NewJavaRef(localID, nil, integer)
	ret1 := core.NewNode(statements.NewReturnStatement(local))
	ret2 := core.NewNode(statements.NewReturnStatement(local))
	exit1 := core.NewNode(statements.NewAssignStatement(local, values.NewJavaLiteral(1, integer), false))
	exit2 := core.NewNode(statements.NewAssignStatement(local, values.NewJavaLiteral(2, integer), false))
	exit1.AddNext(ret1)
	exit2.AddNext(ret2)

	if plan := planSharedExit([]*core.Node{exit1, exit2}); plan != nil {
		t.Fatalf("independent terminal exits were collapsed: %#v", plan)
	}
}
