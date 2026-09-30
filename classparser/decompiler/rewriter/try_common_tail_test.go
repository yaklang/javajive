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

func tryTailFixture() (*core.Node, *statements.TryCatchStatement, *statements.IfStatement) {
	region := &core.Node{HasProtectedRange: true, SharedProtectedHandler: true, ProtectedStartPC: 8, ProtectedEndPC: 14,
		SharedProtectedRanges: []core.HandlerRange{{StartPc: 8, EndPc: 14, HandlerPc: 31}, {StartPc: 20, EndPc: 25, HandlerPc: 31}}}
	ref := func(typ string) *values.JavaRef {
		return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(typ))
	}
	resource, left, right, failure := ref("example.Resource"), ref("Object"), ref("example.Result"), ref("Throwable")
	call := func(pc int, name, descriptor string, result types.JavaType) *values.FunctionCallExpression {
		return &values.FunctionCallExpression{Object: resource, ClassName: "example/Resource", FunctionName: name, Descriptor: descriptor,
			Kind: values.InvokeVirtual, OriginPC: pc, HasOriginPC: true, FuncType: types.NewJavaFuncType(descriptor, nil, result)}
	}
	store := func(pc int, target, rhs values.JavaValue) *statements.AssignStatement {
		return &statements.AssignStatement{LeftValue: target, JavaValue: rhs, OriginPC: pc, HasOriginPC: true}
	}
	branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		[]statements.Statement{store(13, left, values.NewJavaLiteral("null", left.Type())), statements.NewExpressionStatement(call(15, "finish", "()V", types.NewJavaPrimer(types.JavaVoid))), statements.NewReturnStatement(left)},
		[]statements.Statement{store(24, right, call(21, "read", "()Lexample/Result;", right.Type())), statements.NewExpressionStatement(call(26, "finish", "()V", types.NewJavaPrimer(types.JavaVoid))), statements.NewReturnStatement(right)})
	throw := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failure" }, func(*utils.VariableId, *utils.VariableId) {})
	throw.ThrownValue = failure
	tr := statements.NewTryCatchStatement([]statements.Statement{branch}, [][]statements.Statement{{throw}})
	tr.Exception = []*values.JavaRef{failure}
	return region, tr, branch
}

func TestTryCommonTailRetainsGapsAndFreshResultIdentity(t *testing.T) {
	region, tr, branch := tryTailFixture()
	old := branch.IfBody[0].(*statements.AssignStatement).LeftValue.(*values.JavaRef)
	// Before lexical naming, a caught slot can have the same VariableId but a
	// different definition UID. The fresh result must not rename that handler.
	tr.Exception[0].Id = old.Id
	catchID := tr.Exception[0].Id
	declaration, tail := factorUnprotectedTryTail(region, tr)
	if declaration == nil || !declaration.IsDeclare || len(tail) != 2 || len(branch.IfBody) != 1 || len(branch.ElseBody) != 1 {
		t.Fatal("shared unprotected tail was not factored")
	}
	result := declaration.LeftValue.(*values.JavaRef)
	if result.Id == old.Id || result.VarUid == old.VarUid || tr.Exception[0].Id != catchID {
		t.Fatal("phi captured an existing branch or handler identity")
	}
	if !sameTryLocal(branch.IfBody[0].(*statements.AssignStatement).LeftValue, result) || !sameTryLocal(tail[1].(*statements.ReturnStatement).JavaValue, result) {
		t.Fatal("result identity was not shared through the normal tail")
	}
	if name, _ := types.RawClassFQN(result.Type()); name != "example.Result" {
		t.Fatal("null arm erased the concrete result type")
	}
}

func TestTryCommonTailRejectsUnprovenCoverageOrDependencies(t *testing.T) {
	for _, scenario := range []string{"no witness", "not shared", "different handler", "bad interval", "not branch", "missing store pc", "outside store", "outside read", "exclusive read", "parameter store", "foreign return", "protected tail", "tail before store", "different call", "different receiver", "opaque receiver", "effectful tail argument", "tail reads result", "incompatible results", "fallthrough handler", "opaque handler", "handler reads result", "opaque decision", "too many rows", "unknown read", "cyclic read"} {
		t.Run(scenario, func(t *testing.T) {
			region, tr, branch := tryTailFixture()
			left := branch.IfBody[0].(*statements.AssignStatement)
			right := branch.ElseBody[0].(*statements.AssignStatement)
			read := right.JavaValue.(*values.FunctionCallExpression)
			a := branch.IfBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			b := branch.ElseBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			opaque := values.NewCustomValue(func(*class_context.ClassContext) string { return "unknown()" }, func() types.JavaType { return types.NewJavaClass("Object") })
			switch scenario {
			case "no witness":
				region.HasProtectedRange = false
			case "not shared":
				region.SharedProtectedHandler = false
			case "different handler":
				region.SharedProtectedRanges[1].HandlerPc++
			case "bad interval":
				region.SharedProtectedRanges[1].EndPc = 20
			case "not branch":
				tr.TryBody[0] = left
			case "missing store pc":
				left.HasOriginPC = false
			case "outside store":
				left.OriginPC = 14
			case "outside read":
				read.OriginPC = 19
			case "exclusive read":
				read.OriginPC = 25
			case "parameter store":
				left.LeftValue.(*values.JavaRef).IsParam = true
			case "foreign return":
				branch.IfBody[2] = statements.NewReturnStatement(right.LeftValue)
			case "protected tail":
				a.OriginPC = 13
			case "tail before store":
				a.OriginPC = 7
			case "different call":
				b.FunctionName = "other"
			case "different receiver":
				b.Object = right.LeftValue
			case "opaque receiver":
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Object"))
				ref.StackVar = ref
				a.Object, b.Object = ref, ref
			case "effectful tail argument":
				a.Arguments, b.Arguments = []values.JavaValue{opaque}, []values.JavaValue{opaque}
			case "tail reads result":
				a.Arguments, b.Arguments = []values.JavaValue{left.LeftValue}, []values.JavaValue{right.LeftValue}
			case "incompatible results":
				left.JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "fallthrough handler":
				tr.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(a)}
			case "opaque handler":
				tr.CatchBodies[0][0].(*statements.CustomStatement).ThrownValue = nil
			case "handler reads result":
				tr.CatchBodies[0][0].(*statements.CustomStatement).ThrownValue = left.LeftValue
			case "opaque decision":
				branch.Condition = opaque
			case "too many rows":
				region.SharedProtectedRanges = make([]core.HandlerRange, 33)
			case "unknown read":
				read.HasOriginPC = false
			case "cyclic read":
				read.Object = read
			}
			before := len(branch.IfBody)
			if decl, _ := factorUnprotectedTryTail(region, tr); decl != nil || len(branch.IfBody) != before {
				t.Fatal("unproven factoring mutated the branch")
			}
		})
	}
}

func TestTryTailDependencyProofRejectsCyclesAndOversizedTrees(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Object"))
	cycle := &values.JavaExpression{}
	cycle.Values = []values.JavaValue{cycle}
	large := &values.JavaExpression{Values: make([]values.JavaValue, 513)}
	large.Values[512] = ref
	for _, value := range []values.JavaValue{cycle, large, (*values.JavaRef)(nil)} {
		if tryValueIndependent(value, []*values.JavaRef{ref}) {
			t.Fatal("malformed dependency proof was accepted")
		}
	}
}
