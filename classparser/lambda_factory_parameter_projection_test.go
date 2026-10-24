package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialLambdaParameterProjectionRequiresExactCloneFactory(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original named lambda metadata")
	}
	fixture := strings.ReplaceAll(guardedFinallyLambdaFixture, "EXTRA_NESTED_METHOD", "")
	fixture = strings.ReplaceAll(fixture, "EXTRA_NESTED_CHECK", "")
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "ArtifactLambdaOwner", "none", true)
	for _, variant := range []string{"first clone", "second clone", "unbound query", "declaration only", "mixed roles", "unsealed", "changed PC", "foreign object", "detached method", "detached code", "changed parameter flags", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["ArtifactLambdaOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var implementation *MemberInfo
			for _, method := range object.Methods {
				descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
				if isSyntheticMethod(method.AccessFlags) && descriptor == "(ZJ)V" {
					implementation = method
				}
			}
			if implementation == nil {
				t.Fatal("original boolean-capture/long-SAM implementation")
			}
			var factories []*lambdaSourceFactory
			for _, method := range object.Methods {
				for _, attribute := range method.Attributes {
					code, known := attribute.(*CodeAttribute)
					if !known {
						continue
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
					if err := decoder.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					for _, op := range constructorMotionOps(decoder) {
						if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
							continue
						}
						factory := &lambdaSourceFactory{object: object, method: method, code: code, pc: int(op.CurrentOffset)}
						p, known := readLambdaParameterMetadata(object, implementation, nil)
						if !known || p == nil || !factory.seal(nil) {
							t.Fatal("original named metadata/factory")
						}
						if nativeLambdaBodyUseProjection(object, implementation, nil, p, factory, false) {
							factories = append(factories, factory)
						}
					}
				}
			}
			if len(factories) != 2 || factories[0].pc == factories[1].pc || factories[0].method != factories[1].method || factories[0].code != factories[1].code {
				t.Fatal("the authored finally must own exactly two distinct raw factory copies")
			}
			p, known := readLambdaParameterMetadata(object, implementation, nil)
			if !known || p == nil || nativeLambdaBodyOriginalUseClosed(object, implementation, nil, p) {
				t.Fatal("an unbound metadata query must retain its original single-factory refusal")
			}
			p, _ = readLambdaParameterMetadata(object, implementation, nil)
			selected := factories[0]
			declarationOnly := false
			var work *workbudget.Budget
			switch variant {
			case "second clone":
				selected = factories[1]
			case "unbound query":
				selected = nil
			case "declaration only":
				selected, declarationOnly = nil, true
			case "mixed roles":
				declarationOnly = true
			case "unsealed":
				selected.sealed = false
			case "changed PC":
				selected.pc++
			case "foreign object":
				copy := *object
				selected.object = &copy
			case "detached method":
				copy := *selected.method
				selected.method = &copy
			case "detached code":
				copy := *selected.code
				selected.code = &copy
			case "changed parameter flags":
				p.flags[0] ^= 8
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeLambdaBodyUseProjection(object, implementation, work, p, selected, declarationOnly)
			want := variant == "first clone" || variant == "second clone" || variant == "declaration only"
			if got != want {
				t.Fatalf("factory-specific original metadata=%v; want %v", got, want)
			}
			if got && declarationOnly && p.factory != nil || got && !declarationOnly && !p.factory.same(selected) {
				t.Fatal("classification cannot grant an origin; regeneration must bind the exact selected occurrence")
			}
		})
	}
}
