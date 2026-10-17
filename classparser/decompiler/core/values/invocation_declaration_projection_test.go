package values

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestInvocationDeclarationProjectionPreservesOriginalScope(t *testing.T) {
	for _, variant := range []string{"exact", "overload", "missing", "empty signature", "wrong owner", "partial members", "partial parents", "sibling shadow", "sibling missing", "sibling unknown", "updated declaration", "unknown metadata", "nil context"} {
		t.Run(variant, func(t *testing.T) {
			meta := callbinding.Class{Name: "proof/Declaration", Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{
				{Name: "get", Desc: "()Ljava/lang/Object;", Signature: "()TT;"},
				{Name: "get", Desc: "(I)Ljava/lang/Object;", Signature: "<U:Ljava/lang/Object;>(I)TU;"},
			}}
			queries := 0
			ctx := &class_context.ClassContext{InvocationMetadata: func(string) (callbinding.Class, bool) { queries++; return meta, variant != "unknown metadata" }}
			descriptor, wantClass, wantMethod, declared, known := "()Ljava/lang/Object;", meta.Signature, "()TT;", true, true
			wantQueries := 1
			switch variant {
			case "overload":
				descriptor, wantMethod = "(I)Ljava/lang/Object;", "<U:Ljava/lang/Object;>(I)TU;"
			case "missing":
				descriptor, wantMethod, declared = "()Ljava/lang/String;", "", false
			case "empty signature":
				meta.Methods[0].Signature, wantMethod = "", ""
			case "wrong owner", "partial members", "partial parents", "unknown metadata", "nil context":
				wantClass, wantMethod, declared, known = "", "", false, false
				switch variant {
				case "wrong owner":
					meta.Name = "proof/Unrelated"
				case "partial members":
					meta.MembersComplete = false
				case "partial parents":
					meta.ParentsComplete = false
				case "nil context":
					ctx, wantQueries = nil, 0
				}
			case "sibling shadow", "sibling missing", "sibling unknown":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) {
					methods := map[string]string{class_context.MethodDescKey("get", "()Ljava/lang/Object;"): "()Ljava/lang/String;"}
					if variant == "sibling missing" {
						methods = nil
					}
					return "Ljava/lang/Object;", methods, variant != "sibling unknown"
				}
				if variant != "sibling unknown" {
					wantClass, wantMethod, wantQueries = "Ljava/lang/Object;", "()Ljava/lang/String;", 0
					if variant == "sibling missing" {
						wantMethod, declared = "", false
					}
				}
			case "updated declaration":
				if _, sig, _, _ := invocationDeclarationSignature(ctx, meta.Name, "get", descriptor); sig != "()TT;" {
					t.Fatal("initial declaration")
				}
				meta.Methods[0].Signature, wantMethod, wantQueries = "()Ljava/lang/Number;", "()Ljava/lang/Number;", 2
			}
			cs, sig, exists, complete := invocationDeclarationSignature(ctx, "proof/Declaration", "get", descriptor)
			if cs != wantClass || sig != wantMethod || exists != declared || complete != known || queries != wantQueries {
				t.Fatalf("projection=(%q,%q,%t,%t) queries=%d want=(%q,%q,%t,%t) queries=%d", cs, sig, exists, complete, queries, wantClass, wantMethod, declared, known, wantQueries)
			}
		})
	}
}

// A selected-method query must not construct a table of every other method.
// Test allocation work directly rather than imposing a machine timing limit.
func TestInvocationDeclarationProjectionDoesNotMaterializeUnrelatedMethods(t *testing.T) {
	for _, size := range []int{9, 64, 512} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			meta := callbinding.Class{Name: "proof/MethodInventory", MembersComplete: true, ParentsComplete: true}
			for i := 0; i < size; i++ {
				meta.Methods = append(meta.Methods, callbinding.Method{Name: fmt.Sprintf("entry%d", i), Desc: "()Ljava/lang/Object;", Signature: "()Ljava/lang/Object;"})
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(string) (callbinding.Class, bool) { return meta, true }}
			selected := meta.Methods[len(meta.Methods)-1].Name
			allocations := testing.AllocsPerRun(10, func() {
				_, sig, exists, known := invocationDeclarationSignature(ctx, meta.Name, selected, "()Ljava/lang/Object;")
				if !known || !exists || sig != "()Ljava/lang/Object;" {
					t.Fatal("selected declaration lost")
				}
			})
			if allocations != 0 {
				t.Fatalf("selected query materialized unrelated method evidence: %.0f allocations", allocations)
			}
		})
	}
}
