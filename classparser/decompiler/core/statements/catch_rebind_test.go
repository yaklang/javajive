package statements

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTryCatchReplaceVarVisitsEveryHandler(t *testing.T) {
	oldID, newID := utils.NewRootVariableId(), utils.NewRootVariableId()
	oldID.SetName("var3")
	newID.SetName("fallback")
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	var reads []*values.JavaRef
	read := func() *values.JavaRef {
		ref := values.NewJavaRef(oldID, nil, boolType)
		reads = append(reads, ref)
		return ref
	}
	// Same spelling, different identity: changing this array would silently
	// corrupt a disjoint slot that happens to receive the old boolean's name.
	arrayID := utils.NewRootVariableId()
	arrayID.SetName("var3")
	array := values.NewJavaRef(arrayID, nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	params := []*values.JavaRef{
		values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.IllegalArgumentException")),
		values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.IllegalStateException")),
		values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.RuntimeException")),
	}
	paramIDs := []*utils.VariableId{params[0].Id, params[1].Id, params[2].Id}
	nested := NewTryCatchStatement([]Statement{NewReturnStatement(read())}, [][]Statement{{NewReturnStatement(read())}})
	nested.Exception = []*values.JavaRef{params[2]}
	branch := NewIfStatement(read(), []Statement{NewReturnStatement(read())}, []Statement{nested})
	call := &values.FunctionCallExpression{
		IsStatic: true, ClassName: "example.Oracle", FunctionName: "accept",
		Arguments: []values.JavaValue{values.NewSlotValue(read(), boolType), array},
		FuncType:  &types.JavaFuncType{ParamTypes: []types.JavaType{boolType, array.Type()}, ReturnType: boolType},
	}
	st := NewTryCatchStatement([]Statement{NewExpressionStatement(call)}, [][]Statement{
		{NewReturnStatement(read())}, {branch}, {NewExpressionStatement(call)},
	})
	st.Exception = params
	st.ReplaceVar(oldID, newID)
	for i, ref := range reads {
		if ref.Id != newID || ref.Type() != boolType {
			t.Errorf("read %d was not rebound without changing its type: %s", i, ref.String(&class_context.ClassContext{}))
		}
	}
	for i, ref := range params {
		if ref.Id != paramIDs[i] {
			t.Errorf("unrelated catch parameter %d was rebound", i)
		}
	}
	if array.Id != arrayID || array.String(&class_context.ClassContext{}) != "var3" {
		t.Fatal("rebound an unrelated local with the same spelling")
	}
}
