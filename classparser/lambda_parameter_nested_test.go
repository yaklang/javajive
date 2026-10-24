package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func nestedLambdaMetadataOriginal(t *testing.T, source string) []byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original nested parameter metadata")
	}
	return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
}

func TestAdversarialLambdaOriginalUseInventoryChargesVisitsAndKeepsPayloadCap(t *testing.T) {
	raw := nestedLambdaMetadataOriginal(t, nestedLambdaParameterFixture)
	for _, oversized := range []bool{false, true} {
		d, method, _, _ := lambdaParameterTestDumper(t, raw, "(Ljava/lang/Object;Ljava/lang/String;)Z")
		// A CP-kind/length inventory does not read unused text. Hashing a
		// consumed pool is a different operation and retains its byte cost.
		count := 1
		if oversized {
			count = 129
		}
		for i := 0; i < count; i++ {
			d.obj.ConstantPool = append(d.obj.ConstantPool, &ConstantUtf8Info{Value: strings.Repeat("x", 65535)})
		}
		work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 10000})
		p, known := readLambdaParameterMetadata(d.obj, method, work)
		if !known || p == nil {
			t.Fatal("original parameter schema")
		}
		before := d.obj.Bytes()
		closed := nativeLambdaBodyOriginalUseClosed(d.obj, method, work, p)
		if closed != !oversized || !bytes.Equal(before, d.obj.Bytes()) {
			t.Fatalf("length-only inventory closed=%v oversized=%v work=%v", closed, oversized, work.Snapshot())
		}
	}
}

func TestAdversarialNestedLambdaParametersRequireOriginalAcyclicFactoryScopes(t *testing.T) {
	raw := nestedLambdaMetadataOriginal(t, nestedLambdaParameterFixture)
	wide := nestedLambdaMetadataOriginal(t, nestedLambdaRegistrationForestFixture)
	for _, variant := range []string{"original", "wrong nested owner prefix", "nonsynthetic parent", "instance parent", "synchronized parent", "bridge parent", "no ordinary root", "missing parent metadata", "parent SAM name collision", "shared enclosing factory", "wide interior write", "active predecessor", "depth bound", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			descriptor, input := "(Ljava/lang/Object;Ljava/lang/String;)Z", raw
			if variant == "wide interior write" {
				descriptor, input = "(JJ)Z", wide
			}
			d, method, _, _ := lambdaParameterTestDumper(t, input, descriptor)
			original, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || original == nil || original.parent == nil || original.parent.parent != nil || original.factory.method != original.parent.method || len(original.captures) != 1 {
				t.Fatal("independent original nested factory chain")
			}
			if variant != "wide interior write" && (!slices.Contains(original.parentNames, "item") || !slices.Contains(original.parentNames, "types") || original.captures[0].slot != 1) {
				t.Fatal("nested lexical namespace must include original SAM and ordinary ancestor")
			}
			parent := original.parent
			active := map[*MemberInfo]bool{}
			switch variant {
			case "wrong nested owner prefix":
				d.obj.ConstantPool[method.NameIndex-1].(*ConstantUtf8Info).Value = "lambda$nested$0"
			case "nonsynthetic parent":
				parent.method.AccessFlags = 0x000a
			case "instance parent":
				parent.method.AccessFlags = 0x1002
			case "synchronized parent":
				parent.method.AccessFlags |= 0x0020
			case "bridge parent":
				parent.method.AccessFlags |= 0x0040
			case "no ordinary root":
				parent.factory.method.AccessFlags = 0x100a
			case "missing parent metadata":
				for i, attribute := range parent.method.Attributes {
					if attribute == parent.table {
						parent.method.Attributes = append(parent.method.Attributes[:i], parent.method.Attributes[i+1:]...)
						break
					}
				}
			case "parent SAM name collision":
				// Keep the physical capture untouched, and collide the inner
				// SAM with an enclosing SAM source binder instead.
				index := binary.BigEndian.Uint16(parent.table.Info[5:7])
				binary.BigEndian.PutUint16(original.table.Info[5:7], index)
			case "shared enclosing factory":
				code := parent.factory.code
				code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), code.Code...)
			case "wide interior write":
				original.factory.code.Code = append([]byte{0x84, 1, 1}, original.factory.code.Code...)
			case "active predecessor":
				active[parent.method] = true
			case "depth bound":
				for i := 0; i < methodSourceLambdaDepthLimit; i++ {
					active[&MemberInfo{}] = true
				}
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
			p, closed := d.lambdaParameterMetadataInScope(method, d.Work, active)
			if (closed && p != nil) != (variant == "original") || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatalf("nested original factory scope closed=%v metadata=%v for %s", closed, p != nil, variant)
			}
		})
	}
}

func TestAdversarialNestedLambdaParametersDoNotBorrowPhysicalCapturePrefix(t *testing.T) {
	const source = `import java.util.function.*; public class UncapturedLambdaNames {
 public static Function<Object,Predicate<Object>> forward(Object outer){return(Object inner)->(Object next)->outer==next&&inner!=next;}
}`
	raw := nestedLambdaMetadataOriginal(t, source)
	d, method, _, _ := lambdaParameterTestDumper(t, raw, "(Ljava/lang/Object;Ljava/lang/Object;Ljava/lang/Object;)Z")
	before := d.obj.Bytes()
	p, known := d.originalLambdaParameterMetadata(method, nil)
	if !known || p == nil || p.parent == nil || len(p.captures) != 2 {
		t.Fatal("original physical capture graph")
	}
	// The old blanket refusal is replaced by a strictly separated obligation:
	// metadata locates the physical port but creates no source binder, parent
	// emission or substitution observation. Actual original-first fixtures
	// separately prove complete source publication and runtime identities.
	if index, known := lambdaForwardedCaptureIndex(p, 0); !known || index != 0 {
		t.Fatal("original outer word lost its parent capture port")
	}
	if _, known := lambdaForwardedCaptureIndex(p, 1); known {
		t.Fatal("same-typed enclosing SAM formal impersonated a capture prefix")
	}
	if (&lambdaParameterBinding{metadata: p}).capturedNamesClosed(nil) {
		t.Fatal("physical metadata borrowed a completed source substitution")
	}
	for _, slot := range []int{p.parent.samSlot(), p.parent.samSlot() + 1} {
		copy := *p
		copy.captures = slices.Clone(p.captures)
		operand := *p.captures[0]
		operand.slot = slot
		copy.captures[0] = &operand
		if _, known := lambdaForwardedCaptureIndex(&copy, 0); known {
			t.Fatal("different physical word borrowed the original capture name")
		}
	}
	if !bytes.Equal(before, d.obj.Bytes()) {
		t.Fatal("capture-port query modified original class bytes")
	}
}
