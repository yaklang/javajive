package javaclassparser

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const lambdaAdjacentPreviewFixture = `public class UncapturedLambdaNames {
 public static java.util.function.IntUnaryOperator choose(boolean branch){
  if(branch){return (int value)->value+1;}else{return (int value)->value+1;}
 }
}`

func TestAdversarialLambdaAdjacentStatelessPreviewsRetainFactoryBijection(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for distinct original factories")
	}
	raw := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, lambdaAdjacentPreviewFixture, "UncapturedLambdaNames", "none", true)["UncapturedLambdaNames.class"]
	for _, variant := range []string{"single render", "adjacent first preview", "adjacent both previews", "many adjacent previews", "nonadjacent repeat", "changed packet", "copied body", "unsealed factory", "changed factory Code", "extra actual source occurrence", "reordered distinct producers", "repeat budget", "repeat canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, source, methods, parent := lambdaOccurrenceTestDumper(t, raw)
			original := parent.lambdaEmission.children
			frame := d.beginLambdaMethodEmission(parent)
			first, second := original[0], original[1]
			var absent *ClassObjectDumper
			if absent.lambdaStatelessRepeatedPreviewClosed(first.body) || d.lambdaStatelessRepeatedPreviewClosed(nil) {
				t.Fatal("missing context granted a source observation")
			}
			if !d.recordLambdaMethodEmission(first.body, first.body.code, first.packet) {
				t.Fatal("first original producer")
			}
			bytes := frame.bytes
			valid := true
			switch variant {
			case "adjacent first preview", "adjacent both previews", "extra actual source occurrence":
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			case "many adjacent previews":
				for i := 0; i < 8; i++ {
					valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet) && valid
				}
			case "changed packet":
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet+"/* different view */")
			case "copied body":
				copy := *first.body
				valid = d.recordLambdaMethodEmission(&copy, copy.code, first.packet)
			case "unsealed factory":
				first.request.factory.sealed = false
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			case "changed factory Code":
				first.request.factory.code.Code[0] ^= 1
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			case "repeat budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			case "repeat canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			}
			if frame.bytes != bytes || len(frame.children) != 1 {
				t.Fatal("an observation acquired another source occurrence")
			}
			if variant == "reordered distinct producers" {
				d.endLambdaMethodEmission(frame, false)
				frame = d.beginLambdaMethodEmission(parent)
				valid = d.recordLambdaMethodEmission(second.body, second.body.code, second.packet) && d.recordLambdaMethodEmission(first.body, first.body.code, first.packet)
			} else {
				valid = d.recordLambdaMethodEmission(second.body, second.body.code, second.packet) && valid
				if variant == "adjacent both previews" {
					valid = d.recordLambdaMethodEmission(second.body, second.body.code, second.packet) && valid
				}
			}
			if variant == "nonadjacent repeat" {
				valid = d.recordLambdaMethodEmission(first.body, first.body.code, first.packet) && valid
			}
			d.endLambdaMethodEmission(frame, true)
			if variant == "extra actual source occurrence" {
				changed := strings.Replace(parent.code, "{", "{ java.util.function.IntUnaryOperator extra = "+first.packet+";", 1)
				source = strings.Replace(source, parent.code, changed, 1)
			}
			closed := d.native8LambdaSourceIdentitiesClosed(source, nil, methods)
			want := variant == "single render" || variant == "adjacent first preview" || variant == "adjacent both previews" || variant == "many adjacent previews"
			if closed != want || want && (!valid || len(frame.children) != 2 || !frame.closed()) {
				t.Fatalf("final original-factory/source bijection=%v, observation=%v for %s", closed, valid, variant)
			}
		})
	}
}

func TestAdversarialLambdaAdjacentPreviewDoesNotBorrowCapturedBinders(t *testing.T) {
	raw := lambdaOccurrenceTestOriginal(t)
	d, _, _, parent := lambdaOccurrenceTestDumper(t, raw)
	first := parent.lambdaEmission.children[0]
	if first.body.sourceRequest.captureCount == 0 {
		t.Fatal("original capture required for this negative control")
	}
	frame := d.beginLambdaMethodEmission(parent)
	if !d.recordLambdaMethodEmission(first.body, first.body.code, first.packet) || d.recordLambdaMethodEmission(first.body, first.body.code, first.packet) {
		t.Fatal("a rendered string borrowed a captured declaration's emission identity")
	}
	d.endLambdaMethodEmission(frame, true)
	if frame.closed() {
		t.Fatal("captured preview admitted without a fresh capture-source proof")
	}
}
