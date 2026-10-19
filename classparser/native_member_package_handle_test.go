package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberPackageHandleRequiresOriginalLookupAndDeclaration(t *testing.T) {
	for _, pkg := range []string{"", "proof.lookup"} {
		prefix, source := "", strings.Replace(nativeOrdinaryHandleFixture, "public Object echo(Object value)", "Object echo(Object value)", 1)
		if pkg != "" {
			prefix, source = strings.ReplaceAll(pkg, ".", "/")+"/", "package "+pkg+";\n"+source
		}
		files := nativeCompileClasses(t, source)
		namespace := pkg
		if namespace == "" {
			namespace = "unnamed"
		}
		for _, variant := range []string{"original", "same package sibling", "same class", "duplicate target", "no source", "nil source", "two sources", "wrong object", "wrong owner", "wrong name", "different flags", "wrong static role", "enum synthesis", "missing self row", "duplicate self row", "invalid physical kind", "missing referencer", "different package", "subpackage", "package prefix", "invalid binary name", "invalid binary token", "private", "protected", "mixed access", "special", "constructor", "bridge", "synthetic", "generic", "static mismatch", "fieldref", "wrong descriptor", "inherited target", "bad second referencer", "budget", "memory", "canceled"} {
			t.Run(namespace+"/"+variant, func(t *testing.T) {
				root, err := Parse(files[prefix+"HandleOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				z := nativeArchive(t, files)
				defer z.Close()
				family := z.nativeMemberReader(root).planNativeMemberFamily()
				if family == nil {
					t.Fatal("original family")
				}
				member := family.children[prefix+"HandleOwner$Child"]
				if member == nil {
					t.Fatal("original member")
				}
				obj := member.object
				originalIndex := z.originalMemberIndex()
				var target nativeMemberHandleTarget
				for _, candidate := range originalIndex.handleTargets[obj.GetClassName()] {
					if candidate.name == "echo" {
						target = candidate
						break
					}
				}
				if !originalIndex.valid || target.kind != 5 || !target.methodRef || target.referencer != prefix+"HandleOracle" {
					t.Fatal("original archive lookup target")
				}
				var method *MemberInfo
				for _, m := range obj.Methods {
					if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == target.name {
						method = m
					}
				}
				if method == nil || method.AccessFlags != 0 {
					t.Fatal("original package declaration")
				}
				var work *workbudget.Budget
				sources := []*nativeMemberClass{member}
				switch variant {
				case "same package sibling":
					target.referencer = prefix + "OtherOriginalLookup"
				case "same class":
					target.referencer = obj.GetClassName()
				case "no source":
					sources = nil
				case "nil source":
					sources[0] = nil
				case "two sources":
					sources = append(sources, member)
				case "wrong object":
					member.object = root
				case "wrong owner":
					member.owner += "$Other"
				case "wrong name":
					member.name += "Other"
				case "different flags":
					member.flags ^= 1
				case "wrong static role":
					member.static = !member.static
				case "enum synthesis":
					member.enumSynthesis = &nativeMemberEnumSynthesis{}
				case "missing self row", "duplicate self row":
					for _, attribute := range obj.Attributes {
						if inner, ok := attribute.(*InnerClassesAttribute); ok {
							if variant == "missing self row" {
								inner.Classes = nil
							} else {
								inner.Classes = append(inner.Classes, inner.Classes...)
							}
						}
					}
				case "invalid physical kind":
					obj.AccessFlags |= 0x0200
				case "missing referencer":
					target.referencer = ""
				case "different package":
					target.referencer = "foreign/HandleOracle"
				case "subpackage":
					target.referencer = prefix + "sub/HandleOracle"
				case "package prefix":
					target.referencer = prefix + "extra/HandleOracle"
				case "invalid binary name":
					target.referencer = prefix + "Handle.Oracle"
				case "invalid binary token":
					target.referencer = prefix + "9Lookup"
				case "private":
					method.AccessFlags = 2
				case "protected":
					method.AccessFlags = 4
				case "mixed access":
					method.AccessFlags = 3
				case "special":
					target.kind = 7
				case "constructor":
					target.kind, target.name, target.descriptor = 8, "<init>", "(L"+prefix+"HandleOwner;)V"
				case "bridge":
					method.AccessFlags |= 0x0040
				case "synthetic":
					method.AccessFlags |= 0x1000
				case "generic":
					method.Attributes = append(method.Attributes, &SignatureAttribute{})
				case "static mismatch":
					method.AccessFlags |= 8
				case "fieldref":
					target.methodRef = false
				case "wrong descriptor":
					target.descriptor = "()Ljava/lang/Object;"
				case "inherited target":
					target.name = "toString"
					target.descriptor = "()Ljava/lang/String;"
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				targets := []nativeMemberHandleTarget{target}
				if variant == "duplicate target" {
					targets = append(targets, target)
				}
				if variant == "bad second referencer" {
					bad := target
					bad.referencer = "foreign/HandleOracle"
					targets = append(targets, bad)
				}
				index := &nativeMemberIndex{valid: true, handles: map[string]bool{obj.GetClassName(): true}, handleTargets: map[string][]nativeMemberHandleTarget{obj.GetClassName(): targets}}
				want := variant == "original" || variant == "same package sibling" || variant == "same class" || variant == "duplicate target"
				if got := nativeMemberOrdinaryHandlesClosed(obj, index, work, sources...); got != want {
					t.Fatalf("admitted=%v want=%v", got, want)
				}
			})
		}
	}
}
