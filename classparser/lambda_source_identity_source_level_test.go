package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialModernLambdaIdentityReopensLanguageLevelAndOriginalReceipts(t *testing.T) {
	compiler, _ := t04Tools(t)
	const fixture = `import java.util.function.IntUnaryOperator;
class ModernIdentityLevel {
 static final IntUnaryOperator WORD=value->value+1;
 static IntUnaryOperator next(int base){return value->base^value;}
}`
	for _, release := range []int{8, 11, 17, 21} {
		t.Run(fmt.Sprint(release), func(t *testing.T) {
			files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, compiler, ModernJavac, fixture, "ModernIdentityLevel", "none", nil, release, true)
			for _, scenario := range []string{"original", "inferred level", "source below lambda", "future source", "negative source", "native wrong level", "unknown compiler", "lost receipt", "detached body request", "changed ordinal", "lost source occurrence", "work", "memory", "canceled"} {
				t.Run(scenario, func(t *testing.T) {
					object, err := Parse(bytes.Clone(files["ModernIdentityLevel.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(object)
					d.options = DecompileOptions{SourceCompiler: ModernJavac, TargetSourceVersion: release, EnvSnapshot: map[string]string{}}
					source, err := d.DumpClass()
					if err != nil {
						t.Fatal("original language-level identity", err)
					}
					methods, err := d.DumpMethods()
					if err != nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) || !d.lambdaInitializerFactorySeen {
						t.Fatal("original initializer identity was not certified", err)
					}
					var receipt *lambdaMethodSourceReceipt
					for _, candidate := range d.lambdaMethodSourceBodies {
						if name, _ := sourceBridgeUTF8(object, candidate.request.factory.method.NameIndex); name == "<clinit>" {
							receipt = candidate
						}
					}
					if receipt == nil {
						t.Fatal("original initializer receipt disappeared")
					}
					switch scenario {
					case "inferred level":
						d.options.TargetSourceVersion = 0
					case "source below lambda":
						d.options.TargetSourceVersion = 7
					case "future source":
						d.options.TargetSourceVersion = 22
					case "negative source":
						d.options.TargetSourceVersion = -1
					case "native wrong level":
						d.options.SourceCompiler, d.options.TargetSourceVersion = NativeJavac8, 11
					case "unknown compiler":
						d.options.SourceCompiler = "unknown"
					case "lost receipt":
						delete(d.lambdaMethodSourceBodies, receipt.method)
					case "detached body request":
						receipt.body.sourceRequest.serial++
					case "changed ordinal":
						object.ConstantPool[receipt.method.NameIndex-1].(*ConstantUtf8Info).Value = "lambda$static$99"
					case "lost source occurrence":
						source = strings.Replace(source, "->", "+", 1)
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
					if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != (scenario == "original" || scenario == "inferred level") {
						t.Fatalf("source %d identity closed=%v for %s", release, got, scenario)
					}
					if !bytes.Equal(before, object.Bytes()) {
						t.Fatal("identity query changed original bytes")
					}
				})
			}
		})
	}
}
