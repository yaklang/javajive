package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// A cached enum synthesis certificate is not authority for a changed emitted
// declaration. Reparse each original archive because Code retains byte slices.
func TestNativeEnumSwitchOwnedDeclarationReprovesOriginalProtocol(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"OwnedEnumSwitch.java": ownedAbstractEnumSwitchFixture}, "none", "8")
	for _, variant := range []string{"original", "cached empty proof", "failed family", "foreign family owner", "missing member", "missing cached proof", "foreign object", "foreign member owner", "foreign member name", "foreign member flags", "missing emitted owner", "missing emitted enum", "nil emitted body", "missing owned body", "changed abstractness", "changed self flags", "duplicate self", "changed factory", "changed body constructor", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["OwnedEnumSwitch.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original enum plan")
			}
			objects, ok := nativeMemberDependencyObjects(root, p, nil)
			if !ok {
				t.Fatal("original emitted packet")
			}
			own := map[string]*ClassObject{}
			for _, o := range objects {
				own[o.GetClassName()] = o
			}
			enum := own["OwnedEnumSwitch$Mode"]
			member := p.children["OwnedEnumSwitch$Mode"]
			if enum == nil || member == nil || len(p.enumConstants) != 2 {
				t.Fatal("original enum declarations")
			}
			var bodyName string
			for name := range p.enumConstants {
				bodyName = name
				break
			}
			var work *workbudget.Budget
			switch variant {
			case "failed family":
				p.failed = true
			case "foreign family owner":
				p.owner = "Foreign"
			case "cached empty proof":
				member.enumSynthesis = &nativeMemberEnumSynthesis{}
			case "missing member":
				delete(p.children, enum.GetClassName())
			case "missing cached proof":
				member.enumSynthesis = nil
			case "foreign object":
				copy := *enum
				member.object = &copy
			case "foreign member owner":
				member.owner = "Foreign"
			case "foreign member name":
				member.name = "Foreign"
			case "foreign member flags":
				member.flags ^= 0x400
			case "missing emitted owner":
				delete(own, member.owner)
			case "missing emitted enum":
				delete(own, enum.GetClassName())
			case "nil emitted body":
				own[bodyName] = nil
			case "missing owned body":
				delete(p.enumConstants, bodyName)
			case "changed abstractness":
				enum.AccessFlags |= 0x10
			case "changed self flags", "duplicate self":
				for _, a := range enum.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(enum, row.InnerClassInfoIndex)
							if n == enum.GetClassName() {
								if variant == "duplicate self" {
									table.Classes = append(table.Classes, row)
								} else {
									row.InnerClassAccessFlags ^= 0x400
								}
								break
							}
						}
					}
				}
			case "changed factory", "changed body constructor":
				target, want := enum, "values"
				if variant == "changed body constructor" {
					target = own[bodyName]
					want = "<init>"
				}
				changed := false
				for _, m := range target.Methods {
					n, _ := sourceBridgeUTF8(target, m.NameIndex)
					if n == want {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code[0] = core.OP_NOP
								changed = true
							}
						}
					}
				}
				if !changed {
					t.Fatal("original executable protocol missing")
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "cached empty proof"
			if got := z.nativeEnumSwitchOwnedDeclaration(p, enum, own, work); got != want {
				t.Fatalf("original owned enum protocol=%v want=%v", got, want)
			}
		})
	}
}
