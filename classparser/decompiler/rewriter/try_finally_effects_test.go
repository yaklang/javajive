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

func effectCleanupPair() (*values.FunctionCallExpression, *values.FunctionCallExpression, *values.JavaRef) {
	self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner"))
	self.IsThis = true
	call := func(pc int) *values.FunctionCallExpression {
		member := &values.RefMember{Member: "stack", Object: self, JavaType: types.NewJavaClass("example.Stack"), HasOriginPC: true, OriginPC: pc - 1}
		return &values.FunctionCallExpression{Object: member, Kind: values.InvokeVirtual, ClassName: "example.Stack", FunctionName: "pop", Descriptor: "()Ljava/lang/Object;", HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType("()Ljava/lang/Object;", nil, types.NewJavaClass("java.lang.Object"))}
	}
	return call(100), call(200), self
}

func TestFinallyEffectTreeRequiresExactUnprotectedEvaluations(t *testing.T) {
	rows := []core.HandlerRange{{StartPc: 10, EndPc: 20}}
	for _, tt := range []struct {
		name   string
		change func(*values.FunctionCallExpression, *values.FunctionCallExpression)
		want   bool
	}{
		{"discarded object result and field receiver", func(a, b *values.FunctionCallExpression) {}, true},
		{"missing call PC", func(a, b *values.FunctionCallExpression) { a.HasOriginPC = false }, false},
		{"protected call", func(a, b *values.FunctionCallExpression) { a.OriginPC = 19 }, false},
		{"call on exclusive boundary", func(a, b *values.FunctionCallExpression) { a.OriginPC = 20 }, true},
		{"missing receiver PC", func(a, b *values.FunctionCallExpression) { a.Object.(*values.RefMember).HasOriginPC = false }, false},
		{"protected receiver evaluation", func(a, b *values.FunctionCallExpression) { a.Object.(*values.RefMember).OriginPC = 10 }, false},
		{"other receiver field", func(a, b *values.FunctionCallExpression) { b.Object.(*values.RefMember).Member = "other" }, false},
		{"other fully qualified field type", func(a, b *values.FunctionCallExpression) {
			b.Object.(*values.RefMember).JavaType = types.NewJavaClass("other.Stack")
		}, false},
		{"other invoke owner", func(a, b *values.FunctionCallExpression) { b.ClassName = "other.Stack" }, false},
		{"other invoke ABI", func(a, b *values.FunctionCallExpression) { b.Descriptor = "()V" }, false},
		{"malformed ABI", func(a, b *values.FunctionCallExpression) { a.Descriptor = "broken"; b.Descriptor = "broken" }, false},
		{"ABI argument count", func(a, b *values.FunctionCallExpression) {
			a.Arguments = []values.JavaValue{values.JavaNull}
			b.Arguments = []values.JavaValue{values.JavaNull}
		}, false},
		{"missing return metadata", func(a, b *values.FunctionCallExpression) { a.FuncType = nil }, false},
		{"other declared return type", func(a, b *values.FunctionCallExpression) { a.FuncType.ReturnType = types.NewJavaClass("other.Object") }, false},
		{"static-kind disagreement", func(a, b *values.FunctionCallExpression) { a.IsStatic = true; b.IsStatic = true }, false},
		{"unknown dynamic target", func(a, b *values.FunctionCallExpression) {
			a.Kind = values.InvokeDynamic
			b.Kind = values.InvokeDynamic
		}, false},
		{"opaque receiver", func(a, b *values.FunctionCallExpression) {
			a.Object = &values.CustomValue{}
			b.Object = &values.CustomValue{}
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b, _ := effectCleanupPair()
			tt.change(a, b)
			if got := sameFinallyEffectValue(rows, a, b, nil, 0); got != tt.want {
				t.Fatalf("proof=%v want%v", got, tt.want)
			}
		})
	}
	a, b, self := effectCleanupPair()
	if sameFinallyEffectValue(rows, a, b, []*values.JavaRef{self}, 0) {
		t.Fatal("must reject mutable/excluded cleanup local")
	}
	// A shared graph can be deep or very wide; both work bounds fail closed.
	deep := values.JavaValue(a)
	for i := 0; i < 40; i++ {
		deep = &values.JavaExpression{Op: values.EQ, Values: []values.JavaValue{deep, deep}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
	}
	if sameFinallyEffectValue(rows, deep, deep, nil, 0) {
		t.Fatal("depth exceeded")
	}
	wide := values.JavaValue(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	for i := 0; i < 12; i++ {
		wide = &values.JavaExpression{Op: values.ADD, Values: []values.JavaValue{wide, wide}, Typ: types.NewJavaPrimer(types.JavaInteger)}
	}
	if sameFinallyEffectValue(rows, wide, wide, nil, 0) {
		t.Fatal("tree work budget exceeded")
	}
	for _, kind := range []types.JavaType{types.NewJavaPrimer(types.JavaString), types.NewJavaPrimer(types.JavaBoolean), types.NewJavaClass("java.lang.Integer")} {
		x := &values.JavaExpression{Op: values.ADD, Values: []values.JavaValue{values.NewJavaLiteral(1, kind), values.NewJavaLiteral(2, kind)}, Typ: kind}
		if sameFinallyEffectValue(rows, x, x, nil, 0) {
			t.Fatal("reference/boxed/boolean arithmetic lacks a primitive effect proof")
		}
	}
}

func TestFinallyRecoveryRetainsFieldReceiverAndDiscardedResult(t *testing.T) {
	tr := finallyFixture()
	a, b, _ := effectCleanupPair()
	a.OriginPC = 12
	a.Object.(*values.RefMember).OriginPC = 12
	b.OriginPC = 42
	b.Object.(*values.RefMember).OriginPC = 42
	tr.TryBody[1] = statements.NewExpressionStatement(a)
	tr.CatchBodies[1][0] = statements.NewExpressionStatement(b)
	view, ok := RecoverCatchAllFinally(tr)
	if !ok || len(view.Cleanup) != 1 || len(view.TryBody) != 2 || len(view.CatchBodies) != 1 {
		t.Fatal("must recover field-loaded cleanup once and preserve handler rethrow")
	}
	if view.Cleanup[0] != tr.CatchBodies[1][0] || view.CatchBodies[0][0] != tr.CatchBodies[0][0] || view.TryBody[1] != tr.TryBody[2] {
		t.Fatal("cleanup and abrupt-return operand identities must remain intact")
	}
}

func TestFinallyAllocationRequiresItsOriginalConstructorReceiver(t *testing.T) {
	makeAllocation := func(pc int) *values.NewExpression {
		x := &values.NewExpression{JavaType: types.NewJavaClass("example.Box"), HasOriginPC: true, OriginPC: pc}
		x.ConstructorCall = &values.FunctionCallExpression{Object: x, Kind: values.InvokeSpecial, IsSpecialInvoke: true, ClassName: "example.Box", FunctionName: "<init>", Descriptor: "()V", OriginPC: pc + 3, HasOriginPC: true, FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid))}
		return x
	}
	for _, tt := range []struct {
		name   string
		change func(*values.NewExpression)
		want   bool
	}{
		{"proved allocation and constructor", func(x *values.NewExpression) {}, true},
		{"missing allocation PC", func(x *values.NewExpression) { x.HasOriginPC = false }, false},
		{"missing constructor PC", func(x *values.NewExpression) { x.ConstructorCall.HasOriginPC = false }, false},
		{"foreign constructor receiver", func(x *values.NewExpression) { x.ConstructorCall.Object = values.JavaNull }, false},
		{"different constructor owner", func(x *values.NewExpression) { x.ConstructorCall.ClassName = "other.Box" }, false},
		{"protected constructor", func(x *values.NewExpression) { x.ConstructorCall.OriginPC = 11 }, false},
		{"protected allocation", func(x *values.NewExpression) { x.OriginPC = 11 }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := makeAllocation(100), makeAllocation(200)
			tt.change(a)
			if got := sameFinallyEffectValue([]core.HandlerRange{{StartPc: 10, EndPc: 20}}, a, b, nil, 0); got != tt.want {
				t.Fatalf("allocation proof=%v want%v", got, tt.want)
			}
		})
	}
}

func TestFinallyFieldStoresKeepOriginalProtectedDomain(t *testing.T) {
	field := &values.JavaClassMember{Name: "example.Owner", Member: "count", Description: "I", JavaType: types.NewJavaPrimer(types.JavaInteger)}
	store := statements.NewAssignStatement(field, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), false)
	store.OriginPC = 12
	store.HasOriginPC = true
	covered := func(pc int) bool { return pc >= 10 && pc < 20 }
	if !finallyCoveredAssignment(store, covered) {
		t.Fatal("exact protected primitive store should remain protected")
	}
	store.OriginPC = 20
	if finallyCoveredAssignment(store, covered) {
		t.Fatal("exclusive-boundary store must not move into protected body")
	}
	store.OriginPC = 12
	store.HasOriginPC = false
	if finallyCoveredAssignment(store, covered) {
		t.Fatal("missing store witness")
	}
	store.HasOriginPC = true
	store.JavaValue = &values.CustomValue{}
	if finallyCoveredAssignment(store, covered) {
		t.Fatal("opaque folded store value")
	}
}

func conditionalFinallyFixture() *statements.TryCatchStatement {
	tr := finallyFixture()
	failure := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.RuntimeException"))
	throw := func(pc int, value values.JavaValue) *statements.CustomStatement {
		st := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw value" }, func(*utils.VariableId, *utils.VariableId) {})
		st.ThrownValue, st.OriginPC, st.HasOriginPC = value, pc, true
		return st
	}
	condition := func(pc int) values.JavaValue {
		return &values.JavaClassMember{Name: "example.Owner", Member: "fails", Description: "Z", JavaType: types.NewJavaPrimer(types.JavaBoolean), OriginPC: pc, HasOriginPC: true}
	}
	tr.TryBody[2] = statements.NewIfStatement(condition(12), []statements.Statement{throw(13, failure)}, []statements.Statement{tr.TryBody[2]})
	tr.CatchBodies[1][1] = statements.NewIfStatement(condition(42), []statements.Statement{throw(43, failure)}, []statements.Statement{tr.CatchBodies[1][1]})
	return tr
}

func TestFinallyConditionalAbruptCleanupKeepsIdentityAndPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*statements.TryCatchStatement)
		want   bool
	}{
		{"proved conditional throw overrides return and rethrow", func(tr *statements.TryCatchStatement) {}, true},
		{"wrong primary rethrow", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[1][1].(*statements.IfStatement).ElseBody[0].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
		}, false},
		{"missing primary rethrow PC", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[1][1].(*statements.IfStatement).ElseBody[0].(*statements.CustomStatement).HasOriginPC = false
		}, false},
		{"protected primary rethrow", func(tr *statements.TryCatchStatement) {
			tr.CatchBodies[1][1].(*statements.IfStatement).ElseBody[0].(*statements.CustomStatement).OriginPC = 39
		}, false},
		{"normal return effects", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).ElseBody[0].(*statements.ReturnStatement).JavaValue = tr.TryBody[0].(*statements.ExpressionStatement).Expression
		}, false},
		{"unknown return PC", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).ElseBody[0].(*statements.ReturnStatement).HasOriginPC = false
		}, false},
		{"protected return PC", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).ElseBody[0].(*statements.ReturnStatement).OriginPC = 11
		}, false},
		{"protected cleanup condition", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).Condition.(*values.JavaClassMember).OriginPC = 11
		}, false},
		{"different cleanup throwable", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).IfBody[0].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
		}, false},
		{"cleanup throw missing PC", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).IfBody[0].(*statements.CustomStatement).HasOriginPC = false
		}, false},
		{"cleanup throw protected", func(tr *statements.TryCatchStatement) {
			tr.TryBody[2].(*statements.IfStatement).IfBody[0].(*statements.CustomStatement).OriginPC = 11
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := conditionalFinallyFixture()
			tt.change(tr)
			view, ok := RecoverCatchAllFinally(tr)
			if ok != tt.want {
				t.Fatalf("recovery=%v want%v", ok, tt.want)
			}
			if ok {
				if len(view.Cleanup) != 2 || len(view.TryBody) != 2 || len(view.CatchBodies) != 1 {
					t.Fatal("expected shared conditional cleanup and original typed catch")
				}
				if _, ok := view.TryBody[1].(*statements.ReturnStatement); !ok {
					t.Fatal("must return inert value before finally evaluates exact cleanup once")
				}
				if len(tr.TryBody[2].(*statements.IfStatement).ElseBody) != 1 || len(tr.CatchBodies[1][1].(*statements.IfStatement).ElseBody) != 1 {
					t.Fatal("must leave original IR untouched")
				}
			}
		})
	}
}

func TestFinallyFoldedTernaryRequiresEveryEffectInOriginalDomain(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*values.TernaryExpression)
		want   bool
	}{
		{"both arms protected or inert", func(x *values.TernaryExpression) {}, true},
		{"missing condition", func(x *values.TernaryExpression) { x.Condition = nil }, false},
		{"missing arm", func(x *values.TernaryExpression) { x.FalseValue = nil }, false},
		{"unprotected true arm", func(x *values.TernaryExpression) { x.TrueValue.(*values.FunctionCallExpression).OriginPC = 20 }, false},
		{"missing true arm PC", func(x *values.TernaryExpression) { x.TrueValue.(*values.FunctionCallExpression).HasOriginPC = false }, false},
		{"unprotected receiver evaluation", func(x *values.TernaryExpression) {
			x.TrueValue.(*values.FunctionCallExpression).Object.(*values.RefMember).OriginPC = 9
		}, false},
		{"opaque false arm", func(x *values.TernaryExpression) { x.FalseValue = &values.CustomValue{} }, false},
		{"cyclic arm", func(x *values.TernaryExpression) { x.FalseValue = x }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			call, _, _ := effectCleanupPair()
			call.OriginPC = 12
			call.Object.(*values.RefMember).OriginPC = 11
			x := &values.TernaryExpression{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), TrueValue: call, FalseValue: values.JavaNull}
			tt.change(x)
			if got := finallyCoveredValue(x, func(pc int) bool { return pc >= 10 && pc < 20 }); got != tt.want {
				t.Fatalf("coverage=%v want%v", got, tt.want)
			}
		})
	}
}
