package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberAnonymousSuperclassNeedsExactCaptureTransfer(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeMemberAnonymousFixture, "none")
	for _, variant := range []string{"original", "no member plan", "missing owner", "static owner", "missing parent", "static parent", "foreign parent owner", "wrong capture name", "missing capture", "wrong getfield owner", "wrong getfield name", "wrong getfield descriptor", "wrong slot", "missing target constructor", "extra computation", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["NestedOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("named source proof missing")
			}
			obj, err := Parse(append([]byte(nil), files["NestedOwner$Actor$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			owner, method, known := originalAnonymousOwner(obj)
			if !known {
				t.Fatal("anonymous original identity")
			}
			var code *CodeAttribute
			var getfield *ConstantFieldrefInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			for _, c := range obj.ConstantPool {
				if f, ok := c.(*ConstantFieldrefInfo); ok {
					cl, _ := sourceBridgeClassName(obj, f.ClassIndex)
					if cl == owner {
						getfield = f
					}
				}
			}
			if code == nil || getfield == nil {
				t.Fatal("original enclosing transfer absent")
			}
			var work *workbudget.Budget
			switch variant {
			case "no member plan":
				p = nil
			case "missing owner":
				delete(p.children, owner)
			case "static owner":
				p.children[owner].static = true
			case "missing parent":
				delete(p.children, obj.GetSupperClassName())
			case "static parent":
				p.children[obj.GetSupperClassName()].static = true
			case "foreign parent owner":
				p.children[obj.GetSupperClassName()].owner = "Foreign"
			case "wrong capture name", "missing capture":
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == "this$1" {
						if variant == "missing capture" {
							f.AccessFlags &^= 0x1000
						} else {
							f.NameIndex = sourceBridgePoolString(t, obj, "this$0")
						}
					}
				}
			case "wrong getfield owner":
				getfield.ClassIndex = obj.ThisClass
			case "wrong getfield name":
				nt := obj.ConstantPool[getfield.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.NameIndex = sourceBridgePoolString(t, obj, "other")
			case "wrong getfield descriptor":
				nt := obj.ConstantPool[getfield.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = sourceBridgePoolString(t, obj, "Ljava/lang/Object;")
			case "wrong slot":
				code.Code[6] = byte(core.OP_ALOAD_2)
			case "missing target constructor":
				p.children[obj.GetSupperClassName()].constructors = nil
			case "extra computation":
				code.Code = append(code.Code[:7], append([]byte{byte(core.OP_ICONST_0), byte(core.OP_POP)}, code.Code[7:]...)...)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousConstructorWithinMembers(obj, owner, method, work, p); (got != nil) != (variant == "original") {
				t.Fatalf("constructor admitted=%v", got != nil)
			}
		})
	}
}

// Each mutation is metadata/code proof input only. It is never executed as JVM
// code; the unchanged authored class owns the positive runtime oracle.
func sourceBridgePoolString(t *testing.T, obj *ClassObject, s string) uint16 {
	t.Helper()
	for i, c := range obj.ConstantPool {
		if u, ok := c.(*ConstantUtf8Info); ok && u.Value == s {
			return uint16(i + 1)
		}
	}
	obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: s})
	return uint16(len(obj.ConstantPool))
}

func TestNativeAnonymousCaptureReadAuthorizesOnlyProjectedConstructorPC(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeMemberAnonymousFixture, "none")
	for _, variant := range []string{"original", "wrong read PC", "wrong constructor descriptor", "missing projected parent", "foreign group owner", "absent group", "wrong requested owner", "extra body read", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["NestedOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("named proof missing")
			}
			const owner = "NestedOwner$Actor"
			const user = "NestedOwner$Actor$1"
			group := z.nativeMemberReader(p.children[owner].object).planNativeAnonymousFamilyWithinMembers(p)
			if group == nil || group.children[user] == nil {
				t.Fatal("original anonymous source graph absent")
			}
			p.anonymousUnits = map[string]*nativeAnonymousFamily{user: group}
			child := group.children[user]
			requested := owner
			var work *workbudget.Budget
			switch variant {
			case "wrong read PC":
				child.memberEnclosingReadPC++
			case "wrong constructor descriptor":
				child.descriptor = "()V"
			case "missing projected parent":
				child.memberSuper = nil
			case "foreign group owner":
				group.owner = "Foreign"
			case "absent group":
				p.anonymousUnits = nil
			case "wrong requested owner":
				requested = "Foreign"
			case "extra body read":
				var fieldIndex uint16
				for i, c := range child.object.ConstantPool {
					if f, ok := c.(*ConstantFieldrefInfo); ok {
						n, _ := sourceBridgeClassName(child.object, f.ClassIndex)
						if n == owner {
							fieldIndex = uint16(i + 1)
						}
					}
				}
				if fieldIndex == 0 {
					t.Fatal("original transfer field missing")
				}
				for _, m := range child.object.Methods {
					n, _ := sourceBridgeUTF8(child.object, m.NameIndex)
					if n != "origin" {
						continue
					}
					for _, a := range m.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							raw := append([]byte(nil), code.Code[:len(code.Code)-1]...)
							raw = append(raw, byte(core.OP_GETFIELD), byte(fieldIndex>>8), byte(fieldIndex), byte(core.OP_ARETURN))
							code.Code = raw
						}
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberProjectedAnonymousCaptureRead(p, child, requested, work); got != (variant == "original") {
				t.Fatalf("capture read authorized=%v", got)
			}
		})
	}
}

func TestNativeAnonymousOrdinalsStayInTheirOriginalOwner(t *testing.T) {
	p := &nativeAnonymousFamily{owner: "Outer$Actor", children: map[string]*nativeAnonymousClass{"Outer$Actor$1": {}, "Outer$Actor$2": {}}}
	for _, row := range []struct {
		source string
		valid  bool
	}{
		{`/*jdec-owned-anonymous-ordinal:1:Outer*/new A(){};/*jdec-owned-anonymous-ordinal:1:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:1:Outer$Other*/new A(){}`, true},
		{`String x="/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/";/*jdec-owned-anonymous-ordinal:1:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/new A(){}`, true},
		{`/*jdec-owned-anonymous-ordinal:1:Outer$Other*/new A(){};/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:1:Outer$Actor*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:1*/new A(){};/*jdec-owned-anonymous-ordinal:2*/new A(){}`, false},
		{`/*jdec-owned-anonymous-ordinal:1:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:2:Outer$Actor*/new A(){};/*jdec-owned-anonymous-ordinal:1:bad scope*/`, false},
	} {
		if got := p.completeSource(row.source); got != row.valid {
			t.Fatalf("layout accepted=%v %s", got, row.source)
		}
	}
}
