package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestReferenceDeclarationRequiresDenotableDefinitionAndEveryUse(t *testing.T) {
	for _, change := range []string{"proved", "renamed", "ordinary named class", "unknown source role", "missing hierarchy", "incompatible consumer", "non-denotable consumer", "inaccessible consumer", "narrowing consumer", "no consumer"} {
		t.Run(change, func(t *testing.T) {
			owner, api := "example.Owner$1", "example.Api"
			if change == "renamed" {
				owner, api = "elsewhere.UnnamedImplementation", "elsewhere.Protocol"
			}
			ctx := &class_context.ClassContext{
				SourceClassDenotable: func(n string) (bool, bool) { return strings.ReplaceAll(n, "/", ".") != owner, true },
				SiblingSuperTypes: func(n string) ([]string, bool) {
					n = strings.ReplaceAll(n, "/", ".")
					switch n {
					case owner:
						return []string{"java/lang/Object", api}, true
					case api, "example.Other":
						return []string{"java/lang/Object"}, true
					case "example.Narrow":
						return []string{owner}, true
					}
					return nil, false
				},
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(owner))
			store := op(OP_ASTORE_1, 1)
			d := &Decompiler{FunctionContext: ctx, opcodeIdToRef: map[*OpCode][][2]any{store: {{ref, true}}}}
			uses := map[*values.JavaRef][]types.JavaType{ref: {types.NewJavaClass(api)}}
			switch change {
			case "ordinary named class":
				ctx.SourceClassDenotable = nil
			case "unknown source role":
				ctx.SourceClassDenotable = func(string) (bool, bool) { return false, false }
			case "missing hierarchy":
				ctx.SiblingSuperTypes = nil
			case "incompatible consumer":
				uses[ref] = append(uses[ref], types.NewJavaClass("example.Other"))
			case "non-denotable consumer":
				ctx.SourceClassDenotable = func(string) (bool, bool) { return false, true }
			case "inaccessible consumer":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
			case "narrowing consumer":
				uses[ref] = []types.JavaType{types.NewJavaClass("example.Narrow")}
			case "no consumer":
				uses[ref] = nil
			}
			got := d.constrainWebDeclaration(ref.Type(), []*OpCode{store}, uses)
			want := owner
			if change == "proved" || change == "renamed" {
				want = api
			}
			if name, _ := types.RawClassFQN(got); name != want {
				t.Fatalf("declaration=%q want=%q", name, want)
			}
			if name, _ := types.RawClassFQN(ref.Type()); name != owner {
				t.Fatal("constraint query changed the original definition")
			}
		})
	}
}

// Exhaust all 1,024 DAGs on five ordered reference types and all 15 nonempty
// consumer sets. The independent transitive closure checks that any admitted
// source declaration accepts the definition and satisfies every consumer.
// This finite domain models declaration constraints, not arbitrary JVM code.
func TestDenotableReferenceDeclarationsEnumerateFiniteHierarchyConstraints(t *testing.T) {
	names := []string{"model.Unnamed", "model.A", "model.B", "model.C", "model.D"}
	index := map[string]int{}
	for i, name := range names {
		index[name] = i
	}
	var edges [][2]int
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			edges = append(edges, [2]int{i, j})
		}
	}
	cases := 0
	for graph := 0; graph < 1<<len(edges); graph++ {
		var closure [5][5]bool
		direct := make([][]string, len(names))
		for i := range names {
			closure[i][i] = true
			direct[i] = []string{"java/lang/Object"}
		}
		for bit, edge := range edges {
			if graph&(1<<bit) != 0 {
				closure[edge[0]][edge[1]] = true
				direct[edge[0]] = append(direct[edge[0]], strings.ReplaceAll(names[edge[1]], ".", "/"))
			}
		}
		for k := range names {
			for i := range names {
				for j := range names {
					closure[i][j] = closure[i][j] || closure[i][k] && closure[k][j]
				}
			}
		}
		ctx := &class_context.ClassContext{
			SourceClassDenotable: func(n string) (bool, bool) { return strings.ReplaceAll(n, "/", ".") != names[0], true },
			SiblingSuperTypes: func(n string) ([]string, bool) {
				i, known := index[strings.ReplaceAll(n, "/", ".")]
				if !known {
					return nil, false
				}
				return direct[i], true
			},
		}
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(names[0]))
		store := op(OP_ASTORE_1, 1)
		d := &Decompiler{FunctionContext: ctx, opcodeIdToRef: map[*OpCode][][2]any{store: {{ref, true}}}}
		for consumers := 1; consumers < 16; consumers++ {
			uses := map[*values.JavaRef][]types.JavaType{}
			var bounds []int
			for bit := 0; bit < 4; bit++ {
				if consumers&(1<<bit) != 0 {
					bounds = append(bounds, bit+1)
					uses[ref] = append(uses[ref], types.NewJavaClass(names[bit+1]))
				}
			}
			feasible := false
			for _, candidate := range bounds {
				valid := closure[0][candidate]
				for _, bound := range bounds {
					valid = valid && closure[candidate][bound]
				}
				feasible = feasible || valid
			}
			got := d.constrainWebDeclaration(ref.Type(), []*OpCode{store}, uses)
			name, known := types.RawClassFQN(got)
			selected, exists := index[name]
			if !known || !exists || feasible != (selected != 0) {
				t.Fatal(fmt.Sprintf("graph=%03x consumers=%x source=%s feasible=%t", graph, consumers, name, feasible))
			}
			if selected != 0 {
				if !closure[0][selected] {
					t.Fatal("source declaration narrowed the original definition")
				}
				for _, bound := range bounds {
					if !closure[selected][bound] {
						t.Fatal("source declaration violated a consumer")
					}
				}
			}
			cases++
		}
	}
	if cases != 15360 {
		t.Fatal(cases)
	}
}
