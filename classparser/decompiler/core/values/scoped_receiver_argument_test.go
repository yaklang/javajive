package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"testing"
)

func scopedReceiverArgumentFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class) {
	desc := "(Ljava/lang/Number;)Ljava/lang/Number;"
	ft, _ := types.ParseMethodDescriptor(desc)
	ctx := &class_context.ClassContext{ClassSig: "<R:Ljava/lang/Number;>Ljava/lang/Object;", TypeParams: []string{"R"}}
	meta := map[string]callbinding.Class{
		"example/Port": {Name: "example/Port", Signature: "<E:Ljava/lang/Number;>Ljava/lang/Object;", Public: true, MembersComplete: true, ParentsComplete: true, IsInterface: true, Methods: []callbinding.Method{{Name: "exchange", Desc: desc, Signature: "(TE;)TE;", Generic: true, Public: true}}},
	}
	ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) { v, ok := meta[n]; return v, ok }
	ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		v, ok := meta[n]
		m := map[string]string{}
		for _, x := range v.Methods {
			m[class_context.MethodDescKey(x.Name, x.Desc)] = x.Signature
		}
		return v.Signature, m, ok
	}
	obj := NewJavaRef(nil, nil, types.NewParameterizedType("example.Port", []types.JavaType{types.NewJavaTypeVariable("R")}))
	call := &FunctionCallExpression{ClassName: "example.Port", FunctionName: "exchange", Descriptor: desc, FuncType: ft.FunctionType(), Object: obj, Kind: InvokeInterface, HasOriginPC: true, OriginPC: 17, Arguments: []JavaValue{NewJavaRef(nil, nil, types.NewJavaClass("java.lang.Number"))}}
	return call, ctx, meta
}

func TestScopedReceiverArgumentRequiresExactDeclarationAndErasure(t *testing.T) {
	for _, variant := range []string{"exact", "renamed", "missing metadata", "incomplete", "wrong identity", "duplicate", "competing", "incomplete parent", "cycle", "wrong signature erasure", "method formal", "foreign formal", "raw receiver", "wrong owner", "wildcard", "concrete argument", "different bound", "narrow operand", "null", "missing PC", "changed descriptor", "static", "special", "wrong kind", "bridge", "varargs", "private", "method shadow", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, meta := scopedReceiverArgumentFixture()
			c := meta["example/Port"]
			switch variant {
			case "renamed":
				f.FunctionName = "replace"
				c.Methods[0].Name = "replace"
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "incomplete":
				c.MembersComplete = false
			case "wrong identity":
				c.Name = "example/Other"
			case "duplicate":
				c.Methods = append(c.Methods, c.Methods[0])
			case "competing":
				m := c.Methods[0]
				m.Desc = "(Ljava/lang/Integer;)Ljava/lang/Number;"
				c.Methods = append(c.Methods, m)
			case "incomplete parent":
				c.Parents = []string{"example/Absent"}
			case "cycle":
				c.Parents = []string{"example/Port"}
			case "wrong signature erasure":
				c.Methods[0].Signature = "(Ljava/lang/Object;)TE;"
			case "method formal":
				c.Methods[0].Signature = "<E:Ljava/lang/Number;>(TE;)TE;"
			case "foreign formal":
				c.Methods[0].Signature = "(TX;)TE;"
			case "raw receiver":
				f.Object = NewJavaRef(nil, nil, types.NewJavaClass("example.Port"))
			case "wrong owner":
				f.ClassName = "example.Other"
			case "wildcard":
				f.Object = NewJavaRef(nil, nil, types.ParseSignature("Lexample/Port<+TR;>;"))
			case "concrete argument":
				f.Object = NewJavaRef(nil, nil, types.ParseSignature("Lexample/Port<Ljava/lang/Number;>;"))
			case "different bound":
				ctx.ClassSig = "<R:Ljava/lang/Integer;>Ljava/lang/Object;"
			case "narrow operand":
				f.Arguments[0] = NewJavaRef(nil, nil, types.NewJavaClass("java.lang.Object"))
			case "null":
				f.Arguments[0] = NewJavaLiteral("null", types.NewJavaClass("java.lang.Number"))
			case "missing PC":
				f.HasOriginPC = false
			case "changed descriptor":
				f.Descriptor = "(Ljava/lang/Object;)Ljava/lang/Number;"
			case "static":
				f.IsStatic = true
			case "special":
				f.IsSpecialInvoke = true
			case "wrong kind":
				f.Kind = InvokeVirtual
			case "bridge":
				c.Methods[0].Bridge = true
			case "varargs":
				c.Methods[0].Varargs = true
			case "private":
				c.Methods[0].Public = false
			case "method shadow":
				ctx.CurrentMethodSig = "<R:Ljava/lang/Number;>()V"
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				base, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(base, workbudget.Limits{})
			}
			meta["example/Port"] = c
			before := append([]JavaValue(nil), f.Arguments...)
			w := f.Witness()
			typ := f.Arguments[0].Type().Copy()
			got := f.scopedReceiverArgument(0, ctx)
			want := variant == "exact" || variant == "renamed"
			if (got != nil) != want {
				t.Fatalf("view=%v want=%v", got, want)
			}
			if got != nil {
				if name, ok := types.TypeVariableName(got); !ok || name != "R" {
					t.Fatal("wrong binder")
				}
			}
			if !reflect.DeepEqual(before, f.Arguments) || w != f.Witness() || !reflect.DeepEqual(typ, f.Arguments[0].Type()) {
				t.Fatal("mutated shared invocation")
			}
		})
	}
}
