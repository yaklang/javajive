package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInterfaceForeignInitializerRequiresOriginalNonconstantDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, `class FieldOrigin {static final int VALUE=Integer.parseInt("37");} interface FieldConsumer {int VALUE=FieldOrigin.VALUE;}`)
	for _, variant := range []string{"control", "mutable", "volatile", "unknown owner", "wrong owner identity", "missing declaration", "duplicate declaration", "ConstantValue", "instance field", "private field", "synthetic field", "changed descriptor", "missing witness", "changed symbolic owner", "changed name", "changed read descriptor", "changed PC", "handle alias", "read after store", "wrong opcode", "wrong CP kind", "no code", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(files["FieldConsumer.class"]))
			owner := mustParseLowering(t, bytes.Clone(files["FieldOrigin.class"]))
			d := NewClassObjectDumper(object)
			var code *CodeAttribute
			for _, m := range object.Methods {
				n, _ := sourceBridgeUTF8(object, m.NameIndex)
				if n == "<clinit>" {
					code = constructorLoweringCode(m)
				}
			}
			if code == nil || code.Code[0] != core.OP_GETSTATIC {
				t.Fatal("original read")
			}
			writes, e := d.interfaceWrites(code)
			if e != nil || len(writes) != 1 {
				t.Fatal("original store", e)
			}
			index := int(code.Code[1])<<8 | int(code.Code[2])
			member := GetValueFromCP(object.ConstantPool, index).(*values.JavaClassMember)
			field := *member
			field.OriginPC, field.HasOriginPC = 0, true
			field.MarkOriginalFieldRead(member, 0)
			target := owner.Fields[0]
			storePC := writes[0].pc
			switch variant {
			case "mutable":
				target.AccessFlags &^= 0x10
			case "volatile":
				target.AccessFlags = 0x48
			case "wrong owner identity":
				owner.ConstantPool[owner.ThisClass-1].(*ConstantClassInfo).NameIndex = uint16(owner.ConstantPoolManager.AddUtf8Info("Foreign"))
			case "missing declaration":
				owner.Fields = nil
			case "duplicate declaration":
				owner.Fields = append(owner.Fields, target)
			case "ConstantValue":
				target.Attributes = append(target.Attributes, &ConstantValueAttribute{})
			case "instance field":
				target.AccessFlags &^= 8
			case "private field":
				target.AccessFlags |= 2
			case "synthetic field":
				target.AccessFlags |= 0x1000
			case "changed descriptor":
				target.DescriptorIndex = uint16(owner.ConstantPoolManager.AddUtf8Info("J"))
			case "missing witness":
				field = *member
				field.OriginPC, field.HasOriginPC = 0, true
			case "changed symbolic owner":
				field.Name = "Other"
			case "changed name":
				field.Member = "Other"
			case "changed read descriptor":
				field.Description = "J"
			case "changed PC":
				field.OriginPC++
			case "handle alias":
				field.RefKind = 2
			case "read after store":
				storePC = 0
			case "wrong opcode":
				code.Code[0] = core.OP_PUTSTATIC
			case "wrong CP kind":
				ref := object.ConstantPool[index-1].(*ConstantFieldrefInfo)
				object.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "no code":
				code = nil
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				if n == "FieldOrigin" && variant != "unknown owner" {
					return owner.Bytes(), true
				}
				return nil, false
			}
			want := variant == "control" || variant == "mutable" || variant == "volatile"
			if accepted := d.interfaceInitializerForeignNonconstantRead(&field, code, storePC); accepted != want {
				t.Fatalf("accepted=%v", accepted)
			}
		})
	}
}
