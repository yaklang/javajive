package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestSourceBranchLayoutRendersEachOperandOnce(t *testing.T) {
	for _, swap := range []bool{false, true} {
		counts := map[string]int{}
		value := func(name string) values.JavaValue {
			return values.NewCustomValue(func(*class_context.ClassContext) string { counts[name]++; return name }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
		}
		ctx := &class_context.ClassContext{SourceBranchSwap: func(left, right string) bool {
			if !strings.Contains(left, "later()") || !strings.Contains(right, "earlier()") {
				t.Fatal("wrong arm proof arguments")
			}
			return swap
		}}
		stmt := NewIfStatement(value("predicate()"), []Statement{&ExpressionStatement{Expression: value("later()")}}, []Statement{&ExpressionStatement{Expression: value("earlier()")}})
		source := stmt.String(ctx)
		for _, name := range []string{"predicate()", "later()", "earlier()"} {
			if counts[name] != 1 {
				t.Fatalf("%s rendered %d times", name, counts[name])
			}
		}
		if swap {
			if !strings.Contains(source, "if (!(predicate()))") || strings.Index(source, "earlier()") > strings.Index(source, "later()") {
				t.Fatal(source)
			}
		} else if strings.Index(source, "later()") > strings.Index(source, "earlier()") {
			t.Fatal(source)
		}
	}
}
