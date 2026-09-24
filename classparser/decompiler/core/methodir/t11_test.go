package methodir

import (
	"encoding/binary"
	"flag"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if f := flag.Lookup("test.run"); f != nil {
		v := f.Value.String()
		nv := regexp.MustCompile(`T(\d+)-C(\d+)`).ReplaceAllString(v, "T${1}_C${2}")
		nv = regexp.MustCompile(`T(\d+)-`).ReplaceAllString(nv, "T${1}_")
		if nv != v {
			_ = flag.Set("test.run", nv)
		}
	}
	os.Exit(m.Run())
}

func TestT11(t *testing.T) {
	t.Run("T11-C02", TestT11_C02_StableIDs)
	t.Run("T11-C03", TestT11_C03_EdgeIdentity)
	t.Run("T11-C04", TestT11_C04_Immutability)
	t.Run("T11-C05", TestT11_C05_ConcurrentIsolation)
	t.Run("T11-C06", TestT11_C06_BadInput)
}

func meta(name string, code []byte) MethodMeta {
	return MethodMeta{ClassName: "T", Name: name, Descriptor: "(I)I", Bytecode: append([]byte(nil), code...), IsStatic: true}
}

func build(t *testing.T, code []byte, ex []*core.ExceptionTableEntry, name string) *MethodIR {
	t.Helper()
	ir, err := BuildFromBytes(code, ex, meta(name, code), nil)
	if err != nil {
		t.Fatalf("BuildFromBytes: %v", err)
	}
	return ir
}

func TestT11_C02_StableIDs(t *testing.T) {
	code := []byte{core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IADD, core.OP_IRETURN}
	var hashes []string
	for i := 0; i < 10; i++ {
		ir := build(t, code, nil, "add")
		hashes = append(hashes, ir.Hash())
		if strings.Contains(string(ir.ID), "0x") && strings.Contains(ir.Canonical(), "0x") {
			t.Fatal("pointer-like identity leaked")
		}
		if ir.Version != SnapshotVersion {
			t.Fatalf("version %d", ir.Version)
		}
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Fatalf("rebuild hash %d %s != %s", i, hashes[i], hashes[0])
		}
	}
	d := core.NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.SnapshotSemanticCFG(d)
	if err != nil {
		t.Fatal(err)
	}
	rev := append([]core.SemanticEdge(nil), cfg.Edges...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	cfg.Edges = rev
	ir2, err := BuildFromCFG(cfg, meta("add", code), d)
	if err != nil {
		t.Fatal(err)
	}
	if ir2.Hash() != hashes[0] {
		t.Fatalf("reversed edge insertion changed hash: %s vs %s", ir2.Hash(), hashes[0])
	}
}

func TestT11_C03_EdgeIdentity(t *testing.T) {
	// iload_0; iconst_1; idiv; lookupswitch with signed keys sharing a target.
	prefix := []byte{core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IDIV}
	swPC := len(prefix)
	shared, other, def := 40, 42, 44
	payload := lookupswitch(swPC, def, [][2]int32{{-5, int32(shared)}, {1, int32(shared)}, {2, int32(other)}})
	code := append(prefix, payload...)
	for len(code) < shared {
		code = append(code, core.OP_NOP)
	}
	code = append(code, core.OP_ICONST_1, core.OP_IRETURN)                    // 40
	code = append(code, core.OP_ICONST_2, core.OP_IRETURN)                    // 42
	code = append(code, core.OP_ICONST_0, core.OP_IRETURN)                    // 44
	code = append(code, core.OP_ASTORE_1, core.OP_ICONST_M1, core.OP_IRETURN) // 46 handler0
	code = append(code, core.OP_ASTORE_1, core.OP_ICONST_M1, core.OP_IRETURN) // 49 handler1
	ex := []*core.ExceptionTableEntry{
		{StartPc: 0, EndPc: uint16(shared), HandlerPc: 46, CatchType: 1},
		{StartPc: 0, EndPc: uint16(shared), HandlerPc: 49, CatchType: 0},
	}
	ir := build(t, code, ex, "sw")
	var cases []int32
	nShared := 0
	for _, e := range ir.Edges {
		if e.Kind == core.EdgeCase {
			cases = append(cases, e.CaseValue)
			if e.To == 40 {
				nShared++
			}
		}
	}
	if nShared < 2 {
		t.Fatalf("shared switch targets collapsed: edges=%v", ir.Edges)
	}
	seen := map[int32]int{}
	for _, c := range cases {
		seen[c]++
	}
	if seen[-5] != 1 || seen[1] != 1 || seen[2] != 1 {
		t.Fatalf("signed/case keys lost: %v", seen)
	}
	var handlers []int
	for _, e := range ir.Edges {
		if e.Kind == core.EdgeException {
			handlers = append(handlers, e.HandlerOrder)
		}
	}
	if len(handlers) < 2 {
		t.Fatalf("exception edges collapsed: %v", ir.Edges)
	}
	found0, found1 := false, false
	for _, h := range handlers {
		if h == 0 {
			found0 = true
		}
		if h == 1 {
			found1 = true
		}
	}
	if !found0 || !found1 {
		t.Fatalf("handler order lost: %v", handlers)
	}
}

func lookupswitch(pc int, def int, pairs [][2]int32) []byte {
	pad := (4 - ((pc + 1) % 4)) % 4
	buf := []byte{core.OP_LOOKUPSWITCH}
	buf = append(buf, make([]byte, pad)...)
	u4 := func(v int32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(v))
		buf = append(buf, b[:]...)
	}
	u4(int32(def - pc))
	u4(int32(len(pairs)))
	for _, p := range pairs {
		u4(p[0])
		u4(int32(int(p[1]) - pc))
	}
	return buf
}

func TestT11_C04_Immutability(t *testing.T) {
	code := []byte{core.OP_ILOAD_0, core.OP_IFEQ, 0, 6, core.OP_ICONST_1, core.OP_IRETURN, core.OP_ICONST_2, core.OP_IRETURN}
	d := core.NewDecompiler(code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.SnapshotSemanticCFG(d)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := BuildFromCFG(cfg, meta("br", code), d)
	if err != nil {
		t.Fatal(err)
	}
	before := ir.Canonical()
	h := ir.Hash()
	for _, n := range cfg.Nodes {
		n.Target = append(n.Target, n)
		if len(n.Data) > 0 {
			n.Data[0] = 0xff
		}
	}
	if ir.Canonical() != before || ir.Hash() != h {
		t.Fatal("MethodIR mutated after opcode.Target rewrite")
	}
	for _, in := range ir.Instrs {
		if in.PC != uint16(in.ID) {
			t.Fatalf("origin PC lost: %+v", in)
		}
	}
}

func TestT11_C05_ConcurrentIsolation(t *testing.T) {
	codes := [][]byte{
		{core.OP_ILOAD_0, core.OP_IRETURN},
		{core.OP_ICONST_1, core.OP_ISTORE_0, core.OP_ILOAD_0, core.OP_IRETURN},
		{core.OP_ILOAD_0, core.OP_ICONST_1, core.OP_IADD, core.OP_IRETURN},
		{core.OP_ILOAD_0, core.OP_IFEQ, 0, 6, core.OP_ICONST_1, core.OP_IRETURN, core.OP_ICONST_0, core.OP_IRETURN},
	}
	names := []string{"a", "b", "c", "d"}
	var wg sync.WaitGroup
	errCh := make(chan error, len(codes)*8)
	for n := 0; n < 8; n++ {
		for i := range codes {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ir, err := BuildFromBytes(codes[i], nil, meta(names[i], codes[i]), nil)
				if err != nil {
					errCh <- err
					return
				}
				if !strings.Contains(string(ir.ID), names[i]) {
					errCh <- errInvalid(0, 0, "method id mixed across goroutines")
				}
				_ = ir.Hash()
			}(i)
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestT11_C06_BadInput(t *testing.T) {
	jsr := []byte{core.OP_JSR, 0, 4, core.OP_IRETURN, core.OP_ASTORE_0, core.OP_RET, 0}
	_, err := BuildFromBytes(jsr, nil, meta("jsr", jsr), nil)
	if err == nil {
		t.Fatal("jsr produced IR")
	}
	e, ok := err.(*Error)
	if !ok || (e.Kind != Unsupported && e.Kind != InvalidInput) {
		t.Fatalf("jsr must be unsupported/invalid, got %T %v", err, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "jsr") && !strings.Contains(strings.ToLower(err.Error()), "unsupported") && !strings.Contains(strings.ToLower(err.Error()), "invalid") {
		t.Fatalf("jsr error %v", err)
	}
	bad := []byte{0xba} // invokedynamic truncated / may fail parse
	_, err = BuildFromBytes(append([]byte{0xff}, bad...), nil, meta("bad", []byte{0xff}), nil)
	if err == nil {
		t.Fatal("illegal opcode produced IR")
	}
	if e, ok := err.(*Error); ok && e.Kind != InvalidInput && e.Kind != Unsupported {
		t.Fatalf("illegal opcode kind %v", e)
	}
}
