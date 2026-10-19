package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func finallyArrayCleanupFixture() (*statements.TryCatchStatement, *statements.AssignStatement, *statements.AssignStatement) {
	tr := finallyFixture()
	object := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	object.IsParam = true
	arrayType := types.NewJavaArrayType(object.Type())
	copy := func(pc int) (*statements.AssignStatement, statements.Statement) {
		array := &values.NewExpression{JavaType: arrayType.Copy(), HasOriginPC: true, OriginPC: pc, HasEvaluationEndPC: true, EvaluationEndPC: pc + 4, Length: []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}, Initializer: []values.JavaValue{object}}
		ref := values.NewJavaRef(utils.NewRootVariableId(), array, arrayType.Copy())
		ref.MarkOriginalStackMaterialization(pc+1, core.OP_DUP, array)
		decl := statements.NewAssignStatement(ref, array, true)
		decl.HasOriginPC, decl.OriginPC = true, pc+1
		call := &values.FunctionCallExpression{ClassName: "p/Cleanup", FunctionName: "finish", Descriptor: "([Ljava/lang/Object;)V", Kind: values.InvokeStatic, IsStatic: true, Arguments: []values.JavaValue{ref}, HasOriginPC: true, OriginPC: pc + 5, FuncType: types.NewJavaFuncType("([Ljava/lang/Object;)V", []types.JavaType{arrayType}, types.NewJavaPrimer(types.JavaVoid))}
		return decl, statements.NewExpressionStatement(call)
	}
	normal, call := copy(12)
	handler, exceptional := copy(42)
	ret := tr.TryBody[2].(*statements.ReturnStatement)
	ret.OriginPC = 18
	tr.TryBody = []statements.Statement{tr.TryBody[0], normal, call, ret}
	tr.CatchBodies[1] = []statements.Statement{handler, exceptional, tr.CatchBodies[1][1]}
	tr.CatchBodies[1][2].(*statements.CustomStatement).OriginPC = 48
	return tr, normal, handler
}

func TestFinallyArrayCleanupBindsOnlyOriginalScopedMaterializations(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "different element", "different length", "different array type", "missing allocation PC", "missing last store PC", "folded store crosses handler", "protected operand", "wrong DUP", "wrong materialization PC", "changed seed", "parameter", "ordinary local", "not first", "call uses foreign local", "return escapes local", "outside use", "expression qualifier escapes local", "opaque source", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			tr, normal, handler := finallyArrayCleanupFixture()
			ref := normal.LeftValue.(*values.JavaRef)
			array := normal.JavaValue.(*values.NewExpression)
			switch variant {
			case "renamed":
				ref.Id.SetName("anotherTemporary")
			case "different element":
				array.Initializer[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "different length":
				array.Length[0] = values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
			case "different array type":
				array.JavaType = types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			case "missing allocation PC":
				array.HasOriginPC = false
			case "missing last store PC":
				array.HasEvaluationEndPC = false
			case "folded store crosses handler":
				array.EvaluationEndPC = 41
			case "protected operand":
				array.Initializer[0] = numericConversionCall(10)
			case "wrong DUP":
				other := values.NewJavaRef(utils.NewRootVariableId(), array, ref.Type())
				other.MarkOriginalStackMaterialization(normal.OriginPC, core.OP_ASTORE, array)
				normal.LeftValue = other
			case "wrong materialization PC":
				normal.OriginPC++
			case "changed seed":
				ref.Val = values.JavaNull
			case "parameter":
				ref.IsParam = true
			case "ordinary local":
				normal.LeftValue = values.NewJavaRef(utils.NewRootVariableId(), array, ref.Type())
			case "not first":
				normal.IsFirst = false
			case "call uses foreign local":
				tr.TryBody[2].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Arguments[0] = values.NewJavaRef(utils.NewRootVariableId(), array, ref.Type())
			case "return escapes local":
				tr.TryBody[3].(*statements.ReturnStatement).JavaValue = ref
			case "outside use":
				tr.TryBody = append([]statements.Statement{statements.NewExpressionStatement(ref)}, tr.TryBody...)
			case "expression qualifier escapes local":
				tr.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object = ref
			case "opaque source":
				array.ArgumentsGetter = func() string { panic("must not render") }
			case "cycle":
				tr.TryBody = append([]statements.Statement{tr}, tr.TryBody...)
			}
			view, got := RecoverCatchAllFinally(tr)
			want := variant == "original" || variant == "renamed"
			if got != want {
				t.Fatalf("cleanup let proof=%v expected=%v", got, want)
			}
			if got && (len(view.Cleanup) != 2 || view.Cleanup[0] != handler || len(view.TryBody) != 2 || len(tr.TryBody) != 4) {
				t.Fatal("must retain canonical allocation/local/call once and leave original AST unchanged")
			}
		})
	}
}
