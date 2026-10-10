package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousInitializerTailRequiresPhysicalStaticAllocation(t *testing.T) {
	files := nativeCompileIndependentRootFixture(t, "StaticTailOwner", nativeMemberStaticTailFixture, "none", "7")
	variants := []string{"original", "foreign child", "named method metadata", "no initializer", "instance initializer", "duplicate initializer", "missing code", "duplicate code", "missing allocation", "missing dup", "wrong constructor owner", "wrong descriptor", "wrong CP tag", "missing invocation", "second allocation", "foreign method allocation", "budget", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			owner, err := Parse(append([]byte(nil), files["StaticTailOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(append([]byte(nil), files["StaticTailOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var initializer *MemberInfo
			var code *CodeAttribute
			for _, m := range owner.Methods {
				name, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if name == "<clinit>" {
					initializer = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if initializer == nil || code == nil || len(code.Code) != 11 || code.Code[0] != core.OP_NEW || code.Code[3] != core.OP_DUP || code.Code[4] != core.OP_INVOKESPECIAL {
				t.Fatal("original initializer allocation packet")
			}
			index := int(binary.BigEndian.Uint16(code.Code[5:7]))
			ref := owner.ConstantPool[index-1].(*ConstantMethodrefInfo)
			cp := NewConstantPoolWithConstant(&owner.ConstantPool)
			var work *workbudget.Budget
			switch variant {
			case "foreign child":
				child.ThisClass = uint16(NewConstantPoolWithConstant(&child.ConstantPool).AddNewClassInfo("ForeignTail"))
			case "named method metadata":
				pool := NewConstantPoolWithConstant(&child.ConstantPool)
				method := uint16(pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info("outside")), DescriptorIndex: uint16(pool.AddUtf8Info("()Ljava/lang/Object;"))}))
				for _, a := range child.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(raw.Info[2:], method)
					}
				}
			case "no initializer":
				initializer.NameIndex = uint16(cp.AddUtf8Info("outside"))
			case "instance initializer":
				initializer.NameIndex = uint16(cp.AddUtf8Info("<init>"))
				initializer.AccessFlags = 0
			case "duplicate initializer":
				owner.Methods = append(owner.Methods, initializer)
			case "missing code":
				initializer.Attributes = nil
			case "duplicate code":
				initializer.Attributes = append(initializer.Attributes, code)
			case "missing allocation":
				code.Code[0] = core.OP_NOP
			case "missing dup":
				code.Code[3] = core.OP_POP
			case "wrong constructor owner":
				ref.ClassIndex = uint16(cp.AddNewClassInfo("java/lang/Object"))
			case "wrong descriptor":
				ref.NameAndTypeIndex = uint16(cp.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(cp.AddUtf8Info("<init>")), DescriptorIndex: uint16(cp.AddUtf8Info("(I)V"))}))
			case "wrong CP tag":
				owner.ConstantPool[index-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "missing invocation":
				code.Code[4] = core.OP_NOP
			case "second allocation":
				packet := append(append([]byte(nil), code.Code[:7]...), core.OP_POP)
				code.Code = append(append(code.Code[:10:10], packet...), core.OP_RETURN)
			case "foreign method allocation":
				copy := *initializer
				copy.NameIndex = uint16(cp.AddUtf8Info("outside"))
				owner.Methods = append(owner.Methods, &copy)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousStandaloneInitializerAllocation(owner, child, work); got != (variant == "original") {
				t.Fatalf("original static allocation admitted=%v", got)
			}
		})
	}
}

func TestNativeAnonymousInitializerHeaderRequiresProvedIndependentScope(t *testing.T) {
	files := nativeCompileIndependentRootFixture(t, "StaticTailOwner", nativeMemberStaticTailFixture, "none", "7")
	for _, variant := range []string{"original", "public class", "nonfinal", "modern metadata", "unknown owner", "wrong owner", "instance allocation", "no resolver", "nil dumper", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			owner, err := Parse(append([]byte(nil), files["StaticTailOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(append([]byte(nil), files["StaticTailOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			c := &ClassObjectDumper{obj: child}
			known := true
			c.foldSiblingResolver = func(name string) ([]byte, bool) { return owner.Bytes(), known && name == "StaticTailOwner" }
			switch variant {
			case "public class":
				child.AccessFlags |= 1
			case "nonfinal":
				child.AccessFlags &^= 0x10
			case "modern metadata":
				child.MajorVersion = 65
			case "unknown owner":
				known = false
			case "wrong owner":
				owner.ThisClass = uint16(NewConstantPoolWithConstant(&owner.ConstantPool).AddNewClassInfo("ForeignInitializer"))
			case "instance allocation":
				for _, m := range owner.Methods {
					name, _ := sourceBridgeUTF8(owner, m.NameIndex)
					if name == "<clinit>" {
						m.AccessFlags = 0
					}
				}
			case "no resolver":
				c.foldSiblingResolver = nil
			case "nil dumper":
				c = nil
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := c.nativeAnonymousIndependentInitializerHeader(); got != (variant == "original") {
				t.Fatalf("original header policy=%v", got)
			}
		})
	}
}
