package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLambdaRenderedPacketRequiresLiveOriginalReceipt(t *testing.T) {
	files := nativeCompileClasses(t, `class PhaseRecorder{static java.util.function.IntUnaryOperator make(int captured){return x->x+captured;}}`)
	for _, variant := range []string{"original", "duplicate observation", "snapshot isolation", "missing receipt", "foreign body", "changed template", "changed body", "removed cache", "foreign object", "missing factory", "unresolved capture", "empty output", "output bound", "packet bound", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(files["PhaseRecorder.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			if err := jdecenv.Run(map[string]string{}, func() error { _, err := d.DumpClass(); return err }); err != nil || len(d.lambdaMethodSourceBodies) != 1 {
				t.Fatal("original body/factory receipt", err)
			}
			var receipt *lambdaMethodSourceReceipt
			for _, r := range d.lambdaMethodSourceBodies {
				receipt = r
			}
			body := receipt.body
			template, output := body.code, "(int x)->{return x+captured;}"
			switch variant {
			case "missing receipt":
				delete(d.lambdaMethodSourceBodies, body.member)
			case "foreign body":
				copy := *body
				body = &copy
			case "changed template":
				template += "/*changed*/"
			case "changed body":
				body.code += "/*changed*/"
			case "removed cache":
				delete(d.dumpedMethodsSet, body.sourceCacheKey)
			case "foreign object":
				d.obj = &ClassObject{}
			case "missing factory":
				receipt.request.factory = nil
			case "unresolved capture":
				output = "x->x+\x00LCAP0\x00"
			case "empty output":
				output = ""
			case "output bound":
				output = strings.Repeat("x", (1<<20)+1)
			case "packet bound":
				for i := 0; i < 32; i++ {
					receipt.renderedPackets = append(receipt.renderedPackets, fmt.Sprint("prior", i))
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
			got := d.recordNativeLambdaRenderedPacket(body, template, output)
			want := variant == "original" || variant == "duplicate observation" || variant == "snapshot isolation"
			if got != want {
				t.Fatalf("rendered packet authority=%v want=%v", got, want)
			}
			if !got {
				return
			}
			if !d.recordNativeLambdaRenderedPacket(body, template, output) || len(receipt.renderedPackets) != 1 || receipt.renderedBytes != len(output) {
				t.Fatal("duplicate render changed receipt inventory")
			}
			if variant == "snapshot isolation" {
				snapshot, known := d.snapshotLambdaSourceConsumption()
				if !known || !d.recordNativeLambdaRenderedPacket(body, template, "x->different") {
					t.Fatal("private consumption snapshot setup")
				}
				d.restoreLambdaSourceConsumption(snapshot)
				if r := d.lambdaMethodSourceBodies[body.member]; len(r.renderedPackets) != 1 || r.renderedPackets[0] != output || r.renderedBytes != len(output) {
					t.Fatal("failed enclosing transaction leaked rendered-source authority")
				}
			}
		})
	}
}
