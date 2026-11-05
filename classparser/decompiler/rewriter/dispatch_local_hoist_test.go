package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestHoistDispatchCrossCaseDeclarations(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)

	stateID := coreutils.NewRootVariableId()
	stateID.SetName("jdecState0")
	stateRef := values.NewJavaRef(stateID, nil, integer)

	localID := coreutils.NewRootVariableId()
	localID.SetName("var6")
	localRef := values.NewJavaRef(localID, nil, integer)

	otherID := coreutils.NewRootVariableId()
	otherID.SetName("var7")
	otherRef := values.NewJavaRef(otherID, nil, integer)

	bare := statements.NewDeclareStatement(localRef)
	store := statements.NewAssignStatement(localRef, values.NewJavaLiteral(1, integer), false)
	read := statements.NewAssignStatement(otherRef, localRef, false)
	sw := statements.NewSwitchStatement(stateRef, []*statements.CaseItem{
		statements.NewCaseItem(0, []statements.Statement{store}),
		statements.NewCaseItem(1, []statements.Statement{read}),
	})
	loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolean), []statements.Statement{bare, sw})
	loop.IrreducibleDispatch = true
	sts := []statements.Statement{loop}

	hoistDispatchCrossCaseDeclarations(&sts)

	if len(sts) != 2 {
		t.Fatalf("root list length = %d, want declaration + loop", len(sts))
	}
	decl, ok := sts[0].(*statements.AssignStatement)
	if !ok {
		t.Fatalf("root declaration type = %T", sts[0])
	}
	if !decl.IsFirst || decl.IsDeclare || decl.JavaValue == nil {
		t.Fatalf("root declaration flags = IsFirst:%v IsDeclare:%v value:%v", decl.IsFirst, decl.IsDeclare, decl.JavaValue)
	}
	declRef, ok := decl.LeftValue.(*values.JavaRef)
	if !ok || declRef != localRef {
		t.Fatalf("root declaration lost the original ref: %#v", decl.LeftValue)
	}
	if got := decl.JavaValue.String(&class_context.ClassContext{}); got != "0" {
		t.Fatalf("root initializer = %q, want 0", got)
	}
	if len(loop.Body) != 1 {
		t.Fatalf("loop body kept a bare declaration: %d statements", len(loop.Body))
	}
	if store.IsFirst || store.IsDeclare {
		t.Fatalf("case store kept declaration flags: IsFirst:%v IsDeclare:%v", store.IsFirst, store.IsDeclare)
	}

	// Idempotence: a second pass must not duplicate the root declaration.
	hoistDispatchCrossCaseDeclarations(&sts)
	if len(sts) != 2 {
		t.Fatalf("second pass changed the root list: %d", len(sts))
	}
}

func TestHoistDispatchLeavesOrdinaryLoopsAlone(t *testing.T) {
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	localID := coreutils.NewRootVariableId()
	localID.SetName("var6")
	localRef := values.NewJavaRef(localID, nil, integer)
	bare := statements.NewDeclareStatement(localRef)
	loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolean), []statements.Statement{bare})
	sts := []statements.Statement{loop}

	hoistDispatchCrossCaseDeclarations(&sts)

	if len(sts) != 1 {
		t.Fatalf("ordinary loop was rewritten: %d statements", len(sts))
	}
	if _, ok := loop.Body[0].(*statements.AssignStatement); !ok {
		t.Fatalf("ordinary loop body changed: %T", loop.Body[0])
	}
}

func TestDefaultInitializerForType(t *testing.T) {
	cases := []struct {
		name string
		typ  types.JavaType
		want string
	}{
		{"int", types.NewJavaPrimer(types.JavaInteger), "0"},
		{"boolean", types.NewJavaPrimer(types.JavaBoolean), "false"},
		{"long", types.NewJavaPrimer(types.JavaLong), "0L"},
		{"float", types.NewJavaPrimer(types.JavaFloat), "0F"},
		{"double", types.NewJavaPrimer(types.JavaDouble), "0D"},
		{"String", types.NewJavaPrimer(types.JavaString), "null"},
		{"reference", types.NewJavaClass("java.util.List"), "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			initializer := defaultInitializerForType(tc.typ)
			if initializer == nil {
				t.Fatal("nil initializer")
			}
			if got := initializer.String(&class_context.ClassContext{}); got != tc.want {
				t.Fatalf("initializer = %q, want %q", got, tc.want)
			}
		})
	}
	if defaultInitializerForType(types.NewJavaPrimer(types.JavaVoid)) != nil {
		t.Fatal("void must not have an initializer")
	}
}
