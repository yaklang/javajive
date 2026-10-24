package javaclassparser

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCapturedLocalLifetimeRequiresExactOriginalLastWriterAtFactory(t *testing.T) {
	raw := nestedLambdaMetadataOriginal(t, lambdaCaptureLifetimeMetadataFixture)
	for _, change := range []string{"original", "wrong read", "wrong store", "wrong slot", "wrong word", "unknown factory", "ordinary instruction", "missing code", "missing node", "different opcode object", "duplicate opcode", "clobber backedge", "exception entry bypass", "earlier overlapping write", "work", "memory", "canceled"} {
		t.Run(change, func(t *testing.T) {
			d, method, _, _ := lambdaParameterTestDumper(t, bytes.Clone(raw), "(Ljava/util/concurrent/Callable;)Ljava/lang/String;")
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil || len(p.captures) != 1 || p.captures[0].local == nil {
				t.Fatal("original typed capture and factory")
			}
			read := *p.captures[0].local
			decoder := core.NewDecompiler(p.factory.code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			flow := nativeEnumParameterOriginalFlow(decoder, p.factory.code, nil)
			ops := constructorMotionOps(decoder)
			if flow == nil || nativeLambdaLocalSingleStore(ops, &read, nil) || !nativeLambdaLocalCaptureAtFactory(ops, flow, &read, p.factory.pc, nil) {
				t.Fatal("fixture must require a point proof across distinct physical lifetimes")
			}
			before := d.obj.Bytes()
			factory := p.factory.pc
			var later *core.OpCode
			for _, op := range ops {
				if int(op.CurrentOffset) > factory && core.LocalAccessOf(op.Instr.OpCode).Write && core.GetStoreIdx(op) == read.slot {
					later = op
					break
				}
			}
			if later == nil {
				t.Fatal("missing independent later slot lifetime")
			}
			var work *workbudget.Budget
			switch change {
			case "wrong read":
				read.pc = read.storePC
			case "wrong store":
				read.storePC = int(later.CurrentOffset)
			case "wrong slot":
				read.slot++
			case "wrong word":
				read.descriptor = "J"
			case "unknown factory":
				factory = len(p.factory.code.Code) + 1
			case "ordinary instruction":
				factory = int(later.CurrentOffset)
			case "missing code":
				flow.code = nil
			case "missing node":
				flow.byPC = maps.Clone(flow.byPC)
				delete(flow.byPC, read.storePC)
			case "different opcode object":
				ops = slices.Clone(ops)
				for i, op := range ops {
					if int(op.CurrentOffset) == read.pc {
						copy := *op
						ops[i] = &copy
					}
				}
			case "duplicate opcode":
				ops = append(slices.Clone(ops), ops[0])
			case "clobber backedge":
				edge := flow.edges[later][0]
				edge.To = flow.byPC[read.pc]
				flow.edges[later] = append(slices.Clone(flow.edges[later]), edge)
			case "exception entry bypass":
				flow.edges[flow.entry] = append(slices.Clone(flow.edges[flow.entry]), core.SemanticEdge{From: flow.entry, To: flow.byPC[read.pc], Kind: core.EdgeException})
			case "earlier overlapping write":
				copy := *later
				copy.CurrentOffset = uint16(read.storePC - 1)
				ops = append(slices.Clone(ops), &copy)
				flow.byPC = maps.Clone(flow.byPC)
				flow.byPC[int(copy.CurrentOffset)] = &copy
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeLambdaLocalCaptureAtFactory(ops, flow, &read, factory, work); got != (change == "original") {
				t.Fatalf("original point capture=%v", got)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("lifetime query changed original method bytes")
			}
			if work != nil && work.Err() == nil {
				t.Fatal("bounded refusal must remain sticky")
			}
		})
	}
}
