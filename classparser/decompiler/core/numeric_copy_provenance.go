package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// A LOAD retains the word defined on its original incoming paths. Its mutable
// JavaRef type alone is not a numeric-domain witness. Follow the exact stack
// cell to the frozen LOAD, then prove every reaching definition, including
// transitive copies. The destination keeps its own stores: this is neither
// value substitution nor propagation of an initializer to another location.
type numericCopyProvenance struct {
	d                 *Decompiler
	original, decoded map[*OpCode]bool
	loads             map[values.JavaValue]*OpCode
	ambiguous         map[values.JavaValue]bool
	producers         map[values.JavaValue]bool
	remaining         int
}

func (d *Decompiler) numericCopyProvenance(producers map[values.JavaValue]bool) *numericCopyProvenance {
	if d == nil || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 8192 || len(d.opCodes) > 8192 {
		return nil
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.semanticCFG.Nodes)+len(d.opCodes))) != nil || d.Work.CheckAlloc(int64(len(d.semanticCFG.Nodes)+len(d.opCodes))*128) != nil) {
		return nil
	}
	p := &numericCopyProvenance{d: d, original: map[*OpCode]bool{}, decoded: map[*OpCode]bool{}, loads: map[values.JavaValue]*OpCode{}, ambiguous: map[values.JavaValue]bool{}, producers: producers, remaining: 65536}
	for _, op := range d.semanticCFG.Nodes {
		if op == nil || op.Instr == nil {
			return nil
		}
		p.original[op] = true
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return nil
		}
		p.decoded[op] = true
		if dynamicPrimitiveLoad(op.Instr.OpCode, types.JavaInteger) && len(op.stackProduced) == 1 {
			cell := op.stackProduced[0]
			if cell != nil && !(reflect.ValueOf(cell).Kind() == reflect.Ptr && reflect.ValueOf(cell).IsNil()) {
				p.ambiguous[cell] = p.ambiguous[cell] || p.loads[cell] != nil
				p.loads[cell] = op
			}
		}
	}
	return p
}

func (p *numericCopyProvenance) charge(n int) bool {
	p.remaining -= n
	return p.remaining >= 0 && (p.d.Work == nil || p.d.Work.Charge(workbudget.CounterGraphScans, int64(n)) == nil)
}

func (p *numericCopyProvenance) known(op *OpCode) bool {
	return op != nil && op.Instr != nil && !op.IsCustom && p.original[op] && p.decoded[op]
}

// Retain the first original LOAD identity before examining its mutable leaf.
// A later fold to a literal must not hide an unknown incoming definition.
func (p *numericCopyProvenance) operand(value values.JavaValue) (*OpCode, values.JavaValue, bool) {
	var load *OpCode
	for depth := 0; depth < 64 && value != nil; depth++ {
		if !p.charge(1) || p.ambiguous[value] || (reflect.ValueOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil()) {
			return nil, nil, false
		}
		if load == nil {
			load = p.loads[value]
		}
		slot, ok := value.(*values.SlotValue)
		if !ok {
			return load, value, true
		}
		if slot == nil {
			return nil, nil, false
		}
		value = slot.GetValue()
	}
	return nil, nil, false
}

func originalIntLocalPacket(op *OpCode, load bool) bool {
	if op == nil || op.Instr == nil {
		return false
	}
	code := op.Instr.OpCode
	generic := OP_ISTORE
	valid := dynamicPrimitiveStore(code, types.JavaInteger)
	if load {
		generic = OP_ILOAD
		valid = dynamicPrimitiveLoad(code, types.JavaInteger)
	}
	if !valid {
		return false
	}
	if code != generic {
		return !op.IsWide && len(op.Data) == 0
	}
	width := 1
	if op.IsWide {
		width = 2
	}
	return len(op.Data) == width
}

func (p *numericCopyProvenance) entryIsNumeric(slot int) bool {
	ctx := p.d.FunctionContext
	if ctx == nil || !p.charge(len(ctx.CurrentMethodDesc)+1) {
		return false
	}
	params, _, err := callbinding.Descriptor(ctx.CurrentMethodDesc)
	if err != nil {
		return false
	}
	index := 0
	if !ctx.IsStatic {
		index = 1
	}
	for _, descriptor := range params {
		if index == slot {
			return numericJoinDescriptor(descriptor)
		}
		index++
		if descriptor == "J" || descriptor == "D" {
			index++
		}
	}
	return false
}

func (p *numericCopyProvenance) provesStore(store *OpCode) bool {
	if p == nil || !p.known(store) || !originalIntLocalPacket(store, false) || len(store.stackConsumed) != 1 {
		return false
	}
	load, _, valid := p.operand(store.stackConsumed[0])
	if !valid || load == nil {
		return false
	}
	pending := []*OpCode{}
	seen := map[*OpCode]bool{}
	witness, seed := false, false
	// Direct values are only leaves of original STOREs. A LOAD must always
	// discharge its frozen inputs, even if its current view resembles a leaf.
	acceptOperand := func(operand values.JavaValue) bool {
		load, value, ok := p.operand(operand)
		if !ok {
			return false
		}
		if load != nil {
			if !p.known(load) || !originalIntLocalPacket(load, true) {
				return false
			}
			// Simulation can combine an immediately preceding local update
			// with this exact LOAD's result. Its numeric domain still comes
			// from the original input definitions, never the expression shape
			// or current JavaRef type. The expression itself stays unchanged.
			pending = append(pending, load)
			return true
		}
		if literal, ok := value.(*values.JavaLiteral); ok {
			if literal == nil {
				return false
			}
			word, integer := literal.Data.(int)
			if !integer || !(isExactPrimer(literal.Type(), types.JavaInteger) || (word == 0 || word == 1) && isExactPrimer(literal.Type(), types.JavaBoolean)) {
				return false
			}
			seed = true
			witness = witness || word != 0 && word != 1
			return true
		}
		if p.producers[value] {
			witness, seed = true, true
			return true
		}
		return false
	}
	if !acceptOperand(store.stackConsumed[0]) {
		return false
	}
	for len(pending) > 0 {
		if !p.charge(1) {
			return false
		}
		read := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[read] {
			continue
		}
		seen[read] = true
		slot := GetRetrieveIdx(read)
		if read.Instr.OpCode == OP_IINC {
			slot = GetStoreIdx(read)
		}
		if slot < 0 {
			return false
		}
		defs, entry := p.d.semanticCFG.ReachingDefinitions(read, slot)
		if p.d.semanticCFG.Err != nil || !p.charge(len(defs)+1) || !entry && len(defs) == 0 {
			return false
		}
		if entry {
			if !p.entryIsNumeric(slot) {
				return false
			}
			witness, seed = true, true
		}
		for _, definition := range defs {
			if !p.known(definition) || GetStoreIdx(definition) != slot {
				return false
			}
			if definition.Instr.OpCode == OP_IINC {
				width := 2
				if definition.IsWide {
					width = 4
				}
				if len(definition.Data) != width {
					return false
				}
				witness = true
				// IINC reads before writing: prove the incoming word too. An
				// unseeded cycle or unavailable entry is not an initializer.
				pending = append(pending, definition)
			} else if !originalIntLocalPacket(definition, false) || len(definition.stackConsumed) != 1 || !acceptOperand(definition.stackConsumed[0]) {
				return false
			}
		}
	}
	return witness && seed
}
