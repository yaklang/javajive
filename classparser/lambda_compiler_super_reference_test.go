package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestAdversarialSuperReferenceRequiresOriginalDispatchAndCompletedReceiver(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, superReferencePortsFixture, "SuffixOwner", "none", false)
	for _, variant := range []string{"control", "quoted decoys", "lost representation", "changed representation role", "lost capture observation", "changed receiver spelling", "changed parent context", "unfinished parent", "changed original dispatch", "changed physical input", "duplicate helper exceptions", "changed superclass", "changed helper flags", "changed factory flags", "changed max stack", "changed max locals", "incomplete overload family", "uncovered checked exception", "wrong source target"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(files["SuffixOwner.class"]))
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }}
			d.foldSiblingResolver = d.options.Resolve
			var source string
			var methods []*dumpedMethods
			err := jdecenv.Run(map[string]string{}, func() error {
				var e error
				source, e = d.DumpClass()
				if e == nil {
					methods, e = d.DumpMethods()
				}
				return e
			})
			if err != nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original reference source did not close", err, source)
			}
			var body *dumpedMethods
			count := 0
			for _, receipt := range d.lambdaMethodSourceBodies {
				if receipt.body.compilerReference != nil && receipt.body.compilerReference.superReceiver {
					count++
					if receipt.body.compilerReference.shape.member.Member == "take" {
						body = receipt.body
					}
				}
			}
			if count != 2 || body == nil || body.compilerReference.captureEmission == nil {
				t.Fatal("missing completed super-reference proof", count, source)
			}
			p := body.compilerReference
			switch variant {
			case "quoted decoys":
				source = strings.Replace(source, "{", "{ /* super::take */ String decoy=\"super::step\";", 1)
			case "lost representation":
				body.compilerReference = nil
			case "changed representation role":
				p.superReceiver = false
			case "lost capture observation":
				p.captureEmission = nil
			case "changed receiver spelling":
				p.captureEmission.text = "this"
			case "changed parent context":
				p.captureEmission.context.FunctionName = "unrelated"
			case "unfinished parent":
				p.captureEmission.frame.complete = false
			case "changed original dispatch":
				// take has ALOAD_0, ILOAD_1, INVOKESPECIAL, RETURN.
				if len(p.shape.code.Code) != 6 || p.shape.code.Code[2] != core.OP_INVOKESPECIAL {
					t.Fatal("fixture dispatch layout")
				}
				p.shape.code.Code[2] = core.OP_INVOKEVIRTUAL
			case "changed physical input":
				p.shape.code.Code[1] = core.OP_ILOAD_2
			case "duplicate helper exceptions":
				p.shape.method.Attributes = append(p.shape.method.Attributes, &ExceptionsAttribute{}, &ExceptionsAttribute{})
			case "changed superclass":
				d.obj.SuperClass = d.obj.ThisClass
			case "changed helper flags":
				p.shape.method.AccessFlags |= StaticFlag
			case "changed factory flags":
				p.factory.method.AccessFlags |= StaticFlag
			case "changed max stack":
				p.shape.code.MaxStack++
			case "changed max locals":
				p.shape.code.MaxLocals++
			case "incomplete overload family", "uncovered checked exception":
				provider := p.context.InvocationMetadata
				p.context.InvocationMetadata = func(name string) (callbinding.Class, bool) {
					cl, known := provider(name)
					cl.Methods = append([]callbinding.Method(nil), cl.Methods...)
					if variant == "incomplete overload family" && name == "ReferenceBase" {
						cl.MembersComplete = false
					}
					if variant == "uncovered checked exception" && name == "CheckedAction" {
						for i := range cl.Methods {
							cl.Methods[i].Exceptions = nil
						}
					}
					return cl, known
				}
			case "wrong source target":
				d.options.TargetSourceVersion = 11
			}
			if closed := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); closed != (variant == "control" || variant == "quoted decoys") {
				t.Fatalf("accepted=%v variant=%s", closed, variant)
			}
		})
	}
}
