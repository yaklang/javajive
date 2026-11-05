package cross

import (
	"reflect"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

func TestBytecodeCFGOracleNaturalLoop(t *testing.T) {
	//  0: iconst_0; istore_0
	//  2: iload_0; ifeq 12
	//  6: iinc 0,1; goto 2
	// 12: return
	// 13: goto 13 (deliberately unreachable cycle)
	code := []byte{
		0x03, 0x3b,
		0x1a, 0x99, 0x00, 0x09,
		0x84, 0x00, 0x01, 0xa7, 0xff, 0xf9,
		0xb1,
		0xa7, 0x00, 0x00,
	}
	got, err := parseCodeCFGOracle(code)
	if err != nil {
		t.Fatal(err)
	}

	if starts := oracleBlockStarts(got.Blocks); !reflect.DeepEqual(starts, []int{0, 2, 6, 12, 13}) {
		t.Fatalf("block starts = %v", starts)
	}
	wantEdges := []bytecodeOracleEdge{
		{FromPC: 0, ToPC: 2, Kind: bytecodeFallthrough},
		{FromPC: 2, ToPC: 12, Kind: bytecodeConditionalTaken},
		{FromPC: 2, ToPC: 6, Kind: bytecodeFallthrough},
		{FromPC: 6, ToPC: 2, Kind: bytecodeGoto},
		{FromPC: 13, ToPC: 13, Kind: bytecodeGoto},
	}
	if !reflect.DeepEqual(got.Edges, wantEdges) {
		t.Fatalf("edges = %#v, want %#v", got.Edges, wantEdges)
	}
	if !reflect.DeepEqual(got.ReachablePCs, []int{0, 2, 6, 12}) {
		t.Fatalf("reachable PCs = %v", got.ReachablePCs)
	}
	if !reflect.DeepEqual(got.ExitPCs, []int{12}) {
		t.Fatalf("exit PCs = %v", got.ExitPCs)
	}
	if !got.Reducible || len(got.IrreducibleCyclePC) != 0 {
		t.Fatalf("natural loop classified irreducible: witness=%v", got.IrreducibleCyclePC)
	}
	if len(got.CyclicSCCs) != 1 {
		t.Fatalf("cyclic SCCs = %#v", got.CyclicSCCs)
	}
	scc := got.CyclicSCCs[0]
	if !reflect.DeepEqual(scc.PCs, []int{2, 6}) || !reflect.DeepEqual(scc.EntryTargets, []int{2}) {
		t.Fatalf("natural SCC = %#v", scc)
	}
	if !reflect.DeepEqual(scc.EntryEdges, []bytecodeOracleEdge{{FromPC: 0, ToPC: 2, Kind: bytecodeFallthrough}}) {
		t.Fatalf("entry edges = %#v", scc.EntryEdges)
	}
	if !reflect.DeepEqual(scc.ExitEdges, []bytecodeOracleEdge{{FromPC: 2, ToPC: 12, Kind: bytecodeConditionalTaken}}) {
		t.Fatalf("exit edges = %#v", scc.ExitEdges)
	}
	if !reflect.DeepEqual(got.Dominators[6], []int{0, 2, 6}) {
		t.Fatalf("dominators(6) = %v", got.Dominators[6])
	}
}

func TestBytecodeCFGOracleMultipleLatchReducibleLoop(t *testing.T) {
	// Header 0 has two distinct latches at PCs 8 and 11. A reducibility
	// oracle must not mistake multiple incoming back edges for multiple entries.
	code := []byte{
		0x1a, 0x99, 0x00, 0x0d, // 0: iload_0; ifeq 14
		0x1b, 0x99, 0x00, 0x06, // 4: iload_1; ifeq 11
		0xa7, 0xff, 0xf8, // 8: goto 0 (latch one)
		0xa7, 0xff, 0xf5, // 11: goto 0 (latch two)
		0xb1, // 14: return
	}
	got, err := parseCodeCFGOracle(code)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reducible || len(got.CyclicSCCs) != 1 {
		t.Fatalf("multiple-latch loop: reducible=%v SCCs=%#v witness=%v", got.Reducible, got.CyclicSCCs, got.IrreducibleCyclePC)
	}
	scc := got.CyclicSCCs[0]
	if !reflect.DeepEqual(scc.PCs, []int{0, 4, 8, 11}) || !reflect.DeepEqual(scc.EntryTargets, []int{0}) {
		t.Fatalf("multiple-latch SCC = %#v", scc)
	}
	if len(scc.EntryEdges) != 0 || len(scc.ReachableEntryEdges) != 0 {
		t.Fatalf("method-entry SCC has physical entry edges: %#v", scc)
	}
	wantBackEdges := []bytecodeOracleEdge{
		{FromPC: 8, ToPC: 0, Kind: bytecodeGoto},
		{FromPC: 11, ToPC: 0, Kind: bytecodeGoto},
	}
	var backEdges []bytecodeOracleEdge
	for _, edge := range got.Edges {
		if edge.ToPC == 0 && (edge.FromPC == 8 || edge.FromPC == 11) {
			backEdges = append(backEdges, edge)
		}
	}
	if !reflect.DeepEqual(backEdges, wantBackEdges) {
		t.Fatalf("latch edges = %#v, want %#v", backEdges, wantBackEdges)
	}
	for _, latch := range []int{8, 11} {
		if !oracleContains(got.Dominators[latch], 0) {
			t.Fatalf("header 0 does not dominate latch %d: %v", latch, got.Dominators[latch])
		}
	}
}

func TestBytecodeCFGOracleDeadSecondaryEntry(t *testing.T) {
	// The reachable natural loop is 2 <-> 6. Dead block 13 also targets 6.
	// Encoded entry reporting must retain that edge, while reachable entries and
	// reducibility must ignore it.
	attribute := &classparser.CodeAttribute{Code: []byte{
		0x03, 0x3b, // 0: iconst_0; istore_0
		0x1a, 0x99, 0x00, 0x09, // 2: iload_0; ifeq 12
		0x84, 0x00, 0x01, 0xa7, 0xff, 0xf9, // 6: iinc 0,1; goto 2
		0xb1,             // 12: return
		0xa7, 0xff, 0xf9, // 13: goto 6 (dead secondary entry)
	}}
	got, err := parseCodeAttributeCFGOracle(attribute)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reducible || len(got.CyclicSCCs) != 1 {
		t.Fatalf("dead secondary entry changed reducibility: reducible=%v SCCs=%#v", got.Reducible, got.CyclicSCCs)
	}
	if oracleContains(got.ReachablePCs, 13) {
		t.Fatalf("dead source reported reachable: %v", got.ReachablePCs)
	}
	scc := got.CyclicSCCs[0]
	if !reflect.DeepEqual(scc.EntryTargets, []int{2, 6}) {
		t.Fatalf("encoded entry targets = %v", scc.EntryTargets)
	}
	wantEntries := []bytecodeOracleEdge{
		{FromPC: 0, ToPC: 2, Kind: bytecodeFallthrough},
		{FromPC: 13, ToPC: 6, Kind: bytecodeGoto},
	}
	if !reflect.DeepEqual(scc.EntryEdges, wantEntries) {
		t.Fatalf("encoded entry edges = %#v, want %#v", scc.EntryEdges, wantEntries)
	}
	if !reflect.DeepEqual(scc.ReachableEntryTargets, []int{2}) ||
		!reflect.DeepEqual(scc.ReachableEntryEdges, wantEntries[:1]) {
		t.Fatalf("reachable entries = targets %v edges %#v", scc.ReachableEntryTargets, scc.ReachableEntryEdges)
	}
}

func TestBytecodeCFGOracleCanonicalTwoEntrySCC(t *testing.T) {
	// The entry block chooses either A or B. A and B form a cycle, so neither
	// one dominates the other and the SCC has two external entry targets.
	// 0: iload_0; ifeq B
	// A=4: goto B
	// B=7: iload_0; ifne A; return
	code := []byte{
		0x1a, 0x99, 0x00, 0x06,
		0xa7, 0x00, 0x03,
		0x1a, 0x9a, 0xff, 0xfc,
		0xb1,
	}
	got, err := parseCodeCFGOracle(code)
	if err != nil {
		t.Fatal(err)
	}

	if got.Reducible {
		t.Fatal("canonical two-entry SCC classified reducible")
	}
	if len(got.IrreducibleCyclePC) < 3 {
		t.Fatalf("missing path-exclusion cycle witness: %v", got.IrreducibleCyclePC)
	}
	if len(got.CyclicSCCs) != 1 {
		t.Fatalf("cyclic SCCs = %#v", got.CyclicSCCs)
	}
	scc := got.CyclicSCCs[0]
	if !reflect.DeepEqual(scc.PCs, []int{4, 7}) {
		t.Fatalf("SCC PCs = %v", scc.PCs)
	}
	if !reflect.DeepEqual(scc.EntryTargets, []int{4, 7}) {
		t.Fatalf("entry targets = %v", scc.EntryTargets)
	}
	wantEntries := []bytecodeOracleEdge{
		{FromPC: 0, ToPC: 7, Kind: bytecodeConditionalTaken},
		{FromPC: 0, ToPC: 4, Kind: bytecodeFallthrough},
	}
	if !reflect.DeepEqual(scc.EntryEdges, wantEntries) {
		t.Fatalf("entry edges = %#v, want %#v", scc.EntryEdges, wantEntries)
	}
	if !reflect.DeepEqual(scc.ExitEdges, []bytecodeOracleEdge{{FromPC: 7, ToPC: 11, Kind: bytecodeFallthrough}}) {
		t.Fatalf("exit edges = %#v", scc.ExitEdges)
	}
	if oracleContains(got.Dominators[4], 7) || oracleContains(got.Dominators[7], 4) {
		t.Fatalf("two entries unexpectedly dominate each other: dom(4)=%v dom(7)=%v", got.Dominators[4], got.Dominators[7])
	}
}

func TestBytecodeCFGOracleSwitchEdgeKinds(t *testing.T) {
	t.Run("tableswitch", func(t *testing.T) {
		// PC 0 switch: default->26, case 0->24, case 1->25.
		code := []byte{
			0xaa, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x1a,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x00, 0x00, 0x18,
			0x00, 0x00, 0x00, 0x19,
			0xb1, 0xb1, 0xb1,
		}
		got, err := parseCodeCFGOracle(code)
		if err != nil {
			t.Fatal(err)
		}
		assertOracleSwitchEdges(t, got, []int{26, 24, 25}, []int32{0, 1})
	})

	t.Run("lookupswitch", func(t *testing.T) {
		// PC 0 switch: default->30, case -5->28, case 9->29.
		code := []byte{
			0xab, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x1e,
			0x00, 0x00, 0x00, 0x02,
			0xff, 0xff, 0xff, 0xfb, 0x00, 0x00, 0x00, 0x1c,
			0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 0x1d,
			0xb1, 0xb1, 0xb1,
		}
		got, err := parseCodeCFGOracle(code)
		if err != nil {
			t.Fatal(err)
		}
		assertOracleSwitchEdges(t, got, []int{30, 28, 29}, []int32{-5, 9})
	})
}

func TestBytecodeCFGOracleRejectsNonBoundaryTarget(t *testing.T) {
	_, err := parseCodeCFGOracle([]byte{0xa7, 0x00, 0x01, 0xb1})
	if err == nil {
		t.Fatal("accepted branch into goto operand")
	}
}

func assertOracleSwitchEdges(t *testing.T, got *bytecodeCFGOracle, targets []int, cases []int32) {
	t.Helper()
	if len(got.Edges) != len(targets) {
		t.Fatalf("edges = %#v", got.Edges)
	}
	for i, edge := range got.Edges {
		wantKind := bytecodeSwitchCase
		if i == 0 {
			wantKind = bytecodeSwitchDefault
		}
		if edge.FromPC != 0 || edge.ToPC != targets[i] || edge.Kind != wantKind {
			t.Fatalf("edge %d = %#v", i, edge)
		}
		if i == 0 {
			if !edge.Default || edge.Case != nil {
				t.Fatalf("default edge = %#v", edge)
			}
			continue
		}
		if edge.Default || edge.Case == nil || *edge.Case != cases[i-1] {
			t.Fatalf("case edge %d = %#v", i, edge)
		}
	}
	if !reflect.DeepEqual(got.ReachablePCs, []int{0, targets[1], targets[2], targets[0]}) {
		t.Fatalf("reachable PCs = %v", got.ReachablePCs)
	}
	if !reflect.DeepEqual(got.ExitPCs, []int{targets[1], targets[2], targets[0]}) {
		t.Fatalf("exit PCs = %v", got.ExitPCs)
	}
	if !got.Reducible || len(got.CyclicSCCs) != 0 {
		t.Fatalf("acyclic switch graph: reducible=%v SCCs=%#v", got.Reducible, got.CyclicSCCs)
	}
}

func oracleBlockStarts(blocks []bytecodeOracleBlock) []int {
	starts := make([]int, len(blocks))
	for i, block := range blocks {
		starts[i] = block.StartPC
	}
	return starts
}
