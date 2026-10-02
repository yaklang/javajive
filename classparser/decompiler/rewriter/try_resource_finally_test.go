package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func primaryResourceFixture() *statements.TryCatchStatement {
	_, tr := resourceTailFixture()
	caught := tr.Exception[0]
	primary := values.NewJavaRef(utils.NewRootVariableId(), nil, caught.Type())
	alias := values.NewJavaRef(utils.NewRootVariableId(), nil, caught.Type())
	outer := values.NewJavaRef(utils.NewRootVariableId(), nil, caught.Type())
	exceptional := tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement)
	exceptional.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object = primary
	normal := tr.TryBody[0].(*statements.IfStatement).IfBody[0]
	tr.TryBody = []statements.Statement{normal, tr.TryBody[1]}
	capture := statements.NewAssignStatement(primary, caught, false)
	capture.OriginPC = 32
	capture.HasOriginPC = true
	aliasCapture := statements.NewAssignStatement(alias, caught, false)
	aliasCapture.OriginPC = 31
	aliasCapture.HasOriginPC = true
	throw := func(ref *values.JavaRef, pc int) *statements.CustomStatement {
		st := statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return "throw " + ref.String(ctx) }, func(old, new *utils.VariableId) { ref.ReplaceVar(old, new) })
		st.ThrownValue = ref
		st.HasOriginPC = true
		st.OriginPC = pc
		return st
	}
	cleanupElse := statements.NewExpressionStatement(&values.FunctionCallExpression{Object: normal.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object, ClassName: "example/Resource", FunctionName: "finish", Descriptor: "()V", Kind: values.InvokeVirtual, OriginPC: 70, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))})
	cleanup := statements.NewIfStatement(values.NewBinaryExpression(primary, values.JavaNull, values.NEQ, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{exceptional}, []statements.Statement{cleanupElse})
	tr.Exception = []*values.JavaRef{caught, outer}
	tr.CatchBodies = [][]statements.Statement{{aliasCapture, capture, throw(alias, 33)}, {cleanup, throw(outer, 79)}}
	tr.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{2, 20}}}, {EntryPC: 40, CatchAll: true, ProtectedRanges: [][2]int{{2, 20}, {30, 36}}}}
	initialized := statements.NewAssignStatement(primary, values.JavaNull, false)
	initialized.OriginPC = 1
	initialized.HasOriginPC = true
	tr.EntryInitializers = []*statements.AssignStatement{initialized}
	return tr
}
func TestPrimaryResourceFinallyRequiresEntryStateAndOriginalIdentity(t *testing.T) {
	tests := []struct {
		name   string
		change func(*statements.TryCatchStatement)
	}{
		{"proved", func(*statements.TryCatchStatement) {}},
		{"intervening folded entry definition", func(tr *statements.TryCatchStatement) {
			primary := tr.EntryInitializers[0].LeftValue
			other := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			as := statements.NewAssignStatement(other, values.NewAssignmentExpression(primary, tr.Exception[0], 1, nil), false)
			as.HasOriginPC = true
			as.OriginPC = 1
			tr.EntryInitializers = append([]*statements.AssignStatement{as}, tr.EntryInitializers...)
		}},
		{"missing initialization", func(tr *statements.TryCatchStatement) { tr.EntryInitializers = nil }},
		{"non-null entry", func(tr *statements.TryCatchStatement) { tr.EntryInitializers[0].JavaValue = tr.Exception[0] }},
		{"missing init origin", func(tr *statements.TryCatchStatement) { tr.EntryInitializers[0].HasOriginPC = false }},
		{"late init", func(tr *statements.TryCatchStatement) { tr.EntryInitializers[0].OriginPC = 2 }},
		{"typed outer catch", func(tr *statements.TryCatchStatement) { tr.Handlers[1].CatchAll = false }},
		{"unprotected capture", func(tr *statements.TryCatchStatement) { tr.Handlers[1].ProtectedRanges[1][0] = 33 }},
		{"wrong capture object", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][1].(*statements.AssignStatement).JavaValue = tr.Exception[1]
		}},
		{"wrong alias rethrow", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][2].(*statements.CustomStatement).ThrownValue = tr.Exception[1]
		}},
		{"wrong outer rethrow", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[1][1].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
		}},
		{"different normal invoke", func(tr *statements.TryCatchStatement) {
			tr.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
		}},
		{"primary changed in body", func(tr *statements.TryCatchStatement) {
			as := *tr.EntryInitializers[0]
			as.OriginPC = 3
			tr.TryBody = append([]statements.Statement{&as}, tr.TryBody...)
		}},
		{"folded primary definition", func(tr *statements.TryCatchStatement) {
			as := values.NewAssignmentExpression(tr.EntryInitializers[0].LeftValue, values.JavaNull, 3, nil)
			condition := values.NewBinaryExpression(as, values.JavaNull, values.EQ, types.NewJavaPrimer(types.JavaBoolean))
			tr.TryBody = append([]statements.Statement{statements.NewIfStatement(condition, nil, nil)}, tr.TryBody...)
		}},
		{"foreign folded array load", func(tr *statements.TryCatchStatement) {
			arr := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
			load := &values.JavaArrayMember{Object: arr, Index: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), HasOriginPC: true, OriginPC: 21}
			as := statements.NewAssignStatement(values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object")), load, false)
			as.HasOriginPC = true
			as.OriginPC = 4
			tr.TryBody = append([]statements.Statement{as}, tr.TryBody...)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := primaryResourceFixture()
			tt.change(tr)
			view, ok := recoverPrimaryResourceFinally(tr)
			if ok != (tt.name == "proved") {
				t.Fatalf("proved=%v", ok)
			}
			if ok {
				if len(view.TryBody) != 1 || len(view.CatchBodies[0]) != 2 || len(view.Cleanup) != 1 {
					t.Fatal("cleanup must leave exactly once and preserve precise same-object rethrow")
				}
				if !sameTryLocal(view.CatchBodies[0][1].(*statements.CustomStatement).ThrownValue, tr.Exception[0]) {
					t.Fatal("must rethrow original catch parameter")
				}
				if len(tr.TryBody) != 2 || len(tr.CatchBodies[0]) != 3 {
					t.Fatal("render view mutated original")
				}
			}
		})
	}
}

func TestResourceOperandCoverageKeepsEveryThrowingOrigin(t *testing.T) {
	covered := func(pc int) bool { return pc >= 10 && pc < 20 }
	arr := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	local := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	index := values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
	for _, test := range []struct {
		name  string
		value values.JavaValue
		want  bool
	}{
		{"last array load", &values.JavaArrayMember{Object: arr, Index: index, HasOriginPC: true, OriginPC: 19}, true},
		{"outside array load", &values.JavaArrayMember{Object: arr, Index: index, HasOriginPC: true, OriginPC: 20}, false},
		{"unwitnessed array load", &values.JavaArrayMember{Object: arr, Index: index, OriginPC: 19}, false},
		{"last array length", &values.ArrayLengthExpression{Array: arr, HasOriginPC: true, OriginPC: 19}, true},
		{"outside array length", &values.ArrayLengthExpression{Array: arr, HasOriginPC: true, OriginPC: 20}, false},
		{"known local assignment", values.NewAssignmentExpression(local, index, 19, nil), true},
		{"outside local assignment", values.NewAssignmentExpression(local, index, 20, nil), false},
		{"unwitnessed assignment", &values.AssignmentExpression{Target: local, Value: index, OriginPC: 19}, false},
		{"folded foreign operand", values.NewAssignmentExpression(local, &values.JavaArrayMember{Object: arr, Index: index, HasOriginPC: true, OriginPC: 20}, 19, nil), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := finallyCoveredValue(test.value, covered); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
	allocation := &values.NewExpression{JavaType: arr.Type(), OriginPC: 10, HasOriginPC: true, Length: []values.JavaValue{index}, Initializer: []values.JavaValue{values.JavaNull}, EvaluationEndPC: 19, HasEvaluationEndPC: true}
	if !finallyCoveredValue(allocation, covered) {
		t.Fatal("fully protected allocation/stores must be retained")
	}
	allocation.EvaluationEndPC = 20
	if finallyCoveredValue(allocation, covered) {
		t.Fatal("initializer crossing handler end must fail closed")
	}
	allocation.HasEvaluationEndPC = false
	if finallyCoveredValue(allocation, covered) {
		t.Fatal("initializer without store interval witness must fail closed")
	}
}

func TestResourceFinallyLabeledTransferNeedsEnclosedTarget(t *testing.T) {
	for _, target := range []string{"inside", "outside", ""} {
		t.Run(target, func(t *testing.T) {
			tr := primaryResourceFixture()
			jump := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "continue rendered" }, func(*utils.VariableId, *utils.VariableId) {})
			jump.LoopTransferKind = "continue"
			jump.LoopTargetLabel = target
			loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{jump})
			loop.Label = "inside"
			tr.TryBody = append([]statements.Statement{loop}, tr.TryBody...)
			_, ok := recoverPrimaryResourceFinally(tr)
			if ok != (target == "inside") {
				t.Fatalf("proved=%v", ok)
			}
		})
	}
}

func TestFinallyArithmeticRejectsMalformedAndThrowingPseudoPrimitives(t *testing.T) {
	covered := func(int) bool { return true }
	for _, op := range []string{values.INC, values.DEC, values.ADD} {
		for _, args := range [][]values.JavaValue{nil, {nil}, {nil, nil}, {values.JavaNull, values.JavaNull}} {
			expression := &values.JavaExpression{Op: op, Values: args}
			if finallyCoveredValue(expression, covered) {
				t.Fatal("malformed or null arithmetic must fail closed")
			}
		}
		for _, typ := range []types.JavaType{types.NewJavaPrimer(types.JavaString), types.NewJavaPrimer(types.JavaBoolean), types.NewJavaPrimer(types.JavaVoid), types.NewJavaClass("java.lang.Integer")} {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			expression := values.NewBinaryExpression(ref, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), op, typ)
			if finallyCoveredValue(expression, covered) {
				t.Fatal("string concatenation, boxing, boolean and void lack nonthrowing numeric proof")
			}
		}
	}
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	if !finallyCoveredValue(values.NewBinaryExpression(ref, values.NewJavaLiteral(1, ref.Type()), values.INC, ref.Type()), covered) {
		t.Fatal("decoded int-local increment remains nonthrowing")
	}
}
