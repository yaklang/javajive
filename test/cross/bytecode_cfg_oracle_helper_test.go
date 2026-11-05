package cross

import (
	"encoding/binary"
	"fmt"
	"sort"

	classparser "github.com/yaklang/javajive/classparser"
)

// bytecodeEdgeKind deliberately does not reuse the decompiler's edge model.
// This oracle consumes only the Code attribute's code[] bytes.
type bytecodeEdgeKind string

const (
	bytecodeFallthrough      bytecodeEdgeKind = "fallthrough"
	bytecodeConditionalTaken bytecodeEdgeKind = "conditional-taken"
	bytecodeGoto             bytecodeEdgeKind = "goto"
	bytecodeSwitchCase       bytecodeEdgeKind = "switch-case"
	bytecodeSwitchDefault    bytecodeEdgeKind = "switch-default"
)

type bytecodeOracleEdge struct {
	FromPC  int
	ToPC    int
	Kind    bytecodeEdgeKind
	Case    *int32
	Default bool
}

type bytecodeOracleBlock struct {
	StartPC int
	EndPC   int // exclusive
	LastPC  int
}

type bytecodeOracleSCC struct {
	PCs                   []int
	Cyclic                bool
	EntryTargets          []int
	EntryEdges            []bytecodeOracleEdge
	ReachableEntryTargets []int
	ReachableEntryEdges   []bytecodeOracleEdge
	ExitEdges             []bytecodeOracleEdge
}

type bytecodeCFGOracle struct {
	CodeLen            int
	Blocks             []bytecodeOracleBlock
	Edges              []bytecodeOracleEdge
	ReachablePCs       []int
	ExitPCs            []int
	CyclicSCCs         []bytecodeOracleSCC
	Dominators         map[int][]int
	Reducible          bool
	IrreducibleCyclePC []int
}

type bytecodeOracleInstruction struct {
	pc       int
	op       byte
	nextPC   int
	targets  []bytecodeOracleSwitchTarget
	terminal bool
}

type bytecodeOracleSwitchTarget struct {
	pc        int
	caseValue int32
	hasCase   bool
	isDefault bool
}

// parseCodeCFGOracle independently decodes enough of JVMS 6.5 to recover the
// normal-flow graph. jsr/ret are intentionally rejected because subroutine
// return edges cannot be represented by the requested normal edge kinds.
func parseCodeAttributeCFGOracle(attribute *classparser.CodeAttribute) (*bytecodeCFGOracle, error) {
	if attribute == nil {
		return nil, fmt.Errorf("nil Code attribute")
	}
	return parseCodeCFGOracle(attribute.Code)
}

func parseCodeCFGOracle(code []byte) (*bytecodeCFGOracle, error) {
	insns, byPC, err := decodeOracleInstructions(code)
	if err != nil {
		return nil, err
	}
	if len(insns) == 0 {
		return nil, fmt.Errorf("empty Code attribute")
	}

	leaders := map[int]bool{0: true}
	for _, insn := range insns {
		for _, target := range insn.targets {
			if _, ok := byPC[target.pc]; !ok {
				return nil, fmt.Errorf("target PC %d from PC %d is not an instruction boundary", target.pc, insn.pc)
			}
			leaders[target.pc] = true
		}
		if oracleEndsBlock(insn.op) && insn.nextPC < len(code) {
			leaders[insn.nextPC] = true
		}
	}

	leaderPCs := sortedOracleKeys(leaders)
	blocks := make([]bytecodeOracleBlock, 0, len(leaderPCs))
	blockForInsn := make(map[int]int, len(insns))
	for i, start := range leaderPCs {
		end := len(code)
		if i+1 < len(leaderPCs) {
			end = leaderPCs[i+1]
		}
		last := start
		for pc := start; pc < end; {
			insn, ok := byPC[pc]
			if !ok {
				return nil, fmt.Errorf("block at PC %d crosses a non-instruction boundary at PC %d", start, pc)
			}
			blockForInsn[pc] = start
			last = pc
			pc = insn.nextPC
		}
		blocks = append(blocks, bytecodeOracleBlock{StartPC: start, EndPC: end, LastPC: last})
	}

	var edges []bytecodeOracleEdge
	for _, block := range blocks {
		last := byPC[block.LastPC]
		addTarget := func(target bytecodeOracleSwitchTarget, kind bytecodeEdgeKind) {
			to := blockForInsn[target.pc]
			edge := bytecodeOracleEdge{FromPC: block.StartPC, ToPC: to, Kind: kind, Default: target.isDefault}
			if target.hasCase {
				value := target.caseValue
				edge.Case = &value
			}
			edges = append(edges, edge)
		}
		switch {
		case oracleConditional(last.op):
			addTarget(last.targets[0], bytecodeConditionalTaken)
			if last.nextPC < len(code) {
				edges = append(edges, bytecodeOracleEdge{FromPC: block.StartPC, ToPC: blockForInsn[last.nextPC], Kind: bytecodeFallthrough})
			}
		case last.op == 0xa7 || last.op == 0xc8: // goto, goto_w
			addTarget(last.targets[0], bytecodeGoto)
		case last.op == 0xaa || last.op == 0xab: // tableswitch, lookupswitch
			for _, target := range last.targets {
				kind := bytecodeSwitchCase
				if target.isDefault {
					kind = bytecodeSwitchDefault
				}
				addTarget(target, kind)
			}
		case last.terminal:
			// A return or athrow is a normal-flow exit.
		default:
			if last.nextPC < len(code) {
				edges = append(edges, bytecodeOracleEdge{FromPC: block.StartPC, ToPC: blockForInsn[last.nextPC], Kind: bytecodeFallthrough})
			}
		}
	}

	oracle := &bytecodeCFGOracle{CodeLen: len(code), Blocks: blocks, Edges: edges}
	oracle.finishGraphAnalysis()
	return oracle, nil
}

func decodeOracleInstructions(code []byte) ([]bytecodeOracleInstruction, map[int]bytecodeOracleInstruction, error) {
	var insns []bytecodeOracleInstruction
	byPC := make(map[int]bytecodeOracleInstruction)
	for pc := 0; pc < len(code); {
		op := code[pc]
		if op == 0xa8 || op == 0xa9 || op == 0xc9 {
			return nil, nil, fmt.Errorf("legacy jsr/ret opcode 0x%02x at PC %d is outside the normal-flow oracle", op, pc)
		}
		if op == 0xca || op >= 0xcb {
			return nil, nil, fmt.Errorf("reserved opcode 0x%02x at PC %d", op, pc)
		}

		insn := bytecodeOracleInstruction{pc: pc, op: op, terminal: oracleTerminal(op)}
		switch op {
		case 0xaa:
			next, targets, err := decodeOracleTableSwitch(code, pc)
			if err != nil {
				return nil, nil, err
			}
			insn.nextPC, insn.targets = next, targets
		case 0xab:
			next, targets, err := decodeOracleLookupSwitch(code, pc)
			if err != nil {
				return nil, nil, err
			}
			insn.nextPC, insn.targets = next, targets
		case 0xc4:
			next, err := decodeOracleWide(code, pc)
			if err != nil {
				return nil, nil, err
			}
			insn.nextPC = next
		default:
			operandLen := oracleFixedOperandLen(op)
			insn.nextPC = pc + 1 + operandLen
			if insn.nextPC > len(code) {
				return nil, nil, fmt.Errorf("truncated opcode 0x%02x at PC %d", op, pc)
			}
			if oracleConditional(op) || op == 0xa7 {
				delta := int(int16(binary.BigEndian.Uint16(code[pc+1 : pc+3])))
				insn.targets = []bytecodeOracleSwitchTarget{{pc: pc + delta}}
			} else if op == 0xc8 {
				delta := int(int32(binary.BigEndian.Uint32(code[pc+1 : pc+5])))
				insn.targets = []bytecodeOracleSwitchTarget{{pc: pc + delta}}
			}
		}
		for _, target := range insn.targets {
			if target.pc < 0 || target.pc >= len(code) {
				return nil, nil, fmt.Errorf("target PC %d from PC %d is outside Code[0,%d)", target.pc, pc, len(code))
			}
		}
		insns = append(insns, insn)
		byPC[pc] = insn
		pc = insn.nextPC
	}
	return insns, byPC, nil
}

func oracleFixedOperandLen(op byte) int {
	switch {
	case op == 0x10 || op == 0x12 || (op >= 0x15 && op <= 0x19) ||
		(op >= 0x36 && op <= 0x3a) || op == 0xa9 || op == 0xbc:
		return 1
	case op == 0x11 || op == 0x13 || op == 0x14 || op == 0x84 ||
		(op >= 0x99 && op <= 0xa8) || (op >= 0xb2 && op <= 0xb8) ||
		op == 0xbb || op == 0xbd || op == 0xc0 || op == 0xc1 || op == 0xc6 || op == 0xc7:
		return 2
	case op == 0xc5:
		return 3
	case op == 0xb9 || op == 0xba || op == 0xc8 || op == 0xc9:
		return 4
	default:
		return 0
	}
}

func decodeOracleWide(code []byte, pc int) (int, error) {
	if pc+2 > len(code) {
		return 0, fmt.Errorf("truncated wide at PC %d", pc)
	}
	modified := code[pc+1]
	if modified == 0x84 { // iinc indexbyte1 indexbyte2 constbyte1 constbyte2
		if pc+6 > len(code) {
			return 0, fmt.Errorf("truncated wide iinc at PC %d", pc)
		}
		return pc + 6, nil
	}
	if modified == 0xa9 {
		return 0, fmt.Errorf("legacy wide ret at PC %d is outside the normal-flow oracle", pc)
	}
	if (modified >= 0x15 && modified <= 0x19) || (modified >= 0x36 && modified <= 0x3a) {
		if pc+4 > len(code) {
			return 0, fmt.Errorf("truncated wide local opcode at PC %d", pc)
		}
		return pc + 4, nil
	}
	return 0, fmt.Errorf("invalid wide opcode 0x%02x at PC %d", modified, pc)
}

func decodeOracleTableSwitch(code []byte, pc int) (int, []bytecodeOracleSwitchTarget, error) {
	payload := alignOracleSwitchPayload(pc)
	if payload+12 > len(code) {
		return 0, nil, fmt.Errorf("truncated tableswitch at PC %d", pc)
	}
	defaultDelta := int(int32(binary.BigEndian.Uint32(code[payload : payload+4])))
	low := int32(binary.BigEndian.Uint32(code[payload+4 : payload+8]))
	high := int32(binary.BigEndian.Uint32(code[payload+8 : payload+12]))
	count := int64(high) - int64(low) + 1
	if count <= 0 || count > int64((len(code)-payload-12)/4) {
		return 0, nil, fmt.Errorf("invalid tableswitch range %d..%d at PC %d", low, high, pc)
	}
	targets := []bytecodeOracleSwitchTarget{{pc: pc + defaultDelta, isDefault: true}}
	for i := int64(0); i < count; i++ {
		at := payload + 12 + int(i)*4
		delta := int(int32(binary.BigEndian.Uint32(code[at : at+4])))
		targets = append(targets, bytecodeOracleSwitchTarget{pc: pc + delta, caseValue: low + int32(i), hasCase: true})
	}
	return payload + 12 + int(count)*4, targets, nil
}

func decodeOracleLookupSwitch(code []byte, pc int) (int, []bytecodeOracleSwitchTarget, error) {
	payload := alignOracleSwitchPayload(pc)
	if payload+8 > len(code) {
		return 0, nil, fmt.Errorf("truncated lookupswitch at PC %d", pc)
	}
	defaultDelta := int(int32(binary.BigEndian.Uint32(code[payload : payload+4])))
	count := int64(int32(binary.BigEndian.Uint32(code[payload+4 : payload+8])))
	if count < 0 || count > int64((len(code)-payload-8)/8) {
		return 0, nil, fmt.Errorf("invalid lookupswitch pair count %d at PC %d", count, pc)
	}
	targets := []bytecodeOracleSwitchTarget{{pc: pc + defaultDelta, isDefault: true}}
	var previous int32
	for i := int64(0); i < count; i++ {
		at := payload + 8 + int(i)*8
		match := int32(binary.BigEndian.Uint32(code[at : at+4]))
		if i > 0 && match <= previous {
			return 0, nil, fmt.Errorf("unsorted lookupswitch keys at PC %d", pc)
		}
		previous = match
		delta := int(int32(binary.BigEndian.Uint32(code[at+4 : at+8])))
		targets = append(targets, bytecodeOracleSwitchTarget{pc: pc + delta, caseValue: match, hasCase: true})
	}
	return payload + 8 + int(count)*8, targets, nil
}

func alignOracleSwitchPayload(pc int) int {
	return (pc + 4) &^ 3
}

func oracleConditional(op byte) bool {
	return (op >= 0x99 && op <= 0xa6) || op == 0xc6 || op == 0xc7
}

func oracleTerminal(op byte) bool {
	return (op >= 0xac && op <= 0xb1) || op == 0xbf
}

func oracleEndsBlock(op byte) bool {
	return oracleConditional(op) || op == 0xa7 || op == 0xc8 || op == 0xaa || op == 0xab || oracleTerminal(op)
}

func (o *bytecodeCFGOracle) finishGraphAnalysis() {
	succ := make(map[int][]int)
	pred := make(map[int][]int)
	for _, edge := range o.Edges {
		succ[edge.FromPC] = appendUniqueOracleInt(succ[edge.FromPC], edge.ToPC)
		pred[edge.ToPC] = appendUniqueOracleInt(pred[edge.ToPC], edge.FromPC)
	}

	reachable := map[int]bool{}
	var visit func(int)
	visit = func(pc int) {
		if reachable[pc] {
			return
		}
		reachable[pc] = true
		for _, next := range succ[pc] {
			visit(next)
		}
	}
	visit(0)
	o.ReachablePCs = sortedOracleKeys(reachable)
	for _, pc := range o.ReachablePCs {
		if len(succ[pc]) == 0 {
			o.ExitPCs = append(o.ExitPCs, pc)
		}
	}

	o.Dominators = oracleDominators(o.ReachablePCs, pred)
	sccs := oracleTarjan(o.ReachablePCs, succ)
	for _, pcs := range sccs {
		members := make(map[int]bool, len(pcs))
		for _, pc := range pcs {
			members[pc] = true
		}
		cyclic := len(pcs) > 1
		if len(pcs) == 1 {
			for _, next := range succ[pcs[0]] {
				cyclic = cyclic || next == pcs[0]
			}
		}
		if !cyclic {
			continue
		}
		scc := bytecodeOracleSCC{PCs: pcs, Cyclic: true}
		entrySet := map[int]bool{}
		reachableEntrySet := map[int]bool{}
		for _, edge := range o.Edges {
			fromIn, toIn := members[edge.FromPC], members[edge.ToPC]
			if !fromIn && toIn {
				scc.EntryEdges = append(scc.EntryEdges, edge)
				entrySet[edge.ToPC] = true
				if reachable[edge.FromPC] {
					scc.ReachableEntryEdges = append(scc.ReachableEntryEdges, edge)
					reachableEntrySet[edge.ToPC] = true
				}
			}
			if fromIn && !toIn {
				scc.ExitEdges = append(scc.ExitEdges, edge)
			}
		}
		if members[0] {
			entrySet[0] = true // virtual method-entry target; there is no synthetic edge
			reachableEntrySet[0] = true
		}
		scc.EntryTargets = sortedOracleKeys(entrySet)
		scc.ReachableEntryTargets = sortedOracleKeys(reachableEntrySet)
		o.CyclicSCCs = append(o.CyclicSCCs, scc)
	}

	// In a reducible flow graph, deleting every edge whose target dominates
	// its source removes all cycles. Find a path closing any residual edge to
	// produce a concrete irreducibility witness without reusing Tarjan's SCCs.
	remaining := make(map[int][]int)
	for _, edge := range o.Edges {
		if !reachable[edge.FromPC] || oracleContains(o.Dominators[edge.FromPC], edge.ToPC) {
			continue
		}
		remaining[edge.FromPC] = appendUniqueOracleInt(remaining[edge.FromPC], edge.ToPC)
	}
	o.Reducible = true
	for _, from := range o.ReachablePCs {
		for _, to := range remaining[from] {
			if path := oraclePath(to, from, remaining); len(path) > 0 {
				o.Reducible = false
				o.IrreducibleCyclePC = append([]int{from}, path...)
				return
			}
		}
	}
}

func oracleDominators(nodes []int, pred map[int][]int) map[int][]int {
	all := append([]int(nil), nodes...)
	dom := make(map[int][]int, len(nodes))
	for _, pc := range nodes {
		if pc == 0 {
			dom[pc] = []int{0}
		} else {
			dom[pc] = append([]int(nil), all...)
		}
	}
	changed := true
	for changed {
		changed = false
		for _, pc := range nodes {
			if pc == 0 {
				continue
			}
			first := true
			var intersection []int
			for _, p := range pred[pc] {
				if !oracleContains(nodes, p) {
					continue
				}
				if first {
					intersection = append([]int(nil), dom[p]...)
					first = false
				} else {
					intersection = oracleIntersection(intersection, dom[p])
				}
			}
			intersection = appendUniqueOracleInt(intersection, pc)
			sort.Ints(intersection)
			if !oracleIntsEqual(dom[pc], intersection) {
				dom[pc] = intersection
				changed = true
			}
		}
	}
	return dom
}

func oracleTarjan(nodes []int, succ map[int][]int) [][]int {
	index := 0
	indices, low := map[int]int{}, map[int]int{}
	onStack := map[int]bool{}
	var stack []int
	var result [][]int
	var strongConnect func(int)
	strongConnect = func(v int) {
		index++
		indices[v], low[v] = index, index
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range succ[v] {
			if !oracleContains(nodes, w) {
				continue
			}
			if indices[w] == 0 {
				strongConnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] != indices[v] {
			return
		}
		var component []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			component = append(component, w)
			if w == v {
				break
			}
		}
		sort.Ints(component)
		result = append(result, component)
	}
	for _, v := range nodes {
		if indices[v] == 0 {
			strongConnect(v)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i][0] < result[j][0] })
	return result
}

func oraclePath(start, goal int, succ map[int][]int) []int {
	seen := map[int]bool{}
	var search func(int) []int
	search = func(pc int) []int {
		if pc == goal {
			return []int{pc}
		}
		if seen[pc] {
			return nil
		}
		seen[pc] = true
		for _, next := range succ[pc] {
			if path := search(next); len(path) > 0 {
				return append([]int{pc}, path...)
			}
		}
		return nil
	}
	return search(start)
}

func sortedOracleKeys[V any](values map[int]V) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func appendUniqueOracleInt(values []int, value int) []int {
	if oracleContains(values, value) {
		return values
	}
	return append(values, value)
}

func oracleContains(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func oracleIntersection(left, right []int) []int {
	var result []int
	for _, value := range left {
		if oracleContains(right, value) {
			result = append(result, value)
		}
	}
	return result
}

func oracleIntsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
