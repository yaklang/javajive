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

func TestAdversarialEnumThisDelegateUsesSyntheticParameterIdentity(t *testing.T) {
	for _, scenario := range []string{"valid", "no payload", "other owner", "other method", "ordinary invoke", "other receiver", "wrong descriptor", "wrong arity", "swapped locals", "same names different locals", "effectful argument"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "example.Choice", FunctionName: "<init>"}
			name := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
			ordinal := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			payload := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			name.Id.SetName("renamedName")
			ordinal.Id.SetName("renamedOrdinal")
			payload.Id.SetName("payload")
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Choice"))
			receiver.IsThis = true
			call := &values.FunctionCallExpression{Object: receiver, ClassName: ctx.ClassName, FunctionName: "<init>", IsSpecialInvoke: true,
				Descriptor: "(Ljava/lang/String;II)V", Arguments: []values.JavaValue{name, ordinal, payload}}
			switch scenario {
			case "no payload":
				call.Descriptor = "(Ljava/lang/String;I)V"
				call.Arguments = call.Arguments[:2]
			case "other owner":
				call.ClassName = "example.Other"
			case "other method":
				ctx.FunctionName = "make"
			case "ordinary invoke":
				call.IsSpecialInvoke = false
			case "other receiver":
				receiver.IsThis = false
			case "wrong descriptor":
				call.Descriptor = "(Ljava/lang/Object;II)V"
			case "wrong arity":
				call.Descriptor = "(Ljava/lang/String;I)V"
			case "swapped locals":
				call.Arguments[0], call.Arguments[1] = ordinal, name
			case "same names different locals":
				other := values.NewJavaRef(utils.NewRootVariableId(), nil, name.Type())
				other.Id.SetName("renamedName")
				call.Arguments[0] = other
			case "effectful argument":
				call.Arguments[0] = &values.CastExpression{Value: name, TargetType: name.Type()}
			}
			ft, err := types.ParseMethodDescriptor(call.Descriptor)
			if err != nil {
				t.Fatal(err)
			}
			call.FuncType = ft.FunctionType()
			got, ok := enumThisDelegateSource(&statements.ExpressionStatement{Expression: call}, []values.JavaValue{name, ordinal}, ctx)
			if scenario == "valid" || scenario == "no payload" {
				want := "this(payload)"
				if scenario == "no payload" {
					want = "this()"
				}
				if !ok || got != want {
					t.Fatalf("got %q/%v want %q", got, ok, want)
				}
			} else if ok || strings.Contains(got, "this(") {
				t.Fatal("discarded unproven synthetic arguments")
			}
		})
	}
}
