package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/rewriter"
	"testing"
)

func conditionalFinallyCheckedProjectionFixture() *statements.TryCatchStatement {
	ref := func(name string) *values.JavaRef {
		return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
	}
	caught, primary, token := ref("java.lang.Throwable"), ref("java.lang.Throwable"), ref("java.io.IOException")
	token.IsParam = true
	condition := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
	condition.IsParam = true
	call := func(pc int, name, descriptor string) *values.FunctionCallExpression {
		result := types.NewJavaPrimer(types.JavaVoid)
		if descriptor != "()V" {
			result = types.NewJavaClass("java.lang.RuntimeException")
		}
		return &values.FunctionCallExpression{ClassName: "example.Actions", FunctionName: name, Descriptor: descriptor,
			IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("example.Actions")),
			HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType(descriptor, nil, result)}
	}
	throw := func(pc int, value values.JavaValue) *statements.CustomStatement {
		st := statements.NewThrowStatement(value)
		st.HasOriginPC, st.OriginPC = true, pc
		return st
	}
	cleanup := func(pc int, completion statements.Statement) statements.Statement {
		return statements.NewIfStatement(condition,
			[]statements.Statement{statements.NewExpressionStatement(call(pc, "finish", "()V")), completion},
			[]statements.Statement{throw(pc+3, call(pc+2, "fail", "()Ljava/lang/RuntimeException;"))})
	}
	ret := statements.NewReturnStatement(values.NewJavaLiteral(29, types.NewJavaPrimer(types.JavaInteger)))
	ret.HasOriginPC, ret.OriginPC = true, 25
	tr := statements.NewTryCatchStatement(
		[]statements.Statement{statements.NewExpressionStatement(call(12, "body", "()V")), cleanup(22, ret)},
		[][]statements.Statement{{cleanup(42, throw(48, token))}, {cleanup(82, throw(88, primary))}})
	tr.Exception = []*values.JavaRef{caught, primary}
	tr.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{10, 20}}},
		{EntryPC: 70, CatchAll: true, ProtectedRanges: [][2]int{{10, 20}, {30, 40}}}}
	return tr
}

func TestCheckedEscapeFinallyContinuationRetainsItsOwnOuterThrowScope(t *testing.T) {
	tr := conditionalFinallyCheckedProjectionFixture()
	view, known := rewriter.RecoverCatchAllFinally(tr)
	if !known || view.Continuation == nil {
		t.Fatal("original continuation view missing")
	}
	got := checkedEscapeThrownTypes([]statements.Statement{tr}, func(string) (callbinding.Class, bool) { return callbinding.Class{}, false })
	if got[48] != "java/io/IOException" {
		t.Fatalf("post-finally checked throw disappeared: %v", got)
	}
	if _, found := got[88]; found {
		t.Fatal("synthetic finally rethrow was reintroduced")
	}
	d := &ClassObjectDumper{}
	precise := d.preciseCatchRethrows([]statements.Statement{tr}, &CodeAttribute{})
	if value, present := precise[48]; !present || value {
		t.Fatalf("continuation borrowed or lost a catch parameter binding: %v", precise)
	}
	if _, found := precise[88]; found {
		t.Fatal("synthetic handler acquired a source rethrow binding")
	}
}
