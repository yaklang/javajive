package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberRecursiveAnonymousAccessRequiresSameCompleteForest(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousNestedMemberFixture("member-depth-instance"))
	user := "NestedOwner$Layer$Middle$1$1"
	for _, variant := range []string{"original", "no joint commit", "foreign forest", "missing anonymous parent", "missing named anchor", "foreign group", "failed group", "missing child", "foreign child identity", "wrong enclosing owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("complete mixed forest")
			}
			group := p.anonymousUnits[user]
			if group == nil {
				t.Fatal("original nested scope")
			}
			child := group.children[user]
			var work *workbudget.Budget
			switch variant {
			case "no joint commit":
				p.anonymousForest = nil
			case "foreign forest":
				copy := *group.forest
				group.forest = &copy
			case "missing anonymous parent":
				delete(group.forest.units, group.owner)
			case "missing named anchor":
				delete(p.children, "NestedOwner$Layer$Middle")
			case "foreign group":
				copy := *group
				group.forest.groups[group.owner] = &copy
			case "failed group":
				group.failed = true
			case "missing child":
				delete(group.children, user)
			case "foreign child identity":
				cp := NewConstantPoolWithConstant(&child.object.ConstantPool)
				child.object.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "wrong enclosing owner":
				for _, a := range child.object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						cp := NewConstantPoolWithConstant(&child.object.ConstantPool)
						binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("NestedOwner")))
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := nativeMemberJointAnonymousAccess(p, user, work); known != (variant == "original") {
				t.Fatalf("lexical access %v", known)
			}
		})
	}
}

func TestNativeMemberRecursiveAnonymousPlanRejectsPartialSuppression(t *testing.T) {
	base := nativeCompileClasses(t, nativeAnonymousNestedMemberFixture("member-depth-instance"))
	for _, variant := range []string{"original", "missing named ancestor", "missing first anonymous", "missing leaf", "foreign leaf identity", "failed leaf constructor", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range base {
				files[n] = append([]byte(nil), b...)
			}
			switch variant {
			case "missing named ancestor":
				delete(files, "NestedOwner$Layer.class")
			case "missing first anonymous":
				delete(files, "NestedOwner$Layer$Middle$1.class")
			case "missing leaf":
				delete(files, "NestedOwner$Layer$Middle$1$1.class")
			case "foreign leaf identity", "failed leaf constructor":
				path := "NestedOwner$Layer$Middle$1$1.class"
				obj, _ := Parse(files[path])
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				if variant == "foreign leaf identity" {
					obj.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
				} else {
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n == "<init>" {
							for _, a := range m.Attributes {
								if code, ok := a.(*CodeAttribute); ok {
									code.Code[1] = 0x01
								}
							}
						}
					}
				}
				files[path] = obj.Bytes()
				if _, e := Parse(files[path]); e != nil {
					t.Fatal(e)
				}
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			d := z.nativeMemberReader(root)
			if variant == "budget" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			p := d.planNativeMemberFamily()
			known := p != nil && d.planNativeMemberAnonymousScopes(p)
			if known != (variant == "original") {
				t.Fatalf("complete plan %v", known)
			}
			if !known && p != nil && len(p.anonymousUnits) != 0 {
				t.Fatal("partial ownership retained after failed joint plan")
			}
		})
	}
}
