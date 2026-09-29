package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDisjointWebsSharingRefDoNotInventGenericElementType(t *testing.T) {
	for _, kind := range []string{"mixed", "same generic", "parameter", "one web"} {
		t.Run(kind, func(t *testing.T) {
			generic := types.NewParameterizedType("java.util.Iterator", []types.JavaType{types.NewJavaClass("E")})
			raw := types.NewJavaClass("java.util.Iterator")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, generic)
			first := types.JavaType(raw)
			if kind == "same generic" {
				first = generic
			}
			ref.IsParam = kind == "parameter"
			a := &OpCode{stackConsumed: []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, first)}}
			b := &OpCode{stackConsumed: []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, generic)}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opcodeIdToRef: map[*OpCode][][2]any{a: {{ref, true}}, b: {{ref, false}}}}
			owners := map[*values.JavaRef]map[int]bool{ref: {1: true, 2: true}}
			if kind == "one web" {
				delete(owners[ref], 2)
			}
			d.eraseConflictingSharedWebTypes(map[int][]*OpCode{1: {a}, 2: {b}}, []int{1, 2}, owners)
			_, stillParameterized := types.AsParameterizedType(ref.Type())
			if stillParameterized != (kind != "mixed") {
				t.Fatalf("declared type=%s", ref.Type().String(d.FunctionContext))
			}
			if kind == "mixed" {
				call := &values.FunctionCallExpression{Object: ref, FunctionName: "next", FuncType: types.NewJavaFuncType("()Ljava/lang/Object;", nil, types.NewJavaClass("java.lang.Object"))}
				if got := call.Type().String(d.FunctionContext); got != "Object" {
					t.Fatalf("a raw reaching definition must not claim E: %s", got)
				}
			}
		})
	}
}
