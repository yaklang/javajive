package ssabuild

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

func TestT13(t *testing.T) {
	t.Run("T13-C01", TestT13_C01_PhiSwap)
	t.Run("T13-C02", TestT13_C02_BranchNull)
	t.Run("T13-C03", TestT13_C03_ThrowSiteLocals)
	t.Run("T13-C04", TestT13_C04_StackJoin)
	t.Run("T13-C05", TestT13_C05_WideIincAndCat2)
	t.Run("T13-C06", TestT13_C06_WorklistDeterminism)
}

func irOf(t *testing.T, code []byte, desc string, ex []*core.ExceptionTableEntry) *methodir.MethodIR {
	t.Helper()
	ir, err := methodir.BuildFromBytes(code, ex, methodir.MethodMeta{
		ClassName: "T", Name: "f", Descriptor: desc, Bytecode: code, IsStatic: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ir
}

func ssaOf(t *testing.T, ir *methodir.MethodIR) *Function {
	t.Helper()
	fn, err := Build(ir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return fn
}

// parseThroughProduction runs production ParseOpcode then SemanticCFG then MethodIR.
func parseThroughProduction(t *testing.T, code []byte, desc string, ex []*core.ExceptionTableEntry) (*core.SemanticCFG, *methodir.MethodIR) {
	t.Helper()
	d := core.NewDecompiler(append([]byte(nil), code...), nil)
	if ex != nil {
		d.ExceptionTable = ex
	}
	if err := d.ParseOpcode(); err != nil {
		t.Fatalf("ParseOpcode: %v", err)
	}
	cfg, err := core.SnapshotSemanticCFG(d)
	if err != nil {
		t.Fatalf("SnapshotSemanticCFG: %v", err)
	}
	ir, err := methodir.BuildFromCFG(cfg, methodir.MethodMeta{
		ClassName: "T", Name: "f", Descriptor: desc, Bytecode: append([]byte(nil), code...), IsStatic: true,
	}, d)
	if err != nil {
		t.Fatalf("MethodIR: %v", err)
	}
	return cfg, ir
}

func opcodeAt(cfg *core.SemanticCFG, pc uint16) *core.OpCode {
	for _, n := range cfg.Nodes {
		if n != nil && n.CurrentOffset == pc {
			return n
		}
	}
	return nil
}

func blockByID(fn *Function, id methodir.BlockID) (BlockFrame, bool) {
	for _, b := range fn.Blocks {
		if b.ID == id {
			return b, true
		}
	}
	return BlockFrame{}, false
}

func assertPhiOperands1to1Preds(t *testing.T, fn *Function, p Phi) {
	t.Helper()
	b, ok := blockByID(fn, p.Block)
	if !ok {
		t.Fatalf("phi block %d missing", p.Block)
	}
	preds := fn.Incoming(b.First)
	if len(preds) < 2 {
		t.Fatalf("phi at pc=%d is not a join (preds=%d)", b.First, len(preds))
	}
	if len(p.Operands) != len(preds) {
		t.Fatalf("phi operands %d want 1-1 with %d predecessor edges", len(p.Operands), len(preds))
	}
	seen := map[string]bool{}
	pred := map[string]bool{}
	for _, e := range preds {
		pred[e.ID.String()] = true
	}
	for _, op := range p.Operands {
		id := op.Edge.String()
		if seen[id] {
			t.Fatalf("phi operands share predecessor EdgeID %s (serial overwrite)", id)
		}
		seen[id] = true
		if !pred[id] {
			t.Fatalf("phi operand EdgeID %s is not a predecessor of pc=%d", id, b.First)
		}
	}
}

func TestT13_C01_PhiSwap(t *testing.T) {
	code := phiSwapBytecode()
	ir := irOf(t, code, "(I)I", nil)
	fn := ssaOf(t, ir)
	if len(fn.Phis) == 0 {
		t.Fatalf("expected loop phis, facts:\n%s", fn.Normalize())
	}
	twoOp := 0
	for _, p := range fn.Phis {
		if len(p.Operands) < 2 {
			continue
		}
		assertPhiOperands1to1Preds(t, fn, p)
		if p.Operands[0].Edge == p.Operands[1].Edge {
			t.Fatal("phi operands share edge")
		}
		twoOp++
	}
	if twoOp == 0 {
		t.Fatalf("expected a 2-operand loop phi whose operands have distinct predecessor EdgeIDs\n%s", fn.Normalize())
	}
	for n := 0; n <= 5; n++ {
		got := interpretInt(t, code, n)
		want := 12
		if n%2 == 1 {
			want = 21
		}
		if got != want {
			t.Fatalf("n=%d got %d want %d", n, got, want)
		}
	}
	wide := phiSwapWideBytecode()
	for n := 0; n <= 5; n++ {
		got := interpretLong(t, wide, n)
		want := int64(1)
		if n%2 == 1 {
			want = 0
		}
		if got != want {
			t.Fatalf("wide n=%d got %d want %d", n, got, want)
		}
	}
	wir := irOf(t, wide, "(I)J", nil)
	wfn := ssaOf(t, wir)
	if len(wfn.Phis) == 0 {
		t.Fatal("wide swap has no phis")
	}
	wideTwo := 0
	for _, p := range wfn.Phis {
		if len(p.Operands) < 2 {
			continue
		}
		assertPhiOperands1to1Preds(t, wfn, p)
		wideTwo++
	}
	if wideTwo == 0 {
		t.Fatalf("wide swap missing 2-operand phi\n%s", wfn.Normalize())
	}
}

func phiSwapBytecode() []byte {
	// static (I)I  slot0=n a=1 b=2 i=3 t=4
	var c []byte
	c = append(c, core.OP_ICONST_1, core.OP_ISTORE_1, core.OP_ICONST_2, core.OP_ISTORE_2, core.OP_ICONST_0, core.OP_ISTORE_3)
	gotoPC := len(c)
	c = append(c, core.OP_GOTO, 0, 0)
	body := len(c)
	c = append(c, core.OP_ILOAD_1, core.OP_ISTORE, 4, core.OP_ILOAD_2, core.OP_ISTORE_1, core.OP_ILOAD, 4, core.OP_ISTORE_2, core.OP_IINC, 3, 1)
	cond := len(c)
	c = append(c, core.OP_ILOAD_3, core.OP_ILOAD_0, core.OP_IF_ICMPLT, 0, 0)
	c = append(c, core.OP_ILOAD_1, core.OP_BIPUSH, 10, core.OP_IMUL, core.OP_ILOAD_2, core.OP_IADD, core.OP_IRETURN)
	off := cond - gotoPC
	c[gotoPC+1] = byte(off >> 8)
	c[gotoPC+2] = byte(off)
	ifPC := cond + 2
	delta := body - ifPC
	c[ifPC+1] = byte(delta >> 8)
	c[ifPC+2] = byte(delta)
	return c
}

func phiSwapWideBytecode() []byte {
	var c []byte
	c = append(c, core.OP_LCONST_1, core.OP_LSTORE_1, core.OP_LCONST_0, core.OP_LSTORE, 3)
	c = append(c, core.OP_ICONST_0, core.OP_ISTORE, 5)
	gotoPC := len(c)
	c = append(c, core.OP_GOTO, 0, 0)
	body := len(c)
	c = append(c, core.OP_LLOAD_1, core.OP_LSTORE, 6, core.OP_LLOAD, 3, core.OP_LSTORE_1, core.OP_LLOAD, 6, core.OP_LSTORE, 3, core.OP_IINC, 5, 1)
	cond := len(c)
	c = append(c, core.OP_ILOAD, 5, core.OP_ILOAD_0, core.OP_IF_ICMPLT, 0, 0)
	c = append(c, core.OP_LLOAD_1, core.OP_LRETURN)
	off := cond - gotoPC
	c[gotoPC+1] = byte(off >> 8)
	c[gotoPC+2] = byte(off)
	ifPC := cond + 3
	delta := body - ifPC
	c[ifPC+1] = byte(delta >> 8)
	c[ifPC+2] = byte(delta)
	return c
}

func TestT13_C02_BranchNull(t *testing.T) {
	// iload_0; ifeq Lnull; aload_1; goto Ljoin; Lnull: aconst_null; Ljoin: areturn
	c := []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 7,
		core.OP_ALOAD_1, core.OP_GOTO, 0, 4,
		core.OP_ACONST_NULL,
		core.OP_ARETURN,
	}
	ir := irOf(t, c, "(ILjava/lang/Object;)Ljava/lang/Object;", nil)
	fn := ssaOf(t, ir)
	found := false
	for _, p := range fn.Phis {
		if p.Slot.Local {
			continue
		}
		assertPhiOperands1to1Preds(t, fn, p)
		if len(p.Operands) != 2 {
			t.Fatalf("stack phi operands %d", len(p.Operands))
		}
		if p.Operands[0].Edge == p.Operands[1].Edge {
			t.Fatal("phi not 1-1 with predecessor edges")
		}
		found = true
	}
	if !found {
		t.Fatalf("missing join phi for null/ref arms\n%s", fn.Normalize())
	}
}

func TestT13_C03_ThrowSiteLocals(t *testing.T) {
	// Stores before each invoke must reach the handler; the try-exit store after the
	// last invoke must not. Layout:
	//   iconst_0; istore_0;
	//   iconst_1; istore_0; invokestatic;   // throw site 1
	//   iconst_2; istore_0; invokestatic;   // throw site 2
	//   iconst_3; istore_0; return;         // try-exit store
	//   handler: astore_1; iload_0; ireturn
	code := []byte{
		core.OP_ICONST_0, core.OP_ISTORE_0,
		core.OP_ICONST_1, core.OP_ISTORE_0, core.OP_INVOKESTATIC, 0, 1,
		core.OP_ICONST_2, core.OP_ISTORE_0, core.OP_INVOKESTATIC, 0, 1,
		core.OP_ICONST_3, core.OP_ISTORE_0, core.OP_RETURN,
		core.OP_ASTORE_1, core.OP_ILOAD_0, core.OP_IRETURN,
	}
	handlerPC := uint16(15)
	loadPC := uint16(16)
	ex := []*core.ExceptionTableEntry{{StartPc: 2, EndPc: 14, HandlerPc: handlerPC, CatchType: 1}}
	cfg, ir := parseThroughProduction(t, code, "()I", ex)
	// This synthetic constant-pool-free fixture explicitly supplies the call
	// signature; production provenance must never guess a missing descriptor.
	for i := range ir.Instrs {
		if ir.Instrs[i].Opcode == core.OP_INVOKESTATIC {
			ir.Instrs[i].Desc = "()V"
		}
	}

	var invokePCs []uint16
	var storePCs []uint16
	var lastStore uint16
	var sawStore bool
	for _, in := range ir.Instrs {
		if in.Opcode == core.OP_ISTORE_0 || (in.Opcode == core.OP_ISTORE && effectiveLocal(in) == 0) {
			lastStore = in.PC
			sawStore = true
			storePCs = append(storePCs, in.PC)
		}
		if in.Opcode == core.OP_INVOKESTATIC {
			if !sawStore {
				t.Fatal("invoke without a prior slot0 store")
			}
			invokePCs = append(invokePCs, in.PC)
		}
	}
	if len(invokePCs) != 2 {
		t.Fatalf("want 2 invoke throw sites, got %v", invokePCs)
	}
	throwStores := make([]uint16, 0, 2)
	for _, ipc := range invokePCs {
		var before uint16
		found := false
		for _, sp := range storePCs {
			if sp < ipc {
				before = sp
				found = true
			}
		}
		if !found {
			t.Fatalf("no slot0 store before invoke pc=%d", ipc)
		}
		throwStores = append(throwStores, before)
	}
	tryExit := lastStore
	if tryExit == throwStores[0] || tryExit == throwStores[1] {
		t.Fatalf("fixture missing distinct try-exit store after last invoke (throw=%v exit=%d)", throwStores, tryExit)
	}

	load := opcodeAt(cfg, loadPC)
	if load == nil {
		t.Fatalf("handler iload_0 pc=%d missing from CFG", loadPC)
	}
	defs, entry := cfg.ReachingDefinitions(load, 0)
	if entry {
		t.Fatal("handler slot0 must not include the method-entry def")
	}
	gotDefs := make([]uint16, 0, len(defs))
	for _, d := range defs {
		gotDefs = append(gotDefs, d.CurrentOffset)
	}
	sort.Slice(gotDefs, func(i, j int) bool { return gotDefs[i] < gotDefs[j] })
	wantDefs := append([]uint16(nil), throwStores...)
	sort.Slice(wantDefs, func(i, j int) bool { return wantDefs[i] < wantDefs[j] })
	if !reflect.DeepEqual(gotDefs, wantDefs) {
		t.Fatalf("SemanticCFG.ReachingDefinitions slot0=%v want throw-site stores %v (not try-exit %d)", gotDefs, wantDefs, tryExit)
	}
	for _, pc := range gotDefs {
		if pc == tryExit {
			t.Fatal("handler used the try-exit store after the last invoke")
		}
	}

	fn, err := Build(ir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var handler BlockFrame
	var haveHandler bool
	for _, b := range fn.Blocks {
		if b.First == handlerPC {
			handler = b
			haveHandler = true
		}
	}
	if !haveHandler {
		t.Fatalf("handler block pc=%d missing\n%s", handlerPC, fn.Normalize())
	}
	if handler.In.Locals[0].Kind != frametransfer.Int {
		t.Fatalf("handler local0 %s", handler.In.Canonical())
	}
	preds := fn.Incoming(handlerPC)
	if len(preds) != 2 {
		t.Fatalf("handler should have 2 throw-site preds, got %d\n%s", len(preds), fn.Normalize())
	}
	for _, e := range preds {
		if e.Kind != core.EdgeException {
			t.Fatalf("handler pred %s is not an exception edge", e.ID)
		}
	}
	phis := fn.PhisOf(handler.ID)
	var slot0 *Phi
	for i := range phis {
		if phis[i].Slot.Local && phis[i].Slot.Index == 0 {
			slot0 = &phis[i]
		}
	}
	if slot0 == nil {
		t.Fatalf("handler slot0 phi missing (throw-site defs must differ)\n%s", fn.Normalize())
	}
	assertPhiOperands1to1Preds(t, fn, *slot0)
	if len(slot0.Operands) != 2 {
		t.Fatalf("handler phi operands %d want 2 (throw sites, not try-exit)", len(slot0.Operands))
	}
	if slot0.Operands[0].Origin.Key() == slot0.Operands[1].Origin.Key() {
		t.Fatal("handler used a single try-exit def")
	}
	var ssaPCs []uint16
	for _, op := range slot0.Operands {
		if op.Origin.Kind != OriginInstr {
			t.Fatalf("handler phi operand origin %s want throw-site store", op.Origin.Key())
		}
		if op.Origin.PC+1 == tryExit {
			t.Fatal("handler phi includes the try-exit store after the last invoke")
		}
		// A store preserves the producing value identity. Verify the store
		// independently, then compare its immediately preceding constant producer.
		producer := op.Origin.PC
		if int(producer)+1 >= len(code) || code[producer] < core.OP_ICONST_0 || code[producer] > core.OP_ICONST_3 || code[producer+1] != core.OP_ISTORE_0 {
			t.Fatalf("unexpected producer/store pair at %d", producer)
		}
		ssaPCs = append(ssaPCs, producer+1)
	}
	sort.Slice(ssaPCs, func(i, j int) bool { return ssaPCs[i] < ssaPCs[j] })
	if !reflect.DeepEqual(ssaPCs, wantDefs) {
		t.Fatalf("SSA handler slot0 origins %v want CFG throw-site stores %v", ssaPCs, wantDefs)
	}
}

func TestT13_C04_StackJoin(t *testing.T) {
	legal := []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 7,
		core.OP_ICONST_1, core.OP_GOTO, 0, 4,
		core.OP_ICONST_2,
		core.OP_IRETURN,
	}
	binary.BigEndian.PutUint16(legal[2:], 7)
	binary.BigEndian.PutUint16(legal[6:], 4)
	_, ir := parseThroughProduction(t, legal, "(I)I", nil)
	if _, err := Build(ir, Options{}); err != nil {
		t.Fatalf("legal join rejected: %v", err)
	}

	// Both arms goto a dedicated ireturn join block so the height mismatch is a join, not mid-block.
	// Arm 1 stack height 2 (iconst_1, iconst_2); arm 2 stack height 1 (iconst_3).
	illegal := []byte{
		core.OP_ILOAD_0, core.OP_IFEQ, 0, 8,
		core.OP_ICONST_1, core.OP_ICONST_2, core.OP_GOTO, 0, 7,
		core.OP_ICONST_3, core.OP_GOTO, 0, 3,
		core.OP_IRETURN,
	}
	binary.BigEndian.PutUint16(illegal[2:], 8)
	binary.BigEndian.PutUint16(illegal[7:], 7)
	binary.BigEndian.PutUint16(illegal[11:], 3)
	cfg2, ir2 := parseThroughProduction(t, illegal, "(I)I", nil)
	joinPC := uint16(len(illegal) - 1)
	if illegal[joinPC] != core.OP_IRETURN {
		t.Fatalf("illegal fixture join is not ireturn")
	}
	var joinPreds int
	var gotos int
	for _, e := range cfg2.Edges {
		if e.To != nil && e.To.CurrentOffset == joinPC {
			joinPreds++
			if e.From != nil && e.From.Instr != nil && (e.From.Instr.OpCode == core.OP_GOTO || e.From.Instr.OpCode == core.OP_GOTO_W) {
				gotos++
			}
		}
	}
	if joinPreds != 2 || gotos != 2 {
		t.Fatalf("illegal fixture must be a two-pred join of goto arms, preds=%d gotos=%d", joinPreds, gotos)
	}
	_, err := Build(ir2, Options{})
	if err == nil {
		t.Fatal("illegal stack height join accepted")
	}
	if !strings.Contains(err.Error(), "stack height mismatch") {
		t.Fatalf("illegal join rejected for the wrong reason: %v", err)
	}
}

func TestT13_C05_WideIincAndCat2(t *testing.T) {
	const wideSlot = 300 // 0x012C
	var idx [2]byte
	binary.BigEndian.PutUint16(idx[:], wideSlot)
	if idx[0] != 1 || idx[1] != 44 {
		t.Fatalf("slot 300 encoding: %v", idx)
	}
	wide := []byte{
		core.OP_ICONST_5,
		core.OP_WIDE, core.OP_ISTORE, idx[0], idx[1],
		core.OP_WIDE, core.OP_IINC, idx[0], idx[1], 0, 1,
		core.OP_WIDE, core.OP_ILOAD, idx[0], idx[1],
		core.OP_IRETURN,
	}
	if !bytes.Contains(wide, []byte{core.OP_WIDE, core.OP_ISTORE, 0x01, 0x2C}) ||
		!bytes.Contains(wide, []byte{core.OP_WIDE, core.OP_IINC, 0x01, 0x2C, 0, 1}) ||
		!bytes.Contains(wide, []byte{core.OP_WIDE, core.OP_ILOAD, 0x01, 0x2C}) {
		t.Fatal("bytecode must contain wide istore/iinc/iload index 300")
	}
	if interpretInt(t, wide, 0) != 6 {
		t.Fatal("wide istore/iinc/iload 300 did not evaluate to 6")
	}

	cfg, ir := parseThroughProduction(t, wide, "()I", nil)
	var sawWideIstore, sawWideIinc, sawWideIload bool
	var storePC, iincPC, loadPC uint16
	for _, n := range cfg.Nodes {
		if n == nil || n.Instr == nil || !n.IsWide {
			continue
		}
		switch n.Instr.OpCode {
		case core.OP_ISTORE:
			if core.GetStoreIdx(n) != wideSlot {
				t.Fatalf("wide istore GetStoreIdx=%d want %d", core.GetStoreIdx(n), wideSlot)
			}
			sawWideIstore, storePC = true, n.CurrentOffset
		case core.OP_IINC:
			if core.GetStoreIdx(n) != wideSlot || core.GetRetrieveIdx(n) != wideSlot {
				t.Fatalf("wide iinc slot store=%d retrieve=%d want %d", core.GetStoreIdx(n), core.GetRetrieveIdx(n), wideSlot)
			}
			sawWideIinc, iincPC = true, n.CurrentOffset
		case core.OP_ILOAD:
			if core.GetRetrieveIdx(n) != wideSlot {
				t.Fatalf("wide iload GetRetrieveIdx=%d want %d", core.GetRetrieveIdx(n), wideSlot)
			}
			sawWideIload, loadPC = true, n.CurrentOffset
		}
	}
	if !sawWideIstore || !sawWideIinc || !sawWideIload {
		t.Fatalf("ParseOpcode missed wide ops istore=%v iinc=%v iload=%v", sawWideIstore, sawWideIinc, sawWideIload)
	}

	for _, in := range ir.Instrs {
		switch in.PC {
		case storePC, iincPC, loadPC:
			if !in.Wide {
				t.Fatalf("MethodIR pc=%d op=%#x not marked wide", in.PC, in.Opcode)
			}
			if in.Local != wideSlot {
				t.Fatalf("MethodIR pc=%d Local=%d want %d (ghost 1/44?)", in.PC, in.Local, wideSlot)
			}
		}
	}

	fn, err := Build(ir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	byPC := map[uint16]BlockFrame{}
	for _, b := range fn.Blocks {
		byPC[b.First] = b
		if len(b.Out.Locals) > 1 && b.Out.Locals[1].Kind == frametransfer.Int {
			t.Fatalf("ghost slot 1 is INT; iinc/store must use slot %d\n%s", wideSlot, fn.Normalize())
		}
		if len(b.Out.Locals) > 44 && b.Out.Locals[44].Kind == frametransfer.Int {
			t.Fatalf("ghost slot 44 is INT; iinc/store must use slot %d\n%s", wideSlot, fn.Normalize())
		}
	}
	afterStore, okS := byPC[storePC]
	afterIinc, okI := byPC[iincPC]
	afterLoad, okL := byPC[loadPC]
	if !okS || !okI || !okL {
		t.Fatalf("missing SSA blocks for wide store/iinc/load pcs %d/%d/%d\n%s", storePC, iincPC, loadPC, fn.Normalize())
	}
	for label, b := range []BlockFrame{afterStore, afterIinc, afterLoad} {
		if len(b.Out.Locals) <= wideSlot || b.Out.Locals[wideSlot].Kind != frametransfer.Int {
			t.Fatalf("locals[%d] not INT after wide op %d (pc=%d): %s\n%s", wideSlot, label, b.First, b.Out.Canonical(), fn.Normalize())
		}
	}
	if len(afterIinc.OutOrig) <= wideSlot {
		t.Fatalf("missing origin for local %d after iinc", wideSlot)
	}
	og := afterIinc.OutOrig[wideSlot]
	if og.Kind != OriginInstr || og.PC != iincPC {
		t.Fatalf("iinc def origin %+v want instr pc=%d slot %d, not ghost 1/44", og, iincPC, wideSlot)
	}
	if og.Slot != 0 && og.Slot != wideSlot {
		t.Fatalf("iinc origin slot %d want %d", og.Slot, wideSlot)
	}
	if len(afterLoad.OutOrig) <= wideSlot {
		t.Fatalf("missing origin for local %d at iload", wideSlot)
	}
	use := afterLoad.OutOrig[wideSlot]
	if use.Kind != OriginInstr || use.PC != iincPC {
		t.Fatalf("iload use of slot %d origin %+v want iinc pc=%d", wideSlot, use, iincPC)
	}

	overlap := []byte{core.OP_LCONST_0, core.OP_LSTORE_0, core.OP_ICONST_1, core.OP_ISTORE_1, core.OP_ILOAD_1, core.OP_IRETURN}
	_, ir2 := parseThroughProduction(t, overlap, "()I", nil)
	ref := slowJVMOverlapLocals(ir2)
	fn2, err := Build(ir2, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var afterTail, afterOverlapLoad, afterRet BlockFrame
	var haveTail, haveOverlapLoad, haveRet bool
	for _, b := range fn2.Blocks {
		ins, ok := ir2.InstrByID(methodir.InstrID(b.First))
		if !ok {
			continue
		}
		switch ins.Opcode {
		case core.OP_LSTORE_0:
			if len(b.Out.Locals) < 2 || b.Out.Locals[0].Kind != frametransfer.Long {
				t.Fatalf("lstore_0 did not occupy slot 0 as long: %s", b.Out.Canonical())
			}
		case core.OP_ISTORE_1:
			afterTail, haveTail = b, true
		case core.OP_ILOAD_1:
			afterOverlapLoad, haveOverlapLoad = b, true
		case core.OP_IRETURN:
			afterRet, haveRet = b, true
		}
		if len(b.Out.Locals) > 1 && b.Out.Locals[1].Kind == frametransfer.Int && b.Out.Locals[0].Kind == frametransfer.Long {
			t.Fatal("ghost category-2 head survived overlap")
		}
	}
	if !haveTail || !haveOverlapLoad || !haveRet {
		t.Fatalf("overlap missing istore_1/iload_1/ireturn frames")
	}
	for _, out := range []frametransfer.Frame{afterTail.Out, afterOverlapLoad.Out, afterRet.Out} {
		if len(out.Locals) < 2 {
			t.Fatalf("overlap frame too small: %s", out.Canonical())
		}
		n := len(ref)
		if len(out.Locals) > n {
			n = len(out.Locals)
		}
		for i := 0; i < n; i++ {
			var rk, sk frametransfer.Kind = frametransfer.Top, frametransfer.Top
			if i < len(ref) {
				rk = ref[i]
			}
			if i < len(out.Locals) {
				sk = out.Locals[i].Kind
			}
			if rk != sk {
				t.Fatalf("overlap slot %d SSA %s independent JVM ref %s (ssa=%s ref=%v)", i, sk, rk, out.Canonical(), ref)
			}
		}
		if out.Locals[0].Kind == frametransfer.Long {
			t.Fatal("head slot 0 remained a live long after cat1 store into the tail")
		}
		if out.Locals[1].Kind != frametransfer.Int {
			t.Fatalf("slot 1 after overlap want int, got %s", out.Locals[1].Kind)
		}
	}
}

func TestT13_C06_WorklistDeterminism(t *testing.T) {
	code := phiSwapBytecode()
	ir := irOf(t, code, "(I)I", nil)
	seeds := []int64{0, 1, 7, 42, 99, 20260921}
	var facts []string
	for _, seed := range seeds {
		fn, err := Build(ir, Options{Shuffle: true, ShuffleSeed: seed})
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, fn.Normalize())
	}
	plain, err := Build(ir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	facts = append(facts, plain.Normalize())
	for i := 1; i < len(facts); i++ {
		if facts[i] != facts[0] {
			label := "plain"
			if i-1 < len(seeds) {
				label = "seed"
			}
			t.Fatalf("worklist order changed facts (%s)\nA:\n%s\nB:\n%s", label, facts[0], facts[i])
		}
	}
	ctr := &LimitCounter{Max: 1}
	_, err = Build(ir, Options{Counter: ctr, MaxUpdates: 1})
	if err == nil {
		t.Fatal("budget Limit=1 did not terminate")
	}
	if !strings.Contains(err.Error(), "analysis_budget_exceeded") {
		t.Fatalf("Limit=1 wrong error: %v", err)
	}
}

// slowJVMOverlapLocals applies JVMS 2.6.1 local overlap independently of Build.
func slowJVMOverlapLocals(ir *methodir.MethodIR) []frametransfer.Kind {
	var slots []frametransfer.Kind
	for _, in := range ir.Instrs {
		acc := core.LocalAccessOf(in.Opcode)
		if !acc.Write {
			continue
		}
		idx := effectiveLocal(in)
		if idx < 0 {
			continue
		}
		head := frametransfer.Int
		switch in.Opcode {
		case core.OP_LSTORE, core.OP_LSTORE_0, core.OP_LSTORE_1, core.OP_LSTORE_2, core.OP_LSTORE_3:
			head = frametransfer.Long
		case core.OP_DSTORE, core.OP_DSTORE_0, core.OP_DSTORE_1, core.OP_DSTORE_2, core.OP_DSTORE_3:
			head = frametransfer.Double
		case core.OP_FSTORE, core.OP_FSTORE_0, core.OP_FSTORE_1, core.OP_FSTORE_2, core.OP_FSTORE_3:
			head = frametransfer.Float
		case core.OP_ASTORE, core.OP_ASTORE_0, core.OP_ASTORE_1, core.OP_ASTORE_2, core.OP_ASTORE_3:
			head = frametransfer.Ref
		case core.OP_ISTORE, core.OP_ISTORE_0, core.OP_ISTORE_1, core.OP_ISTORE_2, core.OP_ISTORE_3, core.OP_IINC:
			head = frametransfer.Int
		default:
			continue
		}
		slots = slowStoreLocal(slots, idx, head)
	}
	return slots
}

func slowStoreLocal(slots []frametransfer.Kind, idx int, head frametransfer.Kind) []frametransfer.Kind {
	width := 1
	if head.IsCat2Head() {
		width = 2
	}
	for len(slots) < idx+width {
		slots = append(slots, frametransfer.Top)
	}
	clearPair := func(at int) {
		if at < 0 || at >= len(slots) {
			return
		}
		if slots[at].IsCat2Head() {
			slots[at] = frametransfer.Top
			if at+1 < len(slots) {
				slots[at+1] = frametransfer.Top
			}
		}
		if slots[at].IsTail() {
			slots[at] = frametransfer.Top
			if at-1 >= 0 {
				slots[at-1] = frametransfer.Top
			}
		}
	}
	clearPair(idx)
	if width == 2 {
		clearPair(idx + 1)
	}
	if idx-1 >= 0 && idx-1 < len(slots) && slots[idx-1].IsCat2Head() {
		slots[idx-1] = frametransfer.Top
		slots[idx] = frametransfer.Top
	}
	slots[idx] = head
	if width == 2 {
		slots[idx+1] = frametransfer.TailOf(frametransfer.T(head)).Kind
	} else if idx+1 < len(slots) && slots[idx+1].IsTail() {
		slots[idx+1] = frametransfer.Top
	}
	return slots
}

func interpretInt(t *testing.T, code []byte, n int) int {
	t.Helper()
	locals := map[int]int64{0: int64(n)}
	var stack []int64
	push := func(v int64) { stack = append(stack, v) }
	pop := func() int64 { v := stack[len(stack)-1]; stack = stack[:len(stack)-1]; return v }
	pc := 0
	for steps := 0; steps < 10000; steps++ {
		if pc < 0 || pc >= len(code) {
			t.Fatalf("pc %d", pc)
		}
		op := int(code[pc])
		switch op {
		case core.OP_ICONST_M1, core.OP_ICONST_0, core.OP_ICONST_1, core.OP_ICONST_2, core.OP_ICONST_3, core.OP_ICONST_4, core.OP_ICONST_5:
			push(int64(op - core.OP_ICONST_0))
			pc++
		case core.OP_ISTORE_1:
			locals[1] = pop()
			pc++
		case core.OP_ISTORE_2:
			locals[2] = pop()
			pc++
		case core.OP_ISTORE_3:
			locals[3] = pop()
			pc++
		case core.OP_ISTORE:
			locals[int(code[pc+1])] = pop()
			pc += 2
		case core.OP_ILOAD_0:
			push(locals[0])
			pc++
		case core.OP_ILOAD_1:
			push(locals[1])
			pc++
		case core.OP_ILOAD_2:
			push(locals[2])
			pc++
		case core.OP_ILOAD_3:
			push(locals[3])
			pc++
		case core.OP_ILOAD:
			push(locals[int(code[pc+1])])
			pc += 2
		case core.OP_IINC:
			locals[int(code[pc+1])] += int64(int8(code[pc+2]))
			pc += 3
		case core.OP_WIDE:
			mop := int(code[pc+1])
			idx := int(binary.BigEndian.Uint16(code[pc+2:]))
			switch mop {
			case core.OP_ISTORE:
				locals[idx] = pop()
				pc += 4
			case core.OP_ILOAD:
				push(locals[idx])
				pc += 4
			case core.OP_IINC:
				locals[idx] += int64(int16(binary.BigEndian.Uint16(code[pc+4:])))
				pc += 6
			default:
				t.Fatalf("unhandled wide %#x at %d", mop, pc)
			}
		case core.OP_GOTO:
			off := int(int16(binary.BigEndian.Uint16(code[pc+1:])))
			pc += off
		case core.OP_IF_ICMPLT:
			b := pop()
			a := pop()
			off := int(int16(binary.BigEndian.Uint16(code[pc+1:])))
			if a < b {
				pc += off
			} else {
				pc += 3
			}
		case core.OP_BIPUSH:
			push(int64(int8(code[pc+1])))
			pc += 2
		case core.OP_IMUL:
			b := pop()
			a := pop()
			push(a * b)
			pc++
		case core.OP_IADD:
			b := pop()
			a := pop()
			push(a + b)
			pc++
		case core.OP_IRETURN:
			return int(pop())
		default:
			t.Fatalf("unhandled %#x at %d", op, pc)
		}
	}
	t.Fatal("timeout")
	return 0
}

func interpretLong(t *testing.T, code []byte, n int) int64 {
	t.Helper()
	locals := map[int]int64{0: int64(n)}
	var stack []int64
	pc := 0
	for steps := 0; steps < 10000; steps++ {
		op := int(code[pc])
		switch op {
		case core.OP_LCONST_0:
			stack = append(stack, 0)
			pc++
		case core.OP_LCONST_1:
			stack = append(stack, 1)
			pc++
		case core.OP_LSTORE_1:
			locals[1] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pc++
		case core.OP_LSTORE:
			locals[int(code[pc+1])] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pc += 2
		case core.OP_LLOAD_1:
			stack = append(stack, locals[1])
			pc++
		case core.OP_LLOAD:
			stack = append(stack, locals[int(code[pc+1])])
			pc += 2
		case core.OP_ICONST_0:
			stack = append(stack, 0)
			pc++
		case core.OP_ISTORE:
			locals[int(code[pc+1])] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pc += 2
		case core.OP_ILOAD:
			stack = append(stack, locals[int(code[pc+1])])
			pc += 2
		case core.OP_ILOAD_0:
			stack = append(stack, locals[0])
			pc++
		case core.OP_IINC:
			locals[int(code[pc+1])] += int64(int8(code[pc+2]))
			pc += 3
		case core.OP_GOTO:
			pc += int(int16(binary.BigEndian.Uint16(code[pc+1:])))
		case core.OP_IF_ICMPLT:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			off := int(int16(binary.BigEndian.Uint16(code[pc+1:])))
			if a < b {
				pc += off
			} else {
				pc += 3
			}
		case core.OP_LRETURN:
			return stack[len(stack)-1]
		default:
			t.Fatalf("unhandled %#x at %d", op, pc)
		}
	}
	t.Fatal("timeout")
	return 0
}
