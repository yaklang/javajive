package core

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A JVM store cannot throw. A literal store at the start of a protected region
// therefore kills a preceding definition on every real exceptional path, while
// Java definite assignment still needs a declaration initialized outside try.
// Keep that original, otherwise dead definition as the source declaration seed.
// This is a source identity relation, not an extra JVM reaching definition: the
// immutable exceptional before-state and its web partition remain unchanged.
func (d *Decompiler) restoreExceptionDeclarationSeeds() {
	if len(d.ExceptionTable) == 0 {
		return
	}
	g, webs := d.semanticCFG, d.slotWebs()
	if g == nil || g.Err != nil || webs == nil {
		return
	}
	present := map[*OpCode]bool{}
	complete := map[int]bool{}
	groups := map[int][]*OpCode{}
	loads := map[int][]*OpCode{}
	for _, op := range g.Nodes {
		present[op] = true
		if op == nil || op.Instr == nil {
			continue
		}
		w, ok := webs.webOf[op]
		if !ok {
			continue
		}
		if isLocalStoreOpcode(op.Instr.OpCode) {
			groups[w] = append(groups[w], op)
		}
		if isLocalLoadOpcode(op.Instr.OpCode) {
			loads[w] = append(loads[w], op)
		}
	}
	for op, web := range webs.webOf {
		if _, known := complete[web]; !known {
			complete[web] = true
		}
		if !present[op] {
			complete[web] = false
		}
	}
	// Only a read actually reached from an original exception handler creates
	// this lexical obligation. Ordinary slot reuse must retain disjoint names.
	handlerLoads := map[*OpCode]bool{}
	byPC := map[uint16]*OpCode{}
	for _, op := range g.Nodes {
		if op != nil {
			byPC[op.CurrentOffset] = op
		}
	}
	queue := []*OpCode{}
	for _, row := range d.ExceptionTable {
		if row != nil {
			queue = append(queue, byPC[row.HandlerPc])
		}
	}
	seen := map[*OpCode]bool{}
	for len(queue) > 0 {
		op := queue[0]
		queue = queue[1:]
		if op == nil || seen[op] {
			continue
		}
		seen[op] = true
		if !d.seedGraphStep(g) {
			return
		}
		if op.Instr != nil && isLocalLoadOpcode(op.Instr.OpCode) {
			handlerLoads[op] = true
		}
		for _, i := range g.outgoing[op] {
			e := g.Edges[i]
			if e.Kind != EdgeException {
				queue = append(queue, e.To)
			}
		}
	}
	for _, first := range g.Nodes {
		if first == nil || first.Instr == nil || !isLocalStoreOpcode(first.Instr.OpCode) {
			continue
		}
		web, ok := webs.webOf[first]
		if !ok || !complete[web] || len(loads[web]) == 0 {
			continue
		}
		stores := groups[web]
		if len(stores) == 0 || stores[0] != first {
			continue
		}
		visible := false
		for _, load := range loads[web] {
			visible = visible || handlerLoads[load]
		}
		if !visible {
			continue
		}
		entry := false
		for _, w := range webs.entryWeb {
			entry = entry || w == web
		}
		if entry {
			continue
		}
		protected := false
		for _, row := range d.ExceptionTable {
			if row != nil && first.CurrentOffset >= row.StartPc && first.CurrentOffset < row.EndPc {
				protected = true
			}
		}
		if !protected {
			continue
		}
		slot := GetStoreIdx(first)
		seed := d.uniqueNormalSeed(first, slot)
		if seed == nil || seed.CurrentOffset >= first.CurrentOffset {
			continue
		}
		seedWeb, ok := webs.webOf[seed]
		if !ok || !complete[seedWeb] || seedWeb == web || len(groups[seedWeb]) != 1 || len(loads[seedWeb]) != 0 {
			continue
		}
		entry = false
		for _, w := range webs.entryWeb {
			entry = entry || w == seedWeb
		}
		if entry {
			continue
		}
		protected = false
		for _, row := range d.ExceptionTable {
			if row != nil && seed.CurrentOffset >= row.StartPc && seed.CurrentOffset < row.EndPc {
				protected = true
			}
		}
		if protected {
			continue
		}
		canonical := d.seedStoreRef(first)
		old := d.seedStoreRef(seed)
		if sourceSeedStoreCategory(seed) != sourceSeedStoreCategory(first) || canonical == nil || old == nil || canonical.Type() == nil || old.Type() == nil || !reflect.DeepEqual(canonical.Type().RawType(), old.Type().RawType()) {
			continue
		}
		// A mutable simulator alias belonging to another web is not proof of one
		// source variable. Require the complete target web already normalized.
		valid := true
		for _, st := range stores {
			if GetStoreIdx(st) != slot || d.seedStoreRef(st) != canonical {
				valid = false
			}
		}
		for _, load := range loads[web] {
			if GetRetrieveIdx(load) != slot || len(load.stackProduced) != 1 || safeSeedOperand(load.stackProduced[0]) != canonical {
				valid = false
			}
		}
		for op, infos := range d.opcodeIdToRef {
			w, known := webs.webOf[op]
			if !known || w == web {
				continue
			}
			for _, info := range infos {
				if info[0] == canonical {
					valid = false
				}
			}
		}
		if !valid {
			continue
		}
		d.opcodeIdToRef[seed][0] = [2]any{canonical, true}
		for _, st := range stores {
			d.opcodeIdToRef[st][0] = [2]any{canonical, false}
		}
		canonical.WebDeclType = canonical.Type().Copy()
		canonical.SolvedWebIdentity = canonical.Id
		d.disFoldRef = append(d.disFoldRef, canonical, old)
	}
}

func (d *Decompiler) seedStoreRef(op *OpCode) *values.JavaRef {
	infos := d.opcodeIdToRef[op]
	if len(infos) != 1 || len(op.stackConsumed) != 1 {
		return nil
	}
	ref, ok := infos[0][0].(*values.JavaRef)
	if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis {
		return nil
	}
	rhs := safeSeedOperand(op.stackConsumed[0])
	if rhs == nil || rhs.Type() == nil {
		return nil
	}
	return ref
}

// Every normal predecessor path must encounter this one store before method
// entry. Exception predecessors and cycles are rejected, not silently omitted.
func (d *Decompiler) uniqueNormalSeed(first *OpCode, slot int) *OpCode {
	g := d.semanticCFG
	seen := map[*OpCode]uint8{}
	var seed *OpCode
	var walk func(*OpCode) bool
	walk = func(op *OpCode) bool {
		if op == nil || !d.seedGraphStep(g) {
			return false
		}
		if seen[op] == 1 {
			return false
		}
		if seen[op] == 2 {
			return true
		}
		if op != first && op.Instr != nil && isLocalStoreOpcode(op.Instr.OpCode) && GetStoreIdx(op) == slot {
			if seed != nil && seed != op {
				return false
			}
			seed = op
			return true
		}
		if op.Instr != nil && op.Instr.OpCode == OP_IINC && GetStoreIdx(op) == slot {
			return false
		}
		seen[op] = 1
		incoming := g.incoming[op]
		if len(incoming) == 0 {
			return false
		}
		for _, i := range incoming {
			e := g.Edges[i]
			if e.Kind == EdgeException || !walk(e.From) {
				return false
			}
		}
		seen[op] = 2
		return true
	}
	if !walk(first) {
		return nil
	}
	return seed
}
func (d *Decompiler) seedGraphStep(g *SemanticCFG) bool {
	g.GraphScans++
	if err := g.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
		g.Err = err
		return false
	}
	return true
}

func sourceSeedStoreCategory(op *OpCode) int {
	if op == nil || op.Instr == nil {
		return -1
	}
	code := op.Instr.OpCode
	switch {
	case code >= OP_ISTORE && code <= OP_ASTORE:
		return code - OP_ISTORE
	case code >= OP_ISTORE_0 && code <= OP_ASTORE_3:
		return (code - OP_ISTORE_0) / 4
	}
	return -1
}

// The proof boundary accepts decoded typed operands, not malformed or cyclic
// SlotValue wrappers. Checking before Type avoids a typed-nil value panic.
func safeSeedOperand(value values.JavaValue) values.JavaValue {
	seen := map[*values.SlotValue]bool{}
	for value != nil {
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Ptr && rv.IsNil() {
			return nil
		}
		slot, ok := value.(*values.SlotValue)
		if !ok {
			return value
		}
		if seen[slot] {
			return nil
		}
		seen[slot] = true
		value = slot.GetValue()
	}
	return nil
}
