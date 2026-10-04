package types

import (
	"sort"
	"strings"
)

const (
	// NullTypeName is a denotable marker for the null reference, not a JVM class.
	NullTypeName = "<null>"
	// UnknownTypeName is an explicit unresolved hierarchy result.
	UnknownTypeName = "<unknown>"
)

// LubResult separates the source-denotable type from the verifier/internal bound.
// Intersection types such as AccessibleObject & Member are recorded as constraints
// and are not emitted as a declaration.
type LubResult struct {
	Source              JavaType
	VerifierBound       JavaType
	InternalConstraints []string
	Unknown             bool
	Reason              string
}

func NullType() JavaType    { return NewJavaClass(NullTypeName) }
func UnknownType() JavaType { return NewJavaClass(UnknownTypeName) }

func isNullType(t JavaType) bool {
	if t == nil {
		return false
	}
	n, ok := classNameOf(t)
	return ok && n == NullTypeName
}

func IsUnknownType(t JavaType) bool {
	if t == nil {
		return false
	}
	n, ok := classNameOf(t)
	return ok && n == UnknownTypeName
}

func isJavaLangObject(t JavaType) bool {
	n, ok := classNameOf(t)
	return ok && n == "java.lang.Object" && !t.IsArray()
}

// reflectInternalAncestors records verifier-visible ancestors that are omitted
// from the source LUB table so Member stays the denotable declaration type.
var reflectInternalAncestors = map[string][]string{
	"java.lang.reflect.Method":             {"java.lang.reflect.AccessibleObject", "java.lang.reflect.Member"},
	"java.lang.reflect.Field":              {"java.lang.reflect.AccessibleObject", "java.lang.reflect.Member"},
	"java.lang.reflect.Constructor":        {"java.lang.reflect.AccessibleObject", "java.lang.reflect.Member"},
	"java.lang.reflect.Executable":         {"java.lang.reflect.AccessibleObject", "java.lang.reflect.Member"},
	"java.lang.reflect.AccessibleObject":   {"java.lang.Object"},
	"java.lang.reflect.Member":             {"java.lang.Object"},
	"java.lang.reflect.GenericDeclaration": {"java.lang.reflect.AnnotatedElement"},
}

func arrayShape(t JavaType) (base JavaType, dim int, ok bool) {
	if t == nil || !t.IsArray() {
		return nil, 0, false
	}
	at, ok := t.RawType().(*JavaArrayType)
	if !ok || at == nil {
		return nil, 0, false
	}
	return at.JavaType, at.Dimension, true
}

func isPrimitiveType(t JavaType) bool {
	if t == nil {
		return false
	}
	_, ok := t.RawType().(*JavaPrimer)
	return ok
}

func arrayOf(base JavaType, dim int) JavaType {
	if base == nil || dim <= 0 {
		return base
	}
	out := base
	for i := 0; i < dim; i++ {
		out = NewJavaArrayType(out)
	}
	return out
}

// joinArrayTypes returns ok=true when every non-null arm is an array. Primitive
// arrays are invariant; reference arrays are covariant only at matching rank.
func joinArrayTypes(arms []JavaType, provider SuperTypeProvider) (JavaType, bool, bool) {
	var arrays []JavaType
	sawArray := false
	sawNonArray := false
	for _, t := range arms {
		if t == nil || isNullType(t) {
			continue
		}
		if t.IsArray() {
			sawArray = true
			arrays = append(arrays, t)
			continue
		}
		sawNonArray = true
	}
	if !sawArray {
		return nil, false, false
	}
	if sawNonArray {
		return NewJavaClass("java.lang.Object"), true, false
	}
	if len(arrays) == 0 {
		return NullType(), true, false
	}
	joined := arrays[0]
	for i := 1; i < len(arrays); i++ {
		j, unknown := arrayLUB2(joined, arrays[i], provider)
		if unknown {
			return UnknownType(), true, true
		}
		joined = j
	}
	return joined, true, false
}

func arrayLUB2(a, b JavaType, provider SuperTypeProvider) (JavaType, bool) {
	ab, ad, aok := arrayShape(a)
	bb, bd, bok := arrayShape(b)
	if !aok || !bok {
		return NewJavaClass("java.lang.Object"), false
	}
	if ad != bd {
		return NewJavaClass("java.lang.Object"), false
	}
	aPrim, bPrim := isPrimitiveType(ab), isPrimitiveType(bb)
	if aPrim || bPrim {
		if aPrim && bPrim && primerName(ab) == primerName(bb) {
			return a, false
		}
		// int[] is not covariant with Object[] or Integer[].
		return NewJavaClass("java.lang.Object"), false
	}
	elem, unknown := joinReference(ab, bb, provider)
	if unknown {
		return UnknownType(), true
	}
	if elem == nil {
		return NewJavaClass("java.lang.Object"), false
	}
	return arrayOf(elem, ad), false
}

func primerName(t JavaType) string {
	if t == nil {
		return ""
	}
	p, ok := t.RawType().(*JavaPrimer)
	if !ok || p == nil {
		return ""
	}
	return p.Name
}

func joinReference(a, b JavaType, provider SuperTypeProvider) (JavaType, bool) {
	if isNullType(a) {
		return b, false
	}
	if isNullType(b) {
		return a, false
	}
	if IsUnknownType(a) || IsUnknownType(b) {
		return UnknownType(), true
	}
	if a.IsArray() || b.IsArray() {
		got, ok, unknown := joinArrayTypes([]JavaType{a, b}, provider)
		if unknown {
			return UnknownType(), true
		}
		if ok {
			return got, false
		}
	}
	an, aok := RawClassFQN(a)
	bn, bok := RawClassFQN(b)
	if !aok || !bok {
		return nil, false
	}
	if an == bn {
		return a, false
	}
	if name := commonSuperName(an, bn); name != "" && name != "java.lang.Object" {
		return NewJavaClass(name), false
	}
	if provider != nil {
		if unknownHierarchyName(an, provider) || unknownHierarchyName(bn, provider) {
			return UnknownType(), true
		}
		got := BridgedCommonSuperType(a, b, provider)
		if got == nil {
			return UnknownType(), true
		}
		if isJavaLangObject(got) {
			if HasKnownDirectSupertypes(an, provider) && HasKnownDirectSupertypes(bn, provider) {
				return got, false
			}
			return UnknownType(), true
		}
		return got, false
	}
	if _, inA := jdkSuperEdges[an]; !inA && an != "java.lang.Object" {
		return UnknownType(), true
	}
	if _, inB := jdkSuperEdges[bn]; !inB && bn != "java.lang.Object" {
		return UnknownType(), true
	}
	return NewJavaClass("java.lang.Object"), false
}

func unknownHierarchyName(name string, provider SuperTypeProvider) bool {
	if name == "" || name == "java.lang.Object" || name == NullTypeName {
		return false
	}
	if _, ok := jdkSuperEdges[name]; ok {
		return false
	}
	if provider == nil {
		return true
	}
	_, ok := provider(dotToInternal(name))
	return !ok
}

// ArrayLUB is the exported array join used by T16 array contracts.
func ArrayLUB(a, b JavaType, provider SuperTypeProvider) LubResult {
	got, ok, unknown := joinArrayTypes([]JavaType{a, b}, provider)
	if unknown {
		return LubResult{Source: UnknownType(), VerifierBound: UnknownType(), Unknown: true, Reason: "unresolved array join"}
	}
	if !ok {
		src, u := joinReference(a, b, provider)
		if u {
			return LubResult{Source: UnknownType(), VerifierBound: UnknownType(), Unknown: true, Reason: "unresolved join"}
		}
		return LubResult{Source: src, VerifierBound: src}
	}
	return LubResult{Source: got, VerifierBound: got}
}

// IsArraySubtype reports JVM array subtyping: primitive arrays are invariant;
// reference arrays are covariant only at the same dimension.
func IsArraySubtype(sub, sup JavaType, provider SuperTypeProvider) bool {
	if sub == nil || sup == nil {
		return false
	}
	if isNullType(sub) {
		return sup.IsArray() || classRef(sup)
	}
	sb, sd, sok := arrayShape(sub)
	ub, ud, uok := arrayShape(sup)
	if !sok || !uok {
		n, ok := classNameOf(sup)
		return sok && ok && (n == "java.lang.Object" || n == "java.lang.Cloneable" || n == "java.io.Serializable")
	}
	if sd != ud {
		return false
	}
	if isPrimitiveType(sb) || isPrimitiveType(ub) {
		return isPrimitiveType(sb) && isPrimitiveType(ub) && primerName(sb) == primerName(ub)
	}
	return isReferenceSubtype(sb, ub, provider)
}

func classRef(t JavaType) bool {
	_, ok := classNameOf(t)
	return ok && !t.IsArray()
}

func isReferenceSubtype(sub, sup JavaType, provider SuperTypeProvider) bool {
	if sub == nil || sup == nil {
		return false
	}
	if sub.IsArray() || sup.IsArray() {
		return IsArraySubtype(sub, sup, provider)
	}
	sn, sok := RawClassFQN(sub)
	un, uok := RawClassFQN(sup)
	if !sok || !uok {
		return false
	}
	if sn == un {
		return true
	}
	if un == "java.lang.Object" {
		return true
	}
	if IsReferenceSubtypeBridged(sn, un, provider) {
		return true
	}
	da := ancestorDepths(sn)
	if da != nil {
		_, ok := da[un]
		return ok
	}
	return false
}

// JoinTypes is the production-facing LUB: JDK table, then provider, then
// explicit unknown. It never substitutes a first-arm concrete class for unknown.
func JoinTypes(a, b JavaType, provider SuperTypeProvider) LubResult {
	return JoinTypesAccessible(a, b, provider, nil)
}

// AccessibilityProvider reports whether a binary name can be named in source.
type AccessibilityProvider func(internalName string) (accessible, known bool)

// JoinTypesAccessible splits an inaccessible implementation out of the source
// declaration while keeping it in the internal constraint set.
func JoinTypesAccessible(a, b JavaType, provider SuperTypeProvider, access AccessibilityProvider) LubResult {
	if a == nil {
		return resultOf(b, provider, access)
	}
	if b == nil {
		return resultOf(a, provider, access)
	}
	if isNullType(a) {
		return resultOf(b, provider, access)
	}
	if isNullType(b) {
		return resultOf(a, provider, access)
	}
	src, unknown := joinReference(a, b, provider)
	if unknown {
		return LubResult{Source: UnknownType(), VerifierBound: UnknownType(), Unknown: true, Reason: "resolver miss, cycle, or cancel"}
	}
	if src == nil {
		return LubResult{Source: UnknownType(), VerifierBound: UnknownType(), Unknown: true, Reason: "no common type"}
	}
	res := LubResult{Source: src, VerifierBound: src}
	res.InternalConstraints = internalConstraintsOf(a, b, src, provider)
	if access != nil {
		res.Source = denotableSource(src, a, b, access)
	}
	return res
}

func resultOf(t JavaType, provider SuperTypeProvider, access AccessibilityProvider) LubResult {
	if t == nil {
		return LubResult{Source: UnknownType(), Unknown: true, Reason: "nil type"}
	}
	res := LubResult{Source: t, VerifierBound: t, InternalConstraints: internalConstraintsOf(t, t, t, provider)}
	if access != nil {
		res.Source = denotableSource(t, t, t, access)
	}
	return res
}

func denotableSource(src, a, b JavaType, access AccessibilityProvider) JavaType {
	if src == nil || access == nil {
		return src
	}
	n, ok := RawClassFQN(src)
	if !ok {
		return src
	}
	acc, known := access(dotToInternal(n))
	if known && !acc {
		// Inaccessible join result: prefer Member/Object-like denotable bound.
		if an, aok := RawClassFQN(a); aok {
			if accA, knownA := access(dotToInternal(an)); knownA && accA {
				return a
			}
		}
		if bn, bok := RawClassFQN(b); bok {
			if accB, knownB := access(dotToInternal(bn)); knownB && accB {
				return b
			}
		}
		if n != "java.lang.reflect.Member" {
			return NewJavaClass("java.lang.Object")
		}
	}
	return src
}

func internalConstraintsOf(a, b, src JavaType, provider SuperTypeProvider) []string {
	left := constraintSet(a, provider)
	right := constraintSet(b, provider)
	out := map[string]bool{}
	for n := range left {
		if right[n] {
			out[n] = true
		}
	}
	if n, ok := RawClassFQN(src); ok {
		out[n] = true
	}
	names := make([]string, 0, len(out))
	for n := range out {
		if n != "" && n != "java.lang.Object" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func constraintSet(t JavaType, provider SuperTypeProvider) map[string]bool {
	out := map[string]bool{}
	n, ok := RawClassFQN(t)
	if !ok {
		return out
	}
	out[n] = true
	if ancs, ok := reflectInternalAncestors[n]; ok {
		for _, a := range ancs {
			out[a] = true
		}
	}
	if da := ancestorDepths(n); da != nil {
		for a := range da {
			out[a] = true
		}
	}
	if provider != nil {
		for a := range bridgedAncestorDepths(n, provider) {
			out[a] = true
		}
	}
	return out
}

// MergeTypesVia is MergeTypes with a SuperTypeProvider. Unknown does not become
// the first concrete arm.
func MergeTypesVia(provider SuperTypeProvider, ts ...JavaType) JavaType {
	nonNil := make([]JavaType, 0, len(ts))
	for _, t := range ts {
		if t != nil {
			nonNil = append(nonNil, t)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	if len(nonNil) == 1 {
		return nonNil[0]
	}
	joined := nonNil[0]
	for i := 1; i < len(nonNil); i++ {
		res := JoinTypes(joined, nonNil[i], provider)
		if res.Unknown {
			return UnknownType()
		}
		joined = res.Source
	}
	return joined
}

func (r LubResult) ConstraintLabel() string {
	return strings.Join(r.InternalConstraints, " & ")
}
