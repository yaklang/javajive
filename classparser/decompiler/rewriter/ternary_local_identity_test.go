package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTernaryReturnSplitUsesLocalIdentity(t *testing.T) {
	for _, kind := range []string{"same spelling", "swapped values", "unrelated value", "opaque value"} {
		t.Run(kind, func(t *testing.T) {
			ctx := &class_context.ClassContext{}
			typ := types.NewJavaPrimer(types.JavaInteger)
			left := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			right := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			if left.String(ctx) != right.String(ctx) || left.VarUid == right.VarUid {
				t.Fatal("fixture needs distinct local identities with equal temporary names")
			}
			st := statements.NewConditionStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), "")
			st.Callback = func(values.JavaValue) {}
			cond := core.NewNode(st)
			trueArm := core.NewNode(statements.NewAssignStatement(left, values.NewJavaLiteral(11, typ), true))
			falseArm := core.NewNode(statements.NewAssignStatement(right, values.NewJavaLiteral(22, typ), true))
			tern := &values.TernaryExpression{Condition: st.Condition, TrueValue: left, FalseValue: right}
			switch kind {
			case "swapped values":
				tern.TrueValue, tern.FalseValue = right, left
			case "unrelated value":
				tern.TrueValue = values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			case "opaque value":
				tern.TrueValue = &values.CustomValue{}
			}
			merge := core.NewNode(statements.NewReturnStatement(tern))
			cond.AddNext(trueArm)
			cond.AddNext(falseArm)
			trueArm.AddNext(merge)
			falseArm.AddNext(merge)
			cond.TrueNode = func() *core.Node { return trueArm }
			cond.FalseNode = func() *core.Node { return falseArm }
			manager := NewRootStatementManager(cond)
			got := manager.trySplitTernaryReturn(ctx, cond)
			if got != (kind == "same spelling") {
				t.Fatalf("split=%v", got)
			}
			if got {
				if st.Callback != nil || trueArm.Next[0].Statement.(*statements.ReturnStatement).JavaValue != left || falseArm.Next[0].Statement.(*statements.ReturnStatement).JavaValue != right {
					t.Fatal("split lost arm ownership")
				}
			} else if st.Callback == nil || trueArm.Next[0] != merge || falseArm.Next[0] != merge {
				t.Fatal("rejected split changed the graph")
			}
		})
	}
}
