package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
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
	uses := d.referenceUseConstraints()
	parameters := d.parameterWebRefs(webs)
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
	// Copy chains can point forward to a web solved later in bytecode order.
	// Iterate until those declarations stop changing (at most one propagation
	// step per web); a single pass leaves caches typed at a provisional branch.
	for round := 0; round <= len(order); round++ {
		changed := false
		for _, web := range order {
			stores := groups[web]
			param := parameters[web]
			if len(stores) < 2 && param == nil {
				if len(stores) != 1 || len(stores[0].stackConsumed) != 1 {
					continue
				}
				if _, ternary := values.UnpackSoltValue(stores[0].stackConsumed[0]).(*values.TernaryExpression); !ternary {
					continue
				}
			}
			refs := map[*values.JavaRef]bool{}
			valid := true
			var canon *values.JavaRef
			var joined types.JavaType
			parameterizations := map[string]types.JavaType{}
			erased := map[string]bool{}
			for _, store := range stores {
				refs[d.opcodeIdToRef[store][0][0].(*values.JavaRef)] = true
			}
			if param != nil {
				refs[param] = true
				canon = param
			}
			for _, store := range stores {
				ref := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
				if (ref.IsParam && ref != param) || len(owners[ref]) != 1 || len(store.stackConsumed) != 1 {
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
				for _, typ := range webDefinitionTypes(value, refs) {
					if typ != nil && d.traceEnabled("var-fold") {
						d.tracef("var-fold", "phi type pc=%d web=%d rhs=%s ref=%s", store.CurrentOffset, web, typ.String(d.FunctionContext), ref.Type().String(d.FunctionContext))
					}
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
					if p, ok := typ.RawType().(*types.JavaParameterizedType); ok {
						name := p.RawClassName
						if previous := parameterizations[name]; previous != nil && !reflect.DeepEqual(previous.RawType(), typ.RawType()) {
							erased[name] = true
						}
						parameterizations[name] = typ.Copy()
					}

					if joined == nil {
						joined = typ
						continue
					}
					lub := joinWebTypes(joined, typ, d.FunctionContext.SiblingSuperTypes)
					joined = lub
				}
			}
			d.tracef("var-fold", "phi web=%d stores=%d refs=%d valid=%v", web, len(stores), len(refs), valid)
			if !valid || len(refs) == 0 {
				continue
			}
			if joined == nil {
				joined = types.NewJavaClass("java.lang.Object")
			}
			if name, ok := types.RawClassFQN(joined); ok && erased[name] {
				// Once two invariant parameterizations conflict, a later raw or
				// parameterized definition must not narrow the web again.
				joined = types.NewJavaClass(name)
			}
			joined = d.constrainWebDeclaration(joined, stores, uses)
			if param != nil {
				// The method signature fixes a parameter's source type. Its entry
				// definition cannot be discarded or narrowed to a branch's value.
				joined = param.Type().Copy()
			}
			if d.traceEnabled("var-fold") {
				d.tracef("var-fold", "phi joined web=%d type=%s", web, joined.String(d.FunctionContext))
			}
			// All aliases are retained as objects (existing SlotValues can reference them),
			// but agree on identity and declaration type before statements are constructed.
			for ref := range refs {
				if ref.WebDeclType == nil || !reflect.DeepEqual(ref.WebDeclType.RawType(), joined.RawType()) {
					changed = true
				}
				ref.Id = canon.Id
				ref.VarUid = canon.VarUid
				ref.ResetVarType(joined.Copy())
				ref.WebDeclType = joined.Copy()
			}
			if len(refs) > 1 || param != nil {
				for i, store := range stores {
					d.opcodeIdToRef[store][0][1] = i == 0 && param == nil
				}
			}
		}
		if !changed {
			break
		}
	}

}

// Preserve array rank and component identity at reference joins. A blanket Object
// fallback loses arraylength and array-load verifier facts.
func joinWebTypes(a, b types.JavaType, provider types.SuperTypeProvider) types.JavaType {
	if reflect.DeepEqual(a.RawType(), b.RawType()) {
		return a
	}
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
			if reflect.DeepEqual(a.RawType(), b.RawType()) {
				return a
			}
			_, ap := a.RawType().(*types.JavaParameterizedType)
			_, bp := b.RawType().(*types.JavaParameterizedType)
			// A raw method-reference type carries no generic constraint; retain
			// the concrete parameterization recovered from the other definition.
			if ap && !bp {
				return a
			}
			if bp && !ap {
				return b
			}
			// Different parameterizations are invariant. Keeping the first arm
			// rejects legal assignments from the other arm; use JVM erasure.
			return types.NewJavaClass(an)
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

// A recurrence contributes no new type constraint. In particular, the cached
// type of `x != null ? x : new T()` includes the provisional Object type of x;
// only the concrete arm and the web's other definitions constrain the solution.
func webDefinitionTypes(value values.JavaValue, self map[*values.JavaRef]bool) []types.JavaType {
	value = values.UnpackSoltValue(value)
	if values.IsNullLiteral(value) {
		return nil
	}
	if ref, ok := value.(*values.JavaRef); ok && self[ref] {
		return nil
	}
	if ternary, ok := value.(*values.TernaryExpression); ok {
		return append(webDefinitionTypes(ternary.TrueValue, self), webDefinitionTypes(ternary.FalseValue, self)...)
	}
	return []types.JavaType{slotDeclType(value)}
}
