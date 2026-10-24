package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialNativeLambdaIdentityScannerRequiresActualArrows(t *testing.T) {
	for _, fixture := range []struct {
		source string
		count  int
		known  bool
	}{
		{`(int value)->{return value;}`, 1, true},
		{`"->"; '>'; /* -> */ // ->
(int value) -> {return "\"->";}`, 1, true},
		{`(int value)->{return other -> other;}`, 2, true},
		{`// ->`, 0, true},
		{`/* ->`, 0, false},
		{`"->`, 0, false},
		{`'->`, 0, false},
		{strings.Repeat("v->v;", 4097), 0, false},
	} {
		got, known := nativeLambdaSourceArrows(fixture.source, nil)
		if known != fixture.known || known && len(got) != fixture.count {
			t.Fatalf("arrows=%v known=%v, want %d/%v in %q", got, known, fixture.count, fixture.known, fixture.source)
		}
	}
}

func TestAdversarialNativeLambdaPacketLocatorKeepsActualOccurrences(t *testing.T) {
	const packet = `(int value)->{return value+1;}`
	for _, fixture := range []struct {
		name, source, body string
		at                 int
		known, stopSearch  bool
		canceled           bool
	}{
		{"single", packet, packet, 11, true, false, false},
		{"same formal distinct bodies", `(int value)->{return value-2;};` + packet, packet, 42, true, false, false},
		{"quoted packet", `"` + packet + `";` + packet, packet, 44, true, false, false},
		{"commented packet", `/*` + packet + `*/` + packet, packet, 45, true, false, false},
		{"independently adapted body", `(int value)->{return (T)value;}`, packet, 11, true, false, false},
		{"ambiguous adapted bodies", `(int value)->{return (T)value;};(int value)->{return (T)value;}`, packet, 0, false, false, false},
		{"identical packets", packet + ";" + packet, packet, 0, false, false, false},
		{"missing packet", `class Plain {}`, packet, 0, false, false, false},
		{"nested packet", `(int value)->{return next -> next;}`, `(int value)->{return next -> next;}`, 0, false, false, false},
		{"unclosed quote", `"` + packet, packet, 0, false, false, false},
		{"search exhausted", packet, packet, 0, false, true, false},
		{"canceled", packet, packet, 0, false, false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var work *workbudget.Budget
			if fixture.canceled {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			at, known := nativeLambdaSourcePacketArrow(fixture.source, fixture.body, work, func(int64) bool { return !fixture.stopSearch })
			if known != fixture.known || known && at != fixture.at {
				t.Fatalf("actual source arrow=%d/%v, want %d/%v", at, known, fixture.at, fixture.known)
			}
		})
	}
}

func TestAdversarialNativeLambdaIdentityReopensFinalPackets(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "literal comments", "completed member counter", "ambiguous member packet", "missing member packet", "empty member packet", "final source order", "implementation renamed", "body changed", "unowned arrow", "missing tail implementation", "missing parent", "detached parent", "missing factory", "source size", "search bound", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
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
				t.Fatal("independent original final source", err)
			}
			var unary, pinned *dumpedMethods
			for _, method := range methods {
				if method.methodName == "unary" {
					unary = method
				} else if method.methodName == "pinned" {
					pinned = method
				}
			}
			if unary == nil || pinned == nil || len(d.lambdaMethodSourceBodies) != 7 {
				t.Fatal("original source/factory inventory")
			}
			var receipt *lambdaMethodSourceReceipt
			for _, body := range d.lambdaMethodSourceBodies {
				if body.request.factory.method == unary.member {
					receipt = body
				}
			}
			if receipt == nil {
				t.Fatal("missing actual original factory receipt")
			}
			var members []string
			switch variant {
			case "literal comments":
				source = "/* -> */\n" + source + "\n// ->"
			case "completed member counter", "ambiguous member packet", "missing member packet":
				// Locator control only: ownership and completed child rendering
				// are upstream obligations. This query must ignore the supplied
				// child span's independent counter, never grant it membership.
				member := "\nstatic class CounterChild {java.util.function.IntUnaryOperator other(){return value -> value;}}\n"
				members = []string{member}
				if variant != "missing member packet" {
					at := strings.LastIndex(source, "}")
					source = source[:at] + member + source[at:]
				}
				if variant == "ambiguous member packet" {
					source += member
				}
			case "empty member packet":
				members = []string{""}
			case "final source order":
				source = strings.Replace(source, unary.code, "/*FIRST*/", 1)
				source = strings.Replace(source, pinned.code, unary.code, 1)
				source = strings.Replace(source, "/*FIRST*/", pinned.code, 1)
			case "implementation renamed":
				object.ConstantPool[receipt.method.NameIndex-1].(*ConstantUtf8Info).Value = "lambda$unary$8"
			case "body changed":
				receipt.body.code += "/* changed */"
			case "unowned arrow":
				source += "\nclass OtherScope {java.util.function.IntUnaryOperator field=value -> value;}"
			case "missing tail implementation":
				for _, method := range methods {
					if method.methodName == "identity" {
						source = strings.Replace(source, method.code, "", 1)
					}
				}
				for method := range d.lambdaMethodSourceBodies {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					if name == "lambda$identity$6" {
						delete(d.lambdaMethodSourceBodies, method)
					}
				}
			case "missing parent":
				methods = nil
			case "detached parent":
				delete(d.dumpedMethodsSet, unary.sourceCacheKey)
			case "missing factory":
				receipt.body.sourceRequest.factory = nil
			case "source size":
				source = strings.Repeat(" ", 16<<20) + source
			case "search bound":
				source = strings.Repeat(" ", (16<<20)-len(source)) + source
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
			got := d.native8LambdaSourceIdentitiesClosed(source, members, methods)
			if got != (variant == "original" || variant == "literal comments" || variant == "completed member counter") || !bytes.Equal(before, object.Bytes()) {
				t.Fatalf("source identity closed=%v for %s", got, variant)
			}
		})
	}
}
