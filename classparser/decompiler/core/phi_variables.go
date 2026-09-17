package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// unifyReferenceWebs lowers reference joins to one source variable AFTER every
// definition has been simulated. Identity comes from the def-use web; the type
// is solved separately from the actual RHS values. This closes the old path that
// simply abandoned a load when multiple distinct VarUids reached it.
func (d *Decompiler) unifyReferenceWebs() {
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	groups := map[int][]*OpCode{}
	order := []int{}
	owners := map[*values.JavaRef]map[int]bool{}
	for _, op := range d.opCodes {
		if !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		web, ok := webs.webOf[op]
		if !ok {
			continue
		}
		infos := d.opcodeIdToRef[op]
		d.tracef("var-fold", "phi store pc=%d web=%d refs=%d consumed=%d", op.CurrentOffset, web, len(infos), len(op.stackConsumed))
		if len(infos) != 1 {
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		if owners[ref] == nil {
			owners[ref] = map[int]bool{}
		}
		owners[ref][web] = true
		if _, ok := groups[web]; !ok {
			order = append(order, web)
		}
		groups[web] = append(groups[web], op)
	}
	for _, web := range order {
		stores := groups[web]
		if len(stores) < 2 {
			continue
		}
		refs := map[*values.JavaRef]bool{}
		valid := true
		var canon *values.JavaRef
		var joined types.JavaType
		for _, store := range stores {
			ref := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
			if ref.IsParam || len(owners[ref]) != 1 || len(store.stackConsumed) != 1 {
				valid = false
				break
			}
			value := values.UnpackSoltValue(store.stackConsumed[0])
			refs[ref] = true
			if canon == nil {
				canon = ref
			}
			if values.IsNullLiteral(value) {
				continue
			}
			typ := slotDeclType(value)
			if typ == nil {
				valid = false
				break
			}
			if primitive, ok := typ.RawType().(*types.JavaPrimer); ok {
				if primitive.Name == types.JavaString {
					typ = types.NewJavaClass("java.lang.String")
				} else {
					valid = false
					break
				}
			}

			if joined == nil {
				joined = typ
				continue
			}
			if a, aok := types.ClassFQNOf(joined); aok {
				if b, bok := types.ClassFQNOf(typ); bok && a == b {
					continue
				}
			}
			lub := joinWebTypes(joined, typ, d.FunctionContext.SiblingSuperTypes)
			joined = lub
		}
		d.tracef("var-fold", "phi web=%d stores=%d refs=%d valid=%v", web, len(stores), len(refs), valid)
		if !valid || len(refs) < 2 {
			continue
		}
		if joined == nil {
			joined = types.NewJavaClass("java.lang.Object")
		}
		// All aliases are retained as objects (existing SlotValues can reference them),
		// but agree on identity and declaration type before statements are constructed.
		for ref := range refs {
			ref.Id = canon.Id
			ref.VarUid = canon.VarUid
			ref.ResetVarType(joined)
		}
		for i, store := range stores {
			d.opcodeIdToRef[store][0][1] = i == 0
		}
	}
}

// Preserve array rank and component identity at reference joins. A blanket Object
// fallback loses arraylength and array-load verifier facts.
func joinWebTypes(a, b types.JavaType, provider types.SuperTypeProvider) types.JavaType {
	if a.IsArray() && b.IsArray() {
		x, y := a.ElementType(), b.ElementType()
		xp, xok := x.RawType().(*types.JavaPrimer)
		yp, yok := y.RawType().(*types.JavaPrimer)
		if xok || yok {
			if xok && yok && xp.Name == yp.Name {
				return a
			}
			return types.NewJavaClass("java.lang.Object")
		}
		return types.NewJavaArrayType(joinWebTypes(x, y, provider))
	}
	if an, ok := types.RawClassFQN(a); ok {
		if bn, bok := types.RawClassFQN(b); bok && an == bn {
			return a
		}
	}
	lub := types.BridgedCommonSuperType(a, b, provider)
	if lub == nil {
		lub = types.CommonSuperType(a, b)
	}
	if lub == nil {
		lub = types.NewJavaClass("java.lang.Object")
	}
	return lub
}
