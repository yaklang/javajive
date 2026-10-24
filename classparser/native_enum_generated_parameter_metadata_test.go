package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeNamedEnumParametersRequireExactCompilerRolesAndWholeUnit(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for original generated metadata")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, memberEnumParameterMetadataFixture, "EnumParameterOwner", "none", true)
			for _, variant := range []string{"original", "unknown compiler", "other compiler lowering", "unknown source", "other source", "other version", "other minor", "missing factory table", "missing constructor table", "duplicate factory table", "factory name", "hidden name", "hidden order", "factory flags", "hidden flags", "factory arity", "table length", "missing outer table", "missing sibling table", "wrong class identity", "missing owning family", "missing original parent", "wrong factory code", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					input := map[string][]byte{}
					for name, data := range files {
						input[name] = bytes.Clone(data)
					}
					parse := func(name string) *ClassObject {
						t.Helper()
						object, err := Parse(bytes.Clone(input[name+".class"]))
						if err != nil {
							t.Fatal(err)
						}
						return object
					}
					root, sibling, enum := parse("EnumParameterOwner"), parse("EnumParameterOwner$Scope"), parse("EnumParameterOwner$Scope$Choice")
					var ctor, factory *MemberInfo
					var ctorTable, factoryTable *UnparsedAttribute
					for _, method := range enum.Methods {
						name, _ := sourceBridgeUTF8(enum, method.NameIndex)
						for _, attribute := range method.Attributes {
							if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
								if name == "<init>" {
									ctor, ctorTable = method, table
								} else if name == "valueOf" {
									factory, factoryTable = method, table
								}
							}
						}
					}
					if ctor == nil || factory == nil || ctorTable == nil || factoryTable == nil {
						t.Fatal("actual compiler named enum protocol missing")
					}
					selected := nativeEnumParameterCompiler{profile: profile, source: 8}
					var work *workbudget.Budget
					removeOrdinary := func(object *ClassObject) {
						t.Helper()
						for _, method := range object.Methods {
							name, _ := sourceBridgeUTF8(object, method.NameIndex)
							if name == "select" {
								for _, attribute := range method.Attributes {
									if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
										removeLambdaParameterTestTable(method, table)
										return
									}
								}
							}
						}
						t.Fatal("actual ordinary named declaration missing")
					}
					switch variant {
					case "unknown compiler":
						selected.profile = ""
					case "other compiler lowering":
						selected.profile = ModernJavac
						if profile == ModernJavac {
							selected.profile = NativeJavac8
						}
					case "unknown source":
						selected.source = 0
					case "other source":
						selected.source = 11
					case "other version":
						enum.MajorVersion = 55
					case "other minor":
						enum.MinorVersion = 1
					case "missing factory table":
						removeLambdaParameterTestTable(factory, factoryTable)
					case "missing constructor table":
						removeLambdaParameterTestTable(ctor, ctorTable)
					case "duplicate factory table":
						factory.Attributes = append(factory.Attributes, factoryTable)
					case "factory name":
						factoryTable.Info[1], factoryTable.Info[2] = byte(factory.NameIndex>>8), byte(factory.NameIndex)
					case "hidden name":
						ctorTable.Info[1], ctorTable.Info[2] = byte(factory.NameIndex>>8), byte(factory.NameIndex)
					case "hidden order":
						ctorTable.Info[1], ctorTable.Info[2], ctorTable.Info[5], ctorTable.Info[6] = ctorTable.Info[5], ctorTable.Info[6], ctorTable.Info[1], ctorTable.Info[2]
					case "factory flags":
						factoryTable.Info[4] |= 0x10
					case "hidden flags":
						ctorTable.Info[4] |= 0x10
					case "factory arity":
						factoryTable.Info[0]++
					case "table length":
						factoryTable.Length++
					case "missing outer table":
						removeOrdinary(root)
					case "missing sibling table":
						removeOrdinary(sibling)
					case "wrong class identity":
						root = sibling
					case "wrong factory code":
						for _, attribute := range factory.Attributes {
							if code, ok := attribute.(*CodeAttribute); ok {
								code.Code[len(code.Code)-1] = byte(core.OP_IRETURN)
							}
						}
					case "work":
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					before := enum.Bytes()
					metadata := nativeEnumGeneratedParameterNames(enum, selected, work)
					_, _, flags, known := originalMemberOwner(enum)
					if !known {
						t.Fatal("original member role disappeared")
					}
					proved := nativeMemberEnumSynthesisWithParameterNames(enum, flags, nil, work, metadata)
					family := &nativeMemberFamily{owner: "EnumParameterOwner", lexicalObjects: map[string]*ClassObject{"EnumParameterOwner": root, "EnumParameterOwner$Scope": sibling, "EnumParameterOwner$Scope$Choice": enum}}
					if variant == "missing owning family" {
						family = nil
					} else if variant == "missing original parent" {
						delete(family.lexicalObjects, "EnumParameterOwner$Scope")
					}
					closed := metadata && proved != nil && nativeMemberEnumParameterUnitClosed(family, enum, work)
					if closed != (variant == "original") {
						t.Fatalf("compiler/factory/whole-unit proof=%v metadata=%v body=%v", closed, metadata, proved != nil)
					}
					if !bytes.Equal(before, enum.Bytes()) || !bytes.Equal(input["EnumParameterOwner$Scope$Choice.class"], files["EnumParameterOwner$Scope$Choice.class"]) {
						t.Fatal("proof changed original enum bytes")
					}
				})
			}
		})
	}
}
