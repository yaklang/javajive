package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInitializerLambdaRegistrationRequiresOriginalRoleAndCompletedEmission(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for original initializer registration")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, initializedLambdaMetadataFixture, "UncapturedLambdaNames", "none")
			for _, variant := range []string{"original", "ordinary before initializer", "missing initializer", "duplicate initializer", "missing emission", "missing emission seal", "changed occurrence", "missing factory seal", "changed factory pc", "changed factory code", "changed factory flags", "renamed factory role", "wrong implementation component", "missing implementation receipt", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					object, err := Parse(bytes.Clone(files["UncapturedLambdaNames.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(object)
					d.options = DecompileOptions{Mode: Precision, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }}
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
						t.Fatal("original compiler initializer/factory inventory", err)
					}
					var initializer, ordinary *dumpedMethods
					for _, method := range methods {
						if method.methodName == "<clinit>" {
							initializer = method
						} else if method.methodName == "pass" {
							ordinary = method
						}
					}
					if initializer == nil || ordinary == nil || !initializer.lambdaEmission.closed() || len(initializer.lambdaEmission.children) != 2 {
						t.Fatal("original initializer must retain two completed factory occurrences")
					}
					child := initializer.lambdaEmission.children[0].body
					factory := child.sourceRequest.factory
					if component, known := lambdaFactoryNameComponent(factory); !known || component != "static" || !factory.sealed {
						t.Fatal("original JVM initializer role")
					}
					switch variant {
					case "ordinary before initializer":
						source = strings.Replace(source, ordinary.code, "", 1)
						source = strings.Replace(source, initializer.code, ordinary.code+initializer.code, 1)
					case "missing initializer":
						source = strings.Replace(source, initializer.code, "", 1)
					case "duplicate initializer":
						source = strings.Replace(source, initializer.code, initializer.code+initializer.code, 1)
					case "missing emission":
						initializer.lambdaEmission = nil
					case "missing emission seal":
						initializer.lambdaEmission.seal = nil
					case "changed occurrence":
						initializer.lambdaEmission.children[0].request.serial++
					case "missing factory seal":
						factory.sealed = false
					case "changed factory pc":
						factory.pc++
					case "changed factory code":
						factory.code.Code[0] ^= 1
					case "changed factory flags":
						factory.method.AccessFlags ^= 8
					case "renamed factory role":
						object.ConstantPool[factory.method.NameIndex-1].(*ConstantUtf8Info).Value = "static"
					case "wrong implementation component":
						object.ConstantPool[child.member.NameIndex-1].(*ConstantUtf8Info).Value = "lambda$other$0"
					case "missing implementation receipt":
						delete(d.lambdaMethodSourceBodies, child.member)
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
					if known := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); known != (variant == "original") {
						t.Fatalf("original initializer registration closed=%v for %s", known, variant)
					}
					result, known := d.native8LambdaSourceIdentityLayout(source, nil, methods)
					want := variant == "original" || variant == "ordinary before initializer"
					if known != want || !bytes.Equal(before, object.Bytes()) {
						t.Fatalf("layout=%v want=%v or changed original class for %s", known, want, variant)
					}
					if known && strings.Replace(result, ordinary.code, "", 1) != strings.Replace(source, ordinary.code, "", 1) {
						t.Fatal("layout changed anchored field/initializer/constructor bytes or their relative order")
					}
				})
			}
		})
	}
}
