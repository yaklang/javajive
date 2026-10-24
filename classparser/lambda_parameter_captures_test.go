package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func capturedLambdaMetadataOriginal(t *testing.T) []byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for captured implementation metadata")
	}
	return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, capturedLambdaParameterFixture, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
}

// These mutations are refusal packets, not additional executed Java worlds.
// The independent round trip exercises actual javac8 and modern originals.
func TestNativeCapturedLambdaParametersRequireOriginalTypedFactoryLoads(t *testing.T) {
	raw := capturedLambdaMetadataOriginal(t)
	for _, variant := range []string{"original", "capture not final", "wrong capture spelling", "missing parent names", "wrong prefix type", "wide interior write", "changed load order", "load from local", "branch into prefix", "modern compiler", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, table, _ := lambdaParameterTestDumper(t, raw, "(JLjava/lang/Object;J)J")
			original, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || original == nil || len(original.captures) != 2 || original.samDescriptor != "(J)J" || original.samSlot() != 3 {
				t.Fatal("original wide physical/SAM partition")
			}
			factory := original.factory
			switch variant {
			case "capture not final":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x1000)
			case "wrong capture spelling":
				binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, d.obj, "different"))
			case "missing parent names":
				for i, attribute := range factory.method.Attributes {
					if named, ok := attribute.(*UnparsedAttribute); ok && named.Name == "MethodParameters" {
						factory.method.Attributes = append(factory.method.Attributes[:i], factory.method.Attributes[i+1:]...)
						break
					}
				}
			case "wrong prefix type":
				method.DescriptorIndex = sourceBridgePoolString(t, d.obj, "(DLjava/lang/Object;J)J")
			case "wide interior write":
				factory.code.Code = append([]byte{0x84, 1, 1}, factory.code.Code...)
			case "changed load order":
				factory.code.Code[0], factory.code.Code[1] = factory.code.Code[1], factory.code.Code[0]
			case "load from local":
				factory.code.Code[0] = 0x21 // LLOAD_3, not the original long formal.
			case "branch into prefix":
				// GOTO targets the second LOAD; the first captured word would
				// then come from an unproved incoming stack instead.
				factory.code.Code = append([]byte{0xa7, 0, 4}, factory.code.Code...)
			case "modern compiler":
				d.options.SourceCompiler = ModernJavac
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
			p, known := d.originalLambdaParameterMetadata(method, d.Work)
			if (known && p != nil) != (variant == "original") {
				t.Fatalf("capture projection=%v known=%v", p != nil, known)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("capture query mutated original class bytes")
			}
		})
	}
}

func TestNativeLambdaCaptureRegistrationRequiresCompleteOrderedMarkers(t *testing.T) {
	for _, row := range []struct {
		name, code string
		count      int
		known      bool
	}{
		{"no captures", "() -> {return 7;}", 0, true},
		{"original", "(int x) -> {return \x00LCAP0\x00+x+\x00LCAP1\x00;}", 2, true},
		{"repeated identity", "\x00LCAP0\x00+\x00LCAP0\x00+\x00LCAP1\x00", 2, true},
		{"reordered", "\x00LCAP1\x00+\x00LCAP0\x00", 2, false},
		{"omitted", "\x00LCAP0\x00", 2, false},
		{"undeclared", "\x00LCAP0\x00", 0, false},
		{"out of range", "\x00LCAP0\x00+\x00LCAP2\x00", 2, false},
		{"alias spelling", "\x00LCAP00\x00", 1, false},
		{"unknown marker", "\x00OTHER0\x00", 1, false},
		{"truncated marker", "\x00LCAP0", 1, false},
		{"negative arity", "", -1, false},
		{"unbounded arity", "", 65, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := lambdaParameterCaptureOrder(row.code, row.count, nil); got != row.known {
				t.Fatalf("capture registration=%v", got)
			}
		})
	}
}
