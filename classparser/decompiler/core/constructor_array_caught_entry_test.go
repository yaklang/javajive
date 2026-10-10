package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestConstructorArrayCaughtEntryDoesNotHideExplicitLocalUses(t *testing.T) {
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	seed := values.NewCaughtExceptionValue(17, types.NewJavaClass("java.lang.RuntimeException"))
	opaque := values.NewCustomValue(func(*class_context.ClassContext) string { return "failureFrom(hiddenArray)" }, seed.Type)
	opaque.Flag = "exception"
	opaque.OriginPC = 17
	opaque.HasOriginPC = true
	alias := *temp // A canonical use copy retains the original local identity.
	alias.CustomValue = seed
	wrapper := values.NewJavaRef(utils.NewRootVariableId(), nil, seed.Type())
	wrapper.StackVar = temp
	for _, tc := range []struct {
		name  string
		value values.JavaValue
		want  bool
	}{
		{"implicit handler word", seed, false},
		{"opaque handler annotation", opaque, true},
		{"same local identity", &alias, true},
		{"explicit stack alias", wrapper, true},
		{"nested explicit use", &values.FunctionCallExpression{FunctionName: "failureFrom", Arguments: []values.JavaValue{seed, temp}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := valueMentionsLocal(tc.value, temp); got != tc.want {
				t.Fatalf("array dependency=%t want=%t", got, tc.want)
			}
		})
	}
}
