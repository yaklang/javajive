package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeForwardedLambdaCapturesReopenOriginalPortsFramesAndCompositions(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original physical capture metadata")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, forwardedLambdaCaptureMetadataFixture, "UncapturedLambdaNames", "none")
	for _, change := range []string{
		"original", "parent port ID", "parent port ref", "wide word interior", "same typed captured value", "frozen source ID",
		"different parent binding", "missing parent binding", "missing parent metadata", "parent physical flags", "receiver words", "factory PC",
		"frame incomplete", "frame request", "frame seal", "forward cycle", "missing parent factory",
		"projection bytes", "projection captures", "projection frame", "projection seal", "duplicate projection",
		"composed bytes", "composed parent packet", "composed captures", "composed origin", "composed seal", "duplicate composition",
		"missing parent observation", "original parameter table", "work", "canceled",
	} {
		t.Run(change, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["UncapturedLambdaNames.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options.Mode, d.options.SourceCompiler, d.options.TargetSourceVersion = Precision, NativeJavac8, 8
			d.options.EnvSnapshot = map[string]string{}
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return raw, known }
			var source string
			err = jdecenv.Run(map[string]string{}, func() error { source, err = d.DumpClass(); return err })
			if err != nil || strings.Contains(source, DecompileStubMarker) {
				t.Fatal("original source proof", err, source)
			}
			var receipt *lambdaMethodSourceReceipt
			for _, r := range d.lambdaMethodSourceBodies {
				if r.body != nil && r.body.lambdaParameterBinding != nil && r.body.lambdaParameterBinding.hasForwardedCaptures() {
					if receipt != nil {
						t.Fatal("ambiguous authored inner factory")
					}
					receipt = r
				}
			}
			if receipt == nil || len(receipt.captureCompositions) == 0 || len(receipt.captureProjections) == 0 {
				t.Fatal("actual original factory lacks completed capture observations")
			}
			binding := receipt.body.lambdaParameterBinding
			parent := binding.forwardParent
			frame := binding.forwardEmission
			composition := receipt.captureCompositions[0]
			projection := composition.origin
			var methods []*dumpedMethods
			for _, body := range d.dumpedMethodsSet {
				if body.sourceRequest.role == methodSourceDeclaration {
					methods = append(methods, body)
				}
			}
			closed := func() bool {
				arrows, known := nativeLambdaSourceArrows(source, d.Work)
				if !known {
					return false
				}
				_, known = d.native8NestedLambdaSourceIdentityEvents(source, arrows, methods, func(n int64) bool { return nativeProofWork(d.Work, n) })
				return known
			}
			if !closed() {
				t.Fatal("independent authored source did not close before mutation")
			}
			forwarded, parentIndex := -1, -1
			for i := range binding.metadata.captures {
				if index, known := lambdaForwardedCaptureIndex(binding.metadata, i); known {
					forwarded, parentIndex = i, index
					break
				}
			}
			if forwarded < 0 || parentIndex < 0 {
				t.Fatal("missing original forwarded port")
			}
			switch change {
			case "parent port ID":
				parent.captureEntryIDs[parentIndex] = coreutils.NewRootVariableId()
			case "parent port ref":
				ref := *parent.captureEntries[parentIndex]
				ref.Id = coreutils.NewRootVariableId()
				parent.captureEntries[parentIndex] = &ref
			case "wide word interior":
				for _, operand := range binding.metadata.captures {
					if operand.result == "J" {
						operand.slot++
					}
				}
			case "same typed captured value":
				indices := []int{}
				for i, operand := range binding.metadata.captures {
					if operand.result == "I" {
						indices = append(indices, i)
					}
				}
				if len(indices) != 2 {
					t.Fatal("authored same-typed distinct ports")
				}
				binding.captured[indices[0]], binding.captured[indices[1]] = binding.captured[indices[1]], binding.captured[indices[0]]
			case "frozen source ID":
				binding.captureSourceIDs[forwarded] = coreutils.NewRootVariableId()
			case "different parent binding":
				other := *parent
				binding.forwardParent = &other
			case "missing parent binding":
				frame.body.lambdaParameterBinding = nil
			case "missing parent metadata":
				parent.metadata = nil
			case "parent physical flags":
				parent.metadata.flags[parentIndex] ^= 0x10
			case "receiver words":
				binding.metadata.receiverWords++
			case "factory PC":
				binding.metadata.factory.pc++
			case "frame incomplete":
				frame.complete = false
			case "frame request":
				frame.request.serial++
			case "frame seal":
				frame.seal = nil
			case "forward cycle":
				binding.forwardParent = binding
			case "missing parent factory":
				binding.metadata.parent.factory = nil
			case "projection bytes":
				projection.packet += " "
			case "projection captures":
				projection.captures[0].Text += "changed"
			case "projection frame":
				other := *frame
				projection.frame = &other
			case "projection seal":
				projection.seal = nil
			case "duplicate projection":
				receipt.captureProjections = append(receipt.captureProjections, projection)
			case "composed bytes":
				composition.packet += " "
			case "composed parent packet":
				composition.parentPacket += " "
			case "composed captures":
				composition.captures[0].Text += "changed"
			case "composed origin":
				other := *projection
				composition.origin = &other
			case "composed seal":
				composition.seal = nil
			case "duplicate composition":
				receipt.captureCompositions = append(receipt.captureCompositions, composition)
			case "missing parent observation":
				d.lambdaMethodSourceBodies[frame.body.member].renderedPackets = nil
			case "original parameter table":
				binding.metadata.table.Info[len(binding.metadata.table.Info)-1] ^= 1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := closed(); known != (change == "original") {
				t.Fatalf("mutated original capture lineage accepted=%v", known)
			}
		})
	}
}

func TestNativeLambdaCaptureByteCompositionKeepsCanonicalPortsAndResourceBounds(t *testing.T) {
	for _, tc := range []struct {
		name, packet, want string
		captures           map[int]string
		valid              bool
	}{
		{"literal", "literal α", "literal α", nil, true},
		{"ordered", "\x00LCAP1\x00/\x00LCAP0\x00/\x00LCAP1\x00", "B/A/B", map[int]string{0: "A", 1: "B"}, true},
		{"symbolic ancestor", "\x00LCAP0\x00", "\x00LCAP17\x00", map[int]string{0: "\x00LCAP17\x00"}, true},
		{"no recursive reinterpretation", "\x00LCAP0\x00", "\x00LCAP1\x00", map[int]string{0: "\x00LCAP1\x00", 1: "not this scope"}, true},
		{"unknown port", "\x00LCAP2\x00", "", map[int]string{0: "A"}, false},
		{"negative port", "\x00LCAP-1\x00", "", map[int]string{-1: "A"}, false},
		{"aliased port", "\x00LCAP00\x00", "", map[int]string{0: "A"}, false},
		{"wrong marker", "\x00OTHER0\x00", "", map[int]string{0: "A"}, false},
		{"truncated marker", "\x00LCAP0", "", map[int]string{0: "A"}, false},
		{"unbounded expansion", "\x00LCAP0\x00/\x00LCAP0\x00", "", map[int]string{0: strings.Repeat("x", 1<<20)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, known := lambdaComposeCaptureBytes(tc.packet, tc.captures, nil)
			if known != tc.valid || known && got != tc.want {
				t.Fatalf("composition=%q known=%v want=%q valid=%v", got, known, tc.want, tc.valid)
			}
		})
	}
	for _, change := range []string{"work", "allocation", "canceled"} {
		t.Run(change, func(t *testing.T) {
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			if change == "allocation" {
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if change == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if _, known := lambdaComposeCaptureBytes("value=\x00LCAP0\x00;", map[int]string{0: "original"}, work); known {
				t.Fatal("resource failure acquired a composed packet")
			}
		})
	}
}
