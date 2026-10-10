package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestReferenceOverloadBindsBeforeUnboxing(t *testing.T) {
	for _, scenario := range []string{"inherited generic", "another reference", "array competitor", "missing parent", "incomplete members", "missing target", "primitive argument", "different target", "multiple arguments"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)Ljava/lang/Object;"
			classes := map[string]callbinding.Class{
				"example/Parent": {Name: "example/Parent", MembersComplete: true, ParentsComplete: true,
					Methods: []callbinding.Method{{Name: "apply", Desc: desc, Generic: true}}},
				"example/Child": {Name: "example/Child", MembersComplete: true, ParentsComplete: true,
					Parents: []string{"example/Parent"}, Methods: []callbinding.Method{
						{Name: "apply", Desc: "(I)Ljava/lang/Object;"},
						{Name: "apply", Desc: "(D)Ljava/lang/Object;"}}},
			}
			arg := JavaValue(NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Integer")))
			child := classes["example/Child"]
			switch scenario {
			case "another reference":
				child.Methods = append(child.Methods, callbinding.Method{Name: "apply", Desc: "(Ljava/lang/Number;)Ljava/lang/Object;"})
			case "array competitor":
				child.Methods = append(child.Methods, callbinding.Method{Name: "apply", Desc: "([Ljava/lang/Object;)Ljava/lang/Object;"})
			case "missing parent":
				delete(classes, "example/Parent")
			case "incomplete members":
				child.MembersComplete = false
			case "missing target":
				parent := classes["example/Parent"]
				parent.Methods = nil
				classes["example/Parent"] = parent
			case "primitive argument":
				arg = NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))
			case "different target":
				desc = "(Ljava/lang/Number;)Ljava/lang/Object;"
			case "multiple arguments":
				desc = "(Ljava/lang/Object;I)Ljava/lang/Object;"
			}
			classes["example/Child"] = child
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
				c, ok := classes[name]
				return c, ok
			}}
			ft, err := types.ParseMethodDescriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			call := &FunctionCallExpression{ClassName: "example.Child", FunctionName: "apply", Descriptor: desc,
				FuncType: ft.FunctionType(), Arguments: []JavaValue{arg}}
			want := scenario == "inherited generic"
			if got := call.objectFormalWinsBeforeUnboxing(ctx); got != want {
				t.Fatalf("strict invocation proof=%t, want=%t", got, want)
			}
			if want {
				if cast := call.witnessDescriptorArgCast(0, arg, ctx); cast != "" {
					t.Fatalf("invented generic-erasure pin %q", cast)
				}
			} else if scenario == "another reference" || scenario == "array competitor" {
				if cast := call.witnessDescriptorArgCast(0, arg, ctx); cast != "Object" {
					t.Fatalf("lost real competing-reference pin: %q", cast)
				}
			}
		})
	}
}
