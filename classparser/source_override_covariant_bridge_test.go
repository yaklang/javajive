package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialProjectedCovarianceRequiresOriginalDispatchBridge(t *testing.T) {
	files := nativeCompileDebugClasses(t, projectedCovariantOverrideFixture, "none")
	for _, variant := range []string{"control", "missing bridge", "duplicate bridge", "not synthetic", "static bridge", "visibility changed", "foreign receiver", "static invocation", "extra instruction", "recursive target", "foreign target", "changed parameters", "same descriptor", "different method", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, bytes.Clone(files["SuffixOwner$Sub.class"]))
			var target, bridge *MemberInfo
			for _, m := range child.Methods {
				n, _ := sourceBridgeUTF8(child, m.NameIndex)
				if n == "apply" {
					if m.AccessFlags&0x40 != 0 {
						bridge = m
					} else {
						target = m
					}
				}
			}
			if target == nil || bridge == nil {
				t.Fatal("fixture original bridge")
			}
			parentDesc, _ := sourceBridgeUTF8(child, bridge.DescriptorIndex)
			code := constructorLoweringCode(bridge)
			decoder := core.NewDecompiler(code.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("original instructions")
			}
			invoke := -1
			var cp uint16
			for _, op := range constructorMotionOps(decoder) {
				if op.Instr.OpCode == core.OP_INVOKEVIRTUAL {
					invoke = int(op.CurrentOffset)
					cp, _ = nativeEnumCPIndex(op)
				}
			}
			if invoke < 0 {
				t.Fatal("original virtual call")
			}
			var work *workbudget.Budget
			switch variant {
			case "missing bridge":
				for i, m := range child.Methods {
					if m == bridge {
						child.Methods = append(child.Methods[:i], child.Methods[i+1:]...)
						break
					}
				}
			case "duplicate bridge":
				child.Methods = append(child.Methods, bridge)
			case "not synthetic":
				bridge.AccessFlags &^= 0x1000
			case "static bridge":
				bridge.AccessFlags |= StaticFlag
			case "visibility changed":
				bridge.AccessFlags ^= 1
			case "foreign receiver":
				code.Code[0] = 0x2b
			case "static invocation":
				code.Code[invoke] = 0xb8
			case "extra instruction":
				code.Code = append(code.Code, 0)
			case "recursive target":
				ref := child.ConstantPool[cp-1].(*ConstantMethodrefInfo)
				nt := child.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = bridge.DescriptorIndex
			case "foreign target":
				child.ConstantPool[cp-1].(*ConstantMethodrefInfo).ClassIndex = child.SuperClass
			case "changed parameters":
				parentDesc = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "same descriptor":
				parentDesc, _ = sourceBridgeUTF8(child, target.DescriptorIndex)
			case "different method":
				target.NameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("other"))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if accepted := projectedOverrideCovariantBridge(child, target, parentDesc, work); accepted != (variant == "control") {
				t.Fatalf("accepted=%v", accepted)
			}
		})
	}
}
