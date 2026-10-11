package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestReferenceArrayStoreObjectViewNeedsDecodedReferenceWitness(t *testing.T) {
	for _, change := range []string{"valid", "unwitnessed", "primitive array", "primitive value", "non-array", "unknown array", "null", "same component", "reference matrix"} {
		t.Run(change, func(t *testing.T) {
			arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
			valueType := types.NewJavaClass("java.lang.Object")
			ctx := &class_context.ClassContext{}
			array := values.NewCustomValue(func(*class_context.ClassContext) string { return "array()" }, func() types.JavaType { return arrayType })
			index := values.NewCustomValue(func(*class_context.ClassContext) string { return "index()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
			value := values.NewCustomValue(func(*class_context.ClassContext) string { return "value()" }, func() types.JavaType { return valueType })
			statement := NewArrayMemberAssignStatement(&values.JavaArrayMember{Object: array, Index: index}, value)
			statement.ReferenceArrayStore = true
			switch change {
			case "unwitnessed":
				statement.ReferenceArrayStore = false
			case "primitive array":
				arrayType = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
			case "primitive value":
				valueType = types.NewJavaPrimer(types.JavaInteger)
			case "non-array":
				arrayType = types.NewJavaClass("java.lang.Object")
			case "unknown array":
				arrayType = nil
			case "null":
				statement.JavaValue = values.JavaNull
			case "same component":
				valueType = types.NewJavaClass("java.lang.String")
			case "reference matrix":
				arrayType = types.NewJavaArrayType(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
			}
			if got := statement.referenceArrayStoreNeedsObjectView(ctx); got != (change == "valid" || change == "reference matrix") {
				t.Fatalf("Object[] view chosen=%v", got)
			}
			if change == "valid" || change == "reference matrix" {
				got := statement.String(ctx)
				if got != "((java.lang.Object[]) (array()))[index()] = value()" {
					t.Fatalf("store added an RHS check/reordered operands: %s", got)
				}
			}
		})
	}
}

func TestReferenceArrayStoreKeepsExplicitCheckedRHS(t *testing.T) {
	ctx := &class_context.ClassContext{}
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
	array := values.NewCustomValue(func(*class_context.ClassContext) string { return "array()" }, func() types.JavaType { return arrayType })
	index := values.NewCustomValue(func(*class_context.ClassContext) string { return "index()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
	value := values.NewCustomValue(func(*class_context.ClassContext) string { return "value()" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
	checked := &values.CastExpression{Value: value, TargetType: types.NewJavaClass("java.lang.String"), OriginPC: 10}
	statement := NewArrayMemberAssignStatement(&values.JavaArrayMember{Object: array, Index: index}, checked)
	statement.ReferenceArrayStore = true
	got := statement.String(ctx)
	if strings.Contains(got, "java.lang.Object[]") || !strings.Contains(got, "String") || strings.Count(got, "array()") != 1 || strings.Count(got, "index()") != 1 || strings.Count(got, "value()") != 1 || strings.Index(got, "index()") > strings.Index(got, "value()") {
		t.Fatalf("lost original CHECKCAST/order/single evaluation: %s", got)
	}
}

func TestReferenceArrayStoreUsesDeclaredWebView(t *testing.T) {
	ctx := &class_context.ClassContext{ClassSig: "<T:Ljava/lang/Object;>Ljava/lang/Object;", TypeParams: []string{"T"}}
	array := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	array.Id.SetName("row")
	array.WebDeclType = types.NewJavaArrayType(types.NewJavaClass("T"))
	value := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	value.Id.SetName("item")
	st := NewArrayMemberAssignStatement(values.NewJavaArrayMember(array, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))), value)
	st.ReferenceArrayStore = true
	if got := st.String(ctx); got != "((java.lang.Object[]) (row))[0] = item" {
		t.Fatal(got)
	}
	if array.Type().String(ctx) != "Object[]" || array.WebDeclType.String(ctx) != "T[]" {
		t.Fatal("shared view changed")
	}
}

func TestReferenceArrayStorePartialLexicalArrayKeepsPhysicalStore(t *testing.T) {
	for _, witness := range []bool{false, true} {
		ctx := &class_context.ClassContext{ClassName: "scope.Reader", ClassSig: "Ljava/lang/Object;", TypeParams: []string{"K"}, LexicalTypeParamSignatures: []string{"<K:Ljava/lang/Number;>Ljava/lang/Object;"}, FieldTypeVars: map[string]string{"rows": "K[][]"}}
		this := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("scope.Reader"))
		this.IsThis = true
		rows := values.NewRefMember(this, "rows", types.NewJavaArrayType(types.NewJavaArrayType(types.NewJavaClass("java.lang.Number"))))
		index := values.NewCustomValue(func(*class_context.ClassContext) string { return "index()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
		value := values.NewCustomValue(func(*class_context.ClassContext) string { return "row()" }, func() types.JavaType { return types.NewJavaArrayType(types.NewJavaClass("java.lang.Number")) })
		st := NewArrayMemberAssignStatement(values.NewJavaArrayMember(rows, index), value)
		st.ReferenceArrayStore = witness
		if st.referenceArrayStoreNeedsObjectView(ctx) != witness {
			t.Fatal("source/physical store mismatch")
		}
		if witness && st.String(ctx) != "((java.lang.Object[]) (this.rows))[index()] = row()" {
			t.Fatal(st.String(ctx))
		}
	}
}
