package types

import "os"

// crossclass_lub.go adds a CROSS-CLASS (jar-internal) least-upper-bound primitive for declaration
// widening. The static JDK table in hierarchy.go only knows JDK families; a merge of two jar-internal
// types that share a jar-internal supertype (e.g. fastjson2 `Any` and `JSONSchema` where
// `Any extends JSONSchema`) is invisible to it, so MergeTypes/CommonSuperType fall back to the first
// arm and a declaration minted from it is too narrow:
//
//	Any var6 = cond ? Any.INSTANCE /*Any*/ : Any.NOT_ANY /*JSONSchema*/;  // javac: bad conditional
//	if (var6 instanceof UnresolvedReference) ...                          // javac: inconvertible types
//
// Here we resolve the DIRECT-subtype case via a raw super_class/Interfaces provider supplied by the
// dumper (ClassContext.SiblingSuperTypes): if one arm is a (transitive) subtype of the other, the LUB
// is that supertype arm. The common-ancestor case (neither arm a subtype of the other but sharing a
// jar-internal ancestor) is a documented residual handled elsewhere. Gated upstream by
// JDEC_TERNARY_DECL_LUB_CROSS_OFF.

// SuperTypeProvider returns the DIRECT raw supertype internal names (slash-form) of a jar-internal
// class: its super_class internal name followed by its direct interface internal names. ok=false for
// JDK/external classes whose bytes are not in the jar. Identical shape to
// class_context.ClassContext.SiblingSuperTypes so the field is assignable without an import cycle.
type SuperTypeProvider func(internalName string) (supers []string, ok bool)

// crossClassSubtypeWalkCap bounds the supertype BFS so a pathological/cyclic hierarchy can never spin.
const crossClassSubtypeWalkCap = 4096

// IsSubtypeVia reports whether dot-FQN `sub` is a (reflexive) subtype of dot-FQN `sup` by walking sub's
// raw supertype chain through the provider. Returns false the moment the chain leaves the jar without
// reaching sup (provider miss) -- conservative: an unknown relationship is treated as "not a subtype",
// so callers never widen on a guess.
func IsSubtypeVia(sub, sup string, provider SuperTypeProvider) bool {
	if provider == nil || sub == "" || sup == "" {
		return false
	}
	subI, supI := dotToInternal(sub), dotToInternal(sup)
	if subI == supI {
		return true
	}
	visited := map[string]bool{}
	queue := []string{subI}
	for len(queue) > 0 && len(visited) < crossClassSubtypeWalkCap {
		cur := queue[0]
		queue = queue[1:]
		if cur == "" || visited[cur] {
			continue
		}
		visited[cur] = true
		supers, ok := provider(cur)
		if !ok {
			continue // left the jar (JDK/external): cannot prove subtyping, stop this branch
		}
		for _, s := range supers {
			if s == "" {
				continue
			}
			if s == supI {
				return true
			}
			if !visited[s] {
				queue = append(queue, s)
			}
		}
	}
	return false
}

// ClassFQNOf returns the non-array class FQN (dot-form) of a JavaType, or ("",false) for
// primitives/arrays/non-class types. Exported wrapper of classNameOf for cross-package callers.
func ClassFQNOf(t JavaType) (string, bool) {
	return classNameOf(t)
}

// IsReferenceSubtypeBridged reports whether dot-FQN `sub` is a (reflexive) subtype of dot-FQN `sup`,
// bridging the jar's own raw supertype chain (via the provider) INTO the JDK table the moment the chain
// reaches a JDK class. Unlike IsSubtypeVia (jar-only, stops at the first JDK hop), this reaches JDK
// ancestors such as java.util.Map for a jar-internal class whose superclass is a JDK collection
// (`AbstractEnvironment$1 extends ReadOnlySystemAttributesMap extends AbstractMap`), and for pure JDK
// types (`java.util.Properties` -> Hashtable -> Map). Returns false when the ancestry cannot be proven.
func IsReferenceSubtypeBridged(sub, sup string, provider SuperTypeProvider) bool {
	if sub == "" || sup == "" {
		return false
	}
	if sub == sup {
		return true
	}
	_, ok := bridgedAncestorDepths(sub, provider)[sup]
	return ok
}

// CrossClassDirectLUB returns the supertype arm when one of a,b is a (transitive) subtype of the other
// via the provider (a<:b -> b, b<:a -> a), else nil. Only the direct-subtype case is handled; the LUB is
// never java.lang.Object (a widening to Object would defeat any later member access and is left to the
// caller's fallback). Gated by JDEC_TERNARY_DECL_LUB_CROSS_OFF.
func CrossClassDirectLUB(a, b JavaType, provider SuperTypeProvider) JavaType {
	if os.Getenv("JDEC_TERNARY_DECL_LUB_CROSS_OFF") != "" || provider == nil {
		return nil
	}
	an, aok := classNameOf(a)
	bn, bok := classNameOf(b)
	if !aok || !bok || an == bn {
		return nil
	}
	if an == "java.lang.Object" || bn == "java.lang.Object" {
		return nil
	}
	if IsSubtypeVia(an, bn, provider) {
		return b
	}
	if IsSubtypeVia(bn, an, provider) {
		return a
	}
	return nil
}

// jarSuperClosure returns every (reflexive, transitive) jar-internal supertype internal-name of `startI`
// reachable through the provider, bounded by crossClassSubtypeWalkCap. Chains that leave the jar
// (provider miss) simply stop; JDK ancestors are therefore NOT included (they are not in the jar bytes),
// which is fine because a jar-internal sibling LUB must itself be a jar-internal class.
func jarSuperClosure(startI string, provider SuperTypeProvider) map[string]bool {
	out := map[string]bool{}
	if provider == nil || startI == "" {
		return out
	}
	queue := []string{startI}
	for len(queue) > 0 && len(out) < crossClassSubtypeWalkCap {
		cur := queue[0]
		queue = queue[1:]
		if cur == "" || out[cur] {
			continue
		}
		out[cur] = true
		supers, ok := provider(cur)
		if !ok {
			continue
		}
		for _, s := range supers {
			if s != "" && !out[s] {
				queue = append(queue, s)
			}
		}
	}
	return out
}

// bridgedAncestorDepths returns the BFS distance from dot-FQN `start` to each of its (reflexive)
// ancestors, walking JAR supertypes via the provider AND crossing into the JDK table (jdkSuperEdges)
// the moment the chain reaches a JDK class. This is what CrossClassDirectLUB/CommonSuperType cannot do
// individually: a jar type such as `com.alibaba.fastjson2.JSONObject extends java.util.LinkedHashMap`
// has its Map-ness hidden behind a JDK superclass the jar provider does not carry, and the JDK table
// does not know the jar leaf. Bridging both yields JSONObject's true ancestor set (LinkedHashMap ->
// HashMap -> AbstractMap/Map -> Object). java.lang.Object is always injected as a universal ancestor so
// any two reference types share at least it. External classes the provider misses and the JDK table does
// not know terminate their branch (recorded, not expanded) but still gain Object.
func bridgedAncestorDepths(start string, provider SuperTypeProvider) map[string]int {
	depth := map[string]int{}
	if start == "" {
		return depth
	}
	depth[start] = 0
	depth["java.lang.Object"] = 1 << 20 // universal fallback; real depth overwrites if reached closer
	queue := []string{start}
	for len(queue) > 0 && len(depth) < crossClassSubtypeWalkCap {
		cur := queue[0]
		queue = queue[1:]
		d := depth[cur]
		var supers []string
		if edges, ok := jdkSuperEdges[cur]; ok {
			supers = edges // JDK class: authoritative JDK edges
		} else if provider != nil {
			if raw, ok := provider(dotToInternal(cur)); ok {
				supers = make([]string, 0, len(raw))
				for _, s := range raw {
					if s != "" {
						supers = append(supers, internalToDot(s))
					}
				}
			}
		}
		for _, sup := range supers {
			if sup == "" {
				continue
			}
			if old, seen := depth[sup]; !seen || d+1 < old {
				depth[sup] = d + 1
				queue = append(queue, sup)
			}
		}
	}
	return depth
}

// BridgedCommonSuperType returns the NEAREST common ancestor (least upper bound) of jar/JDK reference
// types a and b, bridging jar supertypes (provider) into the JDK table so a jar type's JDK ancestry is
// visible. Ties prefer a concrete class over an interface. Returns java.lang.Object (never nil for two
// valid reference types) when nothing closer is shared. Callers gate their own behaviour on whether the
// result is Object. Gated by JDEC_TERNARY_DECL_LUB_CROSS_OFF (shared cross-class kill-switch).
func BridgedCommonSuperType(a, b JavaType, provider SuperTypeProvider) JavaType {
	if os.Getenv("JDEC_TERNARY_DECL_LUB_CROSS_OFF") != "" {
		return nil
	}
	an, aok := classNameOf(a)
	bn, bok := classNameOf(b)
	if !aok || !bok {
		return nil
	}
	if an == bn {
		return a
	}
	da := bridgedAncestorDepths(an, provider)
	db := bridgedAncestorDepths(bn, provider)
	best := ""
	bestScore := 1 << 30
	bestIsClass := false
	for anc, d1 := range da {
		d2, ok := db[anc]
		if !ok {
			continue
		}
		score := d1 + d2
		isClass := !jdkInterfaceSet[anc]
		better := false
		switch {
		case best == "":
			better = true
		case score < bestScore:
			better = true
		case score == bestScore:
			if isClass != bestIsClass {
				better = isClass
			} else {
				better = anc < best
			}
		}
		if better {
			best, bestScore, bestIsClass = anc, score, isClass
		}
	}
	if best == "" {
		return nil
	}
	return NewJavaClass(best)
}

// CrossClassCommonSuperType returns the NEAREST-to-b jar-internal common ancestor of a and b when
// NEITHER is a subtype of the other (the SIBLING case CrossClassDirectLUB leaves as a residual, e.g.
// jsoup `TextNode` and `DataNode` both extending `LeafNode`). It BFS-walks b's raw supertype chain and
// returns the first ancestor also present in a's supertype closure, excluding java.lang.Object (never a
// useful widening target). Returns nil when the arms share no jar-internal ancestor, when either is
// unknown, or when one subtypes the other (that case belongs to CrossClassDirectLUB). Gated by
// JDEC_TERNARY_DECL_LUB_CROSS_OFF.
func CrossClassCommonSuperType(a, b JavaType, provider SuperTypeProvider) JavaType {
	if os.Getenv("JDEC_TERNARY_DECL_LUB_CROSS_OFF") != "" || provider == nil {
		return nil
	}
	an, aok := classNameOf(a)
	bn, bok := classNameOf(b)
	if !aok || !bok || an == bn {
		return nil
	}
	if an == "java.lang.Object" || bn == "java.lang.Object" {
		return nil
	}
	// A subtype relation is CrossClassDirectLUB's job; here we handle only true siblings.
	if IsSubtypeVia(an, bn, provider) || IsSubtypeVia(bn, an, provider) {
		return nil
	}
	aClosure := jarSuperClosure(dotToInternal(an), provider)
	// BFS b's ancestors level-by-level so the FIRST common hit is the nearest common ancestor to b.
	startI := dotToInternal(bn)
	visited := map[string]bool{}
	queue := []string{startI}
	for len(queue) > 0 && len(visited) < crossClassSubtypeWalkCap {
		cur := queue[0]
		queue = queue[1:]
		if cur == "" || visited[cur] {
			continue
		}
		visited[cur] = true
		if cur != startI && cur != "java/lang/Object" && aClosure[cur] {
			return NewJavaClass(internalToDot(cur))
		}
		supers, ok := provider(cur)
		if !ok {
			continue
		}
		for _, s := range supers {
			if s != "" && !visited[s] {
				queue = append(queue, s)
			}
		}
	}
	return nil
}
