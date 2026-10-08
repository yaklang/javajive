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

// These are source-certificate mutations, not JVM-valid bytecode fixtures.
// Actual original-first programs separately exercise the production decoder,
// branch structurer, declaration placement and compiler/JVM observations.
func TestNativeLambdaJoinedCaptureRequiresEveryRetainedOriginalDefinition(t *testing.T) {
	variants := []string{"original", "source rename", "logical source ID rename", "nested arms", "loop scoped", "loop scoped break", "loop scoped continue", "initialized unrelated loop", "reference null", "primitive null", "loop capture escapes", "loop uninitialized condition", "loop reassign", "transfer outside loop", "missing predecessor", "same path twice", "write after capture", "swapped RHS", "wrong original PC", "wrong original slot", "missing witness", "different ref", "different UID", "missing frontier", "extra frontier", "duplicate frontier", "missing declaration", "branch declaration", "duplicate declaration", "missing snapshot", "duplicate snapshot", "wrong factory", "wrong operand", "wrong source type", "changed snapshot seed", "wrong original read", "wrong original descriptor", "parameter", "custom", "loop write", "caught write", "opaque effect", "work", "memory", "depth", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			var typ types.JavaType = types.NewJavaPrimer(types.JavaLong)
			if variant == "reference null" {
				typ = types.NewJavaClass("java.lang.Object")
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			blank := statements.NewDeclareStatement(ref)
			write := func(pc int, n int64) *statements.AssignStatement {
				a := statements.NewAssignStatement(ref, values.NewJavaLiteral(n, typ), false)
				a.OriginPC, a.HasOriginPC = pc, true
				a.MarkOriginalLocalStore(pc, 3)
				return a
			}
			left, right := write(11, 17), write(19, -29)
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			branch := &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{left}, ElseBody: []statements.Statement{right}}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), ref, typ)
			snapshot.MarkOriginalDynamicOperand(31, 0, ref)
			snapshot.MarkOriginalDynamicOperandDeclaration(31, ref)
			capture := statements.NewAssignStatement(snapshot, ref, true)
			capture.OriginPC, capture.HasOriginPC = 31, true
			body := []statements.Statement{blank, branch, capture, statements.NewReturnStatement(snapshot)}
			operand := &nativeEnumSelectorProducer{pc: 30, opcode: core.OP_LLOAD_3, slot: 3, result: "J", local: &nativeEnumLocalRead{pc: 30, opcode: core.OP_LLOAD_3, slot: 3, storePC: -1, descriptor: "J", storePCs: []int{11, 19}}}
			pc, index := 31, 0
			var work *workbudget.Budget
			want := variant == "original" || variant == "source rename" || variant == "logical source ID rename" || variant == "nested arms" || variant == "loop scoped" || variant == "loop scoped break" || variant == "loop scoped continue" || variant == "initialized unrelated loop" || variant == "reference null"
			switch variant {
			case "loop scoped", "loop scoped break", "loop scoped continue":
				loop := &statements.WhileStatement{ConditionValue: condition, Body: body[:3]}
				if variant != "loop scoped" {
					kind := "break"
					if variant == "loop scoped continue" {
						kind = "continue"
					}
					loop.Body = append(loop.Body, statements.NewSourceTransferStatement(kind, ""))
				}
				body = []statements.Statement{loop}
			case "initialized unrelated loop":
				body = []statements.Statement{blank, branch, &statements.WhileStatement{ConditionValue: condition}, capture}
			case "reference null", "primitive null":
				nullWrite := statements.NewAssignStatement(ref, values.JavaNull, false)
				nullWrite.OriginPC, nullWrite.HasOriginPC = 11, true
				nullWrite.MarkOriginalLocalStore(11, 3)
				branch.IfBody = []statements.Statement{nullWrite}
				if variant == "reference null" {
					operand.opcode, operand.local.opcode = core.OP_ALOAD_3, core.OP_ALOAD_3
					operand.result, operand.local.descriptor = "Ljava/lang/Object;", "Ljava/lang/Object;"
				}
			case "loop capture escapes":
				body = []statements.Statement{&statements.WhileStatement{ConditionValue: condition, Body: body[:2]}, capture}
			case "loop uninitialized condition":
				body = []statements.Statement{&statements.WhileStatement{ConditionValue: ref, Body: body[:3]}}
			case "loop reassign":
				body = []statements.Statement{blank, branch, &statements.WhileStatement{ConditionValue: condition, Body: []statements.Statement{left}}, capture}
			case "transfer outside loop":
				branch.ElseBody = append(branch.ElseBody, statements.NewSourceTransferStatement("break", ""))
			case "source rename":
				ref.Id.SetName("renamed")
			case "logical source ID rename":
				ref.Id = utils.NewRootVariableId()
			case "nested arms":
				branch.ElseBody = []statements.Statement{&statements.IfStatement{Condition: condition, IfBody: []statements.Statement{right}, ElseBody: []statements.Statement{statements.NewThrowStatement(values.JavaNull)}}}
			case "missing predecessor":
				branch.ElseBody = nil
			case "same path twice":
				branch.IfBody = append(branch.IfBody, right)
			case "write after capture":
				body = append(body, write(39, 9))
			case "swapped RHS":
				left.JavaValue, right.JavaValue = right.JavaValue, left.JavaValue
			case "wrong original PC":
				left.OriginPC++
			case "wrong original slot":
				operand.slot, operand.local.slot = 4, 4
			case "missing witness":
				branch.IfBody = []statements.Statement{statements.NewAssignStatement(ref, left.JavaValue, false)}
			case "different ref":
				left.LeftValue = values.NewJavaRef(ref.Id, nil, typ)
			case "different UID":
				ref.VarUid += "other"
			case "missing frontier":
				operand.local.storePCs = []int{11, 29}
			case "extra frontier":
				operand.local.storePCs = append(operand.local.storePCs, 29)
			case "duplicate frontier":
				operand.local.storePCs = []int{11, 11}
			case "missing declaration":
				body = body[1:]
			case "branch declaration":
				body = body[1:]
				branch.IfBody = append([]statements.Statement{blank}, branch.IfBody...)
			case "duplicate declaration":
				body = append([]statements.Statement{statements.NewDeclareStatement(ref)}, body...)
			case "missing snapshot":
				body = body[:2]
			case "duplicate snapshot":
				body = append(body, capture)
			case "wrong factory":
				pc++
			case "wrong operand":
				index++
			case "wrong source type":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaDouble))
			case "changed snapshot seed":
				snapshot.Val = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "wrong original read":
				operand.local.pc++
			case "wrong original descriptor":
				operand.local.descriptor = "D"
			case "parameter":
				ref.IsParam = true
			case "custom":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "other" }, func() types.JavaType { return typ })
			case "loop write":
				body[1] = &statements.WhileStatement{ConditionValue: condition, Body: []statements.Statement{branch}}
			case "caught write":
				body[1] = &statements.TryCatchStatement{TryBody: []statements.Statement{left}, CatchBodies: [][]statements.Statement{{right}}}
			case "opaque effect":
				branch.Condition = values.NewCustomValue(func(*class_context.ClassContext) string { return "true" }, func() types.JavaType { return condition.Type() })
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "depth":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeLambdaJoinedLocalSource(operand, snapshot, pc, index, body, &class_context.ClassContext{}, work); got != want {
				t.Fatalf("source closed=%v want=%v", got, want)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("budget refusal was not sticky")
			}
		})
	}
}
