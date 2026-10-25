package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestConstructorDeclarationWideningRequiresExactResolvedOriginalHierarchy(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, expression := range []string{"SuffixMaker.INSTANCE", "SuffixOps.make()", "new SuffixMaker()"} {
		fixture := strings.NewReplacer("OPERAND", expression, "PREFIX", `""`, "CAN_FAIL", "false").Replace(constructorForwardHierarchyFixture)
		files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "SuffixOwner", "none", false)
		for _, variant := range []string{"control", "missing subtype", "wrong identity", "unrelated parent", "missing hierarchy", "canceled", "budget"} {
			t.Run(expression+"/"+variant, func(t *testing.T) {
				objects := map[string]*ClassObject{}
				before := map[string][]byte{}
				for name, raw := range files {
					n := strings.TrimSuffix(name, ".class")
					objects[n] = mustParseLowering(t, bytes.Clone(raw))
				}
				child := objects["SuffixOwner$Member"]
				var method *MemberInfo
				for _, m := range child.Methods {
					n, _ := sourceBridgeUTF8(child, m.NameIndex)
					d, _ := sourceBridgeUTF8(child, m.DescriptorIndex)
					if n == "<init>" && d == "(LSuffixOwner;Ljava/util/List;)V" {
						method = m
					}
				}
				if method == nil {
					t.Fatal("original declaration")
				}
				var work *workbudget.Budget
				if variant == "unrelated parent" {
					objects["SuffixMaker"].Interfaces = nil
				}
				if variant == "canceled" {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				if variant == "budget" {
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				}
				for n, o := range objects {
					before[n] = bytes.Clone(o.Bytes())
				}
				resolve := func(n string) (*ClassObject, bool) {
					if variant == "missing hierarchy" || variant == "missing subtype" && n == "SuffixMaker" {
						return nil, false
					}
					if variant == "wrong identity" && n == "SuffixMaker" {
						return objects["SuffixOps"], true
					}
					o, ok := objects[n]
					return o, ok
				}
				got, known := constructorEnclosingDeclarationSignature(child, method, work, resolve)
				if known != (variant == "control") || known && got != "(LSuffixOwner;Ljava/util/List<Ljava/lang/String;>;)V" {
					t.Fatal("unproved declaration", known, got)
				}
				for n, o := range objects {
					if !bytes.Equal(before[n], o.Bytes()) {
						t.Fatal("original hierarchy changed", n)
					}
				}
			})
		}
	}
}
