package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousArgumentSourceTypeKeepsOriginalThisBinder(t *testing.T) {
	for _, variant := range []string{"original", "method shadow", "non-this raw parameter", "foreign this", "static", "nongeneric", "malformed signature", "already parameterized", "nil context", "nil value", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "probe.Owner", ClassSig: "<V:Ljava/lang/Object;>Ljava/lang/Object;", ClassTypeParams: []string{"V"}}
			operand := values.NewJavaRef(nil, nil, types.NewJavaClass("probe.Owner"))
			operand.IsThis = true
			var work *workbudget.Budget
			switch variant {
			case "method shadow":
				ctx.CurrentMethodSig = "<V:Ljava/lang/Number;>(TV;)V"
			case "non-this raw parameter":
				operand.IsThis = false
			case "foreign this":
				operand = values.NewJavaRef(nil, nil, types.NewJavaClass("probe.Other"))
				operand.IsThis = true
			case "static":
				ctx.IsStatic = true
			case "nongeneric":
				ctx.ClassSig = "Ljava/lang/Object;"
			case "malformed signature":
				ctx.ClassSig = "<V:Ljava/lang/Object;>"
			case "already parameterized":
				operand = values.NewJavaRef(nil, nil, types.NewParameterizedType("probe.Owner", []types.JavaType{types.NewJavaClass("java.lang.String")}))
				operand.IsThis = true
			case "nil context":
				ctx = nil
			case "nil value":
				operand = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(c, workbudget.Limits{})
			}
			typ := nativeAnonymousArgumentSourceType(operand, ctx, work)
			if variant == "nil value" || variant == "budget" || variant == "canceled" {
				if typ != nil {
					t.Fatal("unproved operand admitted")
				}
				return
			}
			pt, parameterized := types.AsParameterizedType(typ)
			expected := variant == "original" || variant == "method shadow" || variant == "already parameterized"
			if parameterized != expected {
				t.Fatalf("parameterized=%v", parameterized)
			}
			if parameterized {
				want := "V"
				if variant == "already parameterized" {
					want = "java.lang.String"
				}
				arg, _ := types.RawClassFQN(pt.TypeArgs[0])
				if arg != want {
					t.Fatalf("argument=%s", arg)
				}
			}
			if operand.Type() != nil {
				if _, changed := types.AsParameterizedType(operand.Type()); changed != (variant == "already parameterized") {
					t.Fatal("original IR type mutated")
				}
			}
		})
	}
}
