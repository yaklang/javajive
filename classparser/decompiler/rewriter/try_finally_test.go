package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func finallyFixture() *statements.TryCatchStatement {
	ref := func(name string) *values.JavaRef {
		return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
	}
	caught, primary := ref("Exception"), ref("Throwable")
	call := func(pc int, name string) statements.Statement {
		return statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "example/Owner", FunctionName: name, Descriptor: "()V",
			IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("example.Owner")),
			OriginPC: pc, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))})
	}
	throw := func(pc int, ref *values.JavaRef) statements.Statement {
		st := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, func(*utils.VariableId, *utils.VariableId) {})
		st.ThrownValue, st.OriginPC, st.HasOriginPC = ref, pc, true
		return st
	}
	ret := statements.NewReturnStatement(nil)
	ret.OriginPC, ret.HasOriginPC = 15, true
	tr := statements.NewTryCatchStatement([]statements.Statement{call(10, "body"), call(12, "cleanup"), ret},
		[][]statements.Statement{{throw(21, caught)}, {call(42, "cleanup"), throw(45, primary)}})
	tr.Exception = []*values.JavaRef{caught, primary}
	tr.Handlers = []statements.CatchHandler{{EntryPC: 20, ProtectedRanges: [][2]int{{10, 12}}},
		{EntryPC: 40, CatchAll: true, ProtectedRanges: [][2]int{{10, 12}, {20, 40}}}}
	return tr
}

func TestCatchAllFinallyUsesCoverageAndInvokeIdentity(t *testing.T) {
	tr := finallyFixture()
	view, ok := RecoverCatchAllFinally(tr)
	if !ok || len(view.TryBody) != 2 || len(view.CatchBodies) != 1 || len(view.Cleanup) != 1 {
		t.Fatal("must recover one cleanup without a sibling catch-all")
	}
	if len(tr.TryBody) != 3 || len(tr.CatchBodies) != 2 || view.TryBody[0] != tr.TryBody[0] || view.TryBody[1] != tr.TryBody[2] {
		t.Fatal("rendering view must retain identities and leave the original tree intact")
	}
	tests := []struct {
		name   string
		mutate func(*statements.TryCatchStatement)
	}{
		{"typed Throwable is not finally", func(tr *statements.TryCatchStatement) { tr.Handlers[1].CatchAll = false }},
		{"no raw witness", func(tr *statements.TryCatchStatement) { tr.Handlers = nil }},
		{"catch entry not protected", func(tr *statements.TryCatchStatement) { tr.Handlers[1].ProtectedRanges[1][0] = 21 }},
		{"throw at exclusive boundary", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][0].(*statements.CustomStatement).OriginPC = 40
		}},
		{"cleanup protected by catch-all", func(tr *statements.TryCatchStatement) { tr.Handlers[1].ProtectedRanges[0][1] = 13 }},
		{"cleanup protected by typed catch", func(tr *statements.TryCatchStatement) { tr.Handlers[0].ProtectedRanges[0][1] = 13 }},
		{"try call outside typed region", func(tr *statements.TryCatchStatement) { tr.Handlers[0].ProtectedRanges[0][0] = 11 }},
		{"different cleanup method", func(tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
		}},
		{"different cleanup owner", func(tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).ClassName = "example/Other"
		}},
		{"different static qualifier", func(tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object = values.NewJavaClassValue(types.NewJavaClass("example.Other"))
		}},
		{"missing cleanup origin", func(tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
		}},
		{"different cleanup operand", func(tr *statements.TryCatchStatement) {
			tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Arguments = []values.JavaValue{tr.Exception[0]}
		}},
		{"cleanup depends on catch local", func(tr *statements.TryCatchStatement) {
			for _, st := range []statements.Statement{tr.TryBody[1], tr.CatchBodies[1][0]} {
				call := st.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
				call.Arguments, call.Descriptor = []values.JavaValue{tr.Exception[0]}, "(Ljava/lang/Exception;)V"
			}
		}},
		{"return replays call after cleanup", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.ReturnStatement).JavaValue = tr.TryBody[0].(*statements.ExpressionStatement).Expression
		}},
		{"wrong synthetic rethrow", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[1][1].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
		}},
		{"synthetic rethrow self protected", func(tr *statements.TryCatchStatement) { tr.Handlers[1].ProtectedRanges[1][1] = 46 }},
		{"normal exit lost cleanup", func(tr *statements.TryCatchStatement) { tr.TryBody = append(tr.TryBody[:1:1], tr.TryBody[2]) }},
		{"opaque effect", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[0][0] = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "unknown()" }, func(*utils.VariableId, *utils.VariableId) {})
		}},
		{"cleanup before continuation", func(tr *statements.TryCatchStatement) {
			tr.TryBody = []statements.Statement{statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), tr.TryBody[:2], nil), tr.TryBody[0], tr.TryBody[1], tr.TryBody[2]}
		}},
		{"statement budget", func(tr *statements.TryCatchStatement) {
			for i := 0; i < 300; i++ {
				tr.TryBody = append(tr.TryBody, statements.NewReturnStatement(nil))
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tr := finallyFixture()
			test.mutate(tr)
			if _, ok := RecoverCatchAllFinally(tr); ok {
				t.Fatal("unsupported or contradictory witness must preserve the original tree")
			}
		})
	}
}

func TestFinallyGuardMayReadUpdatedOuterLocalButNotTryDeclaration(t *testing.T) {
	for _, declared := range []bool{false, true} {
		tr := finallyFixture()
		flag := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
		flag.Id.SetName("release")
		assign := statements.NewAssignStatement(flag, values.NewJavaLiteral(false, flag.Type()), declared)
		assign.OriginPC, assign.HasOriginPC = 11, true
		normal, handler := tr.TryBody[1], tr.CatchBodies[1][0]
		tr.TryBody = []statements.Statement{tr.TryBody[0], assign, statements.NewIfStatement(flag, []statements.Statement{normal}, nil), tr.TryBody[2]}
		tr.CatchBodies[1][0] = statements.NewIfStatement(flag, []statements.Statement{handler}, nil)
		view, ok := RecoverCatchAllFinally(tr)
		if ok == declared {
			t.Fatalf("declaration=%v recovery=%v", declared, ok)
		}
		if ok && (len(view.Cleanup) != 1 || len(view.TryBody) != 3 || view.TryBody[1] != assign || len(tr.CatchBodies) != 2 || len(tr.TryBody) != 4) {
			t.Fatal("rendering view lost the protected update or changed original exception ownership")
		}
	}
}

func TestFinallyCoverageRejectsCyclesAndUnprotectedOperands(t *testing.T) {
	covered := func(pc int) bool { return pc >= 10 && pc < 20 }
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Exception"))
	cast := &values.CastExpression{Value: ref, TargetType: ref.Type(), OriginPC: 19}
	if !finallyCoveredValue(cast, covered) {
		t.Fatal("last protected cast is valid")
	}
	cast.OriginPC = 20
	if finallyCoveredValue(cast, covered) {
		t.Fatal("half-open end must be excluded")
	}
	cycle := &values.CustomValue{Flag: "instanceof", CapturesKnown: true, HasOriginPC: true, OriginPC: 11}
	cycle.Captures = []values.JavaValue{cycle}
	if finallyCoveredValue(cycle, covered) {
		t.Fatal("cyclic opaque dependency must fail closed")
	}
	var deep values.JavaValue = ref
	for i := 0; i < 600; i++ {
		deep = values.NewUnaryExpression(deep, values.Not, types.NewJavaPrimer(types.JavaBoolean))
	}
	if finallyCoveredValue(deep, covered) {
		t.Fatal("dependency budget must bound traversal")
	}
}

func TestFinallyFieldRestoreAndGuardRequireIdentityAndOrigins(t *testing.T) {
	for _, name := range []string{"proved", "other field", "other receiver", "other local", "declared", "missing origin", "protected write", "excluded local", "changed guard", "protected return"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaClass("example.Resource")
			self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner"))
			self.IsThis = true
			saved := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			restore := func(pc int) *statements.AssignStatement {
				st := statements.NewAssignStatement(values.NewRefMember(self, "current", typ), saved, false)
				st.OriginPC = pc
				st.HasOriginPC = true
				return st
			}
			a, b := restore(30), restore(40)
			rows := []core.HandlerRange{{StartPc: 10, EndPc: 20}}
			excluded := []*values.JavaRef{}
			switch name {
			case "other field":
				b.LeftValue.(*values.RefMember).Member = "different"
			case "other receiver":
				b.LeftValue.(*values.RefMember).Object = values.NewJavaRef(utils.NewRootVariableId(), nil, self.Type())
			case "other local":
				b.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "declared":
				b.IsDeclare = true
			case "missing origin":
				b.HasOriginPC = false
			case "protected write":
				b.OriginPC = 19
			case "excluded local":
				excluded = append(excluded, saved)
			}
			condition := func() values.JavaValue {
				return values.NewBinaryExpression(saved, values.JavaNull, values.NEQ, types.NewJavaPrimer(types.JavaBoolean))
			}
			guardA, guardB := statements.NewIfStatement(condition(), []statements.Statement{a}, nil), statements.NewIfStatement(condition(), []statements.Statement{b}, nil)
			if name == "changed guard" {
				guardB.Condition = values.NewBinaryExpression(saved, values.JavaNull, values.EQ, types.NewJavaPrimer(types.JavaBoolean))
			}
			if name == "protected return" {
				ret := statements.NewReturnStatement(nil)
				ret.OriginPC = 19
				ret.HasOriginPC = true
				guardB.ElseBody = []statements.Statement{ret}
			}
			if got := sameFinallyCleanup(rows, guardA, guardB, excluded, 0); got != (name == "proved") {
				t.Fatalf("proof=%v", got)
			}
			if len(guardA.IfBody) != 1 || len(guardB.IfBody) != 1 {
				t.Fatal("mutated input cleanup")
			}
		})
	}
}
