package core

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestReferenceDeclarationUsesAllDefinitionsInsteadOfLossyJoin(t *testing.T) {
	for _, change := range []string{"complete", "unknown definition", "incompatible definition", "missing store", "incomplete value", "null alternative", "anchored recurrence", "unanchored recurrence", "inaccessible consumer", "unnamed consumer", "typed nil value", "cyclic value", "budget", "cancelled", "invariant generic use"} {
		t.Run(change, func(t *testing.T) {
			edges := map[string][]string{"model.A": {"model/First", "model/Second"}, "model.B": {"model/First", "model/Second"}, "model.C": {"model/Second"}, "model.First": {"java/lang/Object"}, "model.Second": {"java/lang/Object"}}
			ctx := &class_context.ClassContext{SiblingSuperTypes: func(n string) ([]string, bool) { v, ok := edges[strings.ReplaceAll(n, "/", ".")]; return v, ok }}
			d := NewDecompiler(nil, nil)
			d.FunctionContext = ctx
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			var stores []*OpCode
			for _, name := range []string{"model.A", "model.B", "model.C"} {
				store := &OpCode{stackConsumed: []values.JavaValue{values.NewJavaLiteral("typed model", types.NewJavaClass(name))}}
				stores = append(stores, store)
				d.opcodeIdToRef[store] = [][2]any{{ref, false}}
			}
			switch change {
			case "typed nil value":
				var v *values.JavaLiteral
				stores[2].stackConsumed[0] = v
			case "cyclic value":
				v := &values.TernaryExpression{Condition: values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), TrueValue: stores[0].stackConsumed[0]}
				v.FalseValue = v
				stores[2].stackConsumed[0] = v
			case "budget":
				d.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(c, workbudget.Limits{})
			case "unknown definition":
				delete(edges, "model.C")
			case "incompatible definition":
				edges["model.C"] = []string{"model/First"}
			case "missing store":
				stores[2].stackConsumed = nil
			case "incomplete value":
				stores[2].stackConsumed[0] = nil
			case "null alternative":
				stores[2].stackConsumed[0] = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "anchored recurrence":
				stores[2].stackConsumed[0] = ref
			case "unanchored recurrence":
				for _, store := range stores {
					store.stackConsumed[0] = ref
				}
			case "inaccessible consumer":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
			case "unnamed consumer":
				ctx.SourceClassDenotable = func(string) (bool, bool) { return false, true }
			}
			candidate := types.NewJavaClass("model.Second")
			if change == "invariant generic use" {
				candidate = types.NewParameterizedType("model.Second", []types.JavaType{types.NewJavaClass("java.lang.String")})
			}
			got := d.constrainWebDeclaration(ref.Type(), stores, map[*values.JavaRef][]types.JavaType{ref: {candidate}})
			want := "java.lang.Object"
			if change == "complete" || change == "null alternative" || change == "anchored recurrence" {
				want = "model.Second"
			}
			if name, _ := types.RawClassFQN(got); name != want {
				t.Fatalf("declaration %s want %s", name, want)
			}
			if name, _ := types.RawClassFQN(ref.Type()); name != "java.lang.Object" {
				t.Fatal("query mutated original declaration")
			}
		})
	}
}
