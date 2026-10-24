package javaclassparser

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeStableCaptureAliasProtectionRequiresExactCompletedMethodOccurrence(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original captured-name authority")
	}
	fixture := strings.ReplaceAll(sharedLambdaParameterFixture, "chunks", "var0")
	raw := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
	for _, change := range []string{"original", "quoted decoy", "unfinished frame", "missing receipt", "missing source identity", "changed physical factory", "changed capture name", "changed source binding", "removed factory", "unowned factory", "several arrows on one line", "work", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, source, methods := lambdaEmissionTestDumper(t, raw)
			var parent *dumpedMethods
			for _, method := range methods {
				if method.methodName == "make" {
					parent = method
				}
			}
			if parent == nil || !parent.lambdaEmission.closed() || len(parent.lambdaEmission.children) != 2 {
				t.Fatal("two original independent capture factories")
			}
			event := parent.lambdaEmission.children[0]
			binding := event.body.lambdaParameterBinding
			declaration, start, known := d.lambdaParentDeclaration(source, parent)
			at, position := d.lambdaEmissionPosition(parent, event.body, declaration, func(int64) bool { return true })
			if !known || !position || binding == nil || len(binding.metadata.names) == 0 || binding.metadata.names[0] != "var0" {
				t.Fatal("original factory and generated source binder")
			}
			line := strings.Count(source[:start+at], "\n")
			switch change {
			case "quoted decoy":
				source = strings.Replace(source, "{", `{ String decoy="var0 -> {";`, 1)
			case "unfinished frame":
				parent.lambdaEmission.complete = false
			case "missing receipt":
				delete(d.lambdaMethodSourceBodies, event.body.member)
			case "missing source identity":
				binding.captureSourceIDs = nil
			case "changed physical factory":
				binding.metadata.factory.code.Code[0] ^= 1
			case "changed capture name":
				binding.metadata.names[0] = "another"
			case "changed source binding":
				binding.captureContext.LocalNames[binding.captureSourceIDs[0]] = "another"
			case "removed factory":
				source = strings.Replace(source, event.packet, "null", 1)
			case "unowned factory":
				changed := strings.Replace(parent.code, "{", "{ java.util.function.IntSupplier extra=()->{return 1;};", 1)
				source = strings.Replace(source, parent.code, changed, 1)
			case "several arrows on one line":
				source = strings.ReplaceAll(source, "\n", " ")
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			proof := d.completedLambdaStableSourceBindings(source, methods)
			want := change == "original" || change == "quoted decoy"
			if proof[line]["var0"] != want || proof[line+1]["var0"] {
				t.Fatalf("factory-specific stable source binder=%v want=%v", proof, want)
			}
		})
	}
}
