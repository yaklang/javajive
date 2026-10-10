package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func resourceTailFixture() (*core.Node, *statements.TryCatchStatement) {
	ref := func(t string) *values.JavaRef {
		return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(t))
	}
	resource, primary, suppressed := ref("example.Resource"), ref("java.lang.Throwable"), ref("java.lang.Throwable")
	call := func(pc int) *values.FunctionCallExpression {
		return &values.FunctionCallExpression{ClassName: "example/Resource", FunctionName: "finish", Descriptor: "()V", Kind: values.InvokeVirtual, Object: resource, OriginPC: pc, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))}
	}
	guard := func(body []statements.Statement) *statements.IfStatement {
		return statements.NewIfStatement(values.NewBinaryExpression(resource, values.JavaNull, values.NEQ, types.NewJavaPrimer(types.JavaBoolean)), body, nil)
	}
	inner := statements.NewTryCatchStatement([]statements.Statement{statements.NewExpressionStatement(call(41))}, [][]statements.Statement{{statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "java/lang/Throwable", FunctionName: "addSuppressed", Descriptor: "(Ljava/lang/Throwable;)V", Kind: values.InvokeVirtual, Object: primary, Arguments: []values.JavaValue{suppressed}, OriginPC: 51, HasOriginPC: true})}})
	inner.Exception = []*values.JavaRef{suppressed}
	inner.Handlers = []statements.CatchHandler{{EntryPC: 50, ProtectedRanges: [][2]int{{40, 45}}}}
	thrown := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, func(*utils.VariableId, *utils.VariableId) {})
	thrown.ThrownValue = primary
	thrown.HasOriginPC = true
	thrown.OriginPC = 59
	value := values.NewJavaLiteral("ok", types.NewJavaClass("java.lang.String"))
	ret := statements.NewReturnStatement(value)
	ret.HasOriginPC = true
	ret.OriginPC = 25
	tr := statements.NewTryCatchStatement([]statements.Statement{guard([]statements.Statement{statements.NewExpressionStatement(call(21))}), ret}, [][]statements.Statement{{guard([]statements.Statement{inner}), thrown}})
	tr.Exception = []*values.JavaRef{primary}
	tr.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{0, 20}}}}
	return &core.Node{}, tr
}

func TestResourceNormalTailRequiresOriginalBoundaryAndIdentity(t *testing.T) {
	tests := []struct {
		name   string
		change func(*core.Node, *statements.TryCatchStatement)
	}{
		{"proved", func(*core.Node, *statements.TryCatchStatement) {}},
		{"shared region", func(n *core.Node, tr *statements.TryCatchStatement) { n.SharedProtectedHandler = true }},
		{"catchall", func(n *core.Node, tr *statements.TryCatchStatement) { tr.Handlers[0].CatchAll = true }},
		{"other catch type", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.Exception[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Exception"))
		}},
		{"missing ranges", func(n *core.Node, tr *statements.TryCatchStatement) { tr.Handlers[0].ProtectedRanges = nil }},
		{"protected normal close", func(n *core.Node, tr *statements.TryCatchStatement) { tr.Handlers[0].ProtectedRanges[0][1] = 22 }},
		{"protected return", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ReturnStatement).OriginPC = 19
		}},
		{"missing return origin", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ReturnStatement).HasOriginPC = false
		}},
		{"different invoke", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
		}},
		{"different receiver", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Resource"))
		}},
		{"unknown close origin", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
		}},
		{"wrong rethrow identity", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][1].(*statements.CustomStatement).ThrownValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
		}},
		{"unprotected effect prefix", func(n *core.Node, tr *statements.TryCatchStatement) {
			st := tr.TryBody[0].(*statements.IfStatement).IfBody[0]
			tr.TryBody = append([]statements.Statement{st}, tr.TryBody...)
		}},
		{"effectful return", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ReturnStatement).JavaValue = tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression
		}},
		{"handler continuation", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.CatchBodies[0] = append(tr.CatchBodies[0], tr.TryBody[1])
		}},
		{"null nested catch", func(n *core.Node, tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement).Exception[0] = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, tr := resourceTailFixture()
			tt.change(node, tr)
			before := append([]statements.Statement{}, tr.TryBody...)
			declaration, tail := factorResourceNormalTail(node, tr)
			want := tt.name == "proved"
			if (declaration != nil) != want {
				t.Fatalf("got proved=%v want %v", declaration != nil, want)
			}
			if want {
				if len(tail) != 2 || len(tr.TryBody) != 1 {
					t.Fatal("normal close must move once outside the body handler")
				}
			} else {
				if len(before) != len(tr.TryBody) {
					t.Fatal("failure must preserve input")
				}
				for i := range before {
					if before[i] != tr.TryBody[i] {
						t.Fatal("failure mutated input")
					}
				}
			}
		})
	}
}

func TestResourceNormalTailResultTypeUsesClassIdentity(t *testing.T) {
	for _, rightOwner := range []string{"example.left.Box", "example.right.Box"} {
		t.Run(rightOwner, func(t *testing.T) {
			node, tr := resourceTailFixture()
			left := *tr.TryBody[1].(*statements.ReturnStatement)
			right := left
			left.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.left.Box"))
			right.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(rightOwner))
			close := tr.TryBody[0]
			branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{close, &left}, []statements.Statement{close, &right})
			tr.TryBody = []statements.Statement{branch}
			declaration, _ := factorResourceNormalTail(node, tr)
			if (declaration != nil) != (rightOwner == "example.left.Box") {
				t.Fatal("equal simple names must not establish a common result type")
			}
		})
	}
}
