package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestExistingReturnCastUsesOnlyItsDeclaredErasure(t *testing.T) {
	ret := types.NewParameterizedType("example.Box", []types.JavaType{types.NewJavaClass("T")})
	ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: ret}, CurrentMethodDesc: "()Lexample/Box;"}
	target := ret.String(ctx)
	if got := renderExistingReturnCast(ctx, target, "effect()"); strings.Count(got, "effect()") != 1 || !strings.Contains(got, "(Box)") {
		t.Fatal(got)
	}
	for _, desc := range []string{"()Ljava/lang/Object;", "()I", "", "()[Lexample/Box;"} {
		ctx.CurrentMethodDesc = desc
		if got := renderExistingReturnCast(ctx, target, "effect()"); strings.Contains(got, "(Box)") {
			t.Fatalf("descriptor %s cannot supply this raw bridge: %s", desc, got)
		}
	}
	ctx.CurrentMethodDesc = "()Lexample/Box;"
	if got := renderExistingReturnCast(ctx, "Object", "effect()"); strings.Contains(got, "(Box)") {
		t.Fatal("unrelated existing cast changed")
	}
}
