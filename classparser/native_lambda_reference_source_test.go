package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaReferenceSourceRequiresOriginalWebAndBothAssignments(t *testing.T) {
	for _, variant := range []string{"original", "same ref", "shared source ID change", "null branch", "source snapshot subtype", "snapshot too narrow", "no web", "same public fields", "foreign web", "missing hierarchy", "incomplete hierarchy", "unassignable local", "unassignable capture", "one wrong producer", "one unknown producer", "changed original RHS", "changed source ID", "changed UID", "old selector boundary"} {
		t.Run(variant, func(t *testing.T) {
			base, contract := types.NewJavaClass("JoinedSourceBase"), types.NewJavaClass("JoinedSourceContract")
			id := utils.NewRootVariableId()
			first, second := values.NewJavaRef(id, nil, base), values.NewJavaRef(id, nil, base)
			second.VarUid = first.VarUid
			if variant != "no web" {
				values.MarkOriginalLocalWeb([]*values.JavaRef{first, second})
			}
			switch variant {
			case "same ref":
				second = first
			case "same public fields", "foreign web":
				second = values.NewJavaRef(id, nil, base)
				second.VarUid = first.VarUid
				if variant == "foreign web" {
					third := values.NewJavaRef(id, nil, base)
					third.VarUid = first.VarUid
					values.MarkOriginalLocalWeb([]*values.JavaRef{second, third})
				}
			case "unassignable local":
				first.ResetVarType(types.NewJavaClass("JoinedSourceRival"))
				second.ResetVarType(types.NewJavaClass("JoinedSourceRival"))
			}
			seed := func(name string) values.JavaValue {
				return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
			}
			leftSeed, rightSeed := seed("JoinedSourceLeft"), seed("JoinedSourceRight")
			switch variant {
			case "null branch":
				leftSeed = values.JavaNull
			case "one wrong producer":
				rightSeed = seed("JoinedSourceRival")
			case "one unknown producer":
				rightSeed = seed("Unknown")
			}
			write := func(ref *values.JavaRef, pc int, value values.JavaValue) *statements.AssignStatement {
				a := statements.NewAssignStatement(ref, value, false)
				a.OriginPC, a.HasOriginPC = pc, true
				a.MarkOriginalLocalStore(pc, 3)
				return a
			}
			left, right := write(first, 11, leftSeed), write(second, 19, rightSeed)
			if variant == "changed original RHS" {
				right.JavaValue = seed("JoinedSourceRight")
			}
			if variant == "shared source ID change" {
				first.Id = utils.NewRootVariableId()
				second.Id = first.Id
			}
			if variant == "changed source ID" {
				second.Id = utils.NewRootVariableId()
			}
			if variant == "changed UID" {
				second.VarUid += "other"
			}
			branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{left}, ElseBody: []statements.Statement{right}}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), first, contract)
			if variant == "source snapshot subtype" {
				snapshot.ResetVarType(base)
			}
			if variant == "snapshot too narrow" {
				snapshot.ResetVarType(types.NewJavaClass("JoinedSourceLeft"))
			}
			snapshot.MarkOriginalDynamicOperand(31, 0, first)
			snapshot.MarkOriginalDynamicOperandDeclaration(31, first)
			capture := statements.NewAssignStatement(snapshot, first, true)
			capture.OriginPC, capture.HasOriginPC = 31, true
			body := []statements.Statement{statements.NewDeclareStatement(first), branch, capture}
			operand := &nativeEnumSelectorProducer{pc: 30, opcode: core.OP_ALOAD_3, slot: 3, result: "LJoinedSourceContract;", local: &nativeEnumLocalRead{pc: 30, opcode: core.OP_ALOAD_3, slot: 3, storePC: -1, descriptor: "LJoinedSourceContract;", storePCs: []int{11, 19}, referenceAssignable: true}}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
				parents, known := map[string][]string{"JoinedSourceLeft": {"JoinedSourceBase"}, "JoinedSourceRight": {"JoinedSourceBase"}, "JoinedSourceBase": {"JoinedSourceContract"}, "JoinedSourceRival": {"JoinedSourceContract"}, "JoinedSourceContract": {}}[name]
				if variant == "unassignable capture" && name == "JoinedSourceBase" {
					parents = nil
				}
				return callbinding.Class{Name: name, Parents: parents, ParentsComplete: known && variant != "incomplete hierarchy"}, known
			}}
			if variant == "missing hierarchy" {
				ctx.InvocationMetadata = nil
			}
			if variant == "old selector boundary" {
				operand.local.referenceAssignable = false
			}
			want := variant == "original" || variant == "same ref" || variant == "shared source ID change" || variant == "null branch" || variant == "source snapshot subtype"
			if got := nativeLambdaJoinedLocalSource(operand, snapshot, 31, 0, body, ctx, nil); got != want {
				t.Fatalf("source assignments and original binding=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeLambdaReferenceSingleStoreRequiresOriginalSeedAndSnapshot(t *testing.T) {
	for _, variant := range []string{"original", "source snapshot subtype", "rename", "missing declaration", "wrong STORE PC", "wrong slot", "wrong LOAD PC", "wrong opcode", "wrong descriptor", "joined frontier", "old boundary", "changed seed", "too narrow local", "too narrow snapshot", "wrong capture", "missing hierarchy", "wrong hierarchy", "wrong factory", "wrong position", "no snapshot witness", "no snapshot declaration", "parameter", "custom", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			base, contract := types.NewJavaClass("SingleSourceBase"), types.NewJavaClass("SingleSourceContract")
			seed := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("SingleSourceLeaf"))
			ref := values.NewJavaRef(utils.NewRootVariableId(), seed, base)
			if variant != "missing declaration" {
				ref.MarkOriginalLocalDeclaration(11, 3, seed)
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), ref, contract)
			if variant != "no snapshot witness" {
				snapshot.MarkOriginalDynamicOperand(31, 0, ref)
			}
			if variant != "no snapshot declaration" {
				snapshot.MarkOriginalDynamicOperandDeclaration(31, ref)
			}
			operand := &nativeEnumSelectorProducer{pc: 30, opcode: core.OP_ALOAD_3, slot: 3, result: "LSingleSourceContract;", local: &nativeEnumLocalRead{pc: 30, opcode: core.OP_ALOAD_3, slot: 3, storePC: 11, descriptor: "LSingleSourceContract;", referenceAssignable: true}}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) {
				parents, known := map[string][]string{"SingleSourceLeaf": {"SingleSourceBase"}, "SingleSourceBase": {"SingleSourceContract"}, "SingleSourceContract": {}}[name]
				if variant == "wrong hierarchy" && name == "SingleSourceBase" {
					parents = nil
				}
				return callbinding.Class{Name: name, Parents: parents, ParentsComplete: known}, known
			}}
			pc, index := 31, 0
			var work *workbudget.Budget
			switch variant {
			case "source snapshot subtype":
				snapshot.ResetVarType(base)
			case "rename":
				ref.Id.SetName("renamed")
			case "wrong STORE PC":
				operand.local.storePC++
			case "wrong slot":
				operand.slot, operand.local.slot = 4, 4
			case "wrong LOAD PC":
				operand.local.pc++
			case "wrong opcode":
				operand.local.opcode = core.OP_ALOAD_2
			case "wrong descriptor":
				operand.local.descriptor = "LSingleSourceBase;"
			case "joined frontier":
				operand.local.storePCs = []int{11, 19}
			case "old boundary":
				operand.local.referenceAssignable = false
			case "changed seed":
				ref.Val = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("SingleSourceLeaf"))
			case "too narrow local":
				ref.ResetVarType(types.NewJavaClass("OtherLeaf"))
			case "too narrow snapshot":
				snapshot.ResetVarType(types.NewJavaClass("SingleSourceLeaf"))
			case "wrong capture":
				operand.result, operand.local.descriptor = "LOtherContract;", "LOtherContract;"
			case "missing hierarchy":
				ctx.InvocationMetadata = nil
			case "wrong factory":
				pc++
			case "wrong position":
				index++
			case "parameter":
				ref.IsParam = true
			case "custom":
				ref.CustomValue = &values.CustomValue{}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "source snapshot subtype" || variant == "rename"
			if got := nativeLambdaLocalCaptureValue(operand, snapshot, pc, index, ctx, work); got != want {
				t.Fatalf("single original STORE and snapshot=%v want=%v", got, want)
			}
		})
	}
}
