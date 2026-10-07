package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"
)

func TestLexicalReceiverParametersRequireOriginalOwnerSubstitution(t *testing.T) {
	for _, tc := range []struct{ name, receiver, leaf, method, descriptor, want string }{
		{"outer binder", "Lscope/Owner<Ljava/lang/String;>.Gate;", "Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Object;)Z", "String"},
		{"own binder", "Lscope/Owner<Ljava/lang/String;>.Gate<Ljava/lang/Integer;>;", "<V:Ljava/lang/Number;>Ljava/lang/Object;", "(TV;)Z", "(Ljava/lang/Number;)Z", "Integer"},
		{"shadow binder", "Lscope/Owner<Ljava/lang/String;>.Gate<Ljava/lang/Integer;>;", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Number;)Z", "Integer"},
		{"raw owner", "Lscope/Owner.Gate<Ljava/lang/Integer;>;", "<V:Ljava/lang/Number;>Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Object;)Z", ""},
		{"raw leaf", "Lscope/Owner<Ljava/lang/String;>.Gate;", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Number;)Z", ""},
		{"wildcard owner", "Lscope/Owner<*>.Gate;", "Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Object;)Z", ""},
		{"unknown binder", "Lscope/Owner<Ljava/lang/String;>.Gate;", "Ljava/lang/Object;", "(TX;)Z", "(Ljava/lang/Object;)Z", ""},
		{"descriptor mismatch", "Lscope/Owner<Ljava/lang/String;>.Gate;", "Ljava/lang/Object;", "(TK;)Z", "(Ljava/lang/Number;)Z", ""},
		{"method binder", "Lscope/Owner<Ljava/lang/String;>.Gate;", "Ljava/lang/Object;", "<K:Ljava/lang/Number;>(TK;)Z", "(Ljava/lang/Number;)Z", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pt, ok := AsParameterizedType(ParseSignature(tc.receiver))
			if !ok {
				t.Fatal("authored signature")
			}
			ctx := &class_context.ClassContext{ClassName: "scope.Caller", TypeParams: []string{"K", "V", "X"}}
			ctx.SiblingLexicalTypeOwners = func(n string) ([]string, bool) {
				return []string{"scope/Owner", "scope/Owner$Gate"}, n == "scope/Owner$Gate"
			}
			provider := func(n string) (string, map[string]string, bool) {
				switch n {
				case "scope/Owner":
					return "<K:Ljava/lang/Object;>Ljava/lang/Object;", nil, true
				case "scope/Owner$Gate":
					return tc.leaf, map[string]string{class_context.MethodDescKey("consume", tc.descriptor): tc.method}, true
				}
				return "", nil, false
			}
			got := ResolveLexicalReceiverParamType(ctx, provider, pt, "consume", tc.descriptor, 1, 0)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("invented binding %s", got.String(ctx))
				}
			} else if got == nil || got.String(ctx) != tc.want {
				t.Fatalf("binding=%v want=%s", got, tc.want)
			}
		})
	}
}

func TestLexicalReceiverSubstitutionRejectsIncompletePaths(t *testing.T) {
	for _, variant := range []string{"original", "no ownership", "unknown ownership", "wrong owner", "missing segment", "wrong raw identity", "missing declaration", "malformed class", "unbound class edge", "nil owner arg", "extra owner arg", "wrong descriptor", "wrong arity", "invalid index", "nil receiver"} {
		t.Run(variant, func(t *testing.T) {
			pt, _ := AsParameterizedType(ParseSignature("Lscope/Owner<Ljava/lang/String;>.Layer<Ljava/lang/Long;>.Gate<Ljava/lang/Integer;>;"))
			ctx := &class_context.ClassContext{ClassName: "scope.Caller", TypeParams: []string{"K", "V", "W"}}
			path := []string{"scope/Owner", "scope/Owner$Layer", "scope/Owner$Layer$Gate"}
			known := true
			ctx.SiblingLexicalTypeOwners = func(string) ([]string, bool) { return path, known }
			sigs := map[string]string{path[0]: "<K:Ljava/lang/Object;>Ljava/lang/Object;", path[1]: "<V:Ljava/lang/Object;>Ljava/lang/Object;", path[2]: "<W:Ljava/lang/Object;>Ljava/lang/Object;"}
			desc := "(Ljava/lang/Object;Ljava/lang/Object;Ljava/lang/Object;)Z"
			argc, index := 3, 0
			switch variant {
			case "no ownership":
				ctx.SiblingLexicalTypeOwners = nil
			case "unknown ownership":
				known = false
			case "wrong owner":
				path[0] = "scope/Foreign"
			case "missing segment":
				path = path[1:]
			case "wrong raw identity":
				pt.RawClassName = "scope/Foreign"
			case "missing declaration":
				delete(sigs, path[0])
			case "malformed class":
				sigs[path[0]] = "<K:broken"
			case "unbound class edge":
				sigs[path[0]] = "<K:Ljava/lang/Object;>Ljava/util/List<TUnknown;>;"
			case "nil owner arg":
				pt.OwnerSegments[0].TypeArgs[0] = nil
			case "extra owner arg":
				pt.OwnerSegments[0].TypeArgs = append(pt.OwnerSegments[0].TypeArgs, NewJavaClass("java.lang.Integer"))
			case "wrong descriptor":
				desc = "(Ljava/lang/Number;Ljava/lang/Object;Ljava/lang/Object;)Z"
			case "wrong arity":
				argc = 2
			case "invalid index":
				index = -1
			case "nil receiver":
				pt = nil
			}
			provider := func(n string) (string, map[string]string, bool) {
				sig, ok := sigs[n]
				m := map[string]string{}
				if n == "scope/Owner$Layer$Gate" {
					m[class_context.MethodDescKey("consume", desc)] = "(TK;TV;TW;)Z"
				}
				return sig, m, ok
			}
			indices := []int{index}
			if variant == "original" {
				indices = []int{0, 1, 2}
			}
			for _, i := range indices {
				got := ResolveLexicalReceiverParamType(ctx, provider, pt, "consume", desc, argc, i)
				if variant != "original" {
					if got != nil {
						t.Fatalf("accepted %v", got)
					}
					break
				}
				want := []string{"String", "Long", "Integer"}[i]
				if got == nil || got.String(ctx) != want {
					t.Fatalf("param%d=%v want %s", i, got, want)
				}
			}
		})
	}
}
