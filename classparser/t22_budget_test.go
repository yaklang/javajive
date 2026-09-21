package javaclassparser

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func inputHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func parseThrowFanout(t *testing.T, throws, handlers int) *core.Decompiler {
	t.Helper()
	code := make([]byte, 0, throws*3+4)
	for i := 0; i < throws; i++ {
		code = append(code, byte(core.OP_INVOKESTATIC), 0, 1)
	}
	code = append(code, byte(core.OP_RETURN))
	handlerPC := len(code)
	code = append(code, byte(core.OP_ASTORE_1), byte(core.OP_IRETURN))
	d := core.NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	for h := 0; h < handlers; h++ {
		d.ExceptionTable = append(d.ExceptionTable, &core.ExceptionTableEntry{
			StartPc:   0,
			EndPc:     uint16(handlerPC - 1),
			HandlerPc: uint16(handlerPC),
			CatchType: uint16(h + 1),
		})
	}
	d.MaxAnalysisUpdates = 1
	return d
}

func parseStoreMerge(t *testing.T, n int) *core.Decompiler {
	t.Helper()
	code := make([]byte, 0, n*5+4)
	for i := 0; i < n; i++ {
		code = append(code, byte(core.OP_ICONST_0), byte(core.OP_ISTORE_0), byte(core.OP_INVOKESTATIC), 0, 1)
	}
	code = append(code, byte(core.OP_RETURN))
	handlerPC := len(code)
	code = append(code, byte(core.OP_ASTORE_1), byte(core.OP_ILOAD_0), byte(core.OP_IRETURN))
	d := core.NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	d.ExceptionTable = []*core.ExceptionTableEntry{{
		StartPc:   0,
		EndPc:     uint16(handlerPC - 1),
		HandlerPc: uint16(handlerPC),
		CatchType: 1,
	}}
	return d
}

func TestT22_C01_graphBeforeSolve(t *testing.T) {
	const throws, handlers = 40, 40
	d := parseThrowFanout(t, throws, handlers)
	t.Logf("T22-C01 input_hash=%s throws=%d handlers=%d", inputHash(d.TestBytecodes()), throws, handlers)

	unlimited := workbudget.New(context.Background(), workbudget.Limits{})
	d.Work = unlimited
	full, err := core.BuildSemanticCFG(d)
	if err != nil {
		t.Fatalf("unlimited graph: %v", err)
	}
	nEdges := len(full.Edges)
	if nEdges < 2 {
		t.Fatalf("constructed graph too small: %d", nEdges)
	}

	t.Run("T22-C01_L-1_edges", func(t *testing.T) {
		d2 := parseThrowFanout(t, throws, handlers)
		b := workbudget.New(context.Background(), workbudget.Limits{MaxGraphEdges: int64(nEdges - 1)})
		d2.Work = b
		d2.MaxAnalysisUpdates = 1
		g, err := core.BuildSemanticCFG(d2)
		if err == nil {
			t.Fatal("expected graph_edges reject at L-1")
		}
		if !workbudget.Is(err) {
			t.Fatalf("want budget error, got %v", err)
		}
		if len(g.Edges) > nEdges-1 || b.Used(workbudget.CounterGraphEdges) > int64(nEdges-1) {
			t.Fatalf("allocated full graph: edges=%d used=%d L-1=%d", len(g.Edges), b.Used(workbudget.CounterGraphEdges), nEdges-1)
		}
		if g.Updates != 0 {
			t.Fatalf("solveSlot ran before graph bound: updates=%d", g.Updates)
		}
	})

	t.Run("T22-C01_small_L", func(t *testing.T) {
		L := int64(25)
		d2 := parseThrowFanout(t, throws, handlers)
		b := workbudget.New(context.Background(), workbudget.Limits{MaxGraphEdges: L})
		d2.Work = b
		d2.MaxAnalysisUpdates = 1
		g, err := core.BuildSemanticCFG(d2)
		if err == nil {
			t.Fatal("expected resource_limit during graph build")
		}
		var be *workbudget.Error
		if !errors.As(err, &be) {
			t.Fatalf("want *workbudget.Error, got %v", err)
		}
		if be.Kind != workbudget.KindResource || (be.Counter != workbudget.CounterGraphEdges && be.Counter != workbudget.CounterGraphScans) {
			t.Fatalf("want graph_edges/graph_scans resource_limit, got %#v", be)
		}
		if int64(len(g.Edges)) > L || b.Used(workbudget.CounterGraphEdges) > L {
			t.Fatalf("full N×M allocated: edges=%d used=%d L=%d", len(g.Edges), b.Used(workbudget.CounterGraphEdges), L)
		}
		if g.Updates != 0 {
			t.Fatal("solveSlot finished despite graph bound")
		}
	})

	t.Run("T22-C01_L_succeeds_build", func(t *testing.T) {
		d2 := parseThrowFanout(t, throws, handlers)
		b := workbudget.New(context.Background(), workbudget.Limits{MaxGraphEdges: int64(nEdges)})
		d2.Work = b
		g, err := core.BuildSemanticCFG(d2)
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Edges) != nEdges {
			t.Fatalf("edges %d want %d", len(g.Edges), nEdges)
		}
	})

	t.Run("T22-C01_public_tiny_analysis_no_graph_edges_field", func(t *testing.T) {
		opts := DecompileOptions{MaxAnalysisUpdates: 1}
		applySecureGraphBounds(&opts)
		if opts.Limits.MaxGraphEdges == 0 || opts.Limits.MaxGraphScans == 0 {
			t.Fatal("public path left graph unlimited")
		}
		d2 := parseThrowFanout(t, throws, handlers)
		d2.Work = workbudget.New(context.Background(), opts.Limits)
		d2.MaxAnalysisUpdates = 1
		g, err := core.BuildSemanticCFG(d2)
		if err == nil {
			t.Fatal("tiny MaxAnalysisUpdates left N×M graph unbounded")
		}
		if !workbudget.Is(err) {
			t.Fatalf("want budget error, got %v", err)
		}
		if int64(len(g.Edges)) > opts.Limits.MaxGraphEdges {
			t.Fatalf("allocated full graph: edges=%d cap=%d", len(g.Edges), opts.Limits.MaxGraphEdges)
		}
		if g.Updates != 0 {
			t.Fatal("solver ran before graph bound")
		}

		raw := throwFanoutClass(throws, handlers)
		r, derr := DecompileWithOptions(raw, DecompileOptions{MaxAnalysisUpdates: 1})
		if r.Status == "complete" {
			t.Fatalf("public DecompileWithOptions completed unbounded graph: %+v %v", r, derr)
		}
		if r.Status != "resource_limit" && derr == nil {
			t.Fatalf("want resource_limit on public path, got %+v %v", r, derr)
		}
		if r.Status == "unsupported" {
			t.Fatal("graph budget disguised as unsupported")
		}
	})
}

func TestT22_C02_setElementWork(t *testing.T) {
	const n, L = 40, int64(20)
	d := parseStoreMerge(t, n)
	t.Logf("T22-C02 input_hash=%s defs=%d", inputHash(d.TestBytecodes()), n)
	b := workbudget.New(context.Background(), workbudget.Limits{MaxSetElementWork: L})
	d.Work = b
	d.MaxAnalysisUpdates = 1_000_000
	g, err := core.BuildSemanticCFG(d)
	if err != nil {
		t.Fatal(err)
	}
	var load *core.OpCode
	for _, op := range g.Nodes {
		if op.Instr != nil && op.Instr.OpCode == core.OP_ILOAD_0 {
			load = op
		}
	}
	if load == nil {
		t.Fatal("missing iload_0")
	}
	g.ReachingDefinitions(load, 0)
	if g.Err == nil {
		t.Fatal("set_element_work did not trip")
	}
	var be *workbudget.Error
	if !errors.As(g.Err, &be) || be.Counter != workbudget.CounterSetElementWork {
		t.Fatalf("want set_element_work, got %v", g.Err)
	}
	if b.Used(workbudget.CounterAnalysisUpdates) >= int64(d.MaxAnalysisUpdates) {
		t.Fatalf("tripped via queue updates instead of set work: updates=%d", b.Used(workbudget.CounterAnalysisUpdates))
	}
	if b.Used(workbudget.CounterSetElementWork) > L {
		t.Fatalf("set work used %d > L=%d", b.Used(workbudget.CounterSetElementWork), L)
	}
	if b.Used(workbudget.CounterSetElementWork) == 0 {
		t.Fatal("set_element_work not billed")
	}
}

func TestT22_C03_boundaries(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("T22-C03 class_hash=%s", inputHash(raw))

	for round := 1; round <= 2; round++ {
		t.Run(fmt.Sprintf("T22-C03_round_%d", round), func(t *testing.T) {
			t.Run("T22-C03_zero_unlimited", func(t *testing.T) {
				b := workbudget.New(context.Background(), workbudget.Limits{})
				if err := b.Charge(workbudget.CounterGraphEdges, 1_000_000); err != nil {
					t.Fatal(err)
				}
				d := core.NewDecompiler([]byte{byte(core.OP_RETURN)}, nil)
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				g, err := core.BuildSemanticCFG(d)
				if err != nil {
					t.Fatal(err)
				}
				if g.MaxUpdates != 1_000_000 {
					t.Fatalf("MaxAnalysisUpdates 0 default got %d", g.MaxUpdates)
				}
			})

			t.Run("T22-C03_negative_invalid_input", func(t *testing.T) {
				cases := []DecompileOptions{
					{Limits: Limits{MaxGraphEdges: -1}},
					{Limits: Limits{MaxSetElementWork: -2}},
					{Limits: Limits{MaxRequestWork: -1}},
					{Limits: Limits{MaxOutputBytes: -1}},
					{Limits: Limits{MaxDiagnostics: -1}},
					{MaxAnalysisUpdates: -1},
				}
				for i, opt := range cases {
					r, err := DecompileWithOptions(raw, opt)
					if err == nil || r.Status != "invalid_input" {
						t.Fatalf("case %d accepted: %+v %v", i, r, err)
					}
					if strings.Contains(strings.ToLower(err.Error()), "invalid class") {
						t.Fatalf("case %d claimed invalid class: %v", i, err)
					}
				}
			})

			const L int64 = 4
			type item struct {
				name    string
				counter workbudget.Counter
				limits  workbudget.Limits
			}
			items := []item{
				{"MaxGraphEdges", workbudget.CounterGraphEdges, workbudget.Limits{MaxGraphEdges: L}},
				{"MaxSetElementWork", workbudget.CounterSetElementWork, workbudget.Limits{MaxSetElementWork: L}},
				{"MaxRequestWork", workbudget.CounterRequestWork, workbudget.Limits{MaxRequestWork: L}},
				{"MaxOutputBytes", workbudget.CounterOutputBytes, workbudget.Limits{MaxOutputBytes: L}},
				{"MaxDiagnostics", workbudget.CounterDiagnostics, workbudget.Limits{MaxDiagnostics: int(L)}},
			}
			for _, it := range items {
				it := it
				t.Run("T22-C03_"+it.name, func(t *testing.T) {
					for _, n := range []int64{L - 1, L} {
						b := workbudget.New(context.Background(), it.limits)
						if err := b.Charge(it.counter, n); err != nil {
							t.Fatalf("n=%d: %v", n, err)
						}
						buf := make([]byte, n)
						_ = buf
					}
					b := workbudget.New(context.Background(), it.limits)
					if err := b.Charge(it.counter, L+1); err == nil {
						t.Fatal("L+1 succeeded")
					} else {
						var be *workbudget.Error
						if !errors.As(err, &be) {
							t.Fatalf("L+1 not budget error: %v", err)
						}
					}
				})
			}

			t.Run("T22-C03_MaxAnalysisUpdates_Lpm1", func(t *testing.T) {
				code := []byte{core.OP_ICONST_0, core.OP_ISTORE_0, core.OP_IINC, 0, 1, core.OP_ILOAD_0, core.OP_IFNE, 255, 252, core.OP_RETURN}
				d := core.NewDecompiler(code, nil)
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				g, err := core.BuildSemanticCFG(d)
				if err != nil {
					t.Fatal(err)
				}
				inc := d.OpcodeByPC(2)
				if inc == nil {
					t.Fatal("missing iinc")
				}
				g.MaxUpdates = 1
				g.ReachingDefinitions(inc, 0)
				if g.Err == nil {
					t.Fatal("MaxUpdates=1 did not trip")
				}
				if !strings.Contains(g.Err.Error(), "analysis_budget_exceeded") {
					t.Fatalf("method-local message: %v", g.Err)
				}
			})
		})
	}
}

func TestT22_C04_crossMethod(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("T22-C04 input_hash=%s", inputHash(raw))
	ok, budget, err := decompileWithBudget(raw, DecompileOptions{MaxAnalysisUpdates: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Status == "resource_limit" {
		t.Fatalf("unlimited request hit resource_limit: %+v", ok)
	}
	total := budget.Used(workbudget.CounterRequestWork)
	t.Logf("T22-C04 unlimited snapshot=%v request_work=%d", budget.Snapshot(), total)
	if total < 2 {
		t.Fatalf("request_work not aggregated on real path: %d snapshot=%v", total, budget.Snapshot())
	}
	limit := total - 1
	fail, used, err := decompileWithBudget(raw, DecompileOptions{
		MaxAnalysisUpdates: 1_000_000,
		Limits:             Limits{MaxRequestWork: limit},
	})
	if err == nil || fail.Status != "resource_limit" {
		t.Fatalf("want resource_limit, got status=%s err=%v", fail.Status, err)
	}
	if fail.Status == "unsupported" {
		t.Fatal("request budget disguised as unsupported")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want budget error, got %v", err)
	}
	got := used.Used(workbudget.CounterRequestWork)
	if got > limit {
		t.Fatalf("used %d exceeded limit %d", got, limit)
	}
	if got == 0 {
		t.Fatal("no aggregated request work")
	}
}

func TestT22_C05_cancelAndDiagnostics(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("T22-C05 input_hash=%s", inputHash(raw))

	t.Run("T22-C05_resolve_cancel", func(t *testing.T) {
		before := runtime.NumGoroutine()
		ctx, cancel := context.WithCancel(context.Background())
		entered := make(chan struct{})
		resolve := func(string) ([]byte, bool) {
			select {
			case <-entered:
			default:
				close(entered)
			}
			select {
			case <-ctx.Done():
				return nil, false
			case <-time.After(10 * time.Second):
				return nil, false
			}
		}
		done := make(chan struct{})
		var res DecompileResult
		var derr error
		go func() {
			res, derr = DecompileWithOptions(raw, DecompileOptions{Context: ctx, Resolve: resolve})
			close(done)
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("Resolve was not called")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("DecompileWithOptions did not return after cancel")
		}
		if res.Status != "canceled" {
			t.Fatalf("status=%s err=%v", res.Status, derr)
		}
		if derr == nil || (!workbudget.Is(derr) && !errors.Is(derr, context.Canceled)) {
			t.Fatalf("canceled error: %v", derr)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if runtime.NumGoroutine() <= before+5 {
				break
			}
			runtime.Gosched()
			time.Sleep(20 * time.Millisecond)
		}
		if n := runtime.NumGoroutine(); n > before+5 {
			t.Fatalf("goroutine leak baseline=%d now=%d", before, n)
		}
	})

	t.Run("T22-C05_diagnostics_truncated", func(t *testing.T) {
		d := &ClassObjectDumper{
			report: &DecompileResult{},
			Work:   workbudget.New(context.Background(), workbudget.Limits{MaxDiagnostics: 3}),
		}
		for i := 0; i < 20; i++ {
			d.appendDiagnostic(DecompileDiagnostic{Code: "flood", Message: fmt.Sprintf("%d", i)})
		}
		if len(d.report.Diagnostics) > 4 {
			t.Fatalf("unbounded diagnostics: %d", len(d.report.Diagnostics))
		}
		if len(d.report.Diagnostics) != 4 {
			t.Fatalf("want 3+truncated, got %d", len(d.report.Diagnostics))
		}
		last := d.report.Diagnostics[len(d.report.Diagnostics)-1]
		if last.Code != "diagnostics_truncated" {
			t.Fatalf("truncated marker: %+v", last)
		}
		n := 0
		for _, diag := range d.report.Diagnostics {
			if diag.Code == "diagnostics_truncated" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("truncation recursed: %d markers", n)
		}
	})
}

func TestT22_C06_noRegression(t *testing.T) {
	for _, name := range []string{"LongTest.class", "IfTest.class"} {
		raw, err := classes.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("T22-C06_"+name, func(t *testing.T) {
			t.Logf("hash=%s", inputHash(raw))
			r, err := DecompileWithOptions(raw, DecompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if r.Status == "resource_limit" || r.Status == "canceled" {
				t.Fatalf("default options newly rejected: %+v", r)
			}
			if r.Status != "complete" && r.Status != "partial" {
				t.Fatalf("status=%s", r.Status)
			}
		})
	}
	t.Run("T22-C06_resource_not_unsupported", func(t *testing.T) {
		raw, err := classes.FS.ReadFile("LongTest.class")
		if err != nil {
			t.Fatal(err)
		}
		r, err := DecompileWithOptions(raw, DecompileOptions{Limits: Limits{MaxRequestWork: 1}})
		if r.Status == "unsupported" {
			t.Fatalf("resource error reported as unsupported: %+v %v", r, err)
		}
		if r.Status != "resource_limit" || err == nil {
			t.Fatalf("want resource_limit, got %+v %v", r, err)
		}
	})
}

func throwFanoutClass(throws, handlers int) []byte {
	var out bytes.Buffer
	u2 := func(v int) { _ = binary.Write(&out, binary.BigEndian, uint16(v)) }
	u4 := func(v int) { _ = binary.Write(&out, binary.BigEndian, uint32(v)) }
	utf := func(s string) { out.WriteByte(1); u2(len(s)); out.WriteString(s) }
	u4(0xcafebabe)
	u2(0)
	u2(49)
	u2(8)
	utf("Fixture")
	out.WriteByte(7)
	u2(1)
	utf("java/lang/Object")
	out.WriteByte(7)
	u2(3)
	utf("f")
	utf("()V")
	utf("Code")
	u2(0x21)
	u2(2)
	u2(4)
	u2(0)
	u2(0)
	u2(1)
	u2(9)
	u2(5)
	u2(6)
	u2(1)
	u2(7)
	code := make([]byte, 0, throws*2+2)
	for i := 0; i < throws; i++ {
		code = append(code, byte(core.OP_ACONST_NULL), byte(core.OP_ATHROW))
	}
	handlerPC := len(code)
	code = append(code, byte(core.OP_ASTORE_1), byte(core.OP_RETURN))
	exceptions := make([][4]int, handlers)
	for h := 0; h < handlers; h++ {
		exceptions[h] = [4]int{0, handlerPC, handlerPC, 4}
	}
	u4(12 + len(code) + 8*len(exceptions))
	u2(8)
	u2(2)
	u4(len(code))
	out.Write(code)
	u2(len(exceptions))
	for _, entry := range exceptions {
		for _, value := range entry {
			u2(value)
		}
	}
	u2(0)
	u2(0)
	return out.Bytes()
}

func TestT22JSRCopiesChargedPerClone(t *testing.T) {
	// Fixture from core/jsr_audit_test.go (parses and contains jsr/ret).
	code := []byte{
		byte(core.OP_ICONST_0), byte(core.OP_ISTORE_0),
		byte(core.OP_JSR), 0, 14, byte(core.OP_NOP),
		byte(core.OP_JSR), 0, 10, byte(core.OP_NOP),
		byte(core.OP_JSR), 0, 6, byte(core.OP_NOP),
		byte(core.OP_ILOAD_0), byte(core.OP_IRETURN),
		byte(core.OP_ASTORE_1), byte(core.OP_IINC), 0, 1, byte(core.OP_RET), 1,
	}
	d := core.NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	// Charge-before-allocate: cap 1 rejects the working-copy clone of the whole method.
	b := workbudget.New(context.Background(), workbudget.Limits{MaxNodeCopies: 1})
	d.Work = b
	err := d.TestInlineJSR()
	if err == nil {
		t.Fatal("expected node_copies reject before allocating working copies")
	}
	if !workbudget.Is(err) {
		t.Fatalf("want workbudget error, got %v", err)
	}
}

func TestT22PublicOutputCapHugeLiteral(t *testing.T) {
	payload := strings.Repeat("x", 4000)
	src := "public class HugeLit { public static String s() { return \"" + payload + "\"; } }"
	raw := compileJavaClassRelease(t, "HugeLit", src, "8")
	r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, Limits: Limits{MaxOutputBytes: 96}})
	if r.Status == "complete" && strings.Count(r.Source, "x") >= 4000 {
		t.Fatalf("emitted full 4000-char literal under MaxOutputBytes=96 status=%s err=%v", r.Status, err)
	}
	if r.Status != "resource_limit" {
		t.Fatalf("want resource_limit, got status=%s err=%v srcLen=%d", r.Status, err, len(r.Source))
	}
	if r.Source != "" {
		t.Fatalf("resource_limit returned apparent source len=%d", len(r.Source))
	}
}

func assertMaxOutputBytesLpm1(t *testing.T, name string, raw []byte, mode DecompileMode) {
	t.Helper()
	unlimited, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode})
	if err != nil && unlimited.Status != "partial" {
		t.Fatalf("%s unlimited: status=%s err=%v", name, unlimited.Status, err)
	}
	if unlimited.Status != "complete" && unlimited.Status != "partial" {
		t.Fatalf("%s unlimited status=%s err=%v", name, unlimited.Status, err)
	}
	L := int64(len(unlimited.Source))
	if L < 8 {
		t.Fatalf("%s source too small: %d %q", name, L, unlimited.Source)
	}
	for _, tc := range []struct {
		max    int64
		wantOK bool
	}{{L - 1, false}, {L, true}, {L + 1, true}} {
		r, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Limits: Limits{MaxOutputBytes: tc.max}})
		if tc.wantOK {
			if r.Status == "resource_limit" {
				t.Fatalf("%s MaxOutputBytes=%d (L=%d) resource_limit err=%v", name, tc.max, L, err)
			}
			if int64(len(r.Source)) != L {
				t.Fatalf("%s MaxOutputBytes=%d changed source len %d want L=%d", name, tc.max, len(r.Source), L)
			}
		} else {
			if r.Status != "resource_limit" {
				t.Fatalf("%s MaxOutputBytes=%d (L=%d) want resource_limit got status=%s err=%v srcLen=%d", name, tc.max, L, r.Status, err, len(r.Source))
			}
			if r.Source != "" {
				t.Fatalf("%s L-1 returned apparent source len=%d L=%d", name, len(r.Source), L)
			}
		}
	}
}

func TestT22OutputBytesLpm1LeafBranchBushyRewrite(t *testing.T) {
	ifTest, err := classes.FS.ReadFile("IfTest.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("precision_iftest", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "IfTest", ifTest, Precision)
	})
	t.Run("compatibility_iftest_rewrites", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "IfTestCompat", ifTest, Compatibility)
	})
	leafSrc := `public class OutLeaf { public static String s() { return "hello"; } }`
	leaf := compileJavaClassRelease(t, "OutLeaf", leafSrc, "8")
	t.Run("leaf_hello", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutLeaf", leaf, Precision)
	})
	escSrc := "public class OutEsc { public static String s() { return \"\\n\\0\\\"\\\\\"; } }"
	esc := compileJavaClassRelease(t, "OutEsc", escSrc, "8")
	t.Run("escaped_control", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutEsc", esc, Precision)
	})
	branchSrc := `public class OutBranch { public static String s(String a, String b) { return a + b; } }`
	branch := compileJavaClassRelease(t, "OutBranch", branchSrc, "8")
	t.Run("branch_concat", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutBranch", branch, Precision)
	})
	bushySrc := `public class OutBushy { public static String s(String a, String b, String c, String d, String e, String f, String g, String h) { return a + b + c + d + e + f + g + h; } }`
	bushy := compileJavaClassRelease(t, "OutBushy", bushySrc, "8")
	t.Run("bushy_concat", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutBushy", bushy, Precision)
	})
	intSrc := `public class OutInt { public static int s() { return 0; } }`
	intc := compileJavaClassRelease(t, "OutInt", intSrc, "8")
	t.Run("integer_zero", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutInt", intc, Precision)
	})
	boolSrc := `public class OutBool { public static boolean s() { return false; } }`
	boolc := compileJavaClassRelease(t, "OutBool", boolSrc, "8")
	t.Run("boolean_false", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutBool", boolc, Precision)
	})
	charSrc := `public class OutChar { public static char s() { return 'A'; } }`
	charc := compileJavaClassRelease(t, "OutChar", charSrc, "8")
	t.Run("char_A", func(t *testing.T) {
		assertMaxOutputBytesLpm1(t, "OutChar", charc, Precision)
	})
}

func TestT22T23SharedWorkBudgetPipeline(t *testing.T) {
	rawClass, err := classes.FS.ReadFile("IfTest.class")
	if err != nil {
		t.Fatal(err)
	}
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, err := zw.Create("IfTest.class")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(rawClass); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipBytes := zipBuf.Bytes()
	work := workbudget.New(context.Background(), workbudget.Limits{
		MaxArchiveEntries:    32,
		MaxArchiveEntryBytes: 1 << 20,
		MaxArchiveTotalBytes: 1 << 20,
		MaxArchiveDepth:      2,
		MaxGraphEdges:        1_000_000,
		MaxGraphScans:        1_000_000,
		MaxSetElementWork:    1_000_000,
		MaxOutputBytes:       1 << 20,
		MaxRequestWork:       8, // smaller than archive bytes + decompile work
	})
	limits := filesys.ArchiveLimitsFromWork(workbudget.Limits{
		MaxArchiveEntries:    32,
		MaxArchiveEntryBytes: 1 << 20,
		MaxArchiveTotalBytes: 1 << 20,
		MaxArchiveDepth:      2,
	})
	zfs, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(zipBytes), int64(len(zipBytes)),
		filesys.WithWorkBudget(work),
		filesys.WithArchiveLimits(limits),
		filesys.WithSafeArchive(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	jar := NewJarFS(zfs)
	_, rerr := jar.ReadFile("IfTest.class")
	if rerr == nil && work.Err() == nil && work.Used(workbudget.CounterRequestWork) <= 8 {
		t.Fatalf("combined pipeline did not bill archive+decompile onto request_work: %+v", work.Snapshot())
	}
	if work.Used(workbudget.CounterArchiveEntries) == 0 {
		t.Fatal("archive entries not billed on shared workbudget")
	}
	if work.Used(workbudget.CounterRequestWork) == 0 {
		t.Fatal("request_work unused")
	}
	// Combined cap 8 must trip: catalog charges entries, read charges bytes, dump charges more.
	if work.Err() == nil {
		t.Fatal("expected shared MaxRequestWork to trip on archive+decompile")
	}
	if !workbudget.Is(work.Err()) {
		t.Fatalf("want workbudget error, got %v", work.Err())
	}
}
