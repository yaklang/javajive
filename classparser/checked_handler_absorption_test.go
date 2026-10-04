package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestTypedHandlerUncheckedThrowRequiresOriginalPCWitness(t *testing.T) {
	for _, test := range []struct {
		name                      string
		pc                        int
		origin, typed, proof, all bool
		want                      bool
	}{{"proven unchecked", 42, true, true, true, false, true}, {"missing original PC", 42, false, true, true, false, false}, {"wrong original PC", 43, true, true, true, false, false}, {"opaque throw", 42, true, false, true, false, false}, {"checked or unknown throw", 42, true, true, false, false, false}, {"catchall never absorbs", 42, true, true, true, true, false}} {
		t.Run(test.name, func(t *testing.T) {
			statement := &statements.CustomStatement{OriginPC: test.pc, HasOriginPC: test.origin}
			if test.typed {
				statement.ThrownValue = values.NewNewExpression(types.NewJavaClass("java.lang.IllegalArgumentException"))
			}
			handler := &statements.TryCatchStatement{Handlers: []statements.CatchHandler{{EntryPC: 30, CatchAll: test.all}}, CatchBodies: [][]statements.Statement{{statement}}}
			proof := map[int]bool{}
			if test.proof {
				proof[42] = true
			}
			if got := typedAbsorbingHandlers([]statements.Statement{handler}, proof)[30]; got != test.want {
				t.Fatalf("absorption=%v want%v", got, test.want)
			}
		})
	}
}

func TestTypedHandlerDiscoveryTraversesOriginalStructuredBodies(t *testing.T) {
	handler := &statements.TryCatchStatement{Handlers: []statements.CatchHandler{{EntryPC: 30}}, CatchBodies: [][]statements.Statement{{statements.NewReturnStatement(nil)}}}
	for _, container := range []statements.Statement{
		&statements.WhileStatement{Body: []statements.Statement{handler}},
		&statements.DoWhileStatement{Body: []statements.Statement{handler}},
		&statements.ForStatement{SubStatements: []statements.Statement{handler}},
		&statements.SynchronizedStatement{Body: []statements.Statement{handler}},
		&statements.IfStatement{ElseBody: []statements.Statement{handler}},
	} {
		if !typedAbsorbingHandlers([]statements.Statement{container}, nil)[30] {
			t.Fatalf("handler evidence lost under %T", container)
		}
	}
}
