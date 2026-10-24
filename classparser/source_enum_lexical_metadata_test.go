package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumLexicalOwnerRequiresReturnedSourceAndConsistentRows(t *testing.T) {
	originals, fresh, a := enumPortTestInputs(t, "source,lines,vars")
	const name = "PortOwner$Mode.class"
	for _, variant := range []string{"control", "child relation", "existing relation", "wrong outer", "wrong simple name", "wrong flags", "fresh enclosing method", "existing relation with enclosing method", "class identity", "original cycle", "original nonstatic", "original local", "stale original", "pending source", "unreturned source", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			file := name
			if variant == "child relation" {
				file = "PortOwner$Mode$1.class"
			}
			want, obj := mustParseLowering(t, bytes.Clone(originals[file])), mustParseLowering(t, bytes.Clone(fresh[file]))
			authority := a.source.jar.sourceConstructorLowering
			var work *workbudget.Budget
			var table *InnerClassesAttribute
			for _, attr := range obj.Attributes {
				if x, ok := attr.(*InnerClassesAttribute); ok {
					table = x
				}
			}
			if table == nil {
				t.Fatal("constant-body table control")
			}
			addRow := func() {
				table.Classes = append(table.Classes, &InnerClassInfo{InnerClassInfoIndex: uint16(obj.ConstantPoolManager.AddNewClassInfo("PortOwner$Mode")), OuterClassInfoIndex: uint16(obj.ConstantPoolManager.AddNewClassInfo("PortOwner")), InnerNameIndex: uint16(obj.ConstantPoolManager.AddUtf8Info("Mode")), InnerClassAccessFlags: 0x4409})
			}
			switch variant {
			case "existing relation", "wrong outer", "wrong simple name", "wrong flags", "existing relation with enclosing method":
				addRow()
				row := table.Classes[len(table.Classes)-1]
				if variant == "wrong outer" {
					row.OuterClassInfoIndex = uint16(obj.ConstantPoolManager.AddNewClassInfo("Unrelated"))
				}
				if variant == "wrong simple name" {
					row.InnerNameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("Other"))
				}
				if variant == "wrong flags" {
					row.InnerClassAccessFlags ^= 8
				}
			case "class identity":
				obj.ThisClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("Unrelated"))
			case "original cycle", "original nonstatic":
				for _, attr := range want.Attributes {
					if x, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range x.Classes {
							key, ok := sourceParameterClassKey(want, row.InnerClassInfoIndex)
							if ok && key == "PortOwner$Mode" {
								if variant == "original cycle" {
									row.OuterClassInfoIndex = want.ThisClass
								} else {
									row.InnerClassAccessFlags ^= 8
								}
							}
						}
					}
				}
			case "stale original":
				want.ConstantPoolManager.AddUtf8Info("unrelated original edit")
			case "pending source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, enumPending: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": {}}}
			case "unreturned source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "original local" || variant == "fresh enclosing method" || variant == "existing relation with enclosing method" {
				o := obj
				if variant == "original local" {
					o = want
				}
				index := uint16(o.ConstantPoolManager.AddNewClassInfo("PortOwner"))
				o.ConstantPoolManager.AddUtf8Info("EnclosingMethod")
				o.Attributes = append(o.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Length: 4, Info: []byte{byte(index >> 8), byte(index), 0, 0}})
			}
			table.NumberOfClasses = uint16(len(table.Classes))
			table.AttrLen = uint32(2 + 8*len(table.Classes))
			original, input := want.Bytes(), obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("structural control", err)
			}
			if variant == "original cycle" || variant == "original nonstatic" || variant == "original local" {
				origin := authority.enumActive["PortOwner$Mode"]
				origin.originalHash = sha256.Sum256(original)
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, enumActive: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": origin}}
			}
			before := bytes.Clone(input)
			done, err := authority.restoreEnumLexicalOwner(original, input, work)
			positive := variant == "control" || variant == "child relation" || variant == "existing relation"
			if positive {
				if err != nil || variant != "existing relation" && bytes.Equal(done, input) {
					t.Fatal("positive lexical relation", err)
				}
				repaired := mustParseLowering(t, done)
				rows, err := sourceAnonymousInnerRows(repaired, nil)
				if err != nil || rows["PortOwner$Mode"].outer != "PortOwner" || rows["PortOwner$Mode"].name != "Mode" {
					t.Fatal("membership absent", err)
				}
				// All executable method packets and preexisting constants remain fresh.
				for i, m := range obj.Methods {
					c, d := constructorLoweringCode(m), constructorLoweringCode(repaired.Methods[i])
					if c != nil && !bytes.Equal(c.Code, d.Code) {
						t.Fatal("changed executable bytes")
					}
				}
				readLayout := func(raw []byte) *compiledMetadataLayout {
					r, err := NewClassReaderWithBudget(raw, nil)
					if err != nil {
						t.Fatal(err)
					}
					l := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
					if _, err := parseWithMetadataLayout(r, nil, l); err != nil {
						t.Fatal(err)
					}
					return l
				}
				oldLayout, newLayout := readLayout(input), readLayout(done)
				for i, span := range oldLayout.constants {
					other := newLayout.constants[i]
					if !bytes.Equal(input[span.start:span.end], done[other.start:other.end]) {
						t.Fatal("shared constant changed")
					}
				}
				again, err := authority.restoreEnumLexicalOwner(original, done, nil)
				if err != nil || !bytes.Equal(again, done) {
					t.Fatal("not idempotent", err)
				}
			} else if err == nil && !bytes.Equal(done, input) {
				t.Fatal("unproved ownership edited")
			} else if err != nil && done != nil {
				t.Fatal("partial result")
			}
			if (variant == "work" || variant == "output" || variant == "canceled") && err == nil {
				t.Fatal("budget bypass")
			}
			if variant == "existing relation with enclosing method" && err == nil {
				t.Fatal("conflicting local membership accepted on idempotent path")
			}
			if !bytes.Equal(before, input) {
				t.Fatal("input mutated")
			}
		})
	}
}
