package javaclassparser

import (
	"context"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaCapturePacketUsesCompletedFactoryOccurrenceInsteadOfPreviewText(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original capture/implementation metadata")
	}
	raw := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, sharedLambdaParameterFixture, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
	for _, change := range []string{"original", "other factory preview", "missing actual packet", "duplicated actual packet", "changed template", "unknown renders", "changed receipt", "missing live receipt", "missing live body", "missing frame", "unfinished frame", "unended frame", "changed event", "changed factory", "wrong parent", "nil parent", "nil receipt", "bounded search", "work", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, _, methods := lambdaEmissionTestDumper(t, raw)
			var parent *dumpedMethods
			for _, method := range methods {
				if method.methodName == "make" {
					parent = method
				}
			}
			if parent == nil || !parent.lambdaEmission.closed() || len(parent.lambdaEmission.children) != 2 {
				t.Fatal("original independent factories require two actual completed occurrences")
			}
			frame := parent.lambdaEmission
			event := frame.children[0]
			receipt := d.lambdaMethodSourceBodies[event.body.member]
			if receipt == nil || event.packet == frame.children[1].packet {
				t.Fatal("original capture packets must be distinct before adding a preview decoy")
			}
			charge := func(n int64) bool { return n >= 0 }
			switch change {
			case "other factory preview":
				// This preview text really occurs in the same declaration, but
				// its completed event belongs to the second original factory.
				receipt.renderedPackets = append(receipt.renderedPackets, frame.children[1].packet)
			case "missing actual packet":
				receipt.renderedPackets = []string{frame.children[1].packet}
			case "duplicated actual packet":
				receipt.renderedPackets = append(receipt.renderedPackets, event.packet)
			case "changed template":
				receipt.renderedTemplate += "/* changed */"
			case "unknown renders":
				receipt.renderedUnknown = true
			case "changed receipt":
				receipt.request.serial++
			case "missing live receipt":
				delete(d.lambdaMethodSourceBodies, event.body.member)
			case "missing live body":
				delete(d.dumpedMethodsSet, receipt.key)
			case "missing frame":
				parent.lambdaEmission = nil
			case "unfinished frame":
				frame.complete = false
			case "unended frame":
				frame.ended = false
			case "changed event":
				frame.children[0].packet = frame.children[1].packet
			case "changed factory":
				receipt.request.factory.code.Code[0] ^= 1
			case "wrong parent":
				copy := *parent
				parent = &copy
			case "nil parent":
				parent = nil
			case "nil receipt":
				receipt = nil
			case "bounded search":
				charge = func(int64) bool { return false }
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			packet, known := d.lambdaCompletedCapturePacket(parent, receipt, charge)
			want := change == "original" || change == "other factory preview"
			if known != want || want && packet != event.packet {
				t.Fatalf("completed packet known=%v want=%v", known, want)
			}
		})
	}
}
