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

func TestAdversarialInterfaceInitializerLambdaRequiresOriginalCompletedFieldEmission(t *testing.T) {
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
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, interfaceLambdaInitializerFixture, "InitPacketDriver", "none", true)
			for _, variant := range []string{"original", "cached repeat", "missing initializer owner", "detached initializer", "duplicate original initializer", "changed original Code", "changed stack limit", "changed local limit", "changed original flags", "changed field order", "changed initializer expression", "changed field packet", "missing field packet seal", "missing emission seal", "changed occurrence", "missing factory seal", "missing implementation receipt", "extra source helper", "comment impersonation", "duplicate field source", "removed field source", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					object, err := Parse(bytes.Clone(files["InitPacketContract.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(object)
					resolve := func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }
					d.options = DecompileOptions{Mode: Precision, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: resolve}
					d.foldSiblingResolver = resolve
					var source string
					var methods []*dumpedMethods
					err = jdecenv.Run(map[string]string{}, func() error {
						var e error
						source, e = d.DumpClass()
						if e == nil {
							methods, e = d.DumpMethods()
						}
						return e
					})
					p := d.interfaceLambdaInitializer
					if err != nil || p == nil || len(p.fields) != 2 || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
						t.Fatalf("original field/factory packet: %v", err)
					}
					parent := p.parent
					child := parent.lambdaEmission.children[0].body
					switch variant {
					case "cached repeat":
						var repeated string
						err = jdecenv.Run(map[string]string{}, func() error { var e error; repeated, e = d.DumpClass(); return e })
						if err != nil || repeated != source {
							t.Fatalf("idempotent source cache changed its original field packets: %v", err)
						}
					case "missing initializer owner":
						kept := []*dumpedMethods{}
						for _, method := range methods {
							if method != parent {
								kept = append(kept, method)
							}
						}
						methods = kept
					case "detached initializer":
						copy := *parent
						p.parent = &copy
					case "duplicate original initializer":
						object.Methods = append(object.Methods, parent.member)
					case "changed original Code":
						p.code.Code[0] ^= 1
					case "changed stack limit":
						p.code.MaxStack++
					case "changed local limit":
						p.code.MaxLocals++
					case "changed original flags":
						parent.member.AccessFlags ^= 8
					case "changed field order":
						p.names[0], p.names[1] = p.names[1], p.names[0]
					case "changed initializer expression":
						d.fieldDefaultValue[p.names[0]] = "null"
					case "changed field packet":
						p.fields[0] += " "
					case "missing field packet seal":
						p.fieldSeal = nil
					case "missing emission seal":
						parent.lambdaEmission.seal = nil
					case "changed occurrence":
						parent.lambdaEmission.children[0].request.serial++
					case "missing factory seal":
						child.sourceRequest.factory.sealed = false
					case "missing implementation receipt":
						delete(d.lambdaMethodSourceBodies, child.member)
					case "extra source helper":
						d.interfaceInitializerHelpers = []*dumpedMethods{{methodName: "extra", code: "static void extra(){}"}}
					case "comment impersonation":
						source = strings.Replace(source, p.fields[0], "/*"+p.fields[0]+"*/", 1)
					case "duplicate field source":
						source = strings.Replace(source, p.fields[0], p.fields[0]+p.fields[0], 1)
					case "removed field source":
						source = strings.Replace(source, p.fields[0], "", 1)
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
					want := variant == "original" || variant == "cached repeat"
					if known := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); known != want {
						t.Fatalf("field emission closure %v want %v", known, want)
					}
					if !bytes.Equal(before, object.Bytes()) {
						t.Fatal("source occurrence query changed the original class")
					}
				})
			}
		})
	}
}
