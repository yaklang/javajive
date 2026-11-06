package ir

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/dex2jar/model"
)

// mayThrow reports whether an instruction can transfer control to an exception
// handler. Moves, constants, comparisons, branches and returns cannot.
func mayThrow(opcode byte) bool {
	switch {
	case opcode == 0x00:
		return false
	case opcode >= 0x01 && opcode <= 0x19:
		return false
	case opcode >= 0x28 && opcode <= 0x3d:
		return false
	default:
		return true
	}
}

func isGoto(opcode byte) bool { return opcode >= 0x28 && opcode <= 0x2a }
func isIf(opcode byte) bool   { return opcode >= 0x32 && opcode <= 0x3d }
func isSwitch(opcode byte) bool {
	return opcode == 0x2b || opcode == 0x2c
}
func isBranch(opcode byte) bool {
	return isGoto(opcode) || isIf(opcode)
}
func isTerminal(opcode byte) bool {
	return opcode >= 0x0e && opcode <= 0x11 || opcode == 0x27
}

// handlerSite is one DEX try item: a protected instruction range and the
// handlers the verifier may transfer control to.
type handlerSite struct {
	start, end uint32
	targets    []handlerTarget
}

type handlerTarget struct {
	address   uint32
	catchType string
}

// handlerSites validates the try table and returns its protected ranges.
func handlerSites(code *model.Code, instructions []Instruction, codeUnits uint32, ctx Context) ([]handlerSite, error) {
	if code == nil || len(code.Tries) == 0 {
		return nil, nil
	}
	boundary := make(map[uint32]bool, len(instructions))
	for _, instruction := range instructions {
		boundary[instruction.Offset] = true
	}
	sites := make([]handlerSite, 0, len(code.Tries))
	for index, try := range code.Tries {
		start := try.StartAddress
		end := uint64(start) + uint64(try.InstructionCount)
		if try.InstructionCount == 0 {
			return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d has an empty protected range", index))
		}
		if end > uint64(codeUnits) {
			return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d covers [%d,%d), outside code units [0,%d)", index, start, end, codeUnits))
		}
		if !boundary[start] {
			return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d starts at %d, which is not an instruction boundary", index, start))
		}
		if uint32(end) != codeUnits && !boundary[uint32(end)] {
			return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d ends at %d, which is not an instruction boundary", index, end))
		}
		site := handlerSite{start: start, end: uint32(end)}
		for _, handler := range try.Handler.Handlers {
			if !boundary[handler.Address] {
				return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d handler address %d is not an instruction boundary", index, handler.Address))
			}
			if handler.Type == "" || handler.Type[0] != 'L' || handler.Type[len(handler.Type)-1] != ';' {
				return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d handler type %q is not an object descriptor", index, handler.Type))
			}
			site.targets = append(site.targets, handlerTarget{address: handler.Address, catchType: handler.Type})
		}
		if try.Handler.CatchAllAddress != nil {
			address := *try.Handler.CatchAllAddress
			if !boundary[address] {
				return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d catch-all address %d is not an instruction boundary", index, address))
			}
			site.targets = append(site.targets, handlerTarget{address: address})
		}
		if len(site.targets) == 0 {
			return nil, diagnostic(ctx, nil, "try", fmt.Errorf("try item %d has no handlers", index))
		}
		sites = append(sites, site)
	}
	return sites, nil
}

func buildBlocks(instructions []Instruction, codeUnits uint32, ctx Context, sites []handlerSite) ([]Block, error) {
	if len(instructions) == 0 {
		return nil, diagnostic(ctx, nil, "code", fmt.Errorf("empty instruction stream"))
	}
	byOffset := make(map[uint32]int, len(instructions))
	for index, instruction := range instructions {
		byOffset[instruction.Offset] = index
	}
	leaders := map[uint32]bool{0: true}
	for _, site := range sites {
		leaders[site.start] = true
		if site.end < codeUnits {
			leaders[site.end] = true
		}
		for _, target := range site.targets {
			leaders[target.address] = true
		}
	}
	for index, instruction := range instructions {
		if isBranch(instruction.Opcode) {
			if instruction.Target < 0 || instruction.Target >= int64(codeUnits) {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("branch target %d is outside code units [0,%d)", instruction.Target, codeUnits))
			}
			target := uint32(instruction.Target)
			if _, ok := byOffset[target]; !ok {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("branch target %d is not an instruction boundary", target))
			}
			if target == instruction.Offset {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("zero-offset branch is invalid"))
			}
			leaders[target] = true
			if index+1 < len(instructions) {
				leaders[instructions[index+1].Offset] = true
			}
		}
		if isSwitch(instruction.Opcode) {
			if index+1 >= len(instructions) {
				return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("switch has no fall-through instruction"))
			}
			for _, target := range instruction.Targets {
				if target < 0 || target >= int64(codeUnits) {
					return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("switch target %d is outside code units [0,%d)", target, codeUnits))
				}
				if _, ok := byOffset[uint32(target)]; !ok {
					return nil, diagnostic(ctx, &instruction, "", fmt.Errorf("switch target %d is not an instruction boundary", target))
				}
				leaders[uint32(target)] = true
			}
			// The instruction after the switch is the default case.
			leaders[instructions[index+1].Offset] = true
		}
		if isTerminal(instruction.Opcode) && index+1 < len(instructions) {
			leaders[instructions[index+1].Offset] = true
		}
	}
	starts := make([]uint32, 0, len(leaders))
	for start := range leaders {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	blocks := make([]Block, len(starts))
	blockAt := make(map[uint32]int, len(starts))
	for index, start := range starts {
		end := codeUnits
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		block := Block{ID: index, Start: start, End: end}
		for instructionIndex, instruction := range instructions {
			if instruction.Offset >= start && instruction.Offset < end {
				block.Instructions = append(block.Instructions, instructionIndex)
			}
		}
		if len(block.Instructions) == 0 {
			return nil, diagnostic(ctx, nil, "cfg", fmt.Errorf("block at code-unit offset %d is empty", start))
		}
		blocks[index] = block
		blockAt[start] = index
	}
	addSuccessor := func(block *Block, successor int) {
		for _, existing := range block.Successors {
			if existing == successor {
				return
			}
		}
		block.Successors = append(block.Successors, successor)
	}
	for index := range blocks {
		block := &blocks[index]
		last := instructions[block.Instructions[len(block.Instructions)-1]]
		if isBranch(last.Opcode) {
			addSuccessor(block, blockAt[uint32(last.Target)])
			if isIf(last.Opcode) && index+1 < len(blocks) {
				addSuccessor(block, index+1)
			}
		} else if isSwitch(last.Opcode) {
			for _, target := range last.Targets {
				addSuccessor(block, blockAt[uint32(target)])
			}
			if index+1 < len(blocks) {
				addSuccessor(block, index+1)
			}
		} else if !isTerminal(last.Opcode) && index+1 < len(blocks) {
			addSuccessor(block, index+1)
		}
	}
	// Exception dispatch is an edge from every protected block to each handler
	// entry. Protected ranges align with block boundaries because their start
	// and end offsets are leaders.
	handlerBlock := make(map[uint32]int, len(sites))
	for _, site := range sites {
		for _, target := range site.targets {
			blockIndex := blockAt[target.address]
			handlerBlock[target.address] = blockIndex
			blocks[blockIndex].Handler = true
		}
	}
	recorded := make(map[int]map[int]bool, len(sites))
	for _, site := range sites {
		for index := range blocks {
			if blocks[index].Start < site.start || blocks[index].Start >= site.end {
				continue
			}
			// Only a block with a potentially throwing instruction can dispatch
			// to the handler. A block of moves or returns inside the protected
			// range does not create an exception edge.
			throwing := false
			for _, instructionIndex := range blocks[index].Instructions {
				instruction := instructions[instructionIndex]
				if instruction.Offset >= site.start && instruction.Offset < site.end && mayThrow(instruction.Opcode) {
					throwing = true
					break
				}
			}
			if !throwing {
				continue
			}
			for _, target := range site.targets {
				handler := handlerBlock[target.address]
				addSuccessor(&blocks[index], handler)
				if recorded[handler] == nil {
					recorded[handler] = make(map[int]bool)
				}
				for _, instructionIndex := range blocks[index].Instructions {
					instruction := instructions[instructionIndex]
					// Only instructions that can throw dispatch to the handler.
					// Merging the state of a return or a move would describe a
					// register value the handler never observes.
					if instruction.Offset >= site.start && instruction.Offset < site.end && mayThrow(instruction.Opcode) {
						recorded[handler][instructionIndex] = true
					}
				}
			}
		}
	}
	for handler, set := range recorded {
		for instructionIndex := range set {
			blocks[handler].Protected = append(blocks[handler].Protected, instructionIndex)
		}
		sort.Ints(blocks[handler].Protected)
	}
	// Reachability follows every edge, including exception dispatch, so handler
	// blocks are live. Only reachable code must end in a terminator; a trailing
	// dead block is emitted as unverifiable dead code rather than rejected.
	reachable := make([]bool, len(blocks))
	reachable[0] = true
	queue := []int{0}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, successor := range blocks[current].Successors {
			if !reachable[successor] {
				reachable[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	for index := range blocks {
		blocks[index].Reachable = reachable[index]
		if !reachable[index] {
			continue
		}
		block := &blocks[index]
		last := instructions[block.Instructions[len(block.Instructions)-1]]
		if !isBranch(last.Opcode) && !isSwitch(last.Opcode) && !isTerminal(last.Opcode) && index+1 >= len(blocks) {
			return nil, diagnostic(ctx, &last, "", fmt.Errorf("reachable execution may fall off end of code"))
		}
	}
	for index := range blocks {
		for _, successor := range blocks[index].Successors {
			blocks[successor].Predecessors = append(blocks[successor].Predecessors, index)
		}
	}
	return blocks, nil
}
