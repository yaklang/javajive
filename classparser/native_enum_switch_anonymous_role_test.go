package javaclassparser

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumSwitchAnonymousRoleRequiresCurrentOwnedPacket(t *testing.T) {
	files := nativeCompileClasses(t, nativeEnumTableNestedAnonymousFixture(true))
	for _, variant := range []string{"original", "nil family", "failed family", "nil observation", "foreign root identity", "missing table", "nil table", "missing planned object", "foreign table name", "fresh version", "cached version", "root version", "fresh class flags", "cached class flags", "fresh body effect", "cached body effect", "cached case", "cached enum", "cached field namespace", "cached order", "missing root row", "duplicate root row", "root row flags", "fresh self flags", "real anonymous", "forged real anonymous membership", "graph budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(bytes.Clone(files["SwitchBridgeOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			name := "SwitchBridgeOwner$1"
			if p == nil || p.enumSwitchTables[name] == nil {
				t.Fatal("original owned table missing")
			}
			cached := p.enumSwitchTables[name]
			object, err := Parse(bytes.Clone(files[name+".class"]))
			if err != nil {
				t.Fatal(err)
			}
			fields := append([]string(nil), cached.initializationOrder...)
			if len(fields) != 2 || cached.usersClosedRoot != nil {
				t.Fatal("expected two provisional initialization packets")
			}
			editRow := func(target *ClassObject, edit func(*InnerClassInfo)) {
				t.Helper()
				for _, a := range target.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							binary, _ := sourceBridgeClassName(target, row.InnerClassInfoIndex)
							if binary == name {
								if variant == "missing root row" {
									table.Classes = append(table.Classes[:i:i], table.Classes[i+1:]...)
								} else if variant == "duplicate root row" {
									table.Classes = append(table.Classes, row)
								} else {
									edit(row)
								}
								return
							}
						}
					}
				}
				t.Fatal("helper row missing")
			}
			switch variant {
			case "nil family":
				p = nil
			case "failed family":
				p.failed = true
			case "nil observation":
				object = nil
			case "foreign root identity":
				copy := *root
				d.obj = &copy
			case "missing table":
				delete(p.enumSwitchTables, name)
			case "nil table":
				p.enumSwitchTables[name] = nil
			case "missing planned object":
				cached.object = nil
			case "foreign table name":
				p.enumSwitchTables = map[string]*nativeEnumSwitchTable{"Foreign$1": cached}
			case "fresh version":
				object.MajorVersion++
			case "cached version":
				cached.object.MajorVersion++
			case "root version":
				root.MajorVersion++
			case "fresh class flags":
				object.AccessFlags = 0x20
			case "cached class flags":
				cached.object.AccessFlags = 0x20
			case "fresh body effect":
				nativeEnumSwitchRoleAddThrow(t, object)
			case "cached body effect":
				nativeEnumSwitchRoleAddThrow(t, cached.object)
			case "cached case":
				cached.tables[fields[0]].entries[1] = "FOREIGN"
			case "cached enum":
				cached.tables[fields[0]].enum = "Foreign"
			case "cached field namespace":
				cached.tables["foreign"] = cached.tables[fields[0]]
				delete(cached.tables, fields[0])
			case "cached order":
				cached.initializationOrder[0], cached.initializationOrder[1] = fields[1], fields[0]
			case "missing root row", "duplicate root row":
				editRow(root, nil)
			case "root row flags":
				editRow(root, func(row *InnerClassInfo) { row.InnerClassAccessFlags = 8 })
			case "fresh self flags":
				editRow(object, func(row *InnerClassInfo) { row.InnerClassAccessFlags = 8 })
			case "real anonymous", "forged real anonymous membership":
				object, err = Parse(bytes.Clone(files["SwitchBridgeOwner$Maker$1.class"]))
				if err != nil {
					t.Fatal(err)
				}
				if variant == "forged real anonymous membership" {
					p.enumSwitchTables[object.GetClassName()] = cached
				}
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.nativeMemberEnumSwitchAnonymousRole(p, object); got != (variant == "original") {
				t.Fatalf("table role=%v for %s", got, variant)
			}
			if cached.usersClosedRoot != nil {
				t.Fatal("discovery granted archive-user/source permission")
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("resource refusal lost")
			}
		})
	}
}

func nativeEnumSwitchRoleAddThrow(t *testing.T, object *ClassObject) {
	t.Helper()
	for _, m := range object.Methods {
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				if len(code.Code) == 0 || code.Code[len(code.Code)-1] != core.OP_RETURN {
					t.Fatal("original initializer return missing")
				}
				code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), core.OP_ACONST_NULL, core.OP_ATHROW)
				return
			}
		}
	}
	t.Fatal("initializer body missing")
}

func TestNativeEnumSwitchAnonymousRoleReopensProviderObservations(t *testing.T) {
	files := nativeCompileClasses(t, nativeEnumTableNestedAnonymousFixture(true))
	for _, variant := range []string{"fresh body", "cached body", "cached case", "root row", "root version", "root identity", "failed family"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(bytes.Clone(files["SwitchBridgeOwner.class"]))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			name := "SwitchBridgeOwner$1"
			if p == nil || p.enumSwitchTables[name] == nil {
				t.Fatal("original table missing")
			}
			cached := p.enumSwitchTables[name]
			object, _ := Parse(bytes.Clone(files[name+".class"]))
			base := d.foldSiblingResolver
			mutated := false
			d.foldSiblingResolver = func(binary string) ([]byte, bool) {
				if !mutated && (binary == "SwitchBridgeMode" || binary == "ExtraBridgeMode") {
					mutated = true
					switch variant {
					case "fresh body":
						nativeEnumSwitchRoleAddThrow(t, object)
					case "cached body":
						nativeEnumSwitchRoleAddThrow(t, cached.object)
					case "cached case":
						var fields []string
						for field := range cached.tables {
							fields = append(fields, field)
						}
						sort.Strings(fields)
						cached.tables[fields[0]].entries[1] = "FOREIGN"
					case "root row":
						root.Attributes = nil
					case "root version":
						root.MajorVersion++
					case "root identity":
						copy := *root
						p.lexicalObjects[p.owner] = &copy
					case "failed family":
						p.failed = true
					}
				}
				return base(binary)
			}
			if accepted := d.nativeMemberEnumSwitchAnonymousRole(p, object); accepted || !mutated {
				t.Fatalf("accepted=%v provider mutation reached=%v", accepted, mutated)
			}
		})
	}
}

func TestNativeEnumSwitchDiscoveryKeepsRealAnonymousChildren(t *testing.T) {
	files := nativeCompileClasses(t, nativeEnumTableNestedAnonymousFixture(false))
	z := nativeArchive(t, files)
	defer z.Close()
	root, _ := Parse(bytes.Clone(files["SwitchBridgeOwner.class"]))
	d := z.nativeMemberReader(root)
	p := d.planNativeMemberFamily()
	if p == nil {
		t.Fatal("original family missing")
	}
	if has, known := d.nativeAnonymousForestHasChildren(p); !known || has {
		t.Fatalf("root cache invented an anonymous expression: has=%v known=%v", has, known)
	}
	child := p.children["SwitchBridgeOwner$Maker"]
	if child == nil {
		t.Fatal("original member missing")
	}
	reader := z.nativeMemberReader(child.object)
	if has, known := reader.nativeAnonymousForestHasChildren(p); !known || !has {
		t.Fatalf("real member-owned anonymous expression was lost: has=%v known=%v", has, known)
	}
	p.enumSwitchTables["SwitchBridgeOwner$Maker$1"] = p.enumSwitchTables["SwitchBridgeOwner$1"]
	if has, known := reader.nativeAnonymousForestHasChildren(p); known || has {
		t.Fatalf("forged cache membership hid a real expression: has=%v known=%v", has, known)
	}
}
