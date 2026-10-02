package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestWidenedErasedReturnRequiresProvenHierarchyAndExactTuple(t *testing.T) {
	for _, name := range []string{"proved", "unknown hierarchy", "unrelated result", "narrowed return", "descriptor mismatch", "raw return", "poly operand"} {
		t.Run(name, func(t *testing.T) {
			target := types.NewParameterizedType("probe.Base", []types.JavaType{types.NewJavaClass("T")})
			metadata := map[string]callbinding.Class{
				"probe/Base":  {Name: "probe/Base", ParentsComplete: true, MembersComplete: true},
				"probe/Child": {Name: "probe/Child", Parents: []string{"probe/Base"}, ParentsComplete: true, MembersComplete: true},
			}
			ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: target}, CurrentMethodDesc: "()Lprobe/Base;",
				InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }}
			arg := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Payload", []types.JavaType{types.NewJavaClass("T")}))
			call := &values.FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "make", IsStatic: true, Kind: values.InvokeStatic, Descriptor: "(Lprobe/Payload;)Lprobe/Child;", Arguments: []values.JavaValue{arg}, OriginPC: 17}
			switch name {
			case "unknown hierarchy":
				delete(metadata, "probe/Child")
			case "unrelated result":
				call.Descriptor = "(Lprobe/Payload;)Lprobe/Other;"
			case "narrowed return":
				call.Descriptor = "(Lprobe/Payload;)Ljava/lang/Object;"
			case "descriptor mismatch":
				ctx.CurrentMethodDesc = "()Lprobe/Child;"
			case "raw return":
				ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Base")}
			case "poly operand":
				poly := values.NewCustomValue(func(*class_context.ClassContext) string { return "x -> x" }, func() types.JavaType { return arg.Type() })
				poly.Flag = "lambda"
				call.Arguments[0] = poly
			}
			planned, ok := erasedWidenedReturnChain(ctx, call)
			if ok != (name == "proved") {
				t.Fatalf("proof=%t", ok)
			}
			if ok && (planned.Witness() != call.Witness() || planned.OriginPC != call.OriginPC || planned.Arguments[0].(*values.CastExpression).Value != arg || call.Arguments[0] != arg) {
				t.Fatal("return adaptation changed invocation, operand identity or origin")
			}
		})
	}
}

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
