package javaclassparser

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSharedLambdaSourceNeedsEachOriginalOccurrenceAndActualPacket(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, `import java.util.function.IntUnaryOperator;
class SharedPacket {
 static final IntUnaryOperator FIRST=value->value+1;
 static final IntUnaryOperator SECOND=value->value+1;
 static final IntUnaryOperator LAST=value->value-1;
}`, "SharedPacket", "none", false)
	for _, variant := range []string{"original", "lost declaration", "lost earlier occurrence", "detached occurrence body", "stale occurrence request", "lost earlier cache", "changed cached body", "changed final body", "both changed final bodies", "reordered factories", "unsealed factory", "aliased factory PC", "detached factory method", "different physical ports", "captured request", "detached frame owner", "debug lines", "native compiler", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["SharedPacket.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options = DecompileOptions{SourceCompiler: ModernJavac, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			source, err := d.DumpClass()
			if err != nil {
				t.Fatal("original shared factories", err)
			}
			var canonical *lambdaMethodSourceReceipt
			for _, receipt := range d.lambdaMethodSourceBodies {
				if len(receipt.shared) != 0 {
					if canonical != nil || len(receipt.shared) != 1 {
						t.Fatal("authored two-occurrence implementation inventory")
					}
					canonical = receipt
				}
			}
			if canonical == nil {
				t.Fatal("original javac shared implementation absent")
			}
			earlier := canonical.shared[0]
			if d.lambdaReceiptForBody(earlier.body) != earlier || !d.sharedStaticLambdaSites(earlier, canonical) {
				t.Fatal("actual earlier request was replaced by the later request")
			}
			var methods []*dumpedMethods
			for _, body := range d.dumpedMethodsSet {
				if body != nil && body.sourceRequest.role == methodSourceDeclaration && !slices.Contains(methods, body) {
					methods = append(methods, body)
				}
			}
			parent := d.dumpedMethodsSet["name:<clinit>,desc:()V"]
			if parent == nil || !d.native8LambdaSourceIdentitiesClosed(source, nil, methods) {
				t.Fatal("original final occurrence graph")
			}
			switch variant {
			case "lost declaration":
				delete(d.lambdaMethodSourceBodies, canonical.method)
			case "lost earlier occurrence":
				canonical.shared = nil
			case "detached occurrence body":
				copy := *earlier.body
				earlier.body = &copy
			case "stale occurrence request":
				earlier.request.serial++
			case "lost earlier cache":
				delete(d.dumpedMethodsSet, earlier.key)
			case "changed cached body":
				earlier.body.code += " /* stale template */"
			case "changed final body", "both changed final bodies":
				declaration, _, owned := d.lambdaParentDeclaration(source, parent)
				charge := func(int64) bool { return true }
				position, known := d.lambdaEmissionPosition(parent, earlier.body, declaration, charge)
				block, closed := d.sharedLambdaFinalBlock(parent, earlier.body, declaration, position, charge)
				mutated := strings.Replace(block, "+", "-", 1)
				if !owned || !known || !closed || mutated == block {
					t.Fatal("authored actual addition packet absent")
				}
				count := 1
				if variant == "both changed final bodies" {
					count = 2
				}
				source = strings.Replace(source, block, mutated, count)
			case "reordered factories":
				parent.lambdaEmission.children = slices.Clone(parent.lambdaEmission.children)
				parent.lambdaEmission.children[0], parent.lambdaEmission.children[1] = parent.lambdaEmission.children[1], parent.lambdaEmission.children[0]
			case "unsealed factory":
				earlier.request.factory.sealed = false
			case "aliased factory PC":
				earlier.request.factory.pc = canonical.request.factory.pc
				earlier.request.factory.sealedPC = canonical.request.factory.pc
			case "detached factory method":
				copy := *earlier.request.factory.method
				earlier.request.factory.method = &copy
			case "different physical ports":
				canonical.method.DescriptorIndex = sourceBridgePoolString(t, object, "(J)J")
			case "captured request":
				earlier.request.captureCount++
			case "detached frame owner":
				copy := *earlier.body
				earlier.body.lambdaEmission.body = &copy
			case "debug lines":
				for _, attribute := range canonical.method.Attributes {
					if code, ok := attribute.(*CodeAttribute); ok {
						code.Attributes = append(slices.Clone(code.Attributes), &UnparsedAttribute{Name: "LineNumberTable"})
					}
				}
			case "native compiler":
				d.options.SourceCompiler = NativeJavac8
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
			if got := d.native8LambdaSourceIdentitiesClosed(source, nil, methods); got != (variant == "original") {
				t.Fatalf("original occurrence identity closed=%v", got)
			}
			if !bytes.Equal(before, object.Bytes()) {
				t.Fatal("shared factory query mutated the original input")
			}
		})
	}
}
