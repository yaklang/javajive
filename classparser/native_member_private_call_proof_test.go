package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func nativePrivateProducerFixture() string {
	fixture := strings.Replace(nativeMemberArraySuperFixture, "Object prepare(Object seed,long n)", "private Object prepare(Object seed,long n)", 1)
	fixture = strings.Replace(fixture, "class Child extends ArraySuperParent{", "Object peek(Child child){return child.recorded;}class Child extends ArraySuperParent{private Object recorded;", 1)
	return strings.Replace(fixture, "prepare(seed,n)});}", "prepare(seed,n)});recorded=token;}", 1)
}

func TestNativeMemberPrivateCallRequiresOriginalNonvirtualPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, nativePrivateProducerFixture(), debug)
		for _, variant := range []string{"original", "not synthetic", "not static", "public target", "static target", "native target", "abstract target", "wrong receiver slot", "wrong wide slot", "extra field effect", "wrong invocation kind", "wrong return", "max locals", "max stack", "handler", "missing checked contract", "different checked contract", "generic target", "missing target", "target descriptor", "noncanonical name", "instruction version", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(append([]byte(nil), files["ArraySuperOwner.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				var bridge, target *MemberInfo
				var code *CodeAttribute
				var throws *ExceptionsAttribute
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "access$100" {
						bridge = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
							if a, ok := a.(*ExceptionsAttribute); ok {
								throws = a
							}
						}
					}
					if n == "prepare" {
						target = m
					}
				}
				if bridge == nil || target == nil || code == nil || throws == nil {
					t.Fatal("original private checked packet")
				}
				var work *workbudget.Budget
				switch variant {
				case "not synthetic":
					bridge.AccessFlags &^= 0x1000
				case "not static":
					bridge.AccessFlags &^= 8
				case "public target":
					target.AccessFlags = (target.AccessFlags &^ 2) | 1
				case "static target":
					target.AccessFlags |= 8
				case "native target":
					target.AccessFlags |= 0x100
				case "abstract target":
					target.AccessFlags |= 0x400
				case "wrong receiver slot":
					code.Code[0] = core.OP_ALOAD_1
				case "wrong wide slot":
					code.Code[2] = core.OP_LLOAD_1
				case "extra field effect":
					idx := 0
					for i, constant := range obj.ConstantPool {
						field, ok := constant.(*ConstantFieldrefInfo)
						if !ok {
							continue
						}
						owner, known := sourceBridgeClassName(obj, field.ClassIndex)
						if known && owner == "ArraySuperEffects" {
							idx = i + 1
							break
						}
					}
					if idx == 0 {
						t.Fatal("original effect field")
					}
					code.Code = append([]byte{core.OP_GETSTATIC, byte(idx >> 8), byte(idx), core.OP_POP}, code.Code...)
				case "wrong invocation kind":
					code.Code[3] = core.OP_INVOKEVIRTUAL
				case "wrong return":
					code.Code[len(code.Code)-1] = core.OP_IRETURN
				case "max locals":
					code.MaxLocals++
				case "max stack":
					code.MaxStack++
				case "handler":
					code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 7, HandlerPc: 0})
				case "missing checked contract":
					for i, a := range bridge.Attributes {
						if a == throws {
							bridge.Attributes = append(bridge.Attributes[:i], bridge.Attributes[i+1:]...)
							break
						}
					}
				case "different checked contract":
					throws.ExceptionIndexTable = append([]uint16(nil), throws.ExceptionIndexTable...)
					throws.ExceptionIndexTable[0] = obj.ThisClass
				case "generic target":
					target.Attributes = append(target.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "(Ljava/lang/Object;J)Ljava/lang/Object;")})
				case "missing target":
					target.NameIndex = sourceBridgePoolString(t, obj, "unrelatedProducer")
				case "target descriptor":
					target.DescriptorIndex = sourceBridgePoolString(t, obj, "(Ljava/lang/String;J)Ljava/lang/Object;")
				case "noncanonical name":
					bridge.NameIndex = sourceBridgePoolString(t, obj, "access$0100")
				case "instruction version":
					obj.MajorVersion = 61
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				got := nativeMemberPrivateCallProof(obj, bridge, work)
				if (got != nil) != (variant == "original") {
					t.Fatalf("private call packet=%#v", got)
				}
			})
		}
	}
}
