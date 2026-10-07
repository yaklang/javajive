package javaclassparser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Independent oracle: a reachable overlapping write is harmful iff one of its
// normal successors can reach the read. This factors two plain graph closures
// rather than simulating the production (node, clobbered) state machine.
func TestNativeEnumParameterBoundedOriginFlowModel(t *testing.T) {
	type write struct {
		opcode, slot, width int
		wide                bool
	}
	writes := []write{}
	for slot := 0; slot <= 4; slot++ {
		writes = append(writes, write{core.OP_ISTORE, slot, 1, false}, write{core.OP_LSTORE, slot, 2, false})
	}
	writes = append(writes, write{core.OP_IINC, 2, 1, false}, write{core.OP_ASTORE, 3, 1, true})
	closure := func(succ [][]int, roots []int) []bool {
		reach := make([]bool, len(succ))
		queue := append([]int(nil), roots...)
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			if reach[i] {
				continue
			}
			reach[i] = true
			queue = append(queue, succ[i]...)
		}
		return reach
	}
	count, accepted := 0, 0
	digest := sha256.New()
	for n := 1; n <= 3; n++ {
		for mask := 0; mask < 1<<(n*n); mask++ {
			succ := make([][]int, n)
			for from := 0; from < n; from++ {
				for to := 0; to < n; to++ {
					if mask&(1<<(from*n+to)) != 0 {
						succ[from] = append(succ[from], to)
					}
				}
			}
			entryReach := closure(succ, []int{0})
			for readIndex := 0; readIndex < n; readIndex++ {
				for writeIndex := -1; writeIndex < n; writeIndex++ {
					if writeIndex == readIndex {
						continue // One instruction cannot be both LOAD and STORE.
					}
					for _, readWidth := range []int{1, 2} {
						for _, changed := range writes {
							ops := make([]*core.OpCode, n)
							for i := range ops {
								ops[i] = &core.OpCode{CurrentOffset: uint16(i), Instr: &core.Instruction{OpCode: core.OP_NOP}}
							}
							readOp, desc := core.OP_ALOAD, "Ljava/lang/Object;"
							if readWidth == 2 {
								readOp, desc = core.OP_LLOAD, "J"
							}
							ops[readIndex].Instr = &core.Instruction{OpCode: readOp}
							ops[readIndex].Data = []byte{2}
							if writeIndex >= 0 {
								op := ops[writeIndex]
								op.Instr = &core.Instruction{OpCode: changed.opcode}
								op.Data = []byte{byte(changed.slot)}
								op.IsWide = changed.wide
								if changed.wide {
									op.Data = []byte{0, byte(changed.slot)}
								}
								if changed.opcode == core.OP_IINC {
									op.Data = append(op.Data, 1)
								}
							}
							flow := &nativeEnumParameterFlow{entry: ops[0], byPC: map[int]*core.OpCode{}, edges: map[*core.OpCode][]core.SemanticEdge{}}
							for i, op := range ops {
								flow.byPC[i] = op
								for _, to := range succ[i] {
									flow.edges[op] = append(flow.edges[op], core.SemanticEdge{From: op, To: ops[to], Kind: core.EdgeTaken})
								}
							}
							want := entryReach[readIndex]
							if writeIndex >= 0 && entryReach[writeIndex] && changed.slot < 2+readWidth && 2 < changed.slot+changed.width {
								want = want && !closure(succ, succ[writeIndex])[readIndex]
							}
							read := &nativeEnumSelectorProducer{opcode: readOp, pc: readIndex, slot: 2, result: desc}
							if got := flow.parameterAt(read, nil); got != want {
								t.Fatalf("seed graph n=%d mask=%d read=%d/%d write=%d %+v got=%v want=%v", n, mask, readIndex, readWidth, writeIndex, changed, got, want)
							}
							if want {
								accepted++
							}
							fmt.Fprintf(digest, "%d/%d/%d/%d/%d/%d/%d/%d/%v/%v\n", n, mask, readIndex, readWidth, writeIndex, changed.opcode, changed.slot, changed.width, changed.wide, want)
							count++
						}
					}
				}
			}
		}
	}
	if count != 112176 || accepted == 0 || accepted == count {
		t.Fatalf("incomplete model count=%d accepted=%d", count, accepted)
	}
	t.Logf("bounded origin graphs: %d (%d accepted), sha256=%x", count, accepted, digest.Sum(nil))
}

func TestNativeEnumParameterOriginalFlowControlAndResources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     []byte
		readPC   int
		handlers []*ExceptionTableEntry
		want     bool
	}{
		{"after read", []byte{core.OP_ALOAD_0, core.OP_POP, core.OP_ACONST_NULL, core.OP_ASTORE_0, core.OP_RETURN}, 0, nil, true},
		{"before read", []byte{core.OP_ACONST_NULL, core.OP_ASTORE_0, core.OP_ALOAD_0, core.OP_POP, core.OP_RETURN}, 2, nil, false},
		{"loop back after write", []byte{core.OP_ALOAD_0, core.OP_POP, core.OP_ACONST_NULL, core.OP_ASTORE_0, core.OP_GOTO, 255, 252}, 0, nil, false},
		{"dead write", []byte{core.OP_GOTO, 0, 4, core.OP_ASTORE_0, core.OP_ALOAD_0, core.OP_POP, core.OP_RETURN}, 4, nil, true},
		// A throwing GETSTATIC reaches a handler that changes the parameter
		// before returning to the selector. This handler edge cannot disappear.
		{"handler write returns to read", []byte{core.OP_ALOAD_0, core.OP_POP, core.OP_GETSTATIC, 0, 1, core.OP_POP, core.OP_RETURN, core.OP_POP, core.OP_ACONST_NULL, core.OP_ASTORE_0, core.OP_GOTO, 255, 246}, 0, []*ExceptionTableEntry{{StartPc: 2, EndPc: 5, HandlerPc: 7}}, false},
		{"malformed branch boundary", []byte{core.OP_ALOAD_0, core.OP_POP, core.OP_GOTO, 0, 1}, 0, nil, false},
		{"jsr is not an ordinary CFG", []byte{core.OP_ALOAD_0, core.OP_POP, core.OP_JSR, 0, 3, core.OP_RETURN}, 0, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := &CodeAttribute{MaxLocals: 1, MaxStack: 1, Code: tc.code, ExceptionTable: tc.handlers}
			d := core.NewDecompiler(code.Code, nil)
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			flow := nativeEnumParameterOriginalFlow(d, code, nil)
			if tc.name != "malformed branch boundary" && tc.name != "jsr is not an ordinary CFG" && flow == nil {
				t.Fatal("counterexample must reach the flow proof, not fail graph construction")
			}
			read := &nativeEnumSelectorProducer{opcode: core.OP_ALOAD_0, pc: tc.readPC, slot: 0, result: "Ljava/lang/Object;"}
			if got := flow.selector(read, nil, 0); got != tc.want {
				t.Fatalf("origin=%v want=%v", got, tc.want)
			}
			if !tc.want {
				return
			}
			for _, variant := range []string{"missing read", "wrong slot", "wrong opcode", "wrong descriptor", "work", "memory", "canceled", "depth", "cyclic selector"} {
				t.Run(variant, func(t *testing.T) {
					copy := *read
					var work *workbudget.Budget
					depth := 0
					if variant == "missing read" {
						copy.pc = 65535
					}
					if variant == "wrong slot" {
						copy.slot++
					}
					if variant == "wrong opcode" {
						copy.opcode = core.OP_LLOAD_0
					}
					if variant == "wrong descriptor" {
						copy.result = "J"
					}
					if variant == "work" {
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					}
					if variant == "memory" {
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					}
					if variant == "canceled" {
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					if variant == "depth" {
						depth = 32
					}
					if variant == "cyclic selector" {
						copy.owner = "model/Owner"
						copy.operands = []*nativeEnumSelectorProducer{&copy}
					}
					if flow.selector(&copy, work, depth) {
						t.Fatal("altered origin/resource state accepted")
					}
				})
			}
		})
	}
}
