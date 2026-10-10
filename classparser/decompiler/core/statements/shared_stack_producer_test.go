package statements

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestOriginalSharedStackProducerIsNotRetypedByBooleanStore(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, word := range []int{0, 1, 2, 3, -1, -2, -2147483648, 2147483647} {
		t.Run(fmt.Sprint(word), func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			boolean := types.NewJavaPrimer(types.JavaBoolean)
			value := values.NewJavaLiteral(word, integer)
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), value, integer)
			ref.Id.SetName("shared")
			ref.MarkOriginalStackMaterialization(12, 89, value)
			field := values.NewRefMember(values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner")), "flag", boolean)
			store := NewAssignStatement(field, ref, false)
			if ref.Type().String(ctx) != "int" || value.String(ctx) != fmt.Sprint(word) {
				t.Fatal("consumer changed shared producer")
			}
			if text := store.String(ctx); !strings.Contains(text, "((shared) & 1) != 0") {
				t.Fatalf("missing consumer-local low bit:%s", text)
			}
			if text := NewAssignStatement(ref, value, true).String(ctx); text != "int shared = "+fmt.Sprint(word) {
				t.Fatalf("DUP definition changed:%s", text)
			}
			if value.String(ctx) != fmt.Sprint(word) || ref.Type().String(ctx) != "int" {
				t.Fatal("rendering changed shared word")
			}
		})
	}
}
