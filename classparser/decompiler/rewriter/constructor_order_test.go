package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestConstructorCallMovesOnlyPastBareDeclarations(t *testing.T) {
	ref := values.NewJavaRef(nil, nil, types.NewJavaClass("Fixture"))
	ref.IsThis = true
	call := statements.NewExpressionStatement(&values.FunctionCallExpression{Object: ref, IsSpecialInvoke: true, FunctionName: "<init>"})
	decl := statements.NewDeclareStatement(ref)
	effect := statements.NewExpressionStatement(&values.FunctionCallExpression{FunctionName: "effect"})
	body := []statements.Statement{decl, call, effect}
	PlaceConstructorCallFirst(&body)
	if body[0] != call || body[1] != decl || body[2] != effect {
		t.Fatal("constructor call must precede hoisted declarations")
	}
	body = []statements.Statement{effect, decl, call}
	PlaceConstructorCallFirst(&body)
	if body[0] != effect || body[2] != call {
		t.Fatal("must not move a constructor past effects")
	}
}
