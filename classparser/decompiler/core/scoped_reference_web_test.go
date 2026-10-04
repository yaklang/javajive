package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestScopedReferenceWebRequiresCompleteOwnedDefinitions(t *testing.T) {
	for _, name := range []string{"proved", "null store", "entry", "shared owner", "missing value", "missing opcode", "narrowing", "parameter", "method shadow", "no seed"} {
		t.Run(name, func(t *testing.T) {
			bound := types.NewJavaClass("java.lang.Number")
			ctx := &class_context.ClassContext{ClassSig: "<T:Ljava/lang/Number;>Ljava/lang/Object;", ClassTypeParams: []string{"T"}, TypeParams: []string{"T"}, FieldSignatures: map[string]string{"seed": "TT;"}}
			self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
			self.IsThis = true
			field := values.NewRefMember(self, "seed", bound)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, bound)
			a, b := op(OP_ASTORE_1, 1), op(OP_ASTORE_1, 2)
			a.stackConsumed = []values.JavaValue{field}
			b.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, bound)}
			webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{FunctionContext: ctx, cachedSlotWebs: webs, opCodes: []*OpCode{a, b}, opcodeIdToRef: map[*OpCode][][2]any{a: {{ref, true}}, b: {{ref, false}}}}
			switch name {
			case "null store":
				b.stackConsumed = []values.JavaValue{values.JavaNull}
			case "entry":
				webs.entryWeb[1] = 1
			case "shared owner":
				webs.webOf[b] = 2
			case "missing value":
				b.stackConsumed = nil
			case "missing opcode":
				d.opCodes = d.opCodes[:1]
			case "narrowing":
				b.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}
			case "parameter":
				ref.IsParam = true
			case "method shadow":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;>()V"
			case "no seed":
				a.stackConsumed = b.stackConsumed
			}
			d.restoreScopedReferenceWebs()
			want := name == "proved" || name == "null store"
			if (ref.WebDeclType != nil) != want {
				t.Fatalf("view=%v want=%v", ref.WebDeclType, want)
			}
			if ref.Type().String(ctx) != "Number" {
				t.Fatal("computational bound changed")
			}
		})
	}
}
