package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Source-certificate countermodels only. The original-first JVM bank checks
// actual slot reuse, decoded STOREs, source effects, ABI and compiler profiles.
func TestNativeLambdaReusedLocalSourceRequiresRetainedDefinitionAndSnapshot(t *testing.T) {
	for _, variant := range []string{"original", "source rename", "logical ID rename", "nested sequence", "null definition", "missing STORE", "unsealed STORE", "old definition only", "wrong STORE pc", "wrong slot", "different target ref", "changed RHS", "changed UID", "duplicate STORE", "overwrite before snapshot", "expression overwrite", "cross branch", "switch entry", "missing snapshot", "duplicate snapshot", "missing factory", "changed factory capture", "changed snapshot seed", "wrong snapshot type", "wrong factory pc", "wrong operand", "parameter", "multiple definitions", "opaque source", "opaque throw annotation", "return before snapshot", "throw before snapshot", "cycle", "large sequence", "depth limit", "work", "memory", "depth", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			write := func(pc int, text string, declare bool) *statements.AssignStatement {
				a := statements.NewAssignStatement(ref, values.NewJavaLiteral(text, types.NewJavaClass("java.lang.String")), declare)
				a.OriginPC, a.HasOriginPC = pc, true
				a.MarkOriginalLocalStore(pc, 3)
				return a
			}
			old, selected := write(5, "old", true), write(11, "selected", false)
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), ref, typ)
			snapshot.MarkOriginalDynamicOperand(21, 0, ref)
			snapshot.MarkOriginalDynamicOperandDeclaration(21, ref)
			capture := statements.NewAssignStatement(snapshot, ref, true)
			capture.OriginPC, capture.HasOriginPC = 21, true
			lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "()->captured" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Supplier") }, nil)
			lambda.Flag, lambda.HasOriginPC, lambda.OriginPC = "lambda", true, 21
			lambda.CapturesKnown, lambda.Captures = true, []values.JavaValue{snapshot}
			source := &nativeLambdaLocalCaptureSource{factoryPC: 21, hasFactoryPC: true, values: []values.JavaValue{snapshot}, body: []statements.Statement{old, selected, capture, statements.NewReturnStatement(lambda)}}
			operand := &nativeEnumSelectorProducer{pc: 20, opcode: core.OP_ALOAD_3, slot: 3, result: "Ljava/lang/Object;", local: &nativeEnumLocalRead{pc: 20, opcode: core.OP_ALOAD_3, slot: 3, descriptor: "Ljava/lang/Object;", storePC: 11, referenceAssignable: true}}
			index := 0
			var work *workbudget.Budget
			want := variant == "original" || variant == "source rename" || variant == "logical ID rename" || variant == "nested sequence" || variant == "null definition"
			switch variant {
			case "source rename":
				ref.Id.SetName("shifted")
				snapshot.Id.SetName("frozen")
			case "logical ID rename":
				ref.Id = utils.NewRootVariableId()
			case "nested sequence":
				source.body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: source.body}}
			case "null definition":
				a := statements.NewAssignStatement(ref, values.JavaNull, false)
				a.OriginPC, a.HasOriginPC = 11, true
				a.MarkOriginalLocalStore(11, 3)
				source.body[1] = a
			case "missing STORE":
				source.body = []statements.Statement{old, capture, statements.NewReturnStatement(lambda)}
			case "unsealed STORE":
				a := statements.NewAssignStatement(ref, selected.JavaValue, false)
				a.OriginPC, a.HasOriginPC = 11, true
				source.body[1] = a
			case "old definition only":
				operand.local.storePC = 5
			case "wrong STORE pc":
				selected.OriginPC++
			case "wrong slot":
				operand.slot, operand.local.slot = 4, 4
			case "different target ref":
				selected.LeftValue = values.NewJavaRef(ref.Id, nil, typ)
			case "changed RHS":
				selected.JavaValue = values.JavaNull
			case "changed UID":
				ref.VarUid += "changed"
			case "duplicate STORE":
				source.body = []statements.Statement{old, selected, selected, capture, statements.NewReturnStatement(lambda)}
			case "overwrite before snapshot":
				source.body = []statements.Statement{old, selected, write(17, "different", false), capture, statements.NewReturnStatement(lambda)}
			case "expression overwrite":
				change := &values.JavaExpression{Op: "=", Values: []values.JavaValue{ref, values.JavaNull}}
				source.body = []statements.Statement{old, selected, &statements.ExpressionStatement{Expression: change}, capture, statements.NewReturnStatement(lambda)}
			case "cross branch":
				source.body = []statements.Statement{old, &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{selected}}, capture, statements.NewReturnStatement(lambda)}
			case "switch entry":
				source.body = []statements.Statement{&statements.SwitchStatement{Value: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), Cases: []*statements.CaseItem{statements.NewCaseItem(0, source.body)}}}
			case "missing snapshot":
				source.body = []statements.Statement{old, selected, statements.NewReturnStatement(lambda)}
			case "duplicate snapshot":
				source.body = []statements.Statement{old, selected, capture, capture, statements.NewReturnStatement(lambda)}
			case "missing factory":
				source.body = source.body[:3]
			case "changed factory capture":
				lambda.Captures[0] = values.JavaNull
			case "changed snapshot seed":
				snapshot.Val = values.JavaNull
			case "wrong snapshot type":
				snapshot.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "wrong factory pc":
				source.factoryPC++
			case "wrong operand":
				index++
			case "parameter":
				ref.IsParam = true
			case "multiple definitions":
				operand.local.storePC, operand.local.storePCs = -1, []int{5, 11}
			case "opaque source":
				source.body = append([]statements.Statement{&statements.CustomStatement{}}, source.body...)
			case "opaque throw annotation":
				source.body = []statements.Statement{old, selected, &statements.CustomStatement{ThrownValue: values.JavaNull}, capture, statements.NewReturnStatement(lambda)}
			case "return before snapshot":
				source.body = []statements.Statement{old, selected, statements.NewReturnStatement(values.JavaNull), capture, statements.NewReturnStatement(lambda)}
			case "throw before snapshot":
				source.body = []statements.Statement{old, selected, statements.NewThrowStatement(values.JavaNull), capture, statements.NewReturnStatement(lambda)}
			case "cycle":
				branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				branch.IfBody = []statements.Statement{branch}
				source.body = append(source.body, branch)
			case "large sequence":
				prefix := make([]statements.Statement, 4097)
				for i := range prefix {
					prefix[i] = &statements.SourceAnchorStatement{}
				}
				source.body = append(prefix, source.body...)
			case "depth limit":
				for i := 0; i < 70; i++ {
					source.body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: source.body}}
				}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
				source.body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: source.body}}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeLambdaReusedLocalSource(operand, source, index, work); got != want {
				t.Fatalf("retained single definition=%v want=%v", got, want)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal must be sticky")
			}
		})
	}
}
