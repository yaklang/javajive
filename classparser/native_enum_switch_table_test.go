package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeEnumSwitchTableRequiresCompleteOriginalPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, nativeEnumSwitchFamilySources("SwitchFamilyOwner"), debug, "8")
		var original []byte
		for _, raw := range files {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			if obj.AccessFlags == 0x1020 {
				if original != nil {
					t.Fatal("ambiguous test artifact")
				}
				original = raw
			}
		}
		if original == nil {
			t.Fatal("original compiler table missing")
		}
		for _, variant := range []string{"original", "ordinary class", "interface", "superclass", "nil field", "field flags", "field descriptor", "duplicate field", "extra method", "duplicate code", "stack", "locals", "extra effect", "wrong allocation type", "foreign store owner", "wrong ordinal method", "wrong constant descriptor", "zero key", "sparse positive key", "duplicate key", "wrong store", "branch enters handler", "wrong catch", "wrong catch start", "wrong catch end", "wrong catch target", "extra handler", "missing handler", "work", "memory", "canceled", "extended frames", "missing frames", "wrong stack type", "wrong frame offset", "trailing frames", "duplicate frames", "named debug locals", "wrong debug locals length"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(append([]byte(nil), original...))
				if e != nil {
					t.Fatal(e)
				}
				m := obj.Methods[0]
				code := m.Attributes[0].(*CodeAttribute)
				var work *workbudget.Budget
				switch variant {
				case "ordinary class":
					obj.AccessFlags &^= 0x1000
				case "interface":
					obj.Interfaces = []uint16{obj.ThisClass}
				case "superclass":
					obj.SuperClass = obj.ThisClass
				case "nil field":
					obj.Fields[0] = nil
				case "field flags":
					obj.Fields[0].AccessFlags &^= 0x10
				case "field descriptor":
					obj.Fields[0].DescriptorIndex = sourceBridgePoolString(t, obj, "[J")
				case "duplicate field":
					obj.Fields = append(obj.Fields, obj.Fields[0])
				case "extra method":
					obj.Methods = append(obj.Methods, m)
				case "duplicate code":
					m.Attributes = append(m.Attributes, code)
				case "stack":
					code.MaxStack++
				case "locals":
					code.MaxLocals++
				case "extra effect":
					code.Code = append([]byte{core.OP_ICONST_0, core.OP_POP}, code.Code...)
				case "wrong allocation type":
					code.Code[5] = 11
				case "foreign store owner":
					ref := obj.ConstantPool[(int(code.Code[7])<<8|int(code.Code[8]))-1].(*ConstantFieldrefInfo)
					ref.ClassIndex = obj.SuperClass
				case "wrong ordinal method":
					ref := obj.ConstantPool[(int(code.Code[16])<<8|int(code.Code[17]))-1].(*ConstantMethodrefInfo)
					nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					nt.NameIndex = sourceBridgePoolString(t, obj, "hashCode")
				case "wrong constant descriptor":
					ref := obj.ConstantPool[(int(code.Code[13])<<8|int(code.Code[14]))-1].(*ConstantFieldrefInfo)
					nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					nt.DescriptorIndex = sourceBridgePoolString(t, obj, "Ljava/lang/Object;")
				case "sparse positive key":
					code.Code[18] = core.OP_ICONST_5
				case "zero key":
					code.Code[18] = core.OP_ICONST_0
				case "duplicate key":
					code.Code[33] = core.OP_ICONST_1
				case "wrong store":
					code.Code[19] = core.OP_AASTORE
				case "branch enters handler":
					code.Code[22] = 3
				case "wrong catch":
					cp := NewConstantPoolWithConstant(&obj.ConstantPool)
					code.ExceptionTable[0].CatchType = uint16(cp.AddNewClassInfo("java/lang/RuntimeException"))
				case "wrong catch start":
					code.ExceptionTable[0].StartPc++
				case "wrong catch end":
					code.ExceptionTable[0].EndPc--
				case "wrong catch target":
					code.ExceptionTable[0].HandlerPc++
				case "extra handler":
					code.ExceptionTable = append(code.ExceptionTable, code.ExceptionTable[0])
				case "missing handler":
					code.ExceptionTable = code.ExceptionTable[1:]
				case "extended frames", "missing frames", "wrong stack type", "wrong frame offset", "trailing frames", "duplicate frames", "named debug locals", "wrong debug locals length":
					var frames *UnparsedAttribute
					for _, a := range code.Attributes {
						if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "StackMapTable" {
							frames = raw
						}
					}
					if frames == nil {
						t.Fatal("original frames")
					}
					switch variant {
					case "extended frames":
						cp := code.ExceptionTable[0].CatchType
						frames.Info = []byte{0, 4, 247, 0, 23, 7, byte(cp >> 8), byte(cp), 251, 0, 0, 247, 0, 13, 7, byte(cp >> 8), byte(cp), 251, 0, 0}
						frames.Length = uint32(len(frames.Info))
					case "missing frames":
						for i, a := range code.Attributes {
							if a == frames {
								code.Attributes = append(code.Attributes[:i], code.Attributes[i+1:]...)
								break
							}
						}
					case "wrong stack type":
						frames.Info[3] = 1
					case "wrong frame offset":
						frames.Info[2]++
					case "trailing frames":
						frames.Info = append(frames.Info, 0)
						frames.Length++
					case "duplicate frames":
						code.Attributes = append(code.Attributes, frames)
					case "named debug locals", "wrong debug locals length":
						var locals *UnparsedAttribute
						for _, a := range code.Attributes {
							if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "LocalVariableTable" {
								locals = raw
							}
						}
						if locals == nil {
							locals = &UnparsedAttribute{Name: "LocalVariableTable", Length: 2, Info: []byte{0, 0}}
							code.Attributes = append(code.Attributes, locals)
						}
						if variant == "named debug locals" {
							locals.Info[1] = 1
						} else {
							locals.Length++
						}
					}
				case "work":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				p := nativeEnumSwitchTableProof(obj, work)
				if (p != nil) != (variant == "original" || variant == "extended frames") {
					t.Fatalf("original complete packet admitted=%v", p != nil)
				}
				if p != nil {
					if len(p.tables) != 1 {
						t.Fatal("original array inventory")
					}
					for _, a := range p.tables {
						if a.enum != "SwitchFamilyOwnerMode" || len(a.entries) != 2 || a.entries[1] != "A" || a.entries[2] != "B" {
							t.Fatal("original table mapping")
						}
					}
				}
			})
		}
	}
}
