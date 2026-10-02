package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestGenericFieldReceiverArrayElementAndBoundedField(t *testing.T) {
	ctx := &class_context.ClassContext{TypeParams: []string{"V"}, ClassSig: "<V:Ljava/lang/Number;>Ljava/lang/Object;"}
	ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
		return "<E:Ljava/lang/Number;>Ljava/lang/Object;", nil, name == "sample/Cell"
	}
	ctx.SiblingFieldSig = func(owner, field string) (string, bool) { return "TE;", owner == "sample/Cell" && field == "value" }
	cell := types.NewParameterizedType("sample.Cell", []types.JavaType{types.NewJavaClass("V")})
	array := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(cell))
	element := &JavaArrayMember{Object: array, Index: NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}
	field := NewRefMember(element, "value", types.NewJavaClass("java.lang.Number"))
	if view := SourceFieldType(ctx, field); view == nil || view.String(ctx) != "V" {
		t.Fatalf("source=%v", view)
	}
	field.JavaType = types.NewJavaClass("java.lang.Object")
	if SourceFieldType(ctx, field) != nil {
		t.Fatal("field erasure mismatch accepted")
	}
	array.ResetVarType(types.NewJavaArrayType(types.NewJavaClass("sample.Cell")))
	field.JavaType = types.NewJavaClass("java.lang.Number")
	if SourceFieldType(ctx, field) != nil {
		t.Fatal("raw owner borrowed caller V")
	}
}

func TestGenericFieldReceiverComposesEachDeclaration(t *testing.T) {
	ctx := &class_context.ClassContext{TypeParams: []string{"V"}}
	ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
		switch name {
		case "sample/Holder":
			return "<X:Ljava/lang/Object;>Ljava/lang/Object;", nil, true
		case "sample/Box":
			return "<Y:Ljava/lang/Object;>Ljava/lang/Object;", nil, true
		}
		return "", nil, false
	}
	ctx.SiblingFieldSig = func(owner, field string) (string, bool) {
		switch owner + "." + field {
		case "sample/Holder.box":
			return "Lsample/Box<TX;>;", true
		case "sample/Box.mapper":
			return "Ljava/util/function/Function<-TY;+TY;>;", true
		}
		return "", false
	}
	receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("sample.Holder", []types.JavaType{types.NewJavaClass("V")}))
	box := NewRefMember(receiver, "box", types.NewJavaClass("sample.Box"))
	mapper := NewRefMember(box, "mapper", types.NewJavaClass("java.util.function.Function"))
	recovered := recoverParameterizedFieldReceiver(ctx, mapper)
	if recovered == nil || recovered.String(ctx) != "Function<? super V, ? extends V>" {
		t.Fatalf("field chain = %v", recovered)
	}
	mt, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{Name: "java.util.function.Function", Member: "apply", Description: "(Ljava/lang/Object;)Ljava/lang/Object;", JavaType: mt}
	call := NewFunctionCallExpression(mapper, member, mt.FunctionType())
	call.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}
	if source := call.String(ctx); !strings.Contains(source, "(V)") {
		t.Fatalf("recovered consumer type did not reach argument rendering: %s", source)
	}

	// Same-spelled V in the caller is not evidence for a raw Box's Y.
	raw := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("sample.Box"))
	if got := recoverParameterizedFieldReceiver(ctx, NewRefMember(raw, "mapper", mapper.Type())); got != nil {
		t.Fatalf("raw receiver invented type arguments: %v", got)
	}
	// Cycles can occur in malformed/intermediate IR; fail closed.
	cycle := NewRefMember(nil, "box", types.NewJavaClass("sample.Box"))
	cycle.Object = cycle
	if got := recoverParameterizedFieldReceiver(ctx, cycle); got != nil {
		t.Fatalf("cycle recovered %v", got)
	}
}

func TestFixedGenericFieldCrossesOnlyNonGenericRawOwners(t *testing.T) {
	for _, scenario := range []string{"fixed", "raw generic parent", "raw generic owner", "shadowed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{}
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				if n == "sample/Owner" {
					if scenario == "raw generic owner" {
						return "<T:Ljava/lang/Object;>Lsample/Base;", nil, true
					}
					return "Lsample/Base;", nil, true
				}
				if n == "sample/Base" {
					if scenario == "raw generic parent" {
						return "<T:Ljava/lang/Object;>Ljava/lang/Object;", nil, true
					}
					return "Ljava/lang/Object;", nil, true
				}
				return "", nil, false
			}
			ctx.SiblingFieldSig = func(n, field string) (string, bool) {
				if n == "sample/Owner" && scenario == "shadowed" {
					return "Ljava/util/Map;", true
				}
				return "Ljava/util/Map<Ljava/lang/String;Ljava/lang/Integer;>;", n == "sample/Base"
			}
			obj := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("sample.Owner"))
			got := recoverParameterizedFieldReceiver(ctx, NewRefMember(obj, "values", types.NewJavaClass("java.util.Map")))
			_, parameterized := types.AsParameterizedType(got)
			if parameterized != (scenario == "fixed") {
				t.Fatalf("recovered %v", got)
			}
		})
	}
}
