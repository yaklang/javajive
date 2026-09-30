package core

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
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
	d.constrainPolyEvaluationSnapshots(uses)
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
	// The legacy stack simulator can reuse one ref for distinct, disjoint
	// local webs with identical erasure. Its last store must not parameterize
	// every earlier/later read. Solve those shared declarations before the
	// per-web loop (which correctly refuses to claim a ref owned by two webs).
	d.eraseConflictingSharedWebTypes(groups, order, owners)
	// Copy chains can point forward to a web solved later in bytecode order.
	// Iterate until those declarations stop changing (at most one propagation
	// step per web); a single pass leaves caches typed at a provisional branch.
	for round := 0; round <= len(order)+len(d.evaluationSnapshots); round++ {
		changed := false
		for _, web := range order {
			stores := groups[web]
			param := parameters[web]
			var polyUseTarget types.JavaType
			if d.getenv("JDEC_GENERIC_USE_CONSTRAINT_OFF") == "" {
				polyUseTarget = d.polyFunctionalUseTarget(stores, uses)
			}
			d.tracef("var-fold", "phi gate web=%d stores=%d poly-use-target=%v", web, len(stores), polyUseTarget != nil)
			if len(stores) < 2 && param == nil && polyUseTarget == nil {
				if len(stores) != 1 || len(stores[0].stackConsumed) != 1 {
					continue
				}
				switch value := values.UnpackSoltValue(stores[0].stackConsumed[0]).(type) {
				case *values.TernaryExpression:
				case *values.JavaRef:
					// A pure copy gets its declaration from the solved source web,
					// not the first branch observed during DFS simulation. No cast
					// is introduced and the source's earlier Val is not a definition.
					if value == nil || value == d.opcodeIdToRef[stores[0]][0][0] {
						continue
					}
				default:
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
			if polyUseTarget != nil {
				joined = polyUseTarget.Copy()
			}
			if param != nil {
				// The method signature fixes a parameter's source type. Its entry
				// definition cannot be discarded or narrowed to a branch's value.
				joined = param.Type().Copy()
			}
			if polyUseTarget != nil {
				for _, store := range stores {
					if store != nil && len(store.stackConsumed) == 1 {
						// A parameter web keeps its declared signature type; an ordinary
						// poly web uses the exact unique consumer target proved above.
						target := polyUseTarget
						if param != nil {
							target = joined
						}
						applyPolyFunctionalTarget(store.stackConsumed[0], target, map[*values.JavaRef]bool{})
					}
				}
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
		if d.refreshReferenceOperandSnapshotTypes() {
			changed = true
		}
		if !changed {
			break
		}
	}

}

func (d *Decompiler) eraseConflictingSharedWebTypes(groups map[int][]*OpCode, order []int, owners map[*values.JavaRef]map[int]bool) {
	definitions := map[*values.JavaRef][]types.JavaType{}
	refs := []*values.JavaRef{}
	for _, web := range order {
		for _, store := range groups[web] {
			ref := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
			if ref.IsParam || len(owners[ref]) < 2 || len(store.stackConsumed) != 1 {
				continue
			}
			if _, seen := definitions[ref]; !seen {
				refs = append(refs, ref)
			}
			definitions[ref] = append(definitions[ref], webDefinitionTypes(store.stackConsumed[0], map[*values.JavaRef]bool{ref: true})...)
		}
	}
	for _, ref := range refs {
		var joined types.JavaType
		for _, typ := range definitions[ref] {
			if typ == nil {
				continue
			}
			if joined == nil {
				joined = typ
			} else {
				joined = joinWebTypes(joined, typ, d.FunctionContext.SiblingSuperTypes)
			}
		}
		// This repair only erases conflicting generic arguments of the same
		// declaration; unrelated classes still need proper web separation.
		current, parameterized := types.AsParameterizedType(ref.Type())
		raw, ok := types.RawClassFQN(joined)
		_, stillParameterized := types.AsParameterizedType(joined)
		if parameterized && ok && !stillParameterized && sameRawTypeName(raw, current.RawClassName) {
			ref.ResetVarType(joined.Copy())
			ref.WebDeclType = joined.Copy()
		}
	}
}

// constrainPolyEvaluationSnapshots handles a direct lambda/method-reference
// result that the evaluator materialized before an ARETURN or other constrained
// use.  Such a temporary has no JVM local-store web, so unifyReferenceWebs cannot
// discover it through groups.  The snapshot is nevertheless an explicit IR
// definition with a stable JavaRef identity, and referenceUseConstraints records
// its exact consumer.  Applying the unique same-erasure generic target here
// restores Java's target typing without inventing information.
func (d *Decompiler) constrainPolyEvaluationSnapshots(uses map[*values.JavaRef][]types.JavaType) {
	if d.getenv("JDEC_GENERIC_USE_CONSTRAINT_OFF") != "" {
		return
	}
	for _, op := range d.opCodes {
		for _, snapshot := range d.evaluationSnapshots[op] {
			if snapshot.Ref == nil {
				continue
			}
			poly, admissible := polyFunctionalDefinition(snapshot.Value)
			if !poly || !admissible {
				continue
			}
			rawName, ok := types.RawClassFQN(snapshot.Ref.Type())
			if !ok {
				continue
			}
			target, conflict := uniqueParameterizedConstraint(rawName, uses[snapshot.Ref])
			if target == nil || conflict {
				continue
			}
			target = polyLambdaInputTarget(snapshot.Value, target)
			applyPolyFunctionalTarget(snapshot.Ref, target, map[*values.JavaRef]bool{})
			d.tracef("var-fold", "poly snapshot pc=%d ref=%s target=%s", op.CurrentOffset, traceRef(snapshot.Ref, d.FunctionContext), target.String(d.FunctionContext))
		}
	}
}

// webHasOnlyPolyFunctionalDefinitions reports whether every substantive value
// stored into the web is a lambda or method reference.  Those expressions are
// poly expressions in Java: their source type comes from the assignment/use
// target, while the classfile preserves only the erased invokedynamic type.
// Null initializers do not constrain the target and are therefore ignored.
func webHasOnlyPolyFunctionalDefinitions(stores []*OpCode) bool {
	hasPoly := false
	for _, store := range stores {
		if store == nil || len(store.stackConsumed) != 1 {
			return false
		}
		poly, admissible := polyFunctionalDefinition(store.stackConsumed[0])
		if !admissible {
			return false
		}
		hasPoly = hasPoly || poly
	}
	return hasPoly
}

func polyFunctionalDefinition(value values.JavaValue) (poly, admissible bool) {
	return polyFunctionalDefinitionSeen(value, map[*values.JavaRef]bool{})
}

func polyFunctionalDefinitionSeen(value values.JavaValue, seen map[*values.JavaRef]bool) (poly, admissible bool) {
	value = values.UnpackSoltValue(value)
	if values.IsNullLiteral(value) {
		return false, true
	}
	if ref, ok := value.(*values.JavaRef); ok && ref != nil && ref.Val != nil && ref.Val != value {
		if seen[ref] {
			return false, false
		}
		seen[ref] = true
		return polyFunctionalDefinitionSeen(ref.Val, seen)
	}
	if custom, ok := value.(*values.CustomValue); ok && custom != nil {
		return custom.Flag == "lambda" || custom.IsMethodRef, custom.Flag == "lambda" || custom.IsMethodRef
	}
	if ternary, ok := value.(*values.TernaryExpression); ok && ternary != nil {
		leftPoly, leftOK := polyFunctionalDefinitionSeen(ternary.TrueValue, seen)
		rightPoly, rightOK := polyFunctionalDefinitionSeen(ternary.FalseValue, seen)
		return leftPoly || rightPoly, leftOK && rightOK
	}
	return false, false
}

// applyPolyFunctionalTarget carries the use-derived target through compiler
// temporaries introduced while preserving invokedynamic evaluation order.  A
// store often consumes `tmp`, where tmp's backing value is the actual lambda;
// later single-use folding removes the store-local and leaves tmp's declaration
// in source.  Refining only the destination web would therefore be correct in
// IR but disappear from the rendered program.  Every traversed ref is a pure
// copy on a path already proven by polyFunctionalDefinition, so assigning the
// same target type preserves identity and runtime erasure.
func applyPolyFunctionalTarget(value values.JavaValue, target types.JavaType, seen map[*values.JavaRef]bool) {
	if value == nil || target == nil {
		return
	}
	value = values.UnpackSoltValue(value)
	if ref, ok := value.(*values.JavaRef); ok && ref != nil {
		if seen[ref] {
			return
		}
		seen[ref] = true
		if poly, admissible := polyFunctionalDefinition(ref); poly && admissible {
			ref.ResetVarType(target.Copy())
			ref.WebDeclType = target.Copy()
			applyPolyFunctionalTarget(ref.Val, target, seen)
		}
		return
	}
	if custom, ok := value.(*values.CustomValue); ok && custom != nil && custom.Flag == "lambda" && !custom.IsMethodRef {
		applyPolyLambdaReturnTarget(custom, target)
		return
	}
	if ternary, ok := value.(*values.TernaryExpression); ok && ternary != nil {
		applyPolyFunctionalTarget(ternary.TrueValue, target, seen)
		applyPolyFunctionalTarget(ternary.FalseValue, target, seen)
	}
}

// applyPolyLambdaReturnTarget bridges the second half of Java poly-expression
// target typing. Retyping the functional local fixes its declaration and SAM
// parameters, but the reconstructed synthetic body may still return the
// instantiated descriptor's erasure (Object, raw Future, raw Map). When the
// unique target proves a more specific SAM result, retain that result on the
// lambda so its deferred writer can emit the source-equivalent unchecked cast.
// Parameterized results use a raw-erasure bridge to avoid an illegal direct
// cast between invariant instantiations. Kill-switch:
// JDEC_POLY_LAMBDA_RETURN_CAST_OFF=1.
func applyPolyLambdaReturnTarget(lambda *values.CustomValue, target types.JavaType) {
	if lambda == nil || lambda.IsMethodRef || lambda.InstantiatedMtdDesc == "" || target == nil ||
		jdecenv.Get("JDEC_POLY_LAMBDA_RETURN_CAST_OFF") != "" ||
		jdecenv.Get("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF") != "" {
		return
	}
	desired := functionalInterfaceReturnType(target)
	current := functionalInterfaceReturnType(lambda.Type())
	if desired == nil || current == nil || reflect.DeepEqual(desired.RawType(), current.RawType()) {
		return
	}
	methodType, err := types.ParseMethodDescriptor(lambda.InstantiatedMtdDesc)
	if err != nil || methodType == nil || methodType.FunctionType() == nil {
		return
	}
	instantiatedReturn := methodType.FunctionType().ReturnType
	if instantiatedReturn == nil {
		return
	}
	if _, primitive := instantiatedReturn.RawType().(*types.JavaPrimer); primitive {
		return
	}
	if parameterized, ok := types.AsParameterizedType(desired); ok && parameterized != nil {
		instRaw, instOK := types.RawClassFQN(instantiatedReturn)
		if !instOK || instRaw != "java.lang.Object" && !sameRawTypeName(instRaw, parameterized.RawClassName) {
			return
		}
		lambda.LambdaReturnTarget = desired.Copy()
		lambda.LambdaReturnRawBridge = true
		return
	}
	// Bare in-scope type variables and type-variable arrays erase to a reference
	// descriptor. A direct cast is sufficient and preserves the runtime check
	// that javac encoded in the original source path.
	lambda.LambdaReturnTarget = desired.Copy()
	lambda.LambdaReturnRawBridge = false
}

func functionalInterfaceReturnType(typ types.JavaType) types.JavaType {
	parameterized, ok := types.AsParameterizedType(typ)
	if !ok || parameterized == nil {
		return nil
	}
	position, known := lambdaFIReturnPosition[parameterized.RawClassName]
	if !known || position < 0 || position >= len(parameterized.TypeArgs) {
		return nil
	}
	result := parameterized.TypeArgs[position]
	if wildcard, ok := result.(*types.JavaWildcardType); ok {
		if wildcard == nil || wildcard.Variant != "extends" || wildcard.Bound == nil {
			return nil
		}
		result = wildcard.Bound
	}
	return result
}

func sameRawTypeName(left, right string) bool {
	return strings.ReplaceAll(left, "/", ".") == strings.ReplaceAll(right, "/", ".")
}

func (d *Decompiler) polyFunctionalUseTarget(stores []*OpCode, uses map[*values.JavaRef][]types.JavaType) types.JavaType {
	if !webHasOnlyPolyFunctionalDefinitions(stores) {
		return nil
	}
	rawName := ""
	for _, store := range stores {
		if store == nil {
			return nil
		}
		for _, info := range d.opcodeIdToRef[store] {
			ref, ok := info[0].(*values.JavaRef)
			if !ok || ref == nil {
				return nil
			}
			name, ok := types.RawClassFQN(ref.Type())
			if !ok {
				return nil
			}
			if rawName == "" {
				rawName = name
			} else if rawName != name {
				return nil
			}
		}
	}
	if rawName == "" {
		return nil
	}
	target := d.uniqueParameterizedUseConstraint(rawName, stores, uses)
	for _, store := range stores {
		target = polyLambdaInputTarget(store.stackConsumed[0], target)
	}
	return target
}

// polyLambdaInputTarget retains the input erasure under which a synthetic
// lambda body was decoded, when the consumer explicitly permits a supertype.
// For example, BiFunction<Object, Entry, Entry<K,V>> is accepted by Map's
// BiFunction<? super Object, ? super Entry<K,V>, ? extends Entry<K,V>>.
// Replacing its raw Entry parameter by Entry<K,V> would change the static
// contract of body calls such as entry.setValue(Object), even though the JVM
// descriptor contains no corresponding cast. Only lower-bounded INPUT slots
// with an exactly matching erasure qualify; invariant slots and results do not.
func polyLambdaInputTarget(value values.JavaValue, target types.JavaType) types.JavaType {
	if target == nil || jdecenv.Get("JDEC_POLY_LAMBDA_INPUT_ERASURE_OFF") != "" {
		return target
	}
	seen := map[*values.JavaRef]bool{}
	var visit func(values.JavaValue)
	visit = func(value values.JavaValue) {
		switch v := values.UnpackSoltValue(value).(type) {
		case *values.JavaRef:
			if v != nil && !seen[v] {
				seen[v] = true
				visit(v.Val)
			}
		case *values.TernaryExpression:
			visit(v.TrueValue)
			visit(v.FalseValue)
		case *values.CustomValue:
			if v == nil || v.Flag != "lambda" || v.IsMethodRef {
				return
			}
			formal, ok := types.AsParameterizedType(target)
			actual, actualOK := types.AsParameterizedType(v.Type())
			if !ok || !actualOK || formal.RawClassName != actual.RawClassName || len(formal.TypeArgs) != len(actual.TypeArgs) {
				return
			}
			var inputs int
			switch formal.RawClassName {
			case "java.util.function.Function", "java.util.function.Consumer", "java.util.function.Predicate":
				inputs = 1
			case "java.util.function.BiFunction", "java.util.function.BiConsumer", "java.util.function.BiPredicate":
				inputs = 2
			}
			args := append([]types.JavaType(nil), formal.TypeArgs...)
			for i := 0; i < inputs && i < len(args); i++ {
				bound, ok := args[i].(*types.JavaWildcardType)
				if !ok || bound == nil || bound.Variant != "super" {
					continue
				}
				parameterized, ok := types.AsParameterizedType(bound.Bound)
				if !ok || actual.TypeArgs[i] == nil {
					continue
				}
				erased, ok := actual.TypeArgs[i].RawType().(*types.JavaClass)
				if ok && erased != nil && sameRawTypeName(erased.Name, parameterized.RawClassName) {
					args[i] = actual.TypeArgs[i].Copy()
				}
			}
			target = types.NewParameterizedType(formal.RawClassName, args)
		}
	}
	visit(value)
	return target
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
			// Every non-poly definition contributes a constraint. A raw
			// Iterator may contain Entry<E>, so joining it with Iterator<E>
			// cannot make all reads produce E. Preserve erasure; functional
			// poly definitions recover a target separately from their uses.
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
