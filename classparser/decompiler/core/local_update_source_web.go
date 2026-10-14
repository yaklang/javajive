package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A reaching-definition web relates definitions observed by the same read.
// It does not, by itself, relate the input and output of a read-modify-write.
// An enumerable ++/-- needs both to name one SOURCE location. Add only the
// constraints re-proved by the original typed update packet; the immutable
// reaching partition and decoded instruction/stack evidence stay unchanged.
func (d *Decompiler) localUpdateSourceWebs() *slotWeb {
	if d == nil {
		return nil
	}
	original := d.slotWebs()
	if original == nil {
		return nil
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.stackValueProducers)+len(original.webOf)+len(original.entryWeb))) != nil || d.Work.CheckAlloc(int64(len(original.webOf)+len(original.entryWeb)+1)*64) != nil) {
		return nil
	}
	type pair struct{ read, write int }
	var pairs []pair
	max := 0
	for _, w := range original.webOf {
		if w > max {
			max = w
		}
	}
	for _, w := range original.entryWeb {
		if w > max {
			max = w
		}
	}
	for value, producer := range d.stackValueProducers {
		load, store, proved := d.localUpdateSourceConstraint(value, producer)
		if !proved {
			continue
		}
		read, r := original.webOf[load]
		write, w := original.webOf[store]
		if r && w && read != write {
			pairs = append(pairs, pair{read, write})
		}
	}
	if d.Work != nil && d.Work.Err() != nil {
		return nil
	}
	if len(pairs) == 0 {
		return original
	}
	if d.Work != nil && d.Work.CheckAlloc(int64(max+1)*8) != nil {
		return nil
	}
	joined := newUnionFind(max + 1)
	for _, p := range pairs {
		a, b := joined.find(p.read), joined.find(p.write)
		// Select the least original representative independent of map visitation.
		if a < b {
			joined.union(b, a)
		} else {
			joined.union(a, b)
		}
	}
	source := &slotWeb{webOf: map[*OpCode]int{}, entryWeb: map[int]int{}}
	for op, w := range original.webOf {
		source.webOf[op] = joined.find(w)
	}
	for slot, w := range original.entryWeb {
		source.entryWeb[slot] = joined.find(w)
	}
	return source
}

func (d *Decompiler) localUpdateSourceConstraint(value values.JavaValue, producer *OpCode) (load, store *OpCode, proved bool) {
	if d == nil {
		return nil, nil, false
	}
	expr, ok := value.(*values.JavaExpression)
	if !ok || expr == nil || producer == nil || producer.Instr == nil || (expr.Op != values.INC && expr.Op != values.DEC) {
		return nil, nil, false
	}
	if producer.LocalStoreFoldedToPostOp && len(expr.Values) == 2 {
		one, literal := expr.Values[1].(*values.JavaLiteral)
		if !literal || one == nil || producer.LocalStoreFoldedToTypedPreOp {
			return nil, nil, false
		}
		number, numbered := one.Data.(int)
		if !numbered || number != 1 {
			return nil, nil, false
		}
		infos := d.opcodeIdToRef[producer]
		if len(infos) != 1 || infos[0][1] != false {
			return nil, nil, false
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok {
			return nil, nil, false
		}
		op, valid := d.immediateLocalPostUpdate(producer, ref, expr.Values[0])
		if valid && op == expr.Op {
			return d.stackValueProducers[expr.Values[0]], producer, true
		}
		return nil, nil, false
	}
	if len(expr.Values) != 1 {
		return nil, nil, false
	}
	if producer.LocalStoreFoldedToTypedPreOp {
		infos := d.opcodeIdToRef[producer]
		if len(infos) != 1 || infos[0][1] != false {
			return nil, nil, false
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || ref.Type() == nil {
			return nil, nil, false
		}
		primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
		if !ok || primitive == nil {
			return nil, nil, false
		}
		protocol, ok := localPostProtocols[primitive.Name]
		if !ok {
			return nil, nil, false
		}
		size := 5
		if protocol.narrow != 0 {
			size++
		}
		path, valid := d.localUpdatePrivatePath(producer, size)
		if !valid {
			return nil, nil, false
		}
		duplicate := path[size-2]
		for _, remaining := range duplicate.stackProduced {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return nil, nil, false
			}
			dup, target, op, valid := d.immediateLocalTypedPreUpdate(producer, ref, remaining, d.slotStoreValue[producer])
			if valid && dup == duplicate && target == expr.Values[0] && op == expr.Op {
				return path[0], producer, true
			}
		}
		return nil, nil, false
	}
	// The write/reload form produces its NEW result at the original reload.
	if len(producer.Source) != 1 || producer.Source[0] == nil || !producer.Source[0].LocalStoreFoldedToTypedPreOp {
		return nil, nil, false
	}
	op, valid := d.immediateLocalTypedReloadUpdate(producer, expr.Values[0])
	if !valid || op != expr.Op {
		return nil, nil, false
	}
	ref, ok := values.UnpackSoltValue(expr.Values[0]).(*values.JavaRef)
	if !ok || ref == nil || ref.Type() == nil {
		return nil, nil, false
	}
	primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive == nil {
		return nil, nil, false
	}
	protocol, ok := localPostProtocols[primitive.Name]
	if !ok {
		return nil, nil, false
	}
	size := 5
	if protocol.narrow != 0 {
		size++
	}
	path, valid := d.localUpdatePrivatePath(producer, size)
	if !valid {
		return nil, nil, false
	}
	return path[0], producer.Source[0], true
}
