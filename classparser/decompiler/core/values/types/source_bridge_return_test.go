package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestSourceBridgeReturnRequiresExactParentAndCompleteSubstitution(t *testing.T) {
	const desc = "()Ljava/util/List;"
	for _, scenario := range []string{"proved", "missing proof", "wrong parent", "self target", "raw receiver", "missing argument", "wrong arity", "arity-only lookup", "nonbridge override"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}}
			target := "probe/Parent"
			if scenario == "wrong parent" {
				target = "probe/Other"
			}
			if scenario == "self target" {
				target = "probe/Child"
			}
			ctx.SourceBridgeTarget = func(owner, name, d string) (string, bool) {
				return target, owner == "probe/Child" && name == "items" && d == desc && scenario != "nonbridge override"
			}
			if scenario == "missing proof" {
				ctx.SourceBridgeTarget = nil
			}
			provider := func(owner string) (string, map[string]string, bool) {
				switch owner {
				case "probe/Child":
					return "<V:Ljava/lang/Object;>Lprobe/Parent<Ljava/util/List<TV;>;>;", map[string]string{class_context.MethodDescKey("items", desc): ""}, true
				case "probe/Parent":
					return "<X:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("items", desc): "()Ljava/util/List<TX;>;"}, true
				}
				return "", nil, false
			}
			args := []JavaType{NewJavaClass("T")}
			argc := 0
			if scenario == "raw receiver" {
				args = nil
			}
			if scenario == "missing argument" {
				args = []JavaType{nil}
			}
			if scenario == "wrong arity" {
				argc = 1
			}
			var got JavaType
			if scenario == "arity-only lookup" {
				_, got = ResolveInstantiatedSignature(ctx, provider, "probe.Child", args, "items", 0)
			} else {
				_, got, _ = ResolveInstantiatedSignatureExact(ctx, provider, "probe.Child", args, "items", desc, argc)
			}
			if scenario == "proved" {
				if got == nil || got.String(ctx) != "java.util.List<java.util.List<T>>" && got.String(ctx) != "List<List<T>>" {
					t.Fatalf("return=%v", got)
				}
			} else if got != nil {
				t.Fatalf("unproved return=%s", got.String(ctx))
			}
		})
	}
}
