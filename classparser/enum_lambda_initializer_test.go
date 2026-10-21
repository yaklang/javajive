package javaclassparser

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumLambdaInitializerKeepsOriginalOwnerAndActualConstants(t *testing.T) {
	compiler, _ := t04Tools(t)
	const fixture = `import java.util.function.IntUnaryOperator;
enum EnumInitPacket {
 LEFT(value->value+1),RIGHT(value->value+1),CHECK(value->{if(value<0)throw new IllegalArgumentException();return value;});
 final IntUnaryOperator operation;
 EnumInitPacket(IntUnaryOperator operation){this.operation=operation;}
}`
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "EnumInitPacket", "none", false)
	for _, scenario := range []string{"original", "lost projection", "lost owner", "detached method", "detached code", "lost method cache", "changed original code", "changed stack", "changed locals", "changed constant expression", "changed actual constant body", "reordered constants", "changed recorded packet", "lost factory", "swapped equal-text factories", "lost backing helper", "changed backing helper", "non-enum class", "changed field role", "work", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["EnumInitPacket.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			source, err := d.DumpClass()
			if err != nil {
				t.Fatal("original enum initialization projection", err)
			}
			parent := d.dumpedMethodsSet["name:<clinit>,desc:()V"]
			if parent == nil || parent.enumLambdaInitializer == nil || !d.enumLambdaInitializerClosed(parent) {
				t.Fatal("lost original initializer occurrence owner")
			}
			var methods []*dumpedMethods
			for _, body := range d.dumpedMethodsSet {
				if body != nil && body.sourceRequest.role == methodSourceDeclaration && !slices.Contains(methods, body) {
					methods = append(methods, body)
				}
			}
			if !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original final source identities")
			}
			p := parent.enumLambdaInitializer
			switch scenario {
			case "lost projection":
				parent.enumLambdaInitializer = nil
			case "lost owner":
				p.parent = nil
			case "detached method":
				copy := *parent.member
				parent.member = &copy
			case "detached code":
				copy := *p.code
				p.code = &copy
			case "lost method cache":
				delete(d.dumpedMethodsSet, parent.sourceCacheKey)
			case "changed original code":
				p.code.Code[0] = core.OP_NOP
			case "changed stack":
				p.code.MaxStack++
			case "changed locals":
				p.code.MaxLocals++
			case "changed constant expression":
				d.fieldDefaultValue[p.names[0]] += " "
			case "changed actual constant body":
				mutated := strings.Replace(p.packets[0], "+", "-", 1)
				if mutated == p.packets[0] {
					t.Fatal("authored addition disappeared")
				}
				source = strings.Replace(source, p.packets[0], mutated, 1)
			case "reordered constants":
				old := strings.Join(p.packets, "")
				permuted := slices.Clone(p.packets)
				permuted[0], permuted[1] = permuted[1], permuted[0]
				source = strings.Replace(source, old, strings.Join(permuted, ""), 1)
			case "changed recorded packet":
				p.packets[0] += " "
			case "lost factory":
				parent.lambdaEmission.children = parent.lambdaEmission.children[1:]
			case "swapped equal-text factories":
				parent.lambdaEmission.children = slices.Clone(parent.lambdaEmission.children)
				parent.lambdaEmission.children[0], parent.lambdaEmission.children[1] = parent.lambdaEmission.children[1], parent.lambdaEmission.children[0]
			case "lost backing helper", "changed backing helper":
				found := false
				for i, method := range object.Methods {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
					generated, err := nativeEnumRegeneratedMethod(object, method, name, descriptor, nil)
					if !generated || err != nil || method.AccessFlags != 0x100a {
						continue
					}
					found = true
					if scenario == "lost backing helper" {
						object.Methods = slices.Delete(slices.Clone(object.Methods), i, i+1)
					} else {
						for _, attribute := range method.Attributes {
							if body, ok := attribute.(*CodeAttribute); ok {
								body.Code[0] = core.OP_ICONST_0
							}
						}
					}
					break
				}
				if !found {
					t.Fatal("original typed enum backing helper absent")
				}
			case "non-enum class":
				object.AccessFlags &^= 0x4000
			case "changed field role":
				object.Fields[0].AccessFlags &^= 0x4000
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != (scenario == "original") {
				t.Fatalf("enum initializer identity closed=%v", got)
			}
			if !bytes.Equal(before, object.Bytes()) {
				t.Fatal("initializer query mutated the original class")
			}
		})
	}
}
