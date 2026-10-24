package javaclassparser

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialDeadStoreStorageTracksRetainedIdentitiesAndPath(t *testing.T) {
	for _, count := range []int{64, 256} {
		for _, lateRead := range []bool{false, true} {
			t.Run(fmt.Sprintf("calls_%d_late_read_%v", count, lateRead), func(t *testing.T) {
				typ := types.NewJavaPrimer(types.JavaInteger)
				local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				input := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				store := statements.NewAssignStatement(local, values.NewJavaLiteral(7, typ), true)
				store.HasOriginPC, store.OriginPC = true, 0
				body := []statements.Statement{store}
				for i := 0; i < count; i++ {
					body = append(body, &statements.ExpressionStatement{Expression: &values.FunctionCallExpression{
						FunctionName: "observe", Arguments: []values.JavaValue{input},
					}})
				}
				if lateRead {
					body = append(body, statements.NewReturnStatement(local))
				}
				work := workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1024, MaxGraphScans: 4096})
				got := sourceWithoutDeadLocalStores(body, nil, work)
				if work.Err() != nil {
					t.Fatal("sequential visits charged as retained storage", work.Err())
				}
				start := 0
				if !lateRead {
					start = 1
				}
				if len(got) != len(body)-start {
					t.Fatalf("source statements=%d want=%d", len(got), len(body)-start)
				}
				for i, st := range got {
					if st != body[i+start] {
						t.Fatal("changed or repeated an observable call or late read", i)
					}
				}
				if body[0] != store || store.LeftValue != local {
					t.Fatal("changed original store or binding")
				}
			})
		}
	}
}

func TestAdversarialDeadStoreCloneAllocationFailureIsAtomic(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	store := statements.NewAssignStatement(local, values.NewJavaLiteral(7, typ), true)
	store.HasOriginPC, store.OriginPC = true, 0
	child := []statements.Statement{store}
	for i := 0; i < 80; i++ {
		child = append(child, &statements.ExpressionStatement{Expression: &values.FunctionCallExpression{FunctionName: "observe"}})
	}
	branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), child, nil)
	body := []statements.Statement{branch}
	work := workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 128, MaxGraphScans: 4096})
	got := sourceWithoutDeadLocalStores(body, nil, work)
	if work.Err() == nil || len(got) != 1 || got[0] != branch || len(branch.IfBody) != len(child) || branch.IfBody[0] != store {
		t.Fatal("failed copy allocation published a partial tree or concealed its limit", work.Err())
	}
}
