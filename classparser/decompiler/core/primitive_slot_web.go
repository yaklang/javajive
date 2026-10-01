package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Equal JVM computational types do not prove that two stores define the same
// Java local: in particular, both a boolean and an int use istore. Split only
// when the old identity has descriptor evidence of a boolean use and the
// reaching-definition partition proves every known definition belongs to a
// different web. A shared read keeps assignments in the same web. Numeric-only
// webs still use the existing continuation logic: a computational type alone
// does not establish the Java source domain of an unresolved phi.
func (d *Decompiler) primitiveStoreStartsDisjointWeb(store *OpCode, current *values.JavaRef, value values.JavaValue) bool {
	if current == nil || current.VarUid == "" || value == nil || value.Type() == nil {
		return false
	}
	if !d.localHasBooleanDescriptorUse(current) {
		return false
	}
	// iconst_0/1 alone cannot establish the Java domain of an int-category
	// store. Leave these provisional initializers to the existing boolean/phi
	// reconstruction until a typed RHS supplies that evidence. Forcing a new
	// identity here can prematurely commit an unresolved numeric phi to boolean.
	if _, ambiguous := intLiteral01(value); ambiguous {
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

// Inspect the raw consumed operand, before call-argument boolean coercion.
// A numeric expression used inside a comparison is not a boolean local. In
// contrast, a direct local operand passed to a Z parameter carries explicit
// descriptor evidence, even if its simulated type is still the JVM's int.
func (d *Decompiler) localHasBooleanDescriptorUse(current *values.JavaRef) bool {
	for _, param := range d.Params {
		if ref, ok := param.(*values.JavaRef); ok && values.SameLocal(ref, current) && isExactPrimer(ref.Type(), types.JavaBoolean) {
			return true
		}
	}
	for op, call := range d.invokeFuncCall {
		if call == nil || call.FuncType == nil || op == nil {
			continue
		}
		params := call.FuncType.ParamTypes
		if len(op.stackConsumed) < len(params) {
			continue
		}
		for index, typ := range params {
			if !isExactPrimer(typ, types.JavaBoolean) {
				continue
			}
			ref, ok := values.UnpackSoltValue(op.stackConsumed[len(params)-1-index]).(*values.JavaRef)
			if ok && values.SameLocal(ref, current) {
				return true
			}
		}
	}
	return false
}
