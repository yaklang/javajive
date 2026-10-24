package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialMethodLocalNamesRepresentDeclarationIdentities(t *testing.T) {
	for _, sameType := range []bool{false, true} {
		first := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Payload"))
		second := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
		first.Id.SetName("var7")
		second.Id.SetName("var7")
		if sameType {
			second = values.NewJavaRef(utils.NewRootVariableId(), nil, first.Type())
			second.Id.SetName("var7")
		}
		root := []statements.Statement{&statements.IfStatement{
			Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
			IfBody:    []statements.Statement{statements.NewAssignStatement(first, values.JavaNull, true), statements.NewReturnStatement(first)},
			ElseBody:  []statements.Statement{statements.NewAssignStatement(second, values.JavaNull, true), statements.NewReturnStatement(second)},
		}}
		resolveLocalNameCollisions(nil, root)
		if first.Id.String() == second.Id.String() || first.Id == second.Id {
			t.Fatal("distinct branch declaration identities still share one method-local name")
		}
		before := first.Id.String() + "/" + second.Id.String()
		resolveLocalNameCollisions(nil, root)
		if before != first.Id.String()+"/"+second.Id.String() {
			t.Fatal("method-local identity naming is not idempotent")
		}
	}
}

func TestAdversarialMethodLocalNamesReserveParametersAndExistingSuffixes(t *testing.T) {
	makeRef := func(name string) *values.JavaRef {
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
		ref.Id.SetName(name)
		return ref
	}
	parameter, first, later := makeRef("var7"), makeRef("var9"), makeRef("var9")
	reserved := makeRef("var9_1")
	root := []statements.Statement{
		&statements.IfStatement{Condition: parameter, IfBody: []statements.Statement{statements.NewAssignStatement(first, values.JavaNull, true)}},
		&statements.IfStatement{Condition: parameter, IfBody: []statements.Statement{statements.NewAssignStatement(later, values.JavaNull, true)}},
		statements.NewAssignStatement(reserved, values.JavaNull, true),
	}
	resolveLocalNameCollisions([]values.JavaValue{parameter}, root)
	seen := map[string]bool{}
	for _, ref := range []*values.JavaRef{parameter, first, later, reserved} {
		name := ref.Id.String()
		if seen[name] {
			t.Fatalf("duplicate method declaration name %q", name)
		}
		seen[name] = true
	}
	if parameter.Id.String() != "var7" || reserved.Id.String() != "var9_1" {
		t.Fatal("a generated suffix stole a parameter or existing declaration's name")
	}
}

// Allocation initialization is forwarded to its invokespecial location. The
// wrapper retains an exact assignment identity; it is not opaque source text.
// Names must include that declaration even when no later assignment exists.
func TestAdversarialMethodLocalNamesIncludeForwardedDeclarationBindings(t *testing.T) {
	for _, base := range []string{"var5", "argument", "local$renamed"} {
		t.Run(base, func(t *testing.T) {
			makeRef := func(name string) *values.JavaRef {
				r := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
				r.Id.SetName(name)
				return r
			}
			first, second, reserved := makeRef(base), makeRef(base), makeRef(base+"_1")
			allocation := statements.NewAssignStatement(first, values.NewJavaLiteral(7, first.Type()), true)
			forwarded := statements.NewSourceAssignmentForward(allocation)
			ordinary := statements.NewAssignStatement(second, values.NewJavaLiteral(13, second.Type()), true)
			root := []statements.Statement{forwarded, ordinary, statements.NewAssignStatement(reserved, values.NewJavaLiteral(23, reserved.Type()), true), statements.NewReturnStatement(first)}
			resolveLocalNameCollisions(nil, root)
			if first.Id.String() == second.Id.String() || first.Id.String() == reserved.Id.String() || second.Id.String() == reserved.Id.String() {
				t.Fatal("forwarded declaration collided with another identity")
			}
			if root[0] != forwarded || allocation.LeftValue != first || ordinary.LeftValue != second || first.Id == second.Id {
				t.Fatal("naming replaced a declaration or merged identities")
			}
			if actual, ok := forwarded.SourceAssignmentForward(); !ok || actual != allocation {
				t.Fatal("forwarding origin lost")
			}
			names := first.Id.String() + second.Id.String() + reserved.Id.String()
			resolveLocalNameCollisions(nil, root)
			if names != first.Id.String()+second.Id.String()+reserved.Id.String() {
				t.Fatal("late binding not idempotent")
			}
		})
	}
}
