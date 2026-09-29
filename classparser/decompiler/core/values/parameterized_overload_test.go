package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestParameterizedOverloadCastRequiresDeclarationAndScopeProof(t *testing.T) {
	const descriptor = "(Ljava/util/List;)I"
	for _, scenario := range []string{"concrete", "missing ancestor", "missing platform method table", "caller variable", "class variable shadowed by caller", "foreign variable", "shadowed method variable", "unrelated method variable", "missing signature", "different owner", "incomplete family", "missing target", "unique", "different erasure", "already parameterized", "varargs", "inapplicable competitor"} {
		t.Run(scenario, func(t *testing.T) {
			sig := "(Ljava/util/List<+Ljava/lang/CharSequence;>;)I"
			want := "List<? extends CharSequence>"
			if scenario != "concrete" && scenario != "missing ancestor" && scenario != "missing platform method table" {
				want = ""
			}
			switch scenario {
			case "caller variable":
				sig, want = "(Ljava/util/List<+TE;>;)I", "List<? extends E>"
			case "class variable shadowed by caller":
				sig = "(Ljava/util/List<+TE;>;)I"
			case "foreign variable":
				sig = "(Ljava/util/List<+TU;>;)I"
			case "shadowed method variable":
				sig = "<E:Ljava/lang/CharSequence;>(Ljava/util/List<+TE;>;)I"
			case "unrelated method variable":
				sig = "<E:Ljava/lang/Object;>(Ljava/util/List<+Ljava/lang/CharSequence;>;)I"
			case "missing signature":
				sig = ""
			}
			methods := []callbinding.Method{
				{Name: "pick", Desc: descriptor, Generic: true},
				{Name: "pick", Desc: "(Ljava/util/Collection;)I", Generic: true},
			}
			if scenario == "unique" {
				methods = methods[:1]
			} else if scenario == "missing target" {
				methods = methods[1:]
			} else if scenario == "varargs" {
				methods[0].Varargs = true
			} else if scenario == "inapplicable competitor" {
				methods[1].Desc = "(Ljava/util/Set;)I"
			}
			ctx := &class_context.ClassContext{
				ClassName: "example.Owner", TypeParams: []string{"E"},
				MethodSignaturesByDesc: map[string]string{class_context.MethodDescKey("pick", descriptor): sig},
				InvocationMetadata: func(name string) (callbinding.Class, bool) {
					c := callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true}
					switch name {
					case "example/Owner", "example/Other":
						c.Methods = methods
						c.MembersComplete = scenario != "incomplete family"
						if scenario == "missing ancestor" {
							c.Parents = []string{"example/Unavailable"}
						}
					case "java/util/List":
						if scenario == "missing platform method table" {
							return callbinding.Class{}, false
						}
						c.Parents = []string{"java/util/Collection"}
					case "java/util/Collection", "java/util/Set":
					default:
						return callbinding.Class{}, false
					}
					return c, true
				},
			}
			if scenario == "class variable shadowed by caller" {
				ctx.ClassTypeParams = []string{"E"}
				ctx.CurrentMethodSig = "<E:Ljava/lang/CharSequence;>(Ljava/lang/Object;)I"
			}
			receiver := NewJavaRef(nil, nil, types.NewJavaClass("example.Owner"))
			receiver.IsThis = true
			argType := types.NewJavaClass("java.util.List")
			if scenario == "different erasure" {
				argType = types.NewJavaClass("java.util.Collection")
			} else if scenario == "already parameterized" {
				argType = types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.String")})
			}
			ft, err := types.ParseMethodDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			call := &FunctionCallExpression{Object: receiver, ClassName: "example.Owner", FunctionName: "pick", Descriptor: descriptor, FuncType: ft.FunctionType(), Arguments: []JavaValue{NewJavaRef(nil, nil, argType)}}
			if scenario == "different owner" {
				call.ClassName = "example.Other"
				ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
					if name == "example/Other" {
						return "", map[string]string{class_context.MethodDescKey("pick", descriptor): "(Ljava/util/List<+Lexample/Hidden;>;)I"}, true
					}
					return "", nil, false
				}
			}
			if got := call.parameterizedOverloadArgCast(0, ctx); got != want {
				t.Fatalf("cast=%q want=%q", got, want)
			}
		})
	}
}
