package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInstanceLambdaMetadataSeparatesReceiverWordsAndPhysicalRows(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original instance metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, instanceLambdaParameterFixture, "UncapturedLambdaNames", "none")
	raw := files["UncapturedLambdaNames.class"]
	for _, variant := range []string{"original", "static implementation", "static factory", "receiver operand", "foreign factory receiver", "missing factory receiver", "receiver clobber", "implementation receiver write", "capture write", "SAM write", "fake receiver row", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(raw))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{SourceCompiler: NativeJavac8, TargetSourceVersion: 8}
			var method *MemberInfo
			for _, candidate := range object.Methods {
				if candidate.AccessFlags == 0x1002 {
					method = candidate
				}
			}
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil || p.receiverWords != 1 || p.receiver == nil || p.receiver.slot != 0 || len(p.captures) != 1 || p.captures[0].slot != 1 || len(p.names) != 2 || p.names[0] != "next" || p.names[1] != "sample" || p.samSlot() != 2 || p.words != 3 || p.factoryCaptureCount() != 2 || p.samDescriptor != "(I)Z" {
				t.Fatal("original THIS/capture/SAM role partition")
			}
			switch variant {
			case "static implementation":
				method.AccessFlags = 0x100a
			case "static factory":
				p.factory.method.AccessFlags |= 8
			case "receiver operand":
				p.factory.code.Code[p.receiver.pc] = core.OP_ALOAD_1
			case "foreign factory receiver", "missing factory receiver":
				index := binary.BigEndian.Uint16(p.factory.code.Code[p.factory.pc+1 : p.factory.pc+3])
				dynamic := object.ConstantPool[index-1].(*ConstantInvokeDynamicInfo)
				nt := object.ConstantPool[dynamic.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				descriptor := "(Ljava/lang/Object;LUncapturedLambdaNames;)Ljava/util/function/IntPredicate;"
				if variant == "missing factory receiver" {
					descriptor = "(LUncapturedLambdaNames;)Ljava/util/function/IntPredicate;"
				}
				nt.DescriptorIndex = sourceBridgePoolString(t, object, descriptor)
			case "receiver clobber":
				p.factory.code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_ASTORE_0}, p.factory.code.Code...)
			case "implementation receiver write", "capture write", "SAM write":
				for _, attribute := range method.Attributes {
					if code, ok := attribute.(*CodeAttribute); ok {
						prefix := []byte{core.OP_ACONST_NULL, core.OP_ASTORE_0}
						if variant == "capture write" {
							prefix[1] = core.OP_ASTORE_1
						} else if variant == "SAM write" {
							prefix = []byte{core.OP_ICONST_0, core.OP_ISTORE_2}
						}
						code.Code = append(prefix, code.Code...)
					}
				}
			case "fake receiver row":
				p.table.Info = append([]byte{3, 0, 0, 0x10, 0x10}, p.table.Info[1:]...)
				p.table.Length = uint32(len(p.table.Info))
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			p, known = d.originalLambdaParameterMetadata(method, d.Work)
			if (known && p != nil) != (variant == "original") || !bytes.Equal(before, object.Bytes()) {
				t.Fatalf("closed=%v projection=%v work=%v", known, p != nil, d.Work)
			}
		})
	}
}

func TestInstanceLambdaReceiverRequiresActualPendingAndCompletedSource(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "foreign seed", "forged THIS", "wrong factory", "wrong ordinal", "changed seed", "foreign type", "materialized alias", "opaque source", "static context", "budget"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("fixture.Receiver")
			seed := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			seed.IsParam, seed.IsThis = true, true
			if variant != "forged THIS" {
				seed.MarkOriginalParameter(0)
				seed.MarkOriginalReceiver()
			}
			if variant == "foreign seed" {
				seed.ResetVarType(types.NewJavaClass("fixture.Other"))
			}
			snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, typ.Copy())
			pc, index := 8, 0
			if variant == "wrong factory" {
				pc++
			}
			if variant == "wrong ordinal" {
				index++
			}
			snapshot.MarkOriginalDynamicOperand(pc, index, seed)
			if variant == "changed seed" {
				snapshot.Val = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
			}
			if variant == "foreign type" {
				snapshot.ResetVarType(types.NewJavaClass("fixture.Other"))
			}
			if variant == "renamed" {
				snapshot.Id.SetName("sameObjectDifferentName")
			}
			object, method, code := &ClassObject{}, &MemberInfo{}, &CodeAttribute{Code: make([]byte, 16)}
			p := &lambdaParameterMetadata{receiverWords: 1, receiver: &nativeEnumSelectorProducer{opcode: core.OP_ALOAD_0, pc: 7, slot: 0, result: "Lfixture/Receiver;"}, factory: &lambdaSourceFactory{object: object, method: method, code: code, pc: 8}}
			ctx := &class_context.ClassContext{ClassName: "fixture.Receiver"}
			if variant == "static context" {
				ctx.IsStatic = true
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			wantPending := variant == "original" || variant == "renamed" || variant == "materialized alias" || variant == "opaque source"
			if got := lambdaParameterReceiverPendingClosed(p, snapshot, ctx, work); got != wantPending {
				t.Fatalf("pending receiver=%v want=%v", got, wantPending)
			}
			if variant == "materialized alias" {
				snapshot.MarkOriginalDynamicOperandDeclaration(pc, seed)
			} else {
				snapshot.StackVar = seed
				snapshot.MarkOriginalDynamicOperandInline(pc, seed)
			}
			if variant == "opaque source" {
				snapshot.CustomValue = &values.CustomValue{}
			}
			if got := lambdaParameterReceiverSourceClosed(p, snapshot, ctx, work); got != (variant == "original" || variant == "renamed") {
				t.Fatalf("completed receiver=%v", got)
			}
		})
	}
}
