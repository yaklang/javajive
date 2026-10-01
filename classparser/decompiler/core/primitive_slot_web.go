package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Equal JVM computational types do not prove that two stores define the same
// Java local: in particular, both a boolean and an int use istore. Split only
// when the reaching-definition partition proves that every known definition
// of the current identity belongs to a different web. A shared read (including
// a loop back edge or parameter live-in) keeps assignments in the same web.
func (d *Decompiler) primitiveStoreStartsDisjointWeb(store *OpCode, current *values.JavaRef, value values.JavaValue) bool {
	if current == nil || current.VarUid == "" || value == nil || value.Type() == nil {
		return false
	}
	p, ok := value.Type().RawType().(*types.JavaPrimer)
	if !ok {
		return false
	}
	switch p.Name {
	case types.JavaBoolean, types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble:
	default:
		return false
	}
	webs := d.slotWebs()
	if webs == nil {
		return false
	}
	target, ok := webs.webOf[store]
	if !ok {
		return false
	}
	if creator := d.refToCreatingStore[current]; creator != nil {
		if owner, ok := webs.webOf[creator]; !ok || owner == target {
			return false
		}
	}
	known := false
	for op, refs := range d.opcodeIdToRef {
		if op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		for _, pair := range refs {
			ref, ok := pair[0].(*values.JavaRef)
			if !ok || ref == nil || ref.VarUid != current.VarUid {
				continue
			}
			owner, ok := webs.webOf[op]
			if !ok || owner == target {
				return false
			}
			known = true
		}
	}
	if current.IsParam || current.IsThis {
		found := false
		for owner, ref := range d.parameterWebRefs(webs) {
			if ref.VarUid == current.VarUid {
				if owner == target {
					return false
				}
				found, known = true, true
			}
		}
		if !found {
			return false
		}
	}
	return known
}
