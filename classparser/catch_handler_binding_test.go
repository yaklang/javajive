package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialMergedHandlerBindsExactParameterIdentity(t *testing.T) {
	makeRef := func(name string) *values.JavaRef {
		id := utils.NewRootVariableId()
		id.SetName(name)
		return values.NewJavaRef(id, nil, types.NewJavaClass("java.lang.Throwable"))
	}
	first, second, unrelated := makeRef("firstFailure"), makeRef("cleanupFailure"), makeRef("cleanupFailure")
	throw := func(ref *values.JavaRef) statements.Statement {
		st := statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return "throw " + ref.String(ctx) }, ref.ReplaceVar)
		st.ThrownValue = ref
		return st
	}
	otherUse := statements.NewExpressionStatement(unrelated)
	inner := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{throw(second)}}
	exceptions, bodies := mergeNestedSameTypeCatches(&class_context.ClassContext{}, []*values.JavaRef{first, second}, [][]statements.Statement{{throw(first)}, {otherUse, inner, throw(second)}})
	if len(exceptions) != 1 || exceptions[0] != first || len(bodies) != 1 {
		t.Fatal("plain rethrow handlers were not merged")
	}
	for _, name := range []string{"firstFailure", "renamedFailure"} {
		first.Id.SetName(name)
		for n := 0; n < 2; n++ {
			text := statements.StatementsString(bodies[0], &class_context.ClassContext{})
			if strings.Count(text, "throw "+name) != 2 || strings.Contains(text, "throw cleanupFailure") || !strings.HasPrefix(text, "cleanupFailure") {
				t.Fatalf("handler binding lost its identity, renamed an unrelated local, or became stale:\n%s", text)
			}
			if second.Id.String() != "cleanupFailure" || unrelated.Id.String() != "cleanupFailure" {
				t.Fatal("rendering changed the original handler or an unrelated identity")
			}
		}
	}
}

func TestAdversarialHandlerMergeRejectsSameNamedDifferentThrow(t *testing.T) {
	makeRef := func(name string) *values.JavaRef {
		id := utils.NewRootVariableId()
		id.SetName(name)
		return values.NewJavaRef(id, nil, types.NewJavaClass("java.lang.Throwable"))
	}
	first, second, unrelated := makeRef("failure"), makeRef("cleanup"), makeRef("failure")
	throw := func(ref *values.JavaRef) statements.Statement {
		st := statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return "throw " + ref.String(ctx) }, ref.ReplaceVar)
		st.ThrownValue = ref
		return st
	}
	exceptions, _ := mergeNestedSameTypeCatches(&class_context.ClassContext{}, []*values.JavaRef{first, second}, [][]statements.Statement{{throw(unrelated)}, {throw(second)}})
	if len(exceptions) != 2 {
		t.Fatal("matching a printed throw name does not prove it rethrows the caught object")
	}
}
