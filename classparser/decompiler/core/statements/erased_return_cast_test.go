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

func TestFixedParameterizedFactoryRequiresExactStaticDeclaration(t *testing.T) {
	for _, name := range []string{"fixed wildcard", "fixed concrete", "true generic factory", "same target", "unknown family", "unknown signature", "foreign signature key", "foreign return", "consumer erasure mismatch", "raw target", "instance producer", "dynamic producer", "missing origin", "ambiguous declaration", "bridge declaration", "varargs declaration", "descriptor arguments", "hidden operand", "trailing signature", "free signature variable", "unavailable ancestor", "return-only overload"} {
		t.Run(name, func(t *testing.T) {
			target := types.NewParameterizedType("probe.Box", []types.JavaType{types.NewJavaClass("T")})
			methods := []callbinding.Method{{Name: "choose", Desc: "()Lprobe/Box;", Static: true, Generic: true}}
			signatures := map[string]string{class_context.MethodDescKey("choose", "()Lprobe/Box;"): "()Lprobe/Box<*>;"}
			metadata := map[string]callbinding.Class{"probe/Factory": {Name: "probe/Factory", MembersComplete: true, ParentsComplete: true, Methods: methods}}
			ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: target}, CurrentMethodDesc: "()Lprobe/Box;", TypeParams: []string{"T"}, InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) { return "", signatures, n == "probe/Factory" }}
			call := &values.FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "choose", Descriptor: "()Lprobe/Box;", IsStatic: true, Kind: values.InvokeStatic, OriginPC: 19, HasOriginPC: true}
			switch name {
			case "fixed concrete":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "()Lprobe/Box<Ljava/lang/String;>;"
			case "true generic factory":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "<U:Ljava/lang/Object;>()Lprobe/Box<TU;>;"
			case "trailing signature":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "()Lprobe/Box<*>;junk"
			case "free signature variable":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "()Lprobe/Box<TGhost;>;"
			case "unavailable ancestor":
				c := metadata["probe/Factory"]
				c.Parents = []string{"probe/Missing"}
				metadata["probe/Factory"] = c
			case "same target":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "()Lprobe/Box<TT;>;"
			case "unknown family":
				delete(metadata, "probe/Factory")
			case "unknown signature":
				ctx.SiblingClassSig = nil
			case "foreign signature key":
				signatures = map[string]string{class_context.MethodDescKey("choose", "(I)Lprobe/Box;"): "()Lprobe/Box<*>;"}
			case "foreign return":
				signatures[class_context.MethodDescKey("choose", call.Descriptor)] = "()Lprobe/Other<*>;"
			case "consumer erasure mismatch":
				ctx.CurrentMethodDesc = "()Ljava/lang/Object;"
			case "raw target":
				ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Box")}
			case "instance producer":
				call.IsStatic = false
				call.Kind = values.InvokeVirtual
			case "dynamic producer":
				call.Kind = values.InvokeDynamic
			case "missing origin":
				call.HasOriginPC = false
			case "return-only overload":
				c := metadata["probe/Factory"]
				c.Methods = append(c.Methods, callbinding.Method{Name: "choose", Desc: "()Lprobe/Other;", Static: true})
				metadata["probe/Factory"] = c
			case "ambiguous declaration":
				c := metadata["probe/Factory"]
				c.Methods = append(c.Methods, c.Methods[0])
				metadata["probe/Factory"] = c
			case "bridge declaration":
				c := metadata["probe/Factory"]
				c.Methods[0].Bridge = true
				metadata["probe/Factory"] = c
			case "varargs declaration":
				c := metadata["probe/Factory"]
				c.Methods[0].Varargs = true
				metadata["probe/Factory"] = c
			case "descriptor arguments":
				call.Descriptor = "(I)Lprobe/Box;"
			case "hidden operand":
				call.Arguments = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Object"))}
			}
			before := call.Witness()
			origin := call.OriginPC
			got := fixedParameterizedFactoryReturn(ctx, call)
			if got != (name == "fixed wildcard" || name == "fixed concrete") {
				t.Fatalf("proof=%t", got)
			}
			if call.Witness() != before || call.OriginPC != origin || len(call.Arguments) != func() int {
				if name == "hidden operand" {
					return 1
				}
				return 0
			}() {
				t.Fatal("proof changed the invoke tuple or operands")
			}
		})
	}
}
