package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaInlineReceiverKeepsExactFactoryAndOriginalThis(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "unmarked inline", "wrong factory", "wrong capture index", "foreign receiver type", "unproved receiver", "receiver slot one", "changed inline source", "opaque inline source", "declared snapshot", "budget"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("p.ReceiverOwner$1")
			seed := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			seed.IsParam, seed.IsThis = true, true
			slot := 0
			if variant == "receiver slot one" {
				slot = 1
			}
			if variant != "unproved receiver" {
				seed.MarkOriginalParameter(slot)
				seed.MarkOriginalReceiver()
			}
			if variant == "foreign receiver type" {
				seed.ResetVarType(types.NewJavaClass("p.OtherOwner$1"))
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, typ.Copy())
			pc, index := 8, 0
			if variant == "wrong factory" {
				pc++
			}
			if variant == "wrong capture index" {
				index++
			}
			snapshot.MarkOriginalDynamicOperand(pc, index, seed)
			if variant == "declared snapshot" {
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, seed)
			}
			snapshot.StackVar = seed
			if variant != "unmarked inline" {
				snapshot.MarkOriginalDynamicOperandInline(pc, seed)
			}
			if variant == "renamed" {
				snapshot.Id.SetName("different")
			}
			if variant == "changed inline source" {
				snapshot.StackVar = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			}
			if variant == "opaque inline source" {
				snapshot.CustomValue = &values.CustomValue{}
			}
			method, code := &MemberInfo{}, &CodeAttribute{Code: make([]byte, 16)}
			site := &nativeLambdaLocalCaptureSite{method: method, code: code, pc: 8, operands: []*nativeEnumSelectorProducer{{opcode: core.OP_ALOAD_0, pc: 7, slot: 0, result: "Lp/ReceiverOwner$1;"}}}
			source := &nativeLambdaLocalCaptureSource{method: method, code: code, factoryPC: 8, hasFactoryPC: true, context: class_context.ClassContext{ClassName: "p.ReceiverOwner$1"}, values: []values.JavaValue{snapshot}}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			want := variant == "original" || variant == "renamed"
			if got := nativeLambdaLocalCaptureSourceClosed(site, source, work); got != want {
				t.Fatalf("inline receiver=%v expected=%v", got, want)
			}
		})
	}
}
