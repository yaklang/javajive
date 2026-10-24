package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialCompilerLocalReferenceRequiresCompletedOriginalBinding(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("original native Java 8 compiler required")
	}
	fixture := strings.Replace(localVarargsReceiverFixture, "$BODY", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;", 1)
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "LocalReceiverOwner", "none", true)
	for _, variant := range []string{"control", "missing local proof", "wrong load PC", "wrong store PC", "wrong slot", "wrong type", "missing capture source", "foreign capture method", "foreign capture code", "wrong factory PC", "lost captured identity", "changed emission context", "changed retained context", "changed capture text", "unfinished parent", "missing factory seal", "work budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["LocalReceiverOwner.class"]))
			d := NewClassObjectDumper(obj)
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }}
			d.foldSiblingResolver = d.options.Resolve
			var source string
			var methods []*dumpedMethods
			err := jdecenv.Run(map[string]string{}, func() error {
				var err error
				source, err = d.DumpClass()
				if err == nil {
					methods, err = d.DumpMethods()
				}
				return err
			})
			if err != nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original local reference did not close", err)
			}
			var body *dumpedMethods
			for _, receipt := range d.lambdaMethodSourceBodies {
				if receipt.body.compilerReference != nil {
					if body != nil {
						t.Fatal("nonunique reference")
					}
					body = receipt.body
				}
			}
			if body == nil || body.compilerReference.local == nil || body.compilerReference.captureEmission == nil {
				t.Fatal("actual local reference proof absent")
			}
			p := body.compilerReference
			e := p.captureEmission
			switch variant {
			case "missing local proof":
				p.local = nil
			case "wrong load PC":
				p.local.pc++
			case "wrong store PC":
				p.local.local.storePC++
			case "wrong slot":
				p.local.local.slot++
			case "wrong type":
				p.local.result = "Ljava/lang/Object;"
			case "missing capture source":
				e.localSource = nil
			case "foreign capture method":
				copy := *e.localSource.method
				e.localSource.method = &copy
			case "foreign capture code":
				copy := *e.localSource.code
				e.localSource.code = &copy
			case "wrong factory PC":
				e.localSource.factoryPC++
			case "lost captured identity":
				e.localSource.values = []values.JavaValue{values.JavaNull}
			case "changed emission context":
				e.context.FunctionName = "foreign"
			case "changed retained context":
				e.localSource.context.FunctionName = "foreign"
			case "changed capture text":
				e.text = "differentBinder"
			case "unfinished parent":
				e.frame.complete = false
			case "missing factory seal":
				p.factory.sealed = false
			case "work budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != (variant == "control") {
				t.Fatalf("final local reference proof=%v variant=%s", got, variant)
			}
		})
	}
}
