package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestAssignmentDeclarationUsesExplicitRequestPolicy(t *testing.T) {
	const key = "JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF"
	for _, explicitOff := range []bool{false, true} {
		ambient := map[string]string{}
		if !explicitOff {
			ambient[key] = "1"
		}
		_ = jdecenv.Run(ambient, func() error {
			ctx := &class_context.ClassContext{Env: func(k string) string {
				if !explicitOff && k == key {
					return ""
				}
				// Isolate this declaration proof from the general reference-LUB
				// proof, which can independently choose Executable as well.
				return "1"
			}}
			left := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.reflect.Executable"))
			right := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.reflect.Method"))
			statement := &AssignStatement{LeftValue: left, JavaValue: right, IsFirst: true}
			want := "Executable"
			if explicitOff {
				want = "Method"
			}
			if got := statement.String(ctx); !strings.HasPrefix(got, want+" ") {
				t.Fatalf("explicit off=%v got=%s want=%s", explicitOff, got, want)
			}
			return nil
		})
	}
}
