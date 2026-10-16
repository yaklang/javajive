package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeCheckedHandlerContainersPreserveThrowProof(t *testing.T) {
	for _, shape := range []string{"while", "do", "for", "switch", "nested"} {
		variants := []string{"normal", "permitted throw", "unpermitted throw", "wrong throw PC", "missing throw PC", "opaque", "nil node", "nil condition", "nil body item", "catchall", "cycle", "budget"}
		if shape == "for" {
			variants = append(variants, "opaque init", "opaque update", "nil init", "nil update")
		}
		if shape == "switch" || shape == "nested" {
			variants = append(variants, "nil case")
		}
		for _, variant := range variants {
			t.Run(shape+"/"+variant, func(t *testing.T) {
				body := []statements.Statement{statements.NewSourceTransferStatement("break", "")}
				permitted := map[int]bool{31: true}
				want := true
				opaque := statements.NewCustomStatement(func(*class_context.ClassContext) string { panic("must not render") }, nil)
				switch variant {
				case "permitted throw", "unpermitted throw", "wrong throw PC", "missing throw PC":
					thrown := statements.NewThrowStatement(values.JavaNull)
					thrown.HasOriginPC = true
					thrown.OriginPC = 31
					body = []statements.Statement{thrown}
					if variant == "unpermitted throw" {
						permitted = nil
						want = false
					}
					if variant == "wrong throw PC" {
						thrown.OriginPC++
						want = false
					}
					if variant == "missing throw PC" {
						thrown.HasOriginPC = false
						want = false
					}
				case "opaque":
					body = []statements.Statement{opaque}
					want = false
				case "nil body item":
					body = []statements.Statement{nil}
					want = false
				case "catchall", "nil node", "nil condition", "cycle", "budget":
					want = false
				}
				var node statements.Statement
				switch shape {
				case "while":
					w := &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: body}
					node = w
					if variant == "nil condition" {
						w.ConditionValue = nil
					}
					if variant == "nil node" {
						node = (*statements.WhileStatement)(nil)
					}
					if variant == "cycle" {
						w.Body = []statements.Statement{w}
					}
				case "do":
					w := &statements.DoWhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: body}
					node = w
					if variant == "nil condition" {
						w.ConditionValue = nil
					}
					if variant == "nil node" {
						node = (*statements.DoWhileStatement)(nil)
					}
					if variant == "cycle" {
						w.Body = []statements.Statement{w}
					}
				case "for":
					w := &statements.ForStatement{InitVar: &statements.AssignStatement{}, Condition: &statements.ConditionStatement{}, EndExp: &statements.ExpressionStatement{}, SubStatements: body}
					node = w
					switch variant {
					case "opaque init":
						w.InitVar = opaque
						want = false
					case "opaque update":
						w.EndExp = opaque
						want = false
					case "nil init":
						w.InitVar = nil
						want = false
					case "nil update":
						w.EndExp = nil
						want = false
					case "nil condition":
						w.Condition = nil
					case "nil node":
						node = (*statements.ForStatement)(nil)
					case "cycle":
						w.SubStatements = []statements.Statement{w}
					}
				case "switch", "nested":
					w := &statements.SwitchStatement{Value: values.JavaNull, Cases: []*statements.CaseItem{{Body: body}}}
					node = w
					switch variant {
					case "nil case":
						w.Cases = append(w.Cases, nil)
						want = false
					case "nil condition":
						w.Value = nil
					case "nil node":
						node = (*statements.SwitchStatement)(nil)
					case "cycle":
						w.Cases[0].Body = []statements.Statement{w}
					}
					if shape == "nested" {
						node = &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: []statements.Statement{node}}
					}
				}
				if variant == "budget" {
					for i := 0; i < 1025; i++ {
						node = &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: []statements.Statement{node}}
					}
				}
				h := &statements.TryCatchStatement{Handlers: []statements.CatchHandler{{EntryPC: 20, CatchAll: variant == "catchall"}}, CatchBodies: [][]statements.Statement{{node}}}
				if got := typedAbsorbingHandlers([]statements.Statement{h}, permitted)[20]; got != want {
					t.Fatalf("contains incoming checked exception=%v want=%v", got, want)
				}
			})
		}
	}
}
