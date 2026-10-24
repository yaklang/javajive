package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Operand order is a semantic boundary, and a failed left proof cannot borrow
// a valid flag from the right. Cycles, repeated DAGs and budgets remain bounded.
func TestNativeAssertionOrderedGuardKeepsBoundariesAndBounds(t *testing.T) {
	for _, variant := range []string{"prefix", "left association", "right association", "first", "suffix OR", "guard OR", "eager AND", "positive flag", "foreign flag", "missing origin", "nil prefix", "cycle prefix", "DAG cap", "depth", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			b := types.NewJavaPrimer(types.JavaBoolean)
			field := &values.JavaClassMember{Name: "p.Scope", Member: nativeAssertionField, Description: "Z", JavaType: b, OriginPC: 7, HasOriginPC: true}
			flag := values.NewUnaryExpression(field, values.Not, b)
			prefix := values.JavaValue(values.NewJavaLiteral(true, b))
			suffix := values.JavaValue(values.NewJavaLiteral(false, b))
			join := func(a, c values.JavaValue) values.JavaValue {
				return values.NewBinaryExpression(a, c, values.LOGICAL_AND, b)
			}
			v := join(prefix, join(flag, suffix))
			depth := 64
			var work *workbudget.Budget
			want := false
			switch variant {
			case "prefix", "right association":
				want = true
			case "left association":
				v = join(join(prefix, flag), suffix)
				want = true
			case "first":
				v = join(flag, suffix)
				want = true
			case "suffix OR":
				suffix = values.NewBinaryExpression(suffix, prefix, values.LOGICAL_OR, b)
				v = join(prefix, join(flag, suffix))
				want = true
			case "guard OR":
				v = values.NewBinaryExpression(prefix, join(flag, suffix), values.LOGICAL_OR, b)
			case "eager AND":
				v = values.NewBinaryExpression(prefix, join(flag, suffix), values.AND, b)
			case "positive flag":
				v = join(prefix, join(field, suffix))
			case "foreign flag":
				field.Name = "p.Other"
			case "missing origin":
				field.HasOriginPC = false
			case "nil prefix":
				v = &values.JavaExpression{Op: values.LOGICAL_AND, Values: []values.JavaValue{nil, join(flag, suffix)}, Typ: b}
			case "cycle prefix":
				x := values.NewBinaryExpression(prefix, prefix, values.LOGICAL_AND, b)
				x.Values[0] = x
				v = join(x, join(flag, suffix))
			case "DAG cap":
				x := prefix
				for i := 0; i < 20; i++ {
					x = join(x, x)
				}
				v = join(x, join(flag, suffix))
			case "depth":
				depth = 1
			case "budget":
				work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, p, f, ok := nativeAssertionOrderedGuard(v, "p.Scope", work, depth)
			if ok != want {
				t.Fatalf("partition=%t want=%t", ok, want)
			}
			if want {
				if got != field || f == nil {
					t.Fatal("original flag/failure identity")
				}
				if variant == "first" {
					if p != nil {
						t.Fatal("invented prefix")
					}
				} else if p != prefix {
					t.Fatal("prefix identity changed")
				}
			}
		})
	}
}
