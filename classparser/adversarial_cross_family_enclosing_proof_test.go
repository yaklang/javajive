package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestAdversarialCrossFamilyEnclosingCertificateRejectsChangedBindings(t *testing.T) {
	files := nativeCompileClasses(t, crossFamilyEnclosingFixture)
	for _, variant := range []string{"original", "foreign outer parameter", "missing parent", "static parent", "private parent", "wrong parent descriptor", "missing ancestor", "wrong ancestor identity", "cyclic ancestor", "static child", "wrong capture flags", "wrong enclosing slot", "wrong call PC", "wrong call descriptor", "graph budget", "memory", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			selected := files
			if variant == "foreign outer parameter" {
				f := strings.Replace(crossFamilyEnclosingFixture, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(BindingBase other,int n)throws java.io.IOException{other.super(n);}", 1)
				f = strings.Replace(f, "new Leaf(n)", "new Leaf(this,n)", 1)
				selected = nativeCompileClasses(t, f)
			}
			z := nativeArchive(t, selected)
			defer z.Close()
			root, _ := Parse(selected["BindingBase.class"])
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original parent proof")
			}
			obj, e := Parse(selected["BindingCurrent$Leaf.class"])
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
				if n != "<init>" {
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
			if got != (variant == "original") {
				t.Fatalf("original SUPER closure=%v", got)
			}
		})
	}
}

func TestAdversarialCrossFamilyEnclosingSourcePublishesBothFamiliesAtomically(t *testing.T) {
	files := nativeCompileClasses(t, crossFamilyEnclosingFixture)
	for _, variant := range []string{"original", "unfinished child family", "missing child"} {
		for _, order := range [][]string{{"BindingBase", "BindingCurrent"}, {"BindingCurrent", "BindingBase"}} {
			t.Run(variant+strings.Join(order, ":"), func(t *testing.T) {
				input := map[string][]byte{}
				for n, b := range files {
					input[n] = append([]byte(nil), b...)
				}
				if variant == "unfinished child family" {
					obj, _ := Parse(input["BindingCurrent.class"])
					obj.MajorVersion = 53
					input["BindingCurrent.class"] = obj.Bytes()
				}
				if variant == "missing child" {
					delete(input, "BindingCurrent$Leaf.class")
				}
				z := nativeArchive(t, input)
				defer z.Close()
				if variant == "original" {
					graph, known := z.nativeMemberOriginalDependencyGraph("BindingBase", nil)
					if !known || !graph["BindingBase"]["BindingCurrent"] || !graph["BindingCurrent"]["BindingBase"] {
						t.Fatal("original SUPER caller not joined to parent ownership transaction")
					}
				}
				for _, n := range order {
					root, _ := Parse(input[n+".class"])
					entry := z.nativeMemberTransactionEntry(root)
					if (entry != nil) != (variant == "original") {
						t.Fatalf("family %s published=%v", n, entry != nil)
					}
					if entry != nil {
						foreign := "BindingCurrent$Leaf"
						if n == "BindingCurrent" {
							foreign = "BindingBase$Member"
						}
						if entry.family.children[foreign] != nil || entry.family.lexicalObjects[foreign] != nil {
							t.Fatal("foreign binding imported private ownership")
						}
					}
				}
				if variant != "original" {
					if z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
						t.Fatal("unfinished foreign child committed parent source")
					}
					for _, tx := range z.nativeMemberTransactions {
						if len(tx.entries) != 0 {
							t.Fatal("partial ownership transaction published")
						}
					}
				}
			})
		}
	}
}
