package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDeclaredFunctionalTypeNeedsConsistentSAMWitnesses(t *testing.T) {
	for _, scenario := range []string{"direct", "inherited", "conflict", "unbound", "missing signature", "method formal", "other overload", "primitive binding", "wrong arity"} {
		t.Run(scenario, func(t *testing.T) {
			classSig := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			methodSig := "(TT;)TT;"
			key := class_context.MethodDescKey("apply", "(Ljava/lang/Object;)Ljava/lang/Object;")
			inst := "(Ljava/lang/String;)Ljava/lang/String;"
			switch scenario {
			case "inherited":
				classSig = "<U:Ljava/lang/Object;>Ljava/lang/Object;Lexample/Parent<TU;>;"
			case "conflict":
				inst = "(Ljava/lang/String;)Ljava/lang/Integer;"
			case "unbound":
				classSig = "<T:Ljava/lang/Object;R:Ljava/lang/Object;>Ljava/lang/Object;"
			case "missing signature":
				methodSig = ""
			case "method formal":
				methodSig = "<T:Ljava/lang/Object;>(TT;)TT;"
			case "other overload":
				key = class_context.MethodDescKey("apply", "(I)Ljava/lang/Object;")
			case "primitive binding":
				inst = "(I)I"
			case "wrong arity":
				inst = "(Ljava/lang/String;I)Ljava/lang/String;"
			}
			ctx := &class_context.ClassContext{SiblingClassSig: func(owner string) (string, map[string]string, bool) {
				if owner == "example/Action" {
					if scenario == "inherited" {
						return classSig, nil, true
					}
					return classSig, map[string]string{key: methodSig}, true
				}
				if owner == "example/Parent" {
					return "<T:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{key: methodSig}, true
				}
				return "", nil, false
			}}
			got := inferDeclaredFunctionalType(ctx, types.NewJavaClass("example.Action"), "apply", "(Ljava/lang/Object;)Ljava/lang/Object;", inst)
			if scenario != "direct" && scenario != "inherited" {
				if got != nil {
					t.Fatal("inferred a type without consistent declaration evidence")
				}
				return
			}
			pt, ok := types.AsParameterizedType(got)
			if !ok || pt.RawClassName != "example.Action" || len(pt.TypeArgs) != 1 {
				t.Fatalf("missing parameterized target: %v", got)
			}
			if arg, _ := types.ClassFQNOf(pt.TypeArgs[0]); arg != "java.lang.String" {
				t.Fatal("SAM input/result did not bind the class variable")
			}
		})
	}
}
