package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestTypedResourceFinallyRetainsPendingCompletions(t *testing.T) {
	for _, v := range []string{"original", "catch-all", "narrow catch", "unqualified Throwable", "missing rows", "protected close", "protected return", "unproved return", "different close", "different receiver", "unknown call PC", "negative close PC", "negative rethrow PC", "negative handler PC", "self-protected handler", "negative suppression PC", "self-protected suppression", "invisible resource", "wrong rethrow", "protected rethrow", "missing rethrow PC", "extra handler effect", "body budget"} {
		t.Run(v, func(t *testing.T) {
			_, tr := resourceTailFixture()
			normal := tr.TryBody[0].(*statements.IfStatement)
			call := normal.IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			resource := call.Object.(*values.JavaRef)
			resource.IsParam = true
			switch v {
			case "catch-all":
				tr.Handlers[0].CatchAll = true
			case "narrow catch":
				tr.Exception[0].ResetVarType(types.NewJavaClass("java.lang.Exception"))
			case "unqualified Throwable":
				tr.Exception[0].ResetVarType(types.NewJavaClass("Throwable"))
			case "missing rows":
				tr.Handlers[0].ProtectedRanges = nil
			case "protected close":
				tr.Handlers[0].ProtectedRanges[0][1] = 22
			case "protected return":
				tr.TryBody[1].(*statements.ReturnStatement).OriginPC = 19
			case "unproved return":
				tr.TryBody[1].(*statements.ReturnStatement).HasOriginPC = false
			case "different close":
				call.FunctionName = "another"
			case "different receiver":
				call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, resource.Type().Copy())
			case "unknown call PC":
				call.HasOriginPC = false
			case "negative close PC":
				call.OriginPC = -1
			case "negative rethrow PC":
				tr.CatchBodies[0][1].(*statements.CustomStatement).OriginPC = -1
			case "negative handler PC":
				tr.Handlers[0].EntryPC = -1
			case "self-protected handler":
				tr.Handlers[0].EntryPC = 15
			case "negative suppression PC":
				tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement).CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = -1
			case "self-protected suppression":
				tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement).CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 42
			case "invisible resource":
				resource.IsParam = false
			case "wrong rethrow":
				tr.CatchBodies[0][1].(*statements.CustomStatement).ThrownValue = resource
			case "protected rethrow":
				tr.CatchBodies[0][1].(*statements.CustomStatement).OriginPC = 19
			case "missing rethrow PC":
				tr.CatchBodies[0][1].(*statements.CustomStatement).HasOriginPC = false
			case "extra handler effect":
				tr.CatchBodies[0] = append(tr.CatchBodies[0], tr.TryBody[1])
			case "body budget":
				for i := 0; i < 600; i++ {
					tr.TryBody = append([]statements.Statement{statements.NewAssignStatement(resource, resource, false)}, tr.TryBody...)
				}
			}
			old := tr.CatchBodies[0][0]
			decl := prepareTypedResourceFinally(tr)
			if (decl != nil) != (v == "original") {
				t.Fatalf("prepared=%v", decl != nil)
			}
			if decl == nil {
				if tr.CatchBodies[0][0] != old {
					t.Fatal("refusal mutated handler")
				}
				return
			}
			if !decl.IsFirst || decl.IsDeclare || !values.IsNullLiteral(decl.JavaValue) {
				t.Fatal("completion carrier lost its actual typed null definition")
			}
			view, ok := RecoverCatchAllFinally(tr)
			if !ok || len(view.TryBody) != 1 || view.TryBody[0] != tr.TryBody[1] || len(view.Cleanup) != 1 || len(view.CatchBodies[0]) != 2 {
				t.Fatalf("lost original pending return: %+v %v", view, ok)
			}
			if tr.TryBody[0] != normal || len(tr.CatchBodies[0]) != 3 {
				t.Fatal("rendering view mutated original control tree")
			}
			branch := view.Cleanup[0].(*statements.IfStatement)
			inner := branch.IfBody[0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement)
			sink := inner.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			if !sameTryLocal(sink.Object, decl.LeftValue.(*values.JavaRef)) || sink.OriginPC != 51 {
				t.Fatal("primary lifetime or suppression invoke identity lost")
			}
		})
	}
}

func TestFinallyArrayStoreRequiresEveryOriginalEffectDomain(t *testing.T) {
	for _, v := range []string{"original", "canonical left", "missing store PC", "exclusive boundary", "missing array", "missing index", "missing value", "conflicting left", "array effect outside", "index effect outside", "value effect outside", "unknown array effect", "unknown index effect", "unknown value effect", "declaration store"} {
		t.Run(v, func(t *testing.T) {
			intType := types.NewJavaPrimer(types.JavaInteger)
			arrayType := types.NewJavaArrayType(intType)
			owner := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Effects"))
			owner.IsParam = true
			call := func(pc int, descriptor string, ret types.JavaType) *values.FunctionCallExpression {
				return &values.FunctionCallExpression{ClassName: "proof/Effects", FunctionName: "evaluate", Kind: values.InvokeVirtual, Object: owner, Descriptor: descriptor, HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType(descriptor, nil, ret)}
			}
			array, index, right := call(11, "()[I", arrayType), call(12, "()I", intType), call(13, "()I", intType)
			member := &values.JavaArrayMember{Object: array, Index: index}
			st := &statements.AssignStatement{ArrayMember: member, JavaValue: right, OriginPC: 14, HasOriginPC: true}
			switch v {
			case "canonical left":
				st.LeftValue = member
			case "missing store PC":
				st.HasOriginPC = false
			case "exclusive boundary":
				st.OriginPC = 20
			case "missing array":
				member.Object = nil
			case "missing index":
				member.Index = nil
			case "missing value":
				st.JavaValue = nil
			case "conflicting left":
				st.LeftValue = owner
			case "array effect outside":
				array.OriginPC = 20
			case "index effect outside":
				index.OriginPC = 20
			case "value effect outside":
				right.OriginPC = 20
			case "unknown array effect":
				array.HasOriginPC = false
			case "unknown index effect":
				index.HasOriginPC = false
			case "unknown value effect":
				right.HasOriginPC = false
			case "declaration store":
				st.IsDeclare = true
			}
			if got := finallyCoveredAssignment(st, func(pc int) bool { return pc >= 10 && pc < 20 }); got != (v == "original" || v == "canonical left") {
				t.Fatalf("array store domain accepted=%v", got)
			}
		})
	}
}
func TestTypedResourceFinallyReopensCarrierState(t *testing.T) {
	for _, v := range []string{"original", "missing initializer", "non-null initializer", "parameter carrier", "this carrier", "different capture", "body write", "changed catch type", "changed carrier type", "lost successful close", "late initializer"} {
		t.Run(v, func(t *testing.T) {
			_, tr := resourceTailFixture()
			resource := tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Object.(*values.JavaRef)
			resource.IsParam = true
			decl := prepareTypedResourceFinally(tr)
			if decl == nil {
				t.Fatal("fixture not admitted")
			}
			primary := decl.LeftValue.(*values.JavaRef)
			switch v {
			case "missing initializer":
				tr.EntryInitializers = nil
			case "non-null initializer":
				decl.JavaValue = resource
			case "parameter carrier":
				primary.IsParam = true
			case "this carrier":
				primary.IsThis = true
			case "different capture":
				tr.CatchBodies[0][0].(*statements.AssignStatement).JavaValue = resource
			case "body write":
				tr.TryBody = append([]statements.Statement{statements.NewAssignStatement(primary, values.JavaNull, false)}, tr.TryBody...)
			case "changed catch type":
				tr.Exception[0].ResetVarType(types.NewJavaClass("java.lang.Exception"))
			case "changed carrier type":
				primary.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "lost successful close":
				tr.TryBody = tr.TryBody[1:]
			case "late initializer":
				decl.HasOriginPC = true
				decl.OriginPC = 100
			}
			_, ok := recoverTypedResourceFinally(tr)
			if ok != (v == "original") {
				t.Fatalf("carrier state admitted=%v", ok)
			}
		})
	}
}

func TestResourcePrimaryWriteCensusKeepsSourceBindersSeparate(t *testing.T) {
	for _, v := range []string{"bare binder", "primary binder", "missing binder", "parameter binder", "folded write", "array write to primary"} {
		t.Run(v, func(t *testing.T) {
			tr := primaryResourceFixture()
			primary := tr.EntryInitializers[0].LeftValue.(*values.JavaRef)
			local := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			decl := statements.NewDeclareStatement(local)
			switch v {
			case "primary binder":
				decl.LeftValue = primary
			case "missing binder":
				decl.LeftValue = nil
			case "parameter binder":
				local.IsParam = true
			case "folded write":
				decl.IsDeclare = false
				decl.JavaValue = values.NewAssignmentExpression(primary, values.JavaNull, 3, nil)
			case "array write to primary":
				decl.IsDeclare = false
				decl.ArrayMember = &values.JavaArrayMember{Object: local, Index: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}
				decl.JavaValue = values.NewAssignmentExpression(primary, values.JavaNull, 3, nil)
			}
			tr.TryBody = append([]statements.Statement{decl}, tr.TryBody...)
			if writes := resourceWritesLocal([]statements.Statement{decl}, primary); writes != (v != "bare binder") {
				t.Fatalf("primary write=%v", writes)
			}
			if _, ok := recoverPrimaryResourceFinally(tr); ok != (v == "bare binder") {
				t.Fatalf("resource finally accepted=%v", ok)
			}
		})
	}
}
