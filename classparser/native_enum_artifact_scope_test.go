package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumArtifactTypeScopeRequiresClosedUsersAndOriginalOwner(t *testing.T) {
	files := nativePrivateEnumCompile(t, nativePrivateEnumScopeFixture("direct", "PrivateEnumScope"), "PrivateEnumScope", "none")
	for _, variant := range []string{"original", "uncertified packet", "foreign certificate root", "missing lexical root", "failed family", "unknown helper", "foreign helper object", "wrong owner", "foreign enclosing owner", "executable enclosing method", "duplicate enclosing", "named self row", "class flags", "changed initialization", "changed key mapping", "changed field order", "missing users", "empty method uses", "nil recorded use", "failed recheck", "foreign reader recheck", "empty constructor marker", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			z.sourceCompiler, z.targetSourceVersion = NativeJavac8, 8
			root, err := Parse(files["PrivateEnumScope.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := z.prepareNativeMemberFamilyUnpublished(root, nil)
			if prepared == nil {
				t.Fatal("original preparation")
			}
			p := prepared.family
			const user = "PrivateEnumScope$1"
			table := p.enumSwitchTables[user]
			if table == nil || table.usersClosedRoot != root {
				t.Fatal("full user closure did not bind the physical source owner")
			}
			var work *workbudget.Budget
			caller := user
			var enclosing *UnparsedAttribute
			for _, attribute := range table.object.Attributes {
				if a, ok := attribute.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
					enclosing = a
				}
			}
			if enclosing == nil {
				t.Fatal("original enclosing owner")
			}
			switch variant {
			case "uncertified packet":
				table.usersClosedRoot = nil
			case "foreign certificate root":
				table.usersClosedRoot = table.object
			case "missing lexical root":
				delete(p.lexicalObjects, p.owner)
			case "failed family":
				p.failed = true
			case "unknown helper":
				caller = "Other$1"
			case "foreign helper object":
				table.object = root
			case "wrong owner":
				p.owner = "Other"
			case "foreign enclosing owner":
				binary.BigEndian.PutUint16(enclosing.Info, table.object.ThisClass)
			case "executable enclosing method":
				pool := NewConstantPoolWithConstant(&table.object.ConstantPool)
				index := pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(pool.AddUtf8Info("step")), DescriptorIndex: uint16(pool.AddUtf8Info("()V"))})
				binary.BigEndian.PutUint16(enclosing.Info[2:], uint16(index))
			case "duplicate enclosing":
				table.object.Attributes = append(table.object.Attributes, enclosing)
			case "named self row":
				pool := NewConstantPoolWithConstant(&table.object.ConstantPool)
				for _, attribute := range table.object.Attributes {
					if a, ok := attribute.(*InnerClassesAttribute); ok {
						for _, row := range a.Classes {
							if row.InnerClassInfoIndex == table.object.ThisClass {
								row.InnerNameIndex = uint16(pool.AddUtf8Info("Named"))
							}
						}
					}
				}
			case "class flags":
				table.object.AccessFlags &^= 0x1000
			case "changed initialization":
				code := table.object.Methods[0].Attributes[0].(*CodeAttribute)
				code.Code[0] = core.OP_NOP
			case "changed key mapping":
				table.tables[table.initializationOrder[0]].entries[1] = "Other"
			case "changed field order":
				table.initializationOrder[0] = "other"
			case "missing users":
				table.uses = nil
			case "empty method uses":
				for owner := range table.uses {
					table.uses[owner] = map[string]map[int]*nativeEnumSwitchUse{}
				}
			case "nil recorded use":
				for _, methods := range table.uses {
					for _, uses := range methods {
						for pc := range uses {
							uses[pc] = nil
						}
					}
				}
			case "failed recheck", "foreign reader recheck":
				index := *z.originalMemberIndex()
				if variant == "failed recheck" {
					index.valid = false
				} else {
					index.typeUsers = map[string]map[string]bool{user: {"Foreign": true}}
				}
				if z.nativeEnumSwitchSourceUsersClosed(p, root, &index, nil) {
					t.Fatal("invalid repeated user closure accepted")
				}
			case "empty constructor marker":
				p.emptyMarkers = map[string]*ClassObject{user: table.object}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				_ = work.CheckAlloc(2)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberJointEnumArtifactTypeAccess(p, caller, work); got != (variant == "original") {
				t.Fatalf("compiler artifact type scope=%v", got)
			}
			if variant == "failed recheck" || variant == "foreign reader recheck" {
				if table.usersClosedRoot != nil {
					t.Fatal("failed user proof retained a successful earlier scope certificate")
				}
			}
		})
	}
}
