package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestStaticInitializerPrefixRequiresDeclarationAndStoreOrder(t *testing.T) {
	for _, kind := range []string{"ordered", "inversion", "prior branch", "foreign owner", "descriptor mismatch", "missing origin", "stale store", "repeated field", "reversed physical PC", "protected store", "invalid handler", "duplicate physical receipt", "opaque marker", "cancelled", "scan budget", "allocation budget"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			fields := []*values.JavaClassMember{{Name: "sample.Owner", Member: "first", Description: "Ljava/lang/Object;", JavaType: typ}, {Name: "sample.Owner", Member: "second", Description: "Ljava/lang/Object;", JavaType: typ}}
			a := statements.NewAssignStatement(fields[0], values.JavaNull, false)
			a.OriginPC = 7
			a.HasOriginPC = true
			b := statements.NewAssignStatement(fields[1], values.JavaNull, false)
			b.OriginPC = 16
			b.HasOriginPC = true
			body := []statements.Statement{a, b}
			declarations := map[staticInitializerFieldKey]int{{"first", "Ljava/lang/Object;"}: 0, {"second", "Ljava/lang/Object;"}: 1}
			writes := []interfaceFieldWrite{{name: "first", descriptor: "Ljava/lang/Object;", pc: 7}, {name: "second", descriptor: "Ljava/lang/Object;", pc: 16}}
			var handlers []*ExceptionTableEntry
			var work *workbudget.Budget
			want := 2
			switch kind {
			case "inversion":
				declarations[staticInitializerFieldKey{"first", "Ljava/lang/Object;"}] = 2
				want = 1
			case "prior branch":
				body = append([]statements.Statement{&statements.IfStatement{}}, body...)
				want = 0
			case "foreign owner":
				fields[1].Name = "another.Owner"
				want = 1
			case "descriptor mismatch":
				fields[1].Description = "Ljava/lang/String;"
				want = 1
			case "missing origin":
				b.HasOriginPC = false
				want = 1
			case "stale store":
				writes[1].name = "different"
				want = 1
			case "repeated field":
				fields[1].Member = "first"
				writes[1].name = "first"
				want = 1
			case "reversed physical PC":
				b.OriginPC = 4
				writes[1].pc = 4
				want = 1
			case "protected store":
				handlers = []*ExceptionTableEntry{{StartPc: 10, EndPc: 20}}
				want = 1
			case "invalid handler":
				handlers = []*ExceptionTableEntry{{StartPc: 20, EndPc: 10}}
				want = 0
			case "duplicate physical receipt":
				writes = append(writes, writes[0])
				want = 0
			case "opaque marker":
				body = append([]statements.Statement{&statements.MiddleStatement{Flag: "start", Data: "unresolved"}}, body...)
				want = 0
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				want = 0
			case "scan budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				want = 0
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				want = 0
			}
			got := orderedStaticStorePrefix(body, "sample/Owner", declarations, writes, handlers, work)
			if len(got) != want || got[a] != (want > 0) || got[b] != (want > 1) {
				t.Fatalf("prefix=%d want %d", len(got), want)
			}
			if a.JavaValue != values.JavaNull || b.JavaValue != values.JavaNull || a.LeftValue != fields[0] || b.LeftValue != fields[1] {
				t.Fatal("planner changed original assignments")
			}
		})
	}
}
