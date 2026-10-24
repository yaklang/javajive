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

func lambdaOccurrenceTestOriginal(t *testing.T) []byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original factory identities")
	}
	return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, lambdaRegistrationOccurrenceFixture, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
}

func lambdaOccurrenceTestDumper(t *testing.T, raw []byte) (*ClassObjectDumper, string, []*dumpedMethods, *dumpedMethods) {
	t.Helper()
	d, source, methods := lambdaEmissionTestDumper(t, raw)
	for _, method := range methods {
		if method.methodName == "choose" {
			if !method.lambdaEmission.closed() || len(method.lambdaEmission.children) != 2 || method.lambdaEmission.children[0].packet != method.lambdaEmission.children[1].packet {
				t.Fatal("MVP must contain two actual identical packets with different factory origins")
			}
			return d, source, methods, method
		}
	}
	t.Fatal("missing enclosing factory declaration")
	return nil, "", nil, nil
}

func lambdaEmissionTestDumper(t *testing.T, raw []byte) (*ClassObjectDumper, string, []*dumpedMethods) {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(object)
	d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
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
		t.Fatal("independently compiled factory source did not close", err)
	}
	return d, source, methods
}

// The independent runtime fixture observes implementation names through stack
// frames. These controls exercise the provenance reader: text equality alone
// cannot replace a completed, immutable sequence of actual render events.
func TestAdversarialLambdaMethodEmissionReopensActualFactoryOccurrences(t *testing.T) {
	raw := lambdaOccurrenceTestOriginal(t)
	for _, variant := range []string{"original", "changed enclosing body", "quoted header decoy", "ambiguous enclosing header", "missing enclosing header", "missing frame", "unfinished", "unended", "invalid", "missing seal", "detached frame", "different parent request", "missing occurrence", "duplicated occurrence", "reordered identical occurrences", "changed occurrence packet", "changed occurrence request", "detached occurrence body", "missing live receipt", "changed live receipt", "changed live key", "changed factory code", "changed parent header", "unowned arrow", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, source, methods, parent := lambdaOccurrenceTestDumper(t, raw)
			frame := parent.lambdaEmission
			first := frame.children[0]
			alterBody := func() {
				// A checked projection elsewhere in the method can change its
				// cached text; comments provide a harmless locator-only control.
				changed := strings.Replace(parent.code, "{", "{ /* -> */", 1)
				source = strings.Replace(source, parent.code, changed, 1)
			}
			switch variant {
			case "changed enclosing body":
				alterBody()
			case "quoted header decoy":
				alterBody()
				brace := javaIndexTopBrace(parent.code)
				source = strings.Replace(source, "{", "{ String decoy = "+strconvQuoteJavaHeader(parent.code[:brace])+";", 1)
			case "ambiguous enclosing header":
				alterBody()
				at := strings.LastIndex(source, "}")
				source = source[:at] + parent.code + source[at:]
			case "missing enclosing header":
				source = strings.Replace(source, parent.code, "", 1)
			case "missing frame":
				parent.lambdaEmission = nil
			case "unfinished":
				frame.complete = false
			case "unended":
				frame.ended = false
			case "invalid":
				frame.invalid = true
			case "missing seal":
				frame.seal = nil
			case "detached frame":
				copy := *parent
				frame.body = &copy
			case "different parent request":
				frame.request.serial++
			case "missing occurrence":
				frame.children = frame.children[:1]
			case "duplicated occurrence":
				frame.children[1] = frame.children[0]
			case "reordered identical occurrences":
				frame.children[0], frame.children[1] = frame.children[1], frame.children[0]
			case "changed occurrence packet":
				frame.children[0].packet += "/* changed */"
			case "changed occurrence request":
				frame.children[0].request.serial++
			case "detached occurrence body":
				copy := *first.body
				frame.children[0].body = &copy
			case "missing live receipt":
				delete(d.lambdaMethodSourceBodies, first.body.member)
			case "changed live receipt":
				d.lambdaMethodSourceBodies[first.body.member].request.serial++
			case "changed live key":
				delete(d.dumpedMethodsSet, first.body.sourceCacheKey)
			case "changed factory code":
				first.request.factory.code.Code[0] ^= 1
			case "changed parent header":
				source = strings.Replace(source, " choose(", " renamed(", 1)
			case "unowned arrow":
				at := strings.LastIndex(source, "}")
				source = source[:at] + "Object extra(){return (String x)->x;}" + source[at:]
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods)
			want := variant == "original" || variant == "changed enclosing body" || variant == "quoted header decoy"
			if got != want || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatalf("actual emission closed=%v, want %v for %s", got, want, variant)
			}
		})
	}
}

func strconvQuoteJavaHeader(header string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(header) + `"`
}

func TestAdversarialLambdaMethodEmissionDistinguishesPreviewAndCompletedRender(t *testing.T) {
	raw := lambdaOccurrenceTestOriginal(t)
	for _, variant := range []string{"completed", "preview only", "aborted", "duplicate render", "wrong template", "changed request", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, _, _, parent := lambdaOccurrenceTestDumper(t, raw)
			original := parent.lambdaEmission.children
			// The completed source has no active emission frame. An analysis
			// preview must neither append an occurrence nor invalidate it.
			if d.lambdaMethodRendering != nil || !d.recordLambdaMethodEmission(original[0].body, original[0].body.code, original[0].packet) || len(parent.lambdaEmission.children) != 2 {
				t.Fatal("analysis preview changed actual render events")
			}
			frame := d.beginLambdaMethodEmission(parent)
			if variant != "preview only" {
				for _, occurrence := range original {
					template := occurrence.body.code
					if variant == "wrong template" {
						template += "/* changed */"
					}
					if variant == "changed request" {
						occurrence.body.sourceRequest.serial++
					}
					d.recordLambdaMethodEmission(occurrence.body, template, occurrence.packet)
				}
			}
			if variant == "duplicate render" {
				d.recordLambdaMethodEmission(original[0].body, original[0].body.code, original[0].packet)
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.endLambdaMethodEmission(frame, variant != "aborted")
			d.endLambdaMethodEmission(frame, false) // deferred failure cannot reopen it
			_, known := d.lambdaEmissionPosition(parent, original[0].body, parent.code, func(int64) bool { return true })
			if known != (variant == "completed") || d.lambdaMethodRendering != nil {
				t.Fatalf("actual emitted occurrence=%v for %s", known, variant)
			}
		})
	}
}
