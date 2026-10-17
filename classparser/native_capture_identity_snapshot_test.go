package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// These controls exercise projection admission and atomicity. The authored
// round trips separately reopen real javac instructions and unchanged callers.
func TestNativeCaptureIdentitySnapshotsRequireOriginalSiteAndStableWord(t *testing.T) {
	for _, mode := range []string{"exclusive writes", "wrapped local", "slot cycle", "deep slot", "empty slot", "initialized local", "single name", "parameter", "copied parameter flags", "custom capture", "stack capture", "missing NEW", "wrong NEW", "wrong allocated class", "extra operand", "negative field index", "missing invoke", "wrong invoke", "wrong descriptor", "wrong kind", "non special invoke", "wrong call receiver", "wrong constructor name", "wrong capture type", "invalid name", "shared capture index", "same path twice", "write after capture", "missing assignment", "self initialization", "not direct return", "loop contour", "caught contour", "opaque value", "value cycle", "statement cycle", "same allocation twice", "unrelated method", "work", "memory", "depth", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, typ)
			declaration := &statements.AssignStatement{LeftValue: ref, IsDeclare: true}
			write := func() *statements.AssignStatement {
				return &statements.AssignStatement{LeftValue: ref, JavaValue: values.NewJavaLiteral(int64(9), typ)}
			}
			first := &nativeAnonymousClass{descriptor: "(J)V", method: "pick()Ljava/lang/Object;", fields: map[string]int{"val$stride": 0}, newPC: 3, invokePC: 7}
			second := &nativeAnonymousClass{descriptor: "(J)V", method: first.method, fields: map[string]int{"val$floor": 0}, newPC: 13, invokePC: 17}
			family := &nativeAnonymousFamily{owner: "Snapshot", children: map[string]*nativeAnonymousClass{"Snapshot$1": first, "Snapshot$2": second}}
			c := &ClassObjectDumper{nativeAnonymousRoot: family, FuncCtx: &class_context.ClassContext{ClassName: "Snapshot", FunctionName: "pick", CurrentMethodDesc: "()Ljava/lang/Object;"}}
			allocation := func(owner string, child *nativeAnonymousClass) *values.NewExpression {
				x := &values.NewExpression{JavaType: types.NewJavaClass(owner), OriginPC: child.newPC, HasOriginPC: true}
				x.ConstructorCall = &values.FunctionCallExpression{Object: x, ClassName: owner, FunctionName: "<init>", Descriptor: child.descriptor, Arguments: []values.JavaValue{ref}, Kind: values.InvokeSpecial, IsSpecialInvoke: true, OriginPC: child.invokePC, HasOriginPC: true}
				return x
			}
			a, b := allocation("Snapshot$1", first), allocation("Snapshot$2", second)
			retA, retB := statements.NewReturnStatement(a), statements.NewReturnStatement(b)
			branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{write(), retA}, ElseBody: []statements.Statement{write(), retB}}
			body := []statements.Statement{declaration, branch}
			var params []values.JavaValue
			good := mode == "exclusive writes" || mode == "wrapped local" || mode == "initialized local" || mode == "single name" || mode == "unrelated method"
			switch mode {
			case "wrapped local":
				a.ConstructorCall.Arguments[0] = values.NewSlotValue(ref, typ)
			case "slot cycle":
				slot := values.NewSlotValue(ref, typ)
				cycle := values.NewUnaryExpression(slot, values.INC, typ)
				slot.ResetValue(cycle)
				a.ConstructorCall.Arguments[0] = slot
			case "deep slot":
				var operand values.JavaValue = ref
				for i := 0; i < 33; i++ {
					operand = values.NewSlotValue(operand, typ)
				}
				a.ConstructorCall.Arguments[0] = operand
			case "empty slot":
				a.ConstructorCall.Arguments[0] = values.NewSlotValue(nil, typ)
			case "initialized local":
				declaration.IsDeclare, declaration.IsFirst, declaration.JavaValue = false, true, values.NewJavaLiteral(int64(1), typ)
				branch.IfBody, branch.ElseBody = []statements.Statement{retA}, []statements.Statement{retB}
			case "single name":
				second.fields = map[string]int{"val$stride": 0}
			case "parameter":
				ref.IsParam = true
			case "copied parameter flags":
				copy := *ref
				copy.IsParam = true
				params = []values.JavaValue{&copy}
			case "custom capture":
				ref.CustomValue = &values.CustomValue{}
			case "stack capture":
				ref.StackVar = values.NewJavaLiteral(int64(1), typ)
			case "missing NEW":
				a.HasOriginPC = false
			case "wrong NEW":
				a.OriginPC++
			case "wrong allocated class":
				a.JavaType = types.NewJavaClass("Other")
			case "extra operand":
				a.ConstructorCall.Arguments = append(a.ConstructorCall.Arguments, ref)
			case "negative field index":
				first.fields = map[string]int{"val$stride": -1}
			case "missing invoke":
				a.ConstructorCall.HasOriginPC = false
			case "wrong invoke":
				a.ConstructorCall.OriginPC++
			case "wrong descriptor":
				a.ConstructorCall.Descriptor = "(I)V"
			case "wrong kind":
				a.ConstructorCall.Kind = values.InvokeVirtual
			case "non special invoke":
				a.ConstructorCall.IsSpecialInvoke = false
			case "wrong call receiver":
				a.ConstructorCall.Object = b
			case "wrong constructor name":
				a.ConstructorCall.FunctionName = "method"
			case "wrong capture type":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "invalid name":
				first.fields = map[string]int{"val$invalid.name": 0}
			case "shared capture index":
				first.fields["val$other"] = 0
			case "same path twice":
				branch.IfBody = append([]statements.Statement{write()}, branch.IfBody...)
			case "write after capture":
				body = append(body, write())
			case "missing assignment":
				branch.IfBody = []statements.Statement{retA}
			case "self initialization":
				branch.IfBody[0].(*statements.AssignStatement).JavaValue = a
				branch.IfBody = branch.IfBody[:1]
			case "not direct return":
				branch.IfBody[1] = &statements.ExpressionStatement{Expression: a}
			case "loop contour":
				body[1] = &statements.WhileStatement{ConditionValue: branch.Condition, Body: []statements.Statement{branch}}
			case "caught contour":
				body[1] = &statements.TryCatchStatement{TryBody: []statements.Statement{branch}}
			case "opaque value":
				branch.Condition = values.NewCustomValue(func(*class_context.ClassContext) string { return "true" }, func() types.JavaType { return typ })
			case "value cycle":
				cycle := &values.JavaExpression{}
				cycle.Values = []values.JavaValue{cycle}
				branch.Condition = cycle
			case "statement cycle":
				branch.IfBody = []statements.Statement{branch}
			case "same allocation twice":
				branch.ElseBody[1] = retA
			case "unrelated method":
				c.FuncCtx.FunctionName = "other"
			case "work":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			beforeA, beforeB := a.ConstructorCall.Arguments[0], b.ConstructorCall.Arguments[0]
			projected, closed := c.prepareNativeCaptureIdentitySnapshots(body, params)
			if closed != good {
				t.Fatalf("closed=%v want=%v", closed, good)
			}
			if !good || mode == "single name" || mode == "unrelated method" {
				if a.ConstructorCall.Arguments[0] != beforeA || b.ConstructorCall.Arguments[0] != beforeB || projected[0] != body[0] {
					t.Fatal("rejected or unnecessary projection mutated its input")
				}
				return
			}
			x, y := a.ConstructorCall.Arguments[0].(*values.JavaRef), b.ConstructorCall.Arguments[0].(*values.JavaRef)
			if x.Id == y.Id || x.Id == ref.Id || y.Id == ref.Id || body[1] != branch || len(branch.IfBody) != 2 && mode != "initialized local" {
				t.Fatal("source identities or original containers were changed")
			}
			c.prepareNativeCaptureBindings(projected, params)
			if family.failed || c.FuncCtx.LocalNames[x.Id] != "stride" || c.FuncCtx.LocalNames[y.Id] != "floor" || !c.FuncCtx.SourceCaptureStable(7, x.Id) || !c.FuncCtx.SourceCaptureStable(17, y.Id) {
				t.Fatal("fresh original capture binders did not close")
			}
		})
	}
}
