package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestConstructorArrayPrefixRequiresOneCommonDefinition(t *testing.T) {
	for _, scenario := range []string{"diamond", "different definition", "cycle", "effect", "return", "unowned condition", "try entry", "catch entry", "too large"} {
		t.Run(scenario, func(t *testing.T) {
			condition := NewNode(&statements.ConditionStatement{TernaryChainArm: true})
			jump := NewNode(&statements.GOTOStatement{})
			array := NewNode(&statements.AssignStatement{})
			condition.AddNext(array)
			condition.AddNext(jump)
			jump.AddNext(array)
			switch scenario {
			case "different definition":
				jump.ReplaceNext(array, NewNode(&statements.AssignStatement{}))
			case "cycle":
				jump.ReplaceNext(array, condition)
			case "effect":
				jump.Statement = &statements.ExpressionStatement{}
			case "return":
				jump.Statement = &statements.ReturnStatement{}
			case "unowned condition":
				condition.Statement.(*statements.ConditionStatement).TernaryChainArm = false
			case "try entry":
				jump.IsTryCatch = true
			case "catch entry":
				jump.IsCatchStart = true
			case "too large":
				jump.RemoveNext(array)
				for i := 0; i < 256; i++ {
					next := NewNode(&statements.GOTOStatement{})
					jump.AddNext(next)
					jump = next
				}
				jump.AddNext(array)
			}
			entry, prefix, conditions, ok := constructorArrayEntry(condition)
			if ok != (scenario == "diamond") {
				t.Fatalf("prefix accepted=%t", ok)
			}
			if ok && (entry != array || !prefix[condition] || !prefix[jump] || len(conditions) != 1) {
				t.Fatal("lost the common definition or its entry scaffolding")
			}
		})
	}
}

func TestConstructorArrayPrefixRejectsCyclicSpills(t *testing.T) {
	array := arrayForConstructorTempTest()
	temp := values.NewJavaRef(nil, nil, array.Type())
	n := NewNode(&statements.AssignStatement{LeftValue: temp, JavaValue: array})
	n.AddNext(n)
	d := &Decompiler{RootNode: n, FunctionContext: &class_context.ClassContext{FunctionName: "<init>"}}
	if d.inlineDelegatingConstructorArrayTemp(nil) {
		t.Fatal("cyclic spills must not be accepted as a constructor entry sequence")
	}
}

func TestConstructorPrefixConditionsRequireUniqueOriginalArgument(t *testing.T) {
	condition := NewNode(&statements.ConditionStatement{TernaryChainArm: true})
	condition.Id = 3
	origins := map[int]*OpCode{3: {Id: 7}}
	ternary := &values.TernaryExpression{ConditionFromOp: 7, TrueValue: values.JavaNull, FalseValue: values.JavaNull}
	cyclic := &values.TernaryExpression{ConditionFromOp: 7}
	cyclic.TrueValue = cyclic
	for _, tc := range []struct {
		name string
		args []values.JavaValue
		want bool
	}{
		{"matching identity", []values.JavaValue{ternary}, true},
		{"no earlier use", nil, false},
		{"duplicate evaluation", []values.JavaValue{ternary, ternary}, false},
		{"same text different origin", []values.JavaValue{&values.TernaryExpression{ConditionFromOp: 8, TrueValue: values.JavaNull, FalseValue: values.JavaNull}}, false},
		{"opaque argument", []values.JavaValue{&values.CustomValue{}, ternary}, false},
		{"cyclic argument", []values.JavaValue{cyclic}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := constructorConditionsBelongToPrefixArguments([]*Node{condition}, origins, tc.args); got != tc.want {
				t.Fatalf("proof=%t want=%t", got, tc.want)
			}
		})
	}
}
