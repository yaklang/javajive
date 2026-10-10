package types

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestLexicalReceiverInheritedOwnerSegmentsKeepSeparateBindings(t *testing.T) {
	for _, kind := range []string{"original", "deep", "own binder", "own shadow", "raw parent owner", "missing ownership", "static parent", "wrong parent path", "missing parent declaration", "missing outer declaration", "wrong descriptor", "unbound parent variable", "empty override", "budget", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "scope.Caller"}
			desc := "(Ljava/lang/Object;)Z"
			receiver, _ := AsParameterizedType(ParseSignature("Lscope/Owner<Ljava/lang/String;>.Derived;"))
			declarations := map[string]string{"scope/Owner": "<K:Ljava/lang/Object;>Ljava/lang/Object;", "scope/Owner$Derived": "Lscope/Owner<TK;>.Base;", "scope/Owner$Base": "Ljava/lang/Object;"}
			signature := "(TK;)Z"
			override := false
			switch kind {
			case "deep":
				declarations["scope/Owner$Derived"] = "Lscope/Owner<TK;>.Middle;"
				declarations["scope/Owner$Middle"] = "Lscope/Owner<TK;>.Base;"
			case "own binder":
				declarations["scope/Owner$Derived"] = "Lscope/Owner<TK;>.Base<Ljava/lang/Integer;>;"
				declarations["scope/Owner$Base"] = "<V:Ljava/lang/Number;>Ljava/lang/Object;"
				signature = "(TV;)Z"
				desc = "(Ljava/lang/Number;)Z"
			case "own shadow":
				declarations["scope/Owner$Derived"] = "Lscope/Owner<TK;>.Base<Ljava/lang/Integer;>;"
				declarations["scope/Owner$Base"] = "<K:Ljava/lang/Number;>Ljava/lang/Object;"
				signature = "(TK;)Z"
				desc = "(Ljava/lang/Number;)Z"
			case "raw parent owner":
				declarations["scope/Owner$Derived"] = "Lscope/Owner.Base;"
			case "missing parent declaration":
				delete(declarations, "scope/Owner$Base")
			case "missing outer declaration":
				delete(declarations, "scope/Owner")
			case "wrong descriptor":
				desc = "(Ljava/lang/Number;)Z"
			case "unbound parent variable":
				signature = "(TX;)Z"
			case "empty override":
				override = true
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			ctx.SiblingLexicalTypeOwners = func(n string) ([]string, bool) {
				if n == "scope/Owner$Base" && kind == "static parent" {
					return nil, false
				}
				if n == "scope/Owner$Base" && kind == "wrong parent path" {
					return []string{"scope/Other", "scope/Owner$Base"}, true
				}
				_, ok := declarations[n]
				return []string{"scope/Owner", n}, ok && n != "scope/Owner"
			}
			if kind == "missing ownership" {
				ctx.SiblingLexicalTypeOwners = nil
			}
			provider := func(n string) (string, map[string]string, bool) {
				s, ok := declarations[n]
				m := map[string]string{}
				if n == "scope/Owner$Base" {
					m[class_context.MethodDescKey("consume", desc)] = signature
				}
				if n == "scope/Owner$Derived" && override {
					m[class_context.MethodDescKey("consume", desc)] = ""
				}
				return s, m, ok
			}
			got := ResolveLexicalReceiverParamType(ctx, provider, receiver, "consume", desc, 1, 0)
			want := ""
			if kind == "original" || kind == "deep" {
				want = "String"
			}
			if kind == "own binder" || kind == "own shadow" {
				want = "Integer"
			}
			if want == "" {
				if got != nil {
					t.Fatalf("unproved inherited parameter=%s", got.String(ctx))
				}
			} else if got == nil || got.String(ctx) != want {
				t.Fatalf("parameter=%v want=%s", got, want)
			}
		})
	}
}
