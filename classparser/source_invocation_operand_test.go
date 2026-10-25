package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInvocationOperandRequiresCompleteInterfaceFormat(t *testing.T) {
	files := nativeCompileClasses(t, `interface InvocationPort {long run(long n,double d,Object value);} class InvocationSource {static long call(InvocationPort port,long n,double d,Object value){return port.run(n,d,value);}}`)
	for _, variant := range []string{"control", "array slots", "max instance slots", "too many slots", "missing receiver", "zero count", "excess count", "reserved byte", "short operand", "long operand", "zero index", "wrong constant kind", "invalid descriptor", "constructor name", "wide prefix", "budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, files["InvocationSource.class"])
			var invoke *core.OpCode
			for _, method := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, method.NameIndex); name != "call" {
					continue
				}
				decoder := core.NewDecompiler(constructorLoweringCode(method).Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				for _, op := range constructorMotionOps(decoder) {
					if op.Instr.OpCode == core.OP_INVOKEINTERFACE {
						invoke = op
					}
				}
			}
			if invoke == nil || len(invoke.Data) != 4 || invoke.Data[2] != 6 {
				t.Fatal("real wide interface fixture")
			}
			invoke.Data = bytes.Clone(invoke.Data)
			index := core.Convert2bytesToInt(invoke.Data[:2])
			var work *workbudget.Budget
			switch variant {
			case "missing receiver":
				invoke.Data[2]--
			case "zero count":
				invoke.Data[2] = 0
			case "excess count":
				invoke.Data[2]++
			case "reserved byte":
				invoke.Data[3] = 1
			case "short operand":
				invoke.Data = invoke.Data[:2]
			case "long operand":
				invoke.Data = append(invoke.Data, 0)
			case "zero index":
				invoke.Data[0], invoke.Data[1] = 0, 0
			case "wrong constant kind":
				ref := obj.ConstantPool[index-1].(*ConstantInterfaceMethodrefInfo)
				obj.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "invalid descriptor", "constructor name", "array slots", "max instance slots", "too many slots":
				ref := obj.ConstantPool[index-1].(*ConstantInterfaceMethodrefInfo)
				pair := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if variant == "constructor name" {
					pair.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<init>"))
				} else {
					descriptor := "(V)J"
					switch variant {
					case "array slots":
						descriptor = "([J[[DLjava/lang/Object;)J"
						invoke.Data[2] = 4
					case "max instance slots":
						descriptor = "(" + strings.Repeat("I", 254) + ")J"
						invoke.Data[2] = 255
					case "too many slots":
						descriptor = "(" + strings.Repeat("I", 255) + ")J"
						invoke.Data[2] = 255
					}
					pair.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(descriptor))
				}
			case "wide prefix":
				invoke.IsWide = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, ok := sourceInvocationCPIndex(obj, invoke, work)
			if ok != (variant == "control" || variant == "array slots" || variant == "max instance slots") || ok && got != index {
				t.Fatal("unproved invocation operand", variant, got, ok)
			}
		})
	}
}
