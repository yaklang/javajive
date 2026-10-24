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

func finallyLambdaLetFixture() (*statements.TryCatchStatement, []*statements.AssignStatement, []*values.CustomValue) {
	tr := finallyFixture()
	id := values.NewLambdaSourceIdentity()
	typ := types.NewJavaClass("java.util.function.LongConsumer")
	parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaLong))
	parameter.IsParam = true
	var stores []*statements.AssignStatement
	var factories []*values.CustomValue
	copy := func(pc int) []statements.Statement {
		snapshot := values.NewJavaRef(utils.NewRootVariableId(), parameter, parameter.Type().Copy())
		snapshot.MarkOriginalDynamicOperand(pc, 0, parameter)
		snapshot.MarkOriginalDynamicOperandDeclaration(pc, parameter)
		capture := statements.NewAssignStatement(snapshot, parameter, true)
		v := values.NewCustomValue(func(*class_context.ClassContext) string { panic("deferred cleanup must not render during proof") }, func() types.JavaType { panic("proof must not query custom type") })
		v.Flag, v.CapturesKnown, v.Captures, v.OriginPC, v.HasOriginPC = "lambda", true, []values.JavaValue{snapshot}, pc, true
		v.InstantiatedMtdDesc = "(J)V"
		v.BindLambdaSourceIdentity(id)
		v.MarkOriginalLambdaFactory(pc)
		v.BindOriginalLambdaFactorySignature("accept", "(J)Ljava/util/function/LongConsumer;", "(J)V")
		ref := values.NewJavaRef(utils.NewRootVariableId(), v, typ.Copy())
		ref.MarkOriginalLambdaResult(pc, v)
		factory := &statements.AssignStatement{LeftValue: ref, JavaValue: v, IsFirst: true}
		call := &values.FunctionCallExpression{ClassName: "p/Owner", FunctionName: "consume", Descriptor: "(Ljava/util/function/LongConsumer;)V", Kind: values.InvokeStatic, IsStatic: true, Arguments: []values.JavaValue{ref}, OriginPC: pc + 3, HasOriginPC: true, FuncType: types.NewJavaFuncType("(Ljava/util/function/LongConsumer;)V", []types.JavaType{typ}, types.NewJavaPrimer(types.JavaVoid))}
		stores, factories = append(stores, capture, factory), append(factories, v)
		return []statements.Statement{capture, factory, statements.NewExpressionStatement(call)}
	}
	normal, handler := copy(12), copy(42)
	ret := tr.TryBody[2].(*statements.ReturnStatement)
	ret.OriginPC = 17
	tr.TryBody = append(append([]statements.Statement{tr.TryBody[0]}, normal...), ret)
	rethrow := tr.CatchBodies[1][1].(*statements.CustomStatement)
	rethrow.OriginPC = 47
	tr.CatchBodies[1] = append(handler, rethrow)
	return tr, stores, factories
}

func TestFinallyLambdaCleanupRequiresScopedOriginalLets(t *testing.T) {
	for _, variant := range []string{"same", "renamed", "different body", "changed capture", "protected factory", "missing factory origin", "ordinary snapshot", "changed snapshot", "ordinary factory temporary", "wrong materialization opcode", "wrong materialization PC", "source temporary escapes", "capture temporary escapes", "same ID wrong UID", "extra effect", "opaque scope", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			tr, stores, factories := finallyLambdaLetFixture()
			ref := stores[1].LeftValue.(*values.JavaRef)
			switch variant {
			case "renamed":
				ref.Id.SetName("anotherSourceName")
			case "different body":
				factories[1] = &values.CustomValue{}
				stores[3].JavaValue = factories[1]
			case "changed capture":
				factories[1].Captures[0] = values.JavaNull
			case "protected factory":
				tr.Handlers[0].ProtectedRanges[0][1] = 13
			case "missing factory origin":
				factories[0].HasOriginPC = false
			case "ordinary snapshot":
				stores[0].LeftValue = values.NewJavaRef(utils.NewRootVariableId(), stores[0].JavaValue, stores[0].JavaValue.Type())
			case "changed snapshot":
				stores[0].JavaValue = values.NewJavaLiteral(int64(7), types.NewJavaPrimer(types.JavaLong))
			case "ordinary factory temporary", "wrong materialization opcode", "wrong materialization PC":
				other := values.NewJavaRef(utils.NewRootVariableId(), ref.Val, ref.Type())
				if variant != "ordinary factory temporary" {
					if variant == "wrong materialization PC" {
						other.MarkOriginalLambdaResult(13, factories[0])
					} else {
						other.MarkOriginalStackMaterialization(12, core.OP_DUP, ref.Val)
					}
				}
				stores[1].LeftValue = other
			case "source temporary escapes":
				tr.TryBody[4].(*statements.ReturnStatement).JavaValue = ref
			case "capture temporary escapes":
				tr.TryBody[4].(*statements.ReturnStatement).JavaValue = stores[0].LeftValue
			case "same ID wrong UID":
				copy := *ref
				copy.VarUid += "::foreign"
				tr.TryBody[3].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Arguments[0] = &copy
			case "extra effect":
				tr.TryBody = append(tr.TryBody[:3:3], append([]statements.Statement{tr.TryBody[0]}, tr.TryBody[3:]...)...)
			case "opaque scope":
				tr.TryBody = append([]statements.Statement{&statements.CustomStatement{}}, tr.TryBody...)
			case "cycle":
				tr.TryBody = append([]statements.Statement{tr}, tr.TryBody...)
			}
			view, got := RecoverCatchAllFinally(tr)
			want := variant == "same" || variant == "renamed"
			if got != want {
				t.Fatalf("lambda cleanup let proof=%v; want %v", got, want)
			}
			if got && (len(view.Cleanup) != 3 || len(view.TryBody) != 2 || len(tr.TryBody) != 5 || view.Cleanup[0] != stores[2]) {
				t.Fatal("must keep one canonical cleanup sequence without mutating original paths")
			}
		})
	}
}
