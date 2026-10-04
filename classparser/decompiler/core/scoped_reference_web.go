package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A source field read can declare a local at an in-scope type variable while
// the JVM web carries only its first bound. Retain that declaration view only
// for a complete, single-owner web whose every store has the identical erasure.
// This does not change the computational types or follow old local .Val links.
func (d *Decompiler) restoreScopedReferenceWebs() {
	webs := d.slotWebs()
	if webs == nil || d.FunctionContext == nil {
		return
	}
	groups := map[int][]*OpCode{}
	decoded := map[*OpCode]bool{}
	for _, op := range d.opCodes {
		decoded[op] = true
	}
	owners := map[*values.JavaRef]map[int]bool{}
	for op, w := range webs.webOf {
		if op == nil || op.Instr == nil || !isReferenceStoreOpcode(op.Instr.OpCode) {
			continue
		}
		groups[w] = append(groups[w], op)
		for _, info := range d.opcodeIdToRef[op] {
			if ref, ok := info[0].(*values.JavaRef); ok {
				if owners[ref] == nil {
					owners[ref] = map[int]bool{}
				}
				owners[ref][w] = true
			}
		}
	}
	for w, stores := range groups {
		entry := false
		for _, e := range webs.entryWeb {
			entry = entry || e == w
		}
		if entry {
			continue
		}
		var ref *values.JavaRef
		var source types.JavaType
		valid := true
		for _, op := range stores {
			infos := d.opcodeIdToRef[op]
			if !decoded[op] || len(infos) != 1 || len(op.stackConsumed) != 1 {
				valid = false
				break
			}
			r, ok := infos[0][0].(*values.JavaRef)
			if !ok || r == nil || r.IsParam || r.IsThis || len(owners[r]) != 1 || (ref != nil && r != ref) {
				valid = false
				break
			}
			ref = r
			if field := values.SourceFieldType(d.FunctionContext, op.stackConsumed[0]); field != nil && values.ScopedErasureView(d.FunctionContext, field, op.stackConsumed[0]) {
				if source != nil && source.String(d.FunctionContext) != field.String(d.FunctionContext) {
					valid = false
					break
				}
				source = field
			}
		}
		if !valid || ref == nil || source == nil {
			continue
		}
		for _, op := range stores {
			v := op.stackConsumed[0]
			if values.UnpackSoltValue(v) == values.JavaNull || values.IsNullLiteral(values.UnpackSoltValue(v)) {
				continue
			}
			if !values.ScopedErasureView(d.FunctionContext, source, v) && (v.Type() == nil || v.Type().String(d.FunctionContext) != source.String(d.FunctionContext)) {
				valid = false
				break
			}
		}
		if valid {
			ref.WebDeclType = source.Copy()
		}
	}
}
