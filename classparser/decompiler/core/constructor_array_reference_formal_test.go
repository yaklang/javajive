package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestDelegationArrayFormalSeparatesRuntimeAllocationFromConsumerErasure(t *testing.T) {
	for _, row := range []struct {
		name, allocated, source, physical string
		want                              bool
	}{
		{"exact", "[Ljava/lang/String;", "[Ljava/lang/String;", "[Ljava/lang/String;", true},
		{"covariant", "[Ljava/lang/String;", "[Ljava/lang/Object;", "[Ljava/lang/Object;", true},
		{"array as Object", "[Ljava/lang/String;", "Ljava/lang/Object;", "Ljava/lang/Object;", true},
		{"primitive array as Object", "[I", "Ljava/lang/Object;", "Ljava/lang/Object;", true},
		{"nested primitive array", "[[I", "[Ljava/lang/Object;", "[Ljava/lang/Object;", true},
		{"interface edge", "[Lproof/FormalLeaf;", "[Lproof/FormalContract;", "[Lproof/FormalContract;", true},
		{"narrowing", "[Ljava/lang/Object;", "[Ljava/lang/String;", "[Ljava/lang/String;", false},
		{"primitive element widening", "[I", "[J", "[J", false},
		{"scalar primitive elements", "[I", "[Ljava/lang/Object;", "[Ljava/lang/Object;", false},
		{"wrong rank", "[[I", "[[Ljava/lang/Object;", "[[Ljava/lang/Object;", false},
		{"changed source formal", "[Ljava/lang/String;", "[Ljava/lang/Object;", "[Ljava/lang/String;", false},
		{"changed physical formal", "[Ljava/lang/String;", "[Ljava/lang/String;", "[Ljava/lang/Object;", false},
		{"scalar allocation", "Ljava/lang/String;", "Ljava/lang/Object;", "Ljava/lang/Object;", false},
		{"unknown edge", "[Lproof/Unproved;", "[Lproof/FormalContract;", "[Lproof/FormalContract;", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			parse := func(desc string) types.JavaType {
				v, err := types.ParseDescriptor(desc)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			ctx := &class_context.ClassContext{SiblingSuperTypes: func(name string) ([]string, bool) {
				return []string{"proof/FormalContract"}, name == "proof/FormalLeaf"
			}}
			d := &Decompiler{FunctionContext: ctx}
			allocated, source, physical := parse(row.allocated), parse(row.source), parse(row.physical)
			before := allocated.String(&class_context.ClassContext{})
			if got := d.delegationArrayFormalAccepts(allocated, source, physical); got != row.want {
				t.Fatalf("original allocation/formal widening=%v", got)
			}
			if allocated.String(&class_context.ClassContext{}) != before {
				t.Fatal("formal assignment changed the runtime allocation type")
			}
		})
	}
}

func TestDelegationArrayReferenceLookupRemainsBoundedAndDeclarationAware(t *testing.T) {
	for _, variant := range []string{"original", "wrong declaration", "missing hierarchy", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			array := types.NewJavaArrayType(types.NewJavaClass("proof/FormalLeaf"))
			formal := types.NewJavaArrayType(types.NewJavaClass("proof/FormalContract"))
			ctx := &class_context.ClassContext{SiblingSuperTypes: func(name string) ([]string, bool) {
				return []string{"proof/FormalContract"}, name == "proof/FormalLeaf"
			}}
			if variant == "wrong declaration" {
				ctx.InvocationMetadata = func(string) (callbinding.Class, bool) {
					return callbinding.Class{Name: "ForeignOwner", Parents: []string{"proof/FormalContract"}, ParentsComplete: true}, true
				}
			}
			if variant == "missing hierarchy" {
				ctx.SiblingSuperTypes = nil
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(c, workbudget.Limits{})
			}
			d := &Decompiler{FunctionContext: ctx, Work: work}
			if got := d.delegationArrayFormalAccepts(array, formal, formal); got != (variant == "original") {
				t.Fatalf("original bounded ancestry=%v", got)
			}
		})
	}
}
