package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func foldDependencyRef(name string) *values.JavaRef {
	id := utils.NewRootVariableId()
	id.SetName(name)
	return values.NewJavaRef(id, nil, types.NewJavaPrimer(types.JavaInteger))
}

func TestFoldLocalDependenciesNeverRenderProducerOrConsumer(t *testing.T) {
	ctx := &class_context.ClassContext{}
	d := &Decompiler{FunctionContext: ctx}
	x, temporary := foldDependencyRef("sourceName"), foldDependencyRef("result")
	seen := 0
	opaque := values.NewCustomValue(func(*class_context.ClassContext) string {
		seen++
		return "(sourceName = 7)"
	}, x.Type)
	assignment := values.NewAssignmentExpression(x, values.NewJavaLiteral(7, x.Type()), 3, func(*class_context.ClassContext) string {
		seen++
		return "(sourceName = 7)"
	})
	owner := types.NewJavaClass("proof.Owner")
	receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, owner)
	hidden := values.NewRefMember(receiver, "this$0", owner)
	ctx.SourceLexicalCapturedField = func(any, int, string) (string, bool) {
		seen++
		return "enclosing", true
	}
	method, err := types.ParseMethodDescriptor("(Lproof/Owner;I)V")
	if err != nil {
		t.Fatal(err)
	}
	allocation := &values.NewExpression{JavaType: types.NewJavaClass("proof.Owner$Entry"), ConstructorCall: &values.FunctionCallExpression{
		ClassName: "proof.Owner$Entry", FunctionName: "<init>", Descriptor: "(Lproof/Owner;I)V", Kind: values.InvokeSpecial,
		IsSpecialInvoke: true, FuncType: method.FunctionType(), Arguments: []values.JavaValue{hidden, x},
	}, ArgumentsGetter: func() string { seen++; return "hidden" }}
	legacyAllocation := &values.NewExpression{JavaType: allocation.JavaType, ArgumentsGetter: func() string { seen++; return "hidden" }}
	for _, test := range []struct {
		name      string
		value     values.JavaValue
		statement statements.Statement
		want      bool
	}{
		{"known allocation", allocation, statements.NewReturnStatement(temporary), false},
		{"opaque producer", opaque, statements.NewReturnStatement(temporary), true},
		{"legacy constructor operands", legacyAllocation, statements.NewReturnStatement(temporary), true},
		{"assignment renderer", assignment, statements.NewReturnStatement(x), true},
		{"opaque consumer", assignment, &statements.CustomStatement{StringFunc: func(*class_context.ClassContext) string { seen++; return "sourceName" }}, true},
		{"opaque operand in consumer", assignment, statements.NewReturnStatement(opaque), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			seen = 0
			if got := d.foldReordersSideEffect(test.value, NewNode(test.statement), temporary); got != test.want {
				t.Fatalf("interference=%v want=%v", got, test.want)
			}
			if seen != 0 {
				t.Fatalf("analysis executed %d source render callbacks", seen)
			}
		})
	}
}

func TestFoldLocalDependenciesUseIdentityAndAllUpdateForms(t *testing.T) {
	d := &Decompiler{FunctionContext: &class_context.ClassContext{}}
	for _, name := range []string{"var1", "counter", "renamed$local"} {
		t.Run(name, func(t *testing.T) {
			x, unrelated, temporary := foldDependencyRef(name), foldDependencyRef(name), foldDependencyRef("result")
			alias := *x
			alias.Id = utils.NewRootVariableId()
			alias.Id.SetName("differentSourceSpelling")
			one := values.NewJavaLiteral(1, x.Type())
			updates := []values.JavaValue{values.NewAssignmentExpression(x, one, 3, nil)}
			for _, operator := range []string{"=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>=", values.INC, values.DEC, "++", "--"} {
				updates = append(updates, values.NewBinaryExpression(x, one, operator, x.Type()))
			}
			for _, update := range updates {
				for _, reader := range []*values.JavaRef{x, &alias, unrelated} {
					statement := statements.NewReturnStatement(values.NewBinaryExpression(reader, temporary, values.ADD, x.Type()))
					if got, want := d.foldReordersSideEffect(update, NewNode(statement), temporary), reader != unrelated; got != want {
						t.Fatalf("%T reader=%s same identity=%v interference=%v", update, reader.Id.String(), want, got)
					}
				}
			}
			// A read of a local does not execute its defining assignment again.
			unrelated.Val = values.NewAssignmentExpression(x, one, 3, nil)
			if d.foldReordersSideEffect(unrelated, NewNode(statements.NewReturnStatement(x)), temporary) {
				t.Fatal("followed a local read back into its definition")
			}
		})
	}
}

func TestOptimizerTraceNeverRendersSource(t *testing.T) {
	ctx := &class_context.ClassContext{}
	ref := foldDependencyRef("originalLocal")
	seen := 0
	opaque := values.NewCustomValue(func(*class_context.ClassContext) string {
		seen++
		return "unavailable source capability"
	}, ref.Type)
	allocation := &values.NewExpression{JavaType: types.NewJavaClass("proof.Owner$Entry"), ArgumentsGetter: func() string { seen++; return "hidden" }}
	ref.CustomValue = opaque
	for _, value := range []values.JavaValue{ref, opaque, allocation, values.NewAssignmentExpression(ref, opaque, 3, nil)} {
		if got := traceValue(value, ctx); got == "" {
			t.Fatal("missing structural diagnostic")
		}
		if seen != 0 {
			t.Fatalf("diagnostic executed %d source callbacks", seen)
		}
	}
}
