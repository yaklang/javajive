package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeProtectedInstanceFieldRequiresCompleteOriginalResolution(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, nativeProtectedInstanceFieldSources("InstanceFieldOwner"), debug, "8")
		for _, variant := range []string{"original", "unknown parent", "wrong parent identity", "parent cycle", "interface search unknown", "own shadow", "intermediate shadow", "duplicate field", "private field", "public field", "static field", "synthetic field", "constant field", "descriptor mismatch", "missing field", "same package", "wrong opcode", "wrong receiver", "wrong field owner", "wrong return", "stack", "extra effect", "handler", "nonsynthetic packet", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				root, _ := Parse(files["instance/use/InstanceFieldOwner$Child.class"])
				middle, _ := Parse(files["instance/base/InstanceMiddle.class"])
				parent, _ := Parse(files["instance/base/InstanceParent.class"])
				var bridge, field *MemberInfo
				var code *CodeAttribute
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "access$000" {
						bridge = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				for _, f := range parent.Fields {
					n, _ := sourceBridgeUTF8(parent, f.NameIndex)
					if n == "value" {
						field = f
					}
				}
				if bridge == nil || field == nil || code == nil {
					t.Fatal("original inherited field packet")
				}
				resolve := func(n string) (*ClassObject, bool) {
					if n == parent.GetClassName() {
						return parent, true
					}
					if n == middle.GetClassName() {
						return middle, true
					}
					return nil, false
				}
				var work *workbudget.Budget
				switch variant {
				case "unknown parent":
					resolve = func(string) (*ClassObject, bool) { return nil, false }
				case "wrong parent identity":
					resolve = func(string) (*ClassObject, bool) { return parent, true }
				case "parent cycle":
					middle.SuperClass = middle.ThisClass
				case "interface search unknown":
					middle.Interfaces = []uint16{middle.ThisClass}
				case "own shadow", "intermediate shadow":
					o := root
					if variant == "intermediate shadow" {
						o = middle
					}
					copy := *field
					copy.NameIndex = sourceBridgePoolString(t, o, "value")
					copy.DescriptorIndex = sourceBridgePoolString(t, o, "Ljava/lang/String;")
					o.Fields = append(o.Fields, &copy)
				case "duplicate field":
					parent.Fields = append(parent.Fields, field)
				case "private field":
					field.AccessFlags = (field.AccessFlags &^ 7) | 2
				case "public field":
					field.AccessFlags = (field.AccessFlags &^ 7) | 1
				case "static field":
					field.AccessFlags |= 8
				case "synthetic field":
					field.AccessFlags |= 0x1000
				case "constant field":
					field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
				case "descriptor mismatch":
					field.DescriptorIndex = sourceBridgePoolString(t, parent, "Ljava/lang/String;")
				case "missing field":
					field.NameIndex = sourceBridgePoolString(t, parent, "otherValue")
				case "same package":
					cp := NewConstantPoolWithConstant(&root.ConstantPool)
					root.ThisClass = uint16(cp.AddNewClassInfo("instance/base/FieldOwner$Child"))
					ref := root.ConstantPool[(int(code.Code[2])<<8|int(code.Code[3]))-1].(*ConstantFieldrefInfo)
					ref.ClassIndex = root.ThisClass
				case "wrong opcode":
					code.Code[1] = core.OP_GETSTATIC
				case "wrong receiver":
					code.Code[0] = core.OP_ALOAD_1
				case "wrong field owner":
					ref := root.ConstantPool[(int(code.Code[2])<<8|int(code.Code[3]))-1].(*ConstantFieldrefInfo)
					cp := NewConstantPoolWithConstant(&root.ConstantPool)
					ref.ClassIndex = uint16(cp.AddNewClassInfo(parent.GetClassName()))
				case "wrong return":
					code.Code[4] = core.OP_IRETURN
				case "stack":
					code.MaxStack++
				case "extra effect":
					code.Code = append([]byte{core.OP_ICONST_0, core.OP_POP}, code.Code...)
				case "handler":
					code.ExceptionTable = []*ExceptionTableEntry{{}}
				case "nonsynthetic packet":
					bridge.AccessFlags &^= 0x1000
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				g := nativeMemberProtectedFieldProof(root, bridge, resolve, work)
				if (g != nil) != (variant == "original") {
					t.Fatalf("inherited instance read admitted=%v", g != nil)
				}
				if g != nil && nativeMemberProtectedStaticFieldProof(root, bridge, resolve, nil) != nil {
					t.Fatal("instance packet admitted by static-only profile")
				}
				if g != nil && (!g.inheritedField || g.staticField || g.setter || g.call != nil) {
					t.Fatal("original operation identity")
				}
			})
		}
	}
}
