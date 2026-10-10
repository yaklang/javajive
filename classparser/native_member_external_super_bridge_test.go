package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeForeignSuperOwnBridgeRequiresExactOriginalPacket(t *testing.T) {
	fixture := strings.Replace(crossFamilyEnclosingFixture, "Leaf(int n)throws java.io.IOException{super(n);}", "private Leaf(int n)throws java.io.IOException{super(n);}", 1)
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "foreign outer parameter", "missing parent", "static parent", "private parent", "wrong parent descriptor", "missing ancestor", "wrong ancestor identity", "cyclic ancestor", "static child", "wrong capture flags", "wrong enclosing slot", "wrong call PC", "wrong call descriptor", "own bridge marker used", "own bridge foreign target", "own bridge extra effect", "own bridge wrong synthetic flags", "ordinary THIS delegate", "own bridge duplicate", "graph budget", "memory", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			selected := files
			if variant == "foreign outer parameter" {
				f := strings.Replace(fixture, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(BindingBase other,int n)throws java.io.IOException{other.super(n);}", 1)
				f = strings.Replace(f, "new Leaf(n)", "new Leaf(this,n)", 1)
				selected = nativeCompileClasses(t, f)
			}
			z := nativeArchive(t, selected)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), selected["BindingBase.class"]...))
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original parent proof")
			}
			obj, e := Parse(append([]byte(nil), selected["BindingCurrent$Leaf.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(obj)
			var method *MemberInfo
			var code *CodeAttribute
			pc := -1
			desc := ""
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "<init>" || m.AccessFlags&0x1000 != 0 {
					continue
				}
				method = m
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original constructor")
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("original body")
			}
			for _, op := range decoder.Opcodes() {
				member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
				if member != nil && member.Name == "BindingBase$Member" {
					pc = int(op.CurrentOffset)
					desc = member.Description
				}
			}
			if pc < 0 {
				t.Fatal("original SUPER call")
			}
			parent := p.children["BindingBase$Member"]
			switch variant {
			case "missing parent":
				delete(p.children, "BindingBase$Member")
			case "static parent":
				parent.static = true
			case "private parent":
				for _, m := range parent.object.Methods {
					n, _ := sourceBridgeUTF8(parent.object, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "wrong parent descriptor":
				parent.constructors = map[string]*nativeMemberConstructor{}
			case "missing ancestor", "wrong ancestor identity", "cyclic ancestor":
				old := d.foldSiblingResolver
				d.foldSiblingResolver = func(n string) ([]byte, bool) {
					if n == "BindingCurrent" {
						if variant == "missing ancestor" {
							return nil, false
						}
						if variant == "wrong ancestor identity" {
							return selected["BindingBase.class"], true
						}
						o, _ := Parse(selected["BindingCurrent.class"])
						o.SuperClass = o.ThisClass
						return o.Bytes(), true
					}
					return old(n)
				}
			case "static child":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == obj.GetClassName() {
								row.InnerClassAccessFlags |= 8
							}
						}
					}
				}
			case "wrong capture flags":
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == "this$0" {
						f.AccessFlags &^= 0x10
					}
				}
			case "wrong enclosing slot":
				code.Code[6] = byte(core.OP_ALOAD_2)
			case "wrong call PC":
				pc++
			case "wrong call descriptor":
				desc = "(LBindingBase;J)V"
			case "own bridge marker used", "own bridge foreign target", "own bridge extra effect", "own bridge wrong synthetic flags", "ordinary THIS delegate", "own bridge duplicate":
				for _, m := range obj.Methods {
					if m.AccessFlags != 0x1000 {
						continue
					}
					for _, attr := range m.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							switch variant {
							case "own bridge marker used":
								code.Code = append(code.Code[:len(code.Code)-1], byte(core.OP_ALOAD_3), byte(core.OP_POP), byte(core.OP_RETURN))
							case "own bridge extra effect":
								code.Code = append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_POP)}, code.Code...)
							case "own bridge wrong synthetic flags":
								m.AccessFlags = 0x1001
							case "ordinary THIS delegate":
								// Ordinary THIS delegation retains a source
								// parameter; this physical SUPER proof does
								// not erase it or certify its source family.
								m.AccessFlags = 0
							case "own bridge duplicate":
								obj.Methods = append(obj.Methods, m)
							case "own bridge foreign target":
								decoder := core.NewDecompiler(code.Code, nil)
								if decoder.ParseOpcode() != nil {
									t.Fatal("original bridge")
								}
								for _, op := range decoder.Opcodes() {
									if op.Instr.OpCode == core.OP_INVOKESPECIAL {
										index := core.Convert2bytesToInt(op.Data)
										ref := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
										ref.ClassIndex = obj.SuperClass
									}
								}
							}
						}
					}
					break
				}
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.nativeMemberForeignOriginalSuper(p, method, "BindingBase$Member", desc, pc)
			if got != (variant == "original" || variant == "ordinary THIS delegate") {
				t.Fatalf("original SUPER closure=%v", got)
			}
		})
	}
}
