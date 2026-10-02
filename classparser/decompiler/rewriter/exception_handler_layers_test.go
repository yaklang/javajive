package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func layerTestRef(name string) *values.JavaRef {
	return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
}
func layerTestCall(pc int) *values.FunctionCallExpression {
	return &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, ClassName: "example.Owner", FunctionName: "read", Descriptor: "()Ljava/lang/Object;", HasOriginPC: true, OriginPC: pc}
}
func layerTestTree() *statements.TryCatchStatement {
	return &statements.TryCatchStatement{
		TryBody:     []statements.Statement{&statements.ReturnStatement{JavaValue: layerTestCall(2)}},
		Exception:   []*values.JavaRef{layerTestRef("java.lang.ClassCastException | java.lang.ClassNotFoundException"), layerTestRef("java.lang.ClassCastException"), layerTestRef("java.lang.Exception")},
		CatchBodies: [][]statements.Statement{{&statements.ReturnStatement{JavaValue: layerTestCall(12)}}, {&statements.ReturnStatement{JavaValue: values.JavaNull}}, {&statements.ReturnStatement{JavaValue: values.JavaNull}}},
		Handlers:    []statements.CatchHandler{{EntryPC: 10, ProtectedRanges: [][2]int{{0, 5}}}, {EntryPC: 30, ProtectedRanges: [][2]int{{0, 5}, {10, 20}}}, {EntryPC: 40, ProtectedRanges: [][2]int{{0, 5}, {0, 5}, {10, 20}, {10, 20}}}},
	}
}

func TestExceptionHandlerLayersKeepPriorityAndWholeOuterGroup(t *testing.T) {
	for _, variant := range []string{"multiple outer", "single contiguous outer", "same type", "range gap", "different outer regions", "inner entry outside", "uncovered fallback", "missing call PC", "opaque effect", "outer reentry", "invalid interval", "missing metadata", "budget", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			tr := layerTestTree()
			switch variant {
			case "single contiguous outer":
				for i := 1; i < 3; i++ {
					tr.Handlers[i].ProtectedRanges = [][2]int{{0, 20}}
				}
			case "same type":
				tr.Exception[0] = layerTestRef("java.lang.Exception")
			case "range gap":
				for i := 1; i < 3; i++ {
					tr.Handlers[i].ProtectedRanges = [][2]int{{0, 4}, {10, 20}}
				}
			case "different outer regions":
				tr.Handlers[2].ProtectedRanges = [][2]int{{0, 5}, {10, 19}}
			case "inner entry outside":
				tr.Handlers[0].EntryPC = 9
			case "uncovered fallback":
				tr.CatchBodies[0][0].(*statements.ReturnStatement).JavaValue.(*values.FunctionCallExpression).OriginPC = 20
			case "missing call PC":
				tr.CatchBodies[0][0].(*statements.ReturnStatement).JavaValue.(*values.FunctionCallExpression).HasOriginPC = false
			case "opaque effect":
				tr.CatchBodies[0] = []statements.Statement{&statements.CustomStatement{}}
			case "outer reentry":
				tr.CatchBodies[1] = []statements.Statement{&statements.ReturnStatement{JavaValue: layerTestCall(12)}}
			case "invalid interval":
				tr.Handlers[1].ProtectedRanges[0] = [2]int{0, 0}
			case "missing metadata":
				tr.Handlers = tr.Handlers[:2]
			case "budget":
				for i := 0; i < 512; i++ {
					tr.TryBody = append(tr.TryBody, &statements.ReturnStatement{JavaValue: values.JavaNull})
				}
			case "cycle":
				nested := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				nested.IfBody = []statements.Statement{nested}
				tr.TryBody = []statements.Statement{nested}
			}
			out, ok := restoreExceptionHandlerLayers(tr)
			want := variant == "multiple outer" || variant == "single contiguous outer" || variant == "same type"
			if ok != want {
				t.Fatalf("proof=%v want=%v", ok, want)
			}
			if ok {
				inner, valid := out.TryBody[0].(*statements.TryCatchStatement)
				if !valid || len(out.Exception) != 2 || out.Exception[0] != tr.Exception[1] || out.Exception[1] != tr.Exception[2] || len(inner.Exception) != 1 || inner.Exception[0] != tr.Exception[0] || inner.TryBody[0] != tr.TryBody[0] || inner.CatchBodies[0][0] != tr.CatchBodies[0][0] || len(tr.Exception) != 3 {
					t.Fatal("priority, bindings, evaluations or original tree changed")
				}
			}
		})
	}
}

func TestExceptionHandlerLayersRequireClosedNonThrowingChildGuard(t *testing.T) {
	for _, variant := range []string{"closed", "unprotected child effect", "narrow handler", "throwing handler", "opaque handler", "uncovered field store", "numeric pure handler", "zero-divisor handler", "missing own ranges"} {
		t.Run(variant, func(t *testing.T) {
			tr := layerTestTree()
			local := layerTestRef("java.lang.Throwable")
			assignment := statements.NewAssignStatement(local, values.JavaNull, false)
			child := &statements.TryCatchStatement{TryBody: []statements.Statement{&statements.ExpressionStatement{Expression: layerTestCall(22)}}, Exception: []*values.JavaRef{layerTestRef("java.lang.Throwable")}, CatchBodies: [][]statements.Statement{{assignment}}, Handlers: []statements.CatchHandler{{EntryPC: 26, ProtectedRanges: [][2]int{{21, 25}}}}}
			tr.CatchBodies[0] = []statements.Statement{child}
			switch variant {
			case "unprotected child effect":
				child.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 25
			case "narrow handler":
				child.Exception[0] = layerTestRef("java.lang.Exception")
			case "throwing handler":
				child.CatchBodies[0] = []statements.Statement{&statements.ExpressionStatement{Expression: layerTestCall(27)}}
			case "opaque handler":
				child.CatchBodies[0] = []statements.Statement{&statements.CustomStatement{}}
			case "uncovered field store":
				child.CatchBodies[0] = []statements.Statement{statements.NewAssignStatement(&values.JavaClassMember{Name: "example.Owner", Member: "saved", Description: "Ljava/lang/Throwable;", JavaType: local.Type()}, local, false)}
			case "numeric pure handler":
				integer := types.NewJavaPrimer(types.JavaInteger)
				child.CatchBodies[0] = []statements.Statement{statements.NewAssignStatement(values.NewJavaRef(utils.NewRootVariableId(), nil, integer), values.NewBinaryExpression(values.NewJavaLiteral(1, integer), values.NewJavaLiteral(2, integer), values.ADD, integer), false)}
			case "zero-divisor handler":
				integer := types.NewJavaPrimer(types.JavaInteger)
				child.CatchBodies[0] = []statements.Statement{statements.NewAssignStatement(values.NewJavaRef(utils.NewRootVariableId(), nil, integer), values.NewBinaryExpression(values.NewJavaLiteral(1, integer), values.NewJavaLiteral(0, integer), values.DIV, integer), false)}
			case "missing own ranges":
				child.Handlers[0].ProtectedRanges = nil
			}
			_, ok := restoreExceptionHandlerLayers(tr)
			if want := variant == "closed" || variant == "numeric pure handler"; ok != want {
				t.Fatalf("proof=%v want=%v", ok, want)
			}
		})
	}
}

func TestExceptionHandlerLayersRecoverRepeatedEnclosingGroups(t *testing.T) {
	tr := layerTestTree()
	tr.Exception = append(tr.Exception, layerTestRef("java.lang.Throwable"))
	tr.CatchBodies = append(tr.CatchBodies, []statements.Statement{&statements.ReturnStatement{JavaValue: values.JavaNull}})
	tr.Handlers = append(tr.Handlers, statements.CatchHandler{EntryPC: 60, ProtectedRanges: [][2]int{{0, 5}, {10, 20}, {30, 50}}})
	out, ok := restoreExceptionHandlerLayers(tr)
	if !ok || len(out.Exception) != 1 || out.Exception[0] != tr.Exception[3] {
		t.Fatal("missing outer priority layer")
	}
	middle, ok := out.TryBody[0].(*statements.TryCatchStatement)
	if !ok || len(middle.Exception) != 2 || middle.Exception[0] != tr.Exception[1] || middle.Exception[1] != tr.Exception[2] {
		t.Fatal("missing middle handler group")
	}
	inner, ok := middle.TryBody[0].(*statements.TryCatchStatement)
	if !ok || len(inner.Exception) != 1 || inner.Exception[0] != tr.Exception[0] || inner.TryBody[0] != tr.TryBody[0] || inner.CatchBodies[0][0] != tr.CatchBodies[0][0] {
		t.Fatal("inner values or bindings lost")
	}
}
