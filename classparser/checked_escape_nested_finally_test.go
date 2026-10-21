package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/rewriter"
)

func TestCheckedEscapeTypedContainmentUsesOnlyProvedFinallyCatchBody(t *testing.T) {
	for _, variant := range []string{"proved nested cleanup", "different normal cleanup", "missing cleanup PC", "checked explicit throw", "unproved outer domain"} {
		t.Run(variant, func(t *testing.T) {
			ref := func() *values.JavaRef {
				return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			}
			call := func(pc int, name string) statements.Statement {
				return statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "proof/Events", FunctionName: name, Descriptor: "()V", Kind: values.InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))})
			}
			throw := func(pc int, v values.JavaValue) statements.Statement {
				return &statements.CustomStatement{ThrownValue: v, HasOriginPC: true, OriginPC: pc}
			}
			ret := func(pc int) statements.Statement {
				return &statements.ReturnStatement{HasOriginPC: true, OriginPC: pc}
			}
			nested := func(pc int) *statements.TryCatchStatement {
				caught := ref()
				return &statements.TryCatchStatement{TryBody: []statements.Statement{call(pc, "publish")}, Exception: []*values.JavaRef{caught},
					Handlers:    []statements.CatchHandler{{EntryPC: pc + 4, CatchAll: true, ProtectedRanges: [][2]int{{pc, pc + 1}, {pc + 4, pc + 5}}}},
					CatchBodies: [][]statements.Statement{{call(pc+5, "unlock"), throw(pc+6, caught)}}}
			}
			caught, primary := ref(), ref()
			normal, typedCleanup, exceptional := nested(12), nested(72), nested(112)
			tr := &statements.TryCatchStatement{
				TryBody:   []statements.Statement{call(10, "read"), normal, call(19, "unlock"), ret(25)},
				Exception: []*values.JavaRef{caught, primary},
				Handlers: []statements.CatchHandler{
					{EntryPC: 40, ProtectedRanges: [][2]int{{10, 12}}},
					{EntryPC: 100, CatchAll: true, ProtectedRanges: [][2]int{{10, 12}, {40, 70}}},
				},
				CatchBodies: [][]statements.Statement{
					{call(41, "record"), statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{throw(43, caught)}, nil), typedCleanup, call(79, "unlock"), ret(90)},
					{exceptional, call(119, "unlock"), throw(145, primary)},
				},
			}
			permitted := map[int]bool{43: true}
			switch variant {
			case "different normal cleanup":
				normal.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
			case "missing cleanup PC":
				typedCleanup.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "checked explicit throw":
				permitted[43] = false
			case "unproved outer domain":
				tr.Handlers[1].ProtectedRanges[1][0] = 41
			}
			_, proved := rewriter.RecoverCatchAllFinally(tr)
			want := variant == "proved nested cleanup"
			if proved != (want || variant == "checked explicit throw") {
				t.Fatalf("finally proof=%v", proved)
			}
			got := typedAbsorbingHandlers([]statements.Statement{tr}, permitted)
			if got[40] != want || got[100] || got[16] || got[76] || got[116] {
				t.Fatalf("typed containment=%v", got)
			}
			if len(tr.CatchBodies[0]) != 5 || len(tr.TryBody) != 4 || typedCleanup.Exception[0] == exceptional.Exception[0] {
				t.Fatal("containment changed the original exception or binding tree")
			}
		})
	}
}
