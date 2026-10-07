package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumSwitchConstructorMarkerNeedsBothOriginalRoles(t *testing.T) {
	originals := nativeCompileClasses(t, enumSwitchTableConstructorMarkerFixture(false))
	for _, variant := range []string{"original", "failed family", "wrong root", "foreign marker", "missing table", "extra table", "empty marker", "real anonymous", "foreign table identity", "foreign enclosing", "wrong self flags", "table annotation", "ordinary field", "mutable field", "cached enum", "cached constant", "cached key", "missing cached array", "changed table packet", "changed handler", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, originals)
			defer z.Close()
			root, e := Parse(append([]byte(nil), originals["EnumAssertionOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original lexical family")
			}
			marker := "EnumAssertionOwner$1"
			table := p.enumSwitchTables[marker]
			if table == nil {
				t.Fatal("original shared table")
			}
			obj := table.object
			var code *CodeAttribute
			for _, a := range obj.Methods[0].Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
			var work *workbudget.Budget
			switch variant {
			case "failed family":
				p.failed = true
			case "wrong root":
				p.owner = "OtherOwner"
			case "foreign marker":
				marker = "OtherOwner$1"
			case "missing table":
				delete(p.enumSwitchTables, marker)
			case "extra table":
				p.enumSwitchTables["EnumAssertionOwner$2"] = table
			case "empty marker":
				p.emptyMarkers["EnumAssertionOwner$2"] = obj
			case "real anonymous":
				p.anonymous = &nativeAnonymousFamily{owner: p.owner, children: map[string]*nativeAnonymousClass{marker: {object: obj}}}
			case "foreign table identity":
				obj.ThisClass = obj.SuperClass
			case "foreign enclosing":
				for _, a := range obj.Attributes {
					if a, ok := a.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(a.Info[:2], obj.SuperClass)
					}
				}
			case "wrong self flags":
				for _, a := range obj.Attributes {
					if a, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range a.Classes {
							if n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex); n == marker {
								row.InnerClassAccessFlags = 8
							}
						}
					}
				}
			case "table annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleAnnotationsAttribute{})
			case "ordinary field":
				obj.Fields[0].AccessFlags &^= 0x1000
			case "mutable field":
				obj.Fields[0].AccessFlags &^= 16
			case "cached enum":
				for _, a := range table.tables {
					a.enum = "OtherMode"
				}
			case "cached constant":
				for _, a := range table.tables {
					a.entries[1] = "NONEXISTENT"
				}
			case "cached key":
				for _, a := range table.tables {
					a.entries[8] = a.entries[1]
					delete(a.entries, 1)
				}
			case "missing cached array":
				for field := range table.tables {
					delete(table.tables, field)
				}
			case "changed table packet":
				code.Code[0] = core.OP_INVOKEVIRTUAL
			case "changed handler":
				code.ExceptionTable[0].CatchType = obj.SuperClass
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if accepted := nativeMemberJointSwitchTableMarker(p, marker, work); accepted != (variant == "original") {
				t.Fatalf("accepted=%v", accepted)
			}
		})
	}
}

func TestNativeEnumSwitchConstructorMarkerCannotEscapeThroughArchive(t *testing.T) {
	originals := nativeCompileClasses(t, enumSwitchTableConstructorMarkerFixture(false))
	for _, variant := range []string{"original", "foreign field", "field handle", "class literal", "ordinary typed declaration", "read marker tail"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for name, raw := range originals {
				files[name] = append([]byte(nil), raw...)
			}
			marker := "EnumAssertionOwner$1"
			if variant == "foreign field" {
				obj, e := Parse(append([]byte(nil), files["TableRoleDriver.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				obj.ThisClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("ForeignTableRoleUser"))
				obj.Fields = append(obj.Fields, &MemberInfo{AccessFlags: 1, NameIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("L" + marker + ";"))})
				files["ForeignTableRoleUser.class"] = obj.Bytes()
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(append([]byte(nil), files["EnumAssertionOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original lexical family")
			}
			if !nativeMemberJointSwitchTableMarker(p, marker, nil) || !nativeMemberJointBridgeMarkersClosed(p, nil) {
				t.Fatal("original joint roles")
			}
			index := z.originalMemberIndex()
			switch variant {
			case "field handle":
				index.handles[marker] = true
			case "class literal":
				for _, m := range root.Methods {
					if n, _ := sourceBridgeUTF8(root, m.NameIndex); n == "choose" {
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								i := uint16(root.ConstantPoolManager.AddNewClassInfo(marker))
								c.Code = append([]byte{core.OP_LDC_W, byte(i >> 8), byte(i), core.OP_POP}, c.Code...)
							}
						}
					}
				}
			case "ordinary typed declaration":
				root.Fields = append(root.Fields, &MemberInfo{AccessFlags: 1, NameIndex: uint16(root.ConstantPoolManager.AddUtf8Info("value")), DescriptorIndex: uint16(root.ConstantPoolManager.AddUtf8Info("L" + marker + ";"))})
			case "read marker tail":
				for _, m := range p.children["EnumAssertionOwner$Mode"].object.Methods {
					if n, _ := sourceBridgeUTF8(p.children["EnumAssertionOwner$Mode"].object, m.NameIndex); n == "<init>" && m.AccessFlags&0x1000 != 0 {
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								c.Code = append([]byte{core.OP_ALOAD, 4, core.OP_POP}, c.Code...)
							}
						}
					}
				}
			}
			if accepted := z.nativeEnumSwitchUsersClosed(p, root, index, nil); accepted != (variant == "original") {
				t.Fatalf("users accepted=%v", accepted)
			}
			if variant == "original" && !z.nativeMemberJointBridgeReferencesClosed(p, index, nil) {
				t.Fatal("original bridge references")
			}
		})
	}
}
