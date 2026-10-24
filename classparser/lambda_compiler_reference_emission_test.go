package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

// A parsed factory and plausible textual :: are not publication receipts.
// Reopen actual completed captures, event kinds and compiler ordinal positions.
func TestAdversarialCompilerReferenceEmissionRequiresCompletedOriginalCapture(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, nullableVarargsReferenceFixture, "NullableReferenceOwner", "none", true)
			for _, variant := range []string{"original", "quoted and commented reference decoys", "missing live reference receipt", "lost representation proof", "lost representation role and proof", "missing capture observation", "wrong emitted source parameter", "changed capture text", "different capture context", "different parent frame", "unfinished parent", "changed receiver identity", "changed physical capture", "unknown reference token", "missing reference occurrence", "arrow replaced reference"} {
				t.Run(variant, func(t *testing.T) {
					object, err := Parse(bytes.Clone(files["NullableReferenceOwner.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(object)
					d.options = DecompileOptions{Mode: Precision, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }}
					d.foldSiblingResolver = d.options.Resolve
					var source string
					var methods []*dumpedMethods
					err = jdecenv.Run(map[string]string{}, func() error {
						var err error
						source, err = d.DumpClass()
						if err == nil {
							methods, err = d.DumpMethods()
						}
						return err
					})
					if err != nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
						t.Fatal("original mixed arrow/reference must close", err)
					}
					var body, parent *dumpedMethods
					for _, r := range d.lambdaMethodSourceBodies {
						if r.body.compilerReference != nil {
							if body != nil {
								t.Fatal("one bound reference")
							}
							body = r.body
						}
					}
					if body == nil || body.compilerReference.captureEmission == nil {
						t.Fatal("actual completed reference capture")
					}
					p := body.compilerReference
					e := p.captureEmission
					for _, m := range methods {
						if m.member == p.factory.method {
							parent = m
						}
					}
					if parent == nil {
						t.Fatal("factory parent")
					}
					switch variant {
					case "quoted and commented reference decoys":
						source = strings.Replace(source, "{", "{ /* :: -> */ String proofOnlyDecoy=\"::->\";", 1)
					case "missing live reference receipt":
						delete(d.lambdaMethodSourceBodies, body.member)
					case "lost representation proof":
						body.compilerReference = nil
					case "lost representation role and proof":
						body.compilerReference = nil
						body.sourceRequest.role = methodSourceLambdaBody
					case "missing capture observation":
						p.captureEmission = nil
					case "wrong emitted source parameter":
						copy := *p.parameter
						e.parameter = &copy
					case "changed capture text":
						e.text = "sameTypeButDifferentBinder"
					case "different capture context":
						e.context.FunctionName = "foreign"
					case "different parent frame":
						copy := *e.frame
						e.frame = &copy
					case "unfinished parent":
						e.frame.complete = false
					case "changed receiver identity":
						copy := *p.receiver
						p.receiver = &copy
					case "changed physical capture":
						if p.receiver == p.parameter {
							p.receiver.ResetVarType(types.NewJavaClass("java/lang/Object"))
						} else {
							p.receiver.StackVar = nil
						}
					case "unknown reference token":
						source = strings.Replace(source, "{", "{ Object extra = this::unknown;", 1)
					case "missing reference occurrence":
						source = strings.Replace(source, "::append", " /* no reference */ ", 1)
					case "arrow replaced reference":
						source = strings.Replace(source, "::append", " -> {}", 1)
					}
					want := variant == "original" || variant == "quoted and commented reference decoys"
					if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != want {
						t.Fatalf("%s publication=%v", variant, got)
					}
				})
			}
		})
	}
}
