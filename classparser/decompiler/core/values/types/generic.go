package types

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// JavaParameterizedType represents a parameterized (generic) class type, e.g.
// BiFunction<Integer, Integer, Integer>. It wraps a raw class name and carries
// concrete type arguments recovered from the Signature attribute.
type JavaParameterizedType struct {
	RawClassName string
	TypeArgs     []JavaType
}

func NewParameterizedType(rawClassName string, typeArgs []JavaType) JavaType {
	return newJavaTypeWrap(&JavaParameterizedType{
		RawClassName: rawClassName,
		TypeArgs:     typeArgs,
	})
}

func (j *JavaParameterizedType) String(funcCtx *class_context.ClassContext) string {
	base := funcCtx.ShortTypeName(j.RawClassName)
	if len(j.TypeArgs) == 0 {
		return base
	}
	// Raw-erase: a flattened non-static inner class that has its OWN formal type parameters cannot also
	// declare the enclosing class's variables (that would break the arity of its `<ownParam>` reference
	// sites), so a parameterization whose DIRECT argument is one of those undeclared enclosing variables
	// is rendered RAW (`Node<K,V>` -> `Node`) -- legal, runtime-identical, and matching the local already
	// emitted raw. Only the THIS-level args are checked: a nested `List<Node<K,V>>` keeps its outer `List`
	// and raw-erases just the inner `Node` (Java permits a raw type as a type argument). The set is empty
	// for every ordinary class, so this is a strict no-op there. See ClassContext.RawEraseTypeVars.
	if funcCtx.HasRawEraseTypeVars() {
		for _, ta := range j.TypeArgs {
			if argRefsRawEraseVar(ta, funcCtx) {
				return base
			}
		}
	}
	parts := make([]string, len(j.TypeArgs))
	for i, ta := range j.TypeArgs {
		parts[i] = ta.String(funcCtx)
	}
	return fmt.Sprintf("%s<%s>", base, strings.Join(parts, ", "))
}

// argRefsRawEraseVar reports whether a parameterized type's direct argument ta is (or, for a bounded
// wildcard, has a bound that is) a bare type-variable name marked for raw-erasure (see
// ClassContext.RawEraseTypeVars). Wildcards are asserted before RawType to avoid the nil-embed panic
// documented on isWildcardType.
func argRefsRawEraseVar(ta JavaType, funcCtx *class_context.ClassContext) bool {
	if ta == nil {
		return false
	}
	if w, ok := ta.(*JavaWildcardType); ok {
		return w.Bound != nil && argRefsRawEraseVar(w.Bound, funcCtx)
	}
	raw := ta.RawType()
	if raw == nil {
		return false
	}
	if w, ok := raw.(*JavaWildcardType); ok {
		return w.Bound != nil && argRefsRawEraseVar(w.Bound, funcCtx)
	}
	if jc, ok := raw.(*JavaClass); ok {
		return funcCtx.RawEraseTypeVar(jc.Name)
	}
	return false
}

func (j *JavaParameterizedType) IsJavaType() {}

var _ javaType = &JavaParameterizedType{}

// AsParameterizedType unwraps t to its *JavaParameterizedType, or (nil,false) if t is not a
// parameterized (generic) class type.
func AsParameterizedType(t JavaType) (*JavaParameterizedType, bool) {
	if t == nil || t.IsArray() {
		return nil, false
	}
	pt, ok := t.RawType().(*JavaParameterizedType)
	return pt, ok
}

// jdkIterableFamily are the JDK single-type-parameter Iterable<E> subtypes whose iterator() returns
// Iterator<E> (the element type threaded unchanged). Concrete impls are included because their slot
// declarations also carry the parameterized type.
var jdkIterableFamily = map[string]bool{
	"java.lang.Iterable":           true,
	"java.util.Collection":         true,
	"java.util.List":               true,
	"java.util.Set":                true,
	"java.util.SortedSet":          true,
	"java.util.NavigableSet":       true,
	"java.util.Queue":              true,
	"java.util.Deque":              true,
	"java.util.ArrayList":          true,
	"java.util.LinkedList":         true,
	"java.util.Vector":             true,
	"java.util.Stack":              true,
	"java.util.HashSet":            true,
	"java.util.LinkedHashSet":      true,
	"java.util.TreeSet":            true,
	"java.util.ArrayDeque":         true,
	"java.util.PriorityQueue":      true,
	"java.util.AbstractList":       true,
	"java.util.AbstractSet":        true,
	"java.util.AbstractCollection": true,
}

// jdkIteratorFamily are the JDK Iterator<E> types whose next() returns E.
var jdkIteratorFamily = map[string]bool{
	"java.util.Iterator":     true,
	"java.util.ListIterator": true,
}

// jdkListFamily are the JDK List<E> types that declare the index-addressed 2-arg mutators
// set(int, E) / add(int, E). Restricted to List (and its concrete impls): Set/Queue/Deque never
// declare a 2-arg set/add, so this gate keeps the element-type-arg resolution provably List-scoped.
var jdkListFamily = map[string]bool{
	"java.util.List":                            true,
	"java.util.AbstractList":                    true,
	"java.util.AbstractSequentialList":          true,
	"java.util.ArrayList":                       true,
	"java.util.LinkedList":                      true,
	"java.util.Vector":                          true,
	"java.util.Stack":                           true,
	"java.util.concurrent.CopyOnWriteArrayList": true,
}

// jdkDequeFamily are the JDK Deque<E> types that declare the head/tail single-element insertion methods
// addFirst/addLast/offerFirst/offerLast/push(E) -- each taking the receiver's E (type arg 0). Queue's
// add/offer(E) are handled by the jdkIterableFamily gate.
var jdkDequeFamily = map[string]bool{
	"java.util.Deque":                            true,
	"java.util.ArrayDeque":                       true,
	"java.util.LinkedList":                       true,
	"java.util.concurrent.ConcurrentLinkedDeque": true,
	"java.util.concurrent.LinkedBlockingDeque":   true,
}

// isWildcardType reports whether t is a wildcard type argument (`?`, `? extends X`, `? super X`).
// Instantiation skips wildcard receivers: capture-of semantics make the substituted return type
// (e.g. Iterator<? extends X>.next() yields a capture, not a nameable type) unsafe to render.
//
// MUST assert the wildcard DIRECTLY before touching RawType: JavaWildcardType embeds a (nil) JavaType
// interface and does not define RawType itself, so the promoted RawType() dereferences that nil embed
// and panics. A wildcard type arg is stored either bare (*JavaWildcardType, e.g. Recognizer<?,?>) or
// wrapped; check the bare form first, then unwrap the wrapped form via RawType (safe for non-wildcards).
func isWildcardType(t JavaType) bool {
	if t == nil {
		return false
	}
	if _, ok := t.(*JavaWildcardType); ok {
		return true
	}
	raw := t.RawType()
	if raw == nil {
		return false
	}
	_, ok := raw.(*JavaWildcardType)
	return ok
}

// IsJDKIterableFamily reports whether name is a JDK single-type-parameter Iterable<E> subtype (the
// exported view of jdkIterableFamily), used by the wildcard-receiver raw-cast heuristic to restrict the
// Collection<E>.add/offer(E) case to genuine JDK collections.
func IsJDKIterableFamily(name string) bool {
	return jdkIterableFamily[name]
}

// IsWildcardType is the exported form of isWildcardType: it reports whether t is a wildcard type
// argument (`?`, `? extends X`, `? super X`) using the panic-safe assertion order documented on
// isWildcardType. Callers outside this package (e.g. the comparator raw-cast heuristics) use it to
// detect a capture-inducing wildcard receiver.
func IsWildcardType(t JavaType) bool {
	return isWildcardType(t)
}

// lowerBoundedWildcard unwraps t to its *JavaWildcardType iff t is a lower-bounded `? super X` wildcard
// (bare or RawType-wrapped) with a non-nil bound, returning (nil,false) otherwise. A `? super X` is a
// CONSUMER position: a value flowing into a parameter that maps to it is source-cast to X (the lower
// bound), so X is the denotable cast target the resolver re-emits. Unbounded `?` and upper-bounded
// `? extends X` capture to an unnameable CAP# with no such target. The bare form is asserted before
// RawType to avoid the nil-embed panic documented on isWildcardType.
func lowerBoundedWildcard(t JavaType) (*JavaWildcardType, bool) {
	if t == nil {
		return nil, false
	}
	if w, ok := t.(*JavaWildcardType); ok {
		if w.Variant == "super" && w.Bound != nil {
			return w, true
		}
		return nil, false
	}
	if raw := t.RawType(); raw != nil {
		if w, ok := raw.(*JavaWildcardType); ok && w.Variant == "super" && w.Bound != nil {
			return w, true
		}
	}
	return nil, false
}

// InstantiateJDKMethodReturn returns the generic return type for a small, provably-correct set of JDK
// container methods, instantiated with the receiver's actual type arguments. It is the conservative
// core of Phase 3 generic inference (Bug AH): the JDK signatures are stable API, so substituting the
// receiver's type args is sound, e.g. Iterable<T>.iterator() -> Iterator<T>, Iterator<T>.next() -> T.
// Returns nil (caller keeps the erased descriptor return) for anything not in the table, for raw
// (no-type-arg) receivers, or for wildcard type args. Gated by the caller's JDEC_GENERIC_INFER_OFF.
func InstantiateJDKMethodReturn(rawClass, method string, argc int, typeArgs []JavaType) JavaType {
	if len(typeArgs) == 0 {
		return nil
	}
	for _, ta := range typeArgs {
		if isWildcardType(ta) {
			return nil
		}
	}
	switch {
	case method == "iterator" && argc == 0 && len(typeArgs) == 1 && jdkIterableFamily[rawClass]:
		return NewParameterizedType("java.util.Iterator", []JavaType{typeArgs[0]})
	case method == "next" && argc == 0 && len(typeArgs) == 1 && jdkIteratorFamily[rawClass]:
		return typeArgs[0]
	}
	return nil
}

// jdkMapFamily are the JDK Map<K,V> types whose put/putIfAbsent/replace(K,V) parameters are the
// receiver's type args (param0=K, param1=V). Concrete impls included: their slot/field declarations
// carry the parameterized type too.
var jdkMapFamily = map[string]bool{
	"java.util.Map":                          true,
	"java.util.AbstractMap":                  true,
	"java.util.SortedMap":                    true,
	"java.util.NavigableMap":                 true,
	"java.util.HashMap":                      true,
	"java.util.LinkedHashMap":                true,
	"java.util.TreeMap":                      true,
	"java.util.IdentityHashMap":              true,
	"java.util.WeakHashMap":                  true,
	"java.util.Hashtable":                    true,
	"java.util.concurrent.ConcurrentMap":     true,
	"java.util.concurrent.ConcurrentHashMap": true,
}

// jdkSortedMapFamily are the JDK sorted/navigable Map<K,V> types that additionally declare KEY-typed
// navigation methods (headMap/tailMap/subMap(K...), floorKey/ceilingKey/higherKey/lowerKey(K),
// floor/ceiling/higher/lowerEntry(K)). Their key parameters are the receiver's K (type arg 0); the
// value-returning `get`/`remove`/`containsKey` take Object BY DESIGN and are deliberately excluded.
var jdkSortedMapFamily = map[string]bool{
	"java.util.SortedMap":                         true,
	"java.util.NavigableMap":                      true,
	"java.util.TreeMap":                           true,
	"java.util.concurrent.ConcurrentNavigableMap": true,
	"java.util.concurrent.ConcurrentSkipListMap":  true,
}

// jdkMethodParamTypeArgIndex returns, for a small provably-correct set of JDK generic methods, the
// receiver type-argument index that the parameter at paramIndex resolves to, or -1 when that
// parameter is NOT a receiver type variable (a fixed Object/int position such as Map.get(Object),
// or a method outside the set). The JDK signatures are stable API, so substituting the receiver's
// type args is sound -- this is the parameter analogue of InstantiateJDKMethodReturn. ntype is the
// number of receiver type args, used to disambiguate arities.
func jdkMethodParamTypeArgIndex(rawClass, method string, argc, paramIndex, ntype int) int {
	switch rawClass {
	case "java.util.function.Consumer":
		if method == "accept" && argc == 1 && ntype == 1 && paramIndex == 0 {
			return 0
		}
	case "java.util.function.BiConsumer":
		if method == "accept" && argc == 2 && ntype == 2 && (paramIndex == 0 || paramIndex == 1) {
			return paramIndex
		}
	case "java.util.function.Function":
		// Function<T,R>.apply(T): only the leading type arg is a parameter (R is the return).
		if method == "apply" && argc == 1 && ntype == 2 && paramIndex == 0 {
			return 0
		}
	case "java.util.function.BiFunction":
		// BiFunction<T,U,R>.apply(T,U): the trailing type arg is the return.
		if method == "apply" && argc == 2 && ntype == 3 && (paramIndex == 0 || paramIndex == 1) {
			return paramIndex
		}
	case "java.util.function.UnaryOperator":
		// UnaryOperator<T> extends Function<T,T>: a single type arg.
		if method == "apply" && argc == 1 && ntype == 1 && paramIndex == 0 {
			return 0
		}
	case "java.util.function.BinaryOperator":
		// BinaryOperator<T> extends BiFunction<T,T,T>: both params are the single type arg T.
		if method == "apply" && argc == 2 && ntype == 1 && (paramIndex == 0 || paramIndex == 1) {
			return 0
		}
	case "java.util.function.Predicate":
		if method == "test" && argc == 1 && ntype == 1 && paramIndex == 0 {
			return 0
		}
	case "java.util.function.BiPredicate":
		if method == "test" && argc == 2 && ntype == 2 && (paramIndex == 0 || paramIndex == 1) {
			return paramIndex
		}
	case "java.util.Comparator":
		// Comparator<T>.compare(T, T): a single type arg shared by BOTH parameters. The descriptor
		// erases both to Object, so an Object-typed argument (e.g. a raw `Iterator.next()` value) is
		// passed without the source's `(T)` cast and javac -- re-resolving against compare(T,T) --
		// rejects it ("Object cannot be converted to T"; guava Comparators.isInOrder/isInStrictOrder,
		// TopKSelector). Both params resolve to the same single type arg.
		if method == "compare" && argc == 2 && ntype == 1 && (paramIndex == 0 || paramIndex == 1) {
			return 0
		}
	}
	// Map<K,V> mutators whose params are exactly (K, V).
	if jdkMapFamily[rawClass] && ntype == 2 && argc == 2 && (paramIndex == 0 || paramIndex == 1) {
		switch method {
		case "put", "putIfAbsent", "replace":
			return paramIndex
		}
	}
	// SortedMap/NavigableMap<K,V> navigation methods whose KEY parameter positions are the receiver's K
	// (type arg 0). The descriptor erases K to Object, so an Object-typed key argument is passed without
	// the source's `(K)` cast and javac -- re-resolving against e.g. headMap(K) -- rejects it ("Object
	// cannot be converted to K"; guava Maps$FilteredEntrySortedMap.lastKey `sortedMap().headMap(objKey)`).
	// Only KEY positions resolve to K; NavigableMap's boolean inclusivity flags and the value-typed
	// get/remove/containsKey(Object) are left as fixed (fall through to -1). Kill-switch
	// JDEC_SORTED_MAP_KEY_PARAM_OFF.
	if jdkSortedMapFamily[rawClass] && ntype == 2 && os.Getenv("JDEC_SORTED_MAP_KEY_PARAM_OFF") == "" {
		switch method {
		case "headMap", "tailMap":
			// SortedMap.headMap(K) [argc 1]; NavigableMap.headMap(K, boolean) [argc 2, only param0=K].
			if paramIndex == 0 && (argc == 1 || argc == 2) {
				return 0
			}
		case "subMap":
			// SortedMap.subMap(K, K) [argc 2: both K]; NavigableMap.subMap(K, boolean, K, boolean)
			// [argc 4: params 0 and 2 = K].
			if argc == 2 && (paramIndex == 0 || paramIndex == 1) {
				return 0
			}
			if argc == 4 && (paramIndex == 0 || paramIndex == 2) {
				return 0
			}
		case "floorKey", "ceilingKey", "higherKey", "lowerKey",
			"floorEntry", "ceilingEntry", "higherEntry", "lowerEntry":
			// NavigableMap key-lookup methods: single K argument.
			if argc == 1 && paramIndex == 0 {
				return 0
			}
		}
	}
	// Collection<E>.add/offer(E): a single type-arg element parameter.
	if (method == "add" || method == "offer") && argc == 1 && ntype == 1 && paramIndex == 0 && jdkIterableFamily[rawClass] {
		return 0
	}
	// Deque<E>.addFirst/addLast/offerFirst/offerLast/push(E): a single type-arg element parameter (the
	// descriptor erases E to its bound, so an Object-typed value flows in without the source's `(E)` cast;
	// guava Iterators$ConcatenatedIterator `this.metaIterators.addFirst(rawDeque.removeLast())`).
	if (method == "addFirst" || method == "addLast" || method == "offerFirst" || method == "offerLast" || method == "push") &&
		argc == 1 && ntype == 1 && paramIndex == 0 && jdkDequeFamily[rawClass] && os.Getenv("JDEC_DEQUE_PARAM_OFF") == "" {
		return 0
	}
	// List<E>.set(int, E) / add(int, E): the SECOND parameter is the element type arg (the first is the
	// int index). The descriptor erases the element to Object, so an Object-typed value (e.g. a
	// `List<T>.get(i)` result the decompiler declared as Object) is passed without the source's `(E)`
	// cast and javac -- re-resolving against set(int, E) -- rejects it ("Object cannot be converted to
	// T"; guava Iterables.removeIfFromRandomAccessList `var0.set(var3, var4)`, var0=`List<T>`). Gated to
	// the List sub-family: only List declares 2-arg set/add(int, E); Set/Queue/Deque never do, so a
	// same-named 2-arg call on them cannot exist in verified bytecode, but the family gate keeps it
	// provably scoped.
	if (method == "set" || method == "add") && argc == 2 && ntype == 1 && paramIndex == 1 && jdkListFamily[rawClass] && os.Getenv("JDEC_LIST_SET_PARAM_OFF") == "" {
		return 0
	}
	// AtomicReference<V>: the V-typed value-parameter methods whose descriptor erases V to Object. The
	// canonical case is `compareAndSet(V, V)` / `weakCompareAndSet(V, V)` (argc 2, both params V) and
	// `getAndSet(V)` / `set(V)` / `lazySet(V)` (argc 1, param 0 V); the accumulator pair
	// `getAndAccumulate(V, BinaryOperator<V>)` / `accumulateAndGet(V, BinaryOperator<V>)` carry V only at
	// param 0 (param 1 is the operator). An Object-typed argument (a `reference.get()` read typed Object)
	// flows in without the source's `(V)` cast and javac -- re-resolving against the generic signature --
	// rejects it ("Object cannot be converted to T"; commons-lang3 AtomicInitializer<T>
	// `this.reference.compareAndSet(null, var1)` where reference is `AtomicReference<T>`). V is the sole
	// type arg, so all qualifying params map to 0. Restricted to ntype==1 (a bare or
	// non-wildcard-parameterized AtomicReference<V>); the existing wildcard gate in
	// InstantiateJDKMethodParam already returns nil for `AtomicReference<?>`. Kill-switch
	// JDEC_ATOMIC_REF_PARAM_OFF.
	if rawClass == "java.util.concurrent.atomic.AtomicReference" && ntype == 1 &&
		os.Getenv("JDEC_ATOMIC_REF_PARAM_OFF") == "" {
		switch method {
		case "compareAndSet", "weakCompareAndSet", "weakCompareAndSetPlain":
			if argc == 2 && (paramIndex == 0 || paramIndex == 1) {
				return 0
			}
		case "getAndSet", "set", "lazySet":
			if argc == 1 && paramIndex == 0 {
				return 0
			}
		case "getAndAccumulate", "accumulateAndGet":
			// (V, BinaryOperator<V>): only the leading V param is the receiver's type arg.
			if argc == 2 && paramIndex == 0 {
				return 0
			}
		}
	}
	return -1
}

// InstantiateJDKMethodParam returns the generic type of the parameter at paramIndex for a small,
// provably-correct set of JDK generic methods, instantiated with the receiver's actual type args.
// Bytecode erases such parameters to their bound (e.g. BiConsumer<T,V>.accept(T,V) erases to
// accept(Object,Object)), so an argument whose static type is a concrete value (BigDecimal, or the
// erased Object) is passed without the cast the source carried, and javac -- re-resolving the call
// against the generic signature -- rejects it ("BigDecimal/Object cannot be converted to V"). Feeding
// the instantiated parameter type back lets the existing ArgumentStrings cast logic re-emit the
// original `(V)` / `(T)` cast (unchecked but behavior-preserving, matching CFR/Fernflower). Returns
// nil (caller keeps the erased descriptor param) for raw receivers, wildcard type args, or anything
// outside the table.
func InstantiateJDKMethodParam(rawClass, method string, argc, paramIndex int, typeArgs []JavaType) JavaType {
	if len(typeArgs) == 0 {
		return nil
	}
	for _, ta := range typeArgs {
		if isWildcardType(ta) {
			return nil
		}
	}
	idx := jdkMethodParamTypeArgIndex(rawClass, method, argc, paramIndex, len(typeArgs))
	if idx < 0 || idx >= len(typeArgs) {
		return nil
	}
	return typeArgs[idx]
}

// ParseSignature parses a JVM Signature attribute string and returns the
// parameterized JavaType. Returns nil if parsing fails.
func ParseSignature(sig string) JavaType {
	t, _, ok := parseSigType(sig)
	if !ok {
		return nil
	}
	return t
}

func parseSigType(sig string) (JavaType, string, bool) {
	if len(sig) == 0 {
		return nil, "", false
	}
	switch sig[0] {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z', 'V':
		return NewJavaPrimer(primerForSig(sig[0])), sig[1:], true
	case 'L':
		return parseSigClassType(sig)
	case 'T':
		end := strings.IndexByte(sig, ';')
		if end < 0 {
			return nil, "", false
		}
		return newJavaTypeWrap(&JavaClass{Name: sig[1:end]}), sig[end+1:], true
	case '[':
		elem, rest, ok := parseSigType(sig[1:])
		if !ok {
			return nil, "", false
		}
		return NewJavaArrayType(elem), rest, true
	default:
		return nil, "", false
	}
}

func parseSigClassType(sig string) (JavaType, string, bool) {
	rest := sig[1:]
	hasTypeArgs := false
	lt := strings.IndexByte(rest, '<')
	sc := strings.IndexByte(rest, ';')
	nameEnd := len(rest)
	if lt >= 0 && (sc < 0 || lt < sc) {
		nameEnd = lt
		hasTypeArgs = true
	} else if sc >= 0 {
		nameEnd = sc
	} else {
		return nil, "", false
	}
	rawName := SlashToDot(rest[:nameEnd])
	rest = rest[nameEnd:]
	var typeArgs []JavaType
	if hasTypeArgs {
		rest = rest[1:]
		for len(rest) > 0 && rest[0] != '>' {
			// Wildcard type arguments: '*' = "?", '+' = "? extends X", '-' = "? super X".
			// '=' is a CaptureMarker used by javac for capture-of; treat as a plain wildcard.
			if rest[0] == '*' || rest[0] == '=' {
				typeArgs = append(typeArgs, &JavaWildcardType{})
				rest = rest[1:]
				continue
			}
			if rest[0] == '+' || rest[0] == '-' {
				variant := "extends"
				if rest[0] == '-' {
					variant = "super"
				}
				rest = rest[1:]
				ta, remaining, ok := parseSigType(rest)
				if !ok {
					return nil, "", false
				}
				typeArgs = append(typeArgs, &JavaWildcardType{Variant: variant, Bound: ta})
				rest = remaining
				continue
			}
			ta, remaining, ok := parseSigType(rest)
			if !ok {
				return nil, "", false
			}
			typeArgs = append(typeArgs, ta)
			rest = remaining
		}
		if len(rest) == 0 || rest[0] != '>' {
			return nil, "", false
		}
		rest = rest[1:]
	}
	for len(rest) > 0 && rest[0] == '.' {
		innerEnd := 1
		for innerEnd < len(rest) && rest[innerEnd] != ';' && rest[innerEnd] != '<' && rest[innerEnd] != '.' {
			innerEnd++
		}
		rawName += "$" + rest[1:innerEnd]
		rest = rest[innerEnd:]
		if len(rest) > 0 && rest[0] == '<' {
			rest = rest[1:]
			var innerArgs []JavaType
			for len(rest) > 0 && rest[0] != '>' {
				if rest[0] == '*' || rest[0] == '=' {
					innerArgs = append(innerArgs, &JavaWildcardType{})
					rest = rest[1:]
					continue
				}
				if rest[0] == '+' || rest[0] == '-' {
					variant := "extends"
					if rest[0] == '-' {
						variant = "super"
					}
					rest = rest[1:]
					ta, remaining, ok := parseSigType(rest)
					if !ok {
						return nil, "", false
					}
					innerArgs = append(innerArgs, &JavaWildcardType{Variant: variant, Bound: ta})
					rest = remaining
					continue
				}
				ta, remaining, ok := parseSigType(rest)
				if !ok {
					return nil, "", false
				}
				innerArgs = append(innerArgs, ta)
				rest = remaining
			}
			if len(rest) > 0 && rest[0] == '>' {
				rest = rest[1:]
			}
			typeArgs = innerArgs
		}
	}
	if len(rest) == 0 || rest[0] != ';' {
		return nil, "", false
	}
	rest = rest[1:]
	if len(typeArgs) > 0 {
		return newJavaTypeWrap(&JavaParameterizedType{
			RawClassName: rawName,
			TypeArgs:     typeArgs,
		}), rest, true
	}
	return newJavaTypeWrap(&JavaClass{Name: rawName}), rest, true
}

func primerForSig(c byte) string {
	switch c {
	case 'B':
		return JavaByte
	case 'C':
		return JavaChar
	case 'D':
		return JavaDouble
	case 'F':
		return JavaFloat
	case 'I':
		return JavaInteger
	case 'J':
		return JavaLong
	case 'S':
		return JavaShort
	case 'Z':
		return JavaBoolean
	case 'V':
		return JavaVoid
	}
	return JavaInteger
}

func ParseMethodSignature(sig string) ([]JavaType, JavaType) {
	if len(sig) == 0 || sig[0] != '(' {
		return nil, nil
	}
	rest := sig[1:]
	var params []JavaType
	for len(rest) > 0 && rest[0] != ')' {
		t, remaining, ok := parseSigType(rest)
		if !ok {
			return nil, nil
		}
		params = append(params, t)
		rest = remaining
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, nil
	}
	rest = rest[1:]
	retType, _, ok := parseSigType(rest)
	if !ok {
		return nil, nil
	}
	return params, retType
}

// ParseClassSignature extracts the type parameters declaration from a class
// signature, e.g. from "<T:Ljava/lang/Object;>Ljava/lang/Object;" returns
// "<T>". Also handles bounds like "<T::Ljava/lang/Comparable<TT;>;>" -> "<T extends Comparable<T>>".
// Returns "" if the class has no type parameters or parsing fails.
//
// funcCtx (when non-nil) is the REAL render context of the class being emitted; the type-variable
// BOUND types (`A extends Annotation` -> java.lang.annotation.Annotation) are rendered against it so
// their imports are REGISTERED. Rendering a bound against a throwaway `&ClassContext{}` (the historical
// behaviour, preserved for a nil funcCtx) yields the correct SHORT spelling but registers the import on
// the discarded context, so a bound in a non-java.lang package (`java.lang.annotation.Annotation`,
// `java.lang.reflect.Type`) recompiles as "cannot find symbol" (spring MergedAnnotationSelector /
// MergedAnnotationPredicates$FirstRunOfPredicate class headers). Passing the real context fixes the
// missing import while keeping the identical short spelling.
func ParseClassSignature(sig string, funcCtx *class_context.ClassContext) string {
	if len(sig) == 0 || sig[0] != '<' {
		return ""
	}
	boundCtx := funcCtx
	if boundCtx == nil {
		boundCtx = &class_context.ClassContext{}
	}
	rest := sig[1:]
	var params []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return ""
		}
		typeParamName := rest[:colonIdx]
		rest = rest[colonIdx:]
		var bounds []string
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:] // skip ':'
			// After skipping ':', if the next char is ':' or '>', the class bound is empty
			// (e.g. "<T::Lcomparable;>" means T has no class bound, only an interface bound).
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return ""
			}
			rest = remaining
			bounds = append(bounds, boundType.String(boundCtx))
		}
		if len(bounds) > 0 {
			params = append(params, fmt.Sprintf("%s extends %s", typeParamName, strings.Join(bounds, " & ")))
		} else {
			params = append(params, typeParamName)
		}
	}
	if len(rest) == 0 || rest[0] != '>' {
		return ""
	}
	return "<" + strings.Join(params, ", ") + ">"
}

// ParseClassSignatureSupers parses a class Signature attribute and returns the (possibly generic)
// superclass type followed by the (possibly generic) directly-implemented interface types. The raw
// super_class and Interfaces constant-pool entries are erased; this recovers the type arguments so a
// generic supertype renders as `extends Converter<Integer, Integer>` / `implements Comparator<int[]>`
// instead of the raw form (which fails to override the erased generic methods). Returns (nil, nil) on
// any parse failure so the caller can fall back to the erased names.
func ParseClassSignatureSupers(sig string) (JavaType, []JavaType) {
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil, nil
		}
		rest = r
	}
	sup, rest, ok := parseSigType(rest)
	if !ok {
		return nil, nil
	}
	var ifaces []JavaType
	for len(rest) > 0 {
		it, remaining, ok := parseSigType(rest)
		if !ok {
			return sup, ifaces
		}
		ifaces = append(ifaces, it)
		rest = remaining
	}
	return sup, ifaces
}

// skipAngleSection consumes a leading '<' ... matching '>' run (honoring nested angle brackets) and
// returns the remainder after the matching '>'. Used to skip a class signature's formal type
// parameter section, whose ':' bound syntax parseSigType does not handle.
func skipAngleSection(sig string) (string, bool) {
	if len(sig) == 0 || sig[0] != '<' {
		return sig, false
	}
	depth := 0
	for i := 0; i < len(sig); i++ {
		switch sig[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return sig[i+1:], true
			}
		}
	}
	return "", false
}

// ClassFormalTypeParamNames returns the bare names declared in a leading formal type parameter
// section of a class signature, e.g. "<K:Ljava/lang/Object;V:Ljava/lang/Object;>L..." -> ["K","V"].
// Returns nil when the signature has no leading "<...>" section or cannot be parsed.
func ClassFormalTypeParamNames(sig string) []string {
	if len(sig) == 0 || sig[0] != '<' {
		return nil
	}
	rest := sig[1:]
	var names []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return names
		}
		names = append(names, rest[:colonIdx])
		rest = rest[colonIdx:]
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			_, remaining, ok := parseSigType(rest)
			if !ok {
				return names
			}
			rest = remaining
		}
	}
	return names
}

// TypeParamBound is the recovered bound of a single class formal type parameter: the rendered Java
// bound clause (without the parameter name, e.g. "Comparable<?>" or "Foo & Bar"; empty when the only
// bound is Object) together with the type-variable names the bound references (used as a safety gate
// when reconstructing a flattened inner class's enclosing type variables: a bound may only be re-emitted
// when every variable it references is itself in scope).
type TypeParamBound struct {
	Clause string
	Refs   []string
}

// ClassFormalTypeParamBounds parses a class signature's leading formal type-parameter section and maps
// each type-variable NAME to its recovered bound (see TypeParamBound). It is the bound-carrying analogue
// of ClassFormalTypeParamNames: a flattened non-static inner class loses its enclosing type variables'
// DECLARATIONS, so injecting them as bare `<C>` drops the bound and a `Range<C>` use (where Range needs
// `C extends Comparable`) fails javac with "type argument C is not within bounds of type-variable C".
// Recovering the enclosing class's `<C extends Comparable<?>>` clause fixes it. A sole Object bound
// produces no entry (the canonical bare `<C>`). Bounds render with funcCtx so other-package bound
// classes register an import. Returns nil for a signature without a leading "<...>" section.
func ClassFormalTypeParamBounds(sig string, funcCtx *class_context.ClassContext) map[string]TypeParamBound {
	if len(sig) == 0 || sig[0] != '<' {
		return nil
	}
	if funcCtx == nil {
		funcCtx = &class_context.ClassContext{}
	}
	out := map[string]TypeParamBound{}
	rest := sig[1:]
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return out
		}
		name := rest[:colonIdx]
		rest = rest[colonIdx:]
		var bounds []string
		var refs []string
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			before := rest
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return out
			}
			// Record the type variables referenced inside this bound's signature slice so the caller can
			// gate re-emission on all of them being in scope.
			consumed := before[:len(before)-len(remaining)]
			scanTypeVarRefs(consumed, &refs)
			rest = remaining
			rendered := boundType.String(funcCtx)
			if rendered == "Object" || rendered == "java.lang.Object" {
				continue
			}
			bounds = append(bounds, rendered)
		}
		if len(bounds) > 0 {
			out[name] = TypeParamBound{Clause: strings.Join(bounds, " & "), Refs: refs}
		}
	}
	return out
}

// ClassFormalTypeParamErasures parses a class signature's leading formal type-parameter section and maps
// each type-variable NAME to the ERASURE of its FIRST class/interface bound: the raw, generic-stripped
// DOTTED class name (e.g. `<E::Lcom/foo/InternalEntry<TK;>;>` -> {"E":"com.foo.InternalEntry"}). A
// parameter whose only bound is Object -- or whose first bound is itself a type variable or an array --
// produces NO entry, so the caller defaults it to java.lang.Object. This is the standalone-position
// analogue of ClassFormalTypeParamBounds (which renders the source bound CLAUSE for a re-declared inner
// type parameter); here only the erasure head is needed, to render a flattened inner class's undeclarable
// enclosing type variable used as a STANDALONE type (`E nextEntry;`). Returns nil for a signature without
// a leading "<...>" section.
func ClassFormalTypeParamErasures(sig string) map[string]string {
	if len(sig) == 0 || sig[0] != '<' {
		return nil
	}
	out := map[string]string{}
	rest := sig[1:]
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return out
		}
		name := rest[:colonIdx]
		rest = rest[colonIdx:]
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			// An empty class-bound slot (`E::LIface;` has no class bound before the interface bound):
			// skip it and keep scanning for the first concrete bound.
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			before := rest
			_, remaining, ok := parseSigType(rest)
			if !ok {
				return out
			}
			if _, done := out[name]; !done {
				if cls, ok := erasureClassName(before[:len(before)-len(remaining)]); ok {
					out[name] = cls
				}
			}
			rest = remaining
		}
	}
	return out
}

// erasureClassName returns the DOTTED raw class name at the head of a single field-type signature slice
// (`Lpkg/Name<args>;` -> "pkg.Name"), or ok=false when the slice is not a plain class type (a type
// variable `T..;`, primitive, or array), whose erasure the caller defaults to java.lang.Object.
func erasureClassName(sig string) (string, bool) {
	if len(sig) == 0 || sig[0] != 'L' {
		return "", false
	}
	end := len(sig)
	for i := 1; i < len(sig); i++ {
		if sig[i] == '<' || sig[i] == ';' {
			end = i
			break
		}
	}
	if end <= 1 {
		return "", false
	}
	return strings.ReplaceAll(sig[1:end], "/", "."), true
}

// scanTypeVarRefs consumes exactly one type signature at sig and appends every TypeVariableSignature
// name (the `T<name>;` form, including those nested inside type arguments and array element types) to
// *out. It mirrors parseSigType's grammar so a `T` is only treated as a type-variable tag when it
// actually starts a type (never when it merely appears inside a class internal name like
// Lcom/example/TestClass;). Returns the remaining string and whether parsing succeeded.
func scanTypeVarRefs(sig string, out *[]string) (string, bool) {
	if len(sig) == 0 {
		return "", false
	}
	switch sig[0] {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z', 'V':
		return sig[1:], true
	case 'T':
		end := strings.IndexByte(sig, ';')
		if end < 0 {
			return "", false
		}
		*out = append(*out, sig[1:end])
		return sig[end+1:], true
	case '[':
		return scanTypeVarRefs(sig[1:], out)
	case 'L':
		return scanClassTypeVarRefs(sig, out)
	default:
		return "", false
	}
}

// scanClassTypeVarRefs consumes one ClassTypeSignature (Lpkg/Name<args>.Inner<args>;) and records the
// type-variable names found in its (possibly nested) type arguments. It does not record the class name
// itself, which is never a type variable.
func scanClassTypeVarRefs(sig string, out *[]string) (string, bool) {
	rest := sig[1:]
	lt := strings.IndexByte(rest, '<')
	sc := strings.IndexByte(rest, ';')
	if lt >= 0 && (sc < 0 || lt < sc) {
		rest = rest[lt:]
	} else if sc >= 0 {
		rest = rest[sc:]
	} else {
		return "", false
	}
	scanArgs := func(r string) (string, bool) {
		r = r[1:] // skip '<'
		for len(r) > 0 && r[0] != '>' {
			switch r[0] {
			case '*':
				r = r[1:]
				continue
			case '+', '-', '=':
				r = r[1:]
				if len(r) > 0 && (r[0] == '>' || r[0] == '*') {
					continue
				}
			}
			remaining, ok := scanTypeVarRefs(r, out)
			if !ok {
				return "", false
			}
			r = remaining
		}
		if len(r) == 0 || r[0] != '>' {
			return "", false
		}
		return r[1:], true
	}
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := scanArgs(rest)
		if !ok {
			return "", false
		}
		rest = r
	}
	for len(rest) > 0 && rest[0] == '.' {
		innerEnd := 1
		for innerEnd < len(rest) && rest[innerEnd] != ';' && rest[innerEnd] != '<' && rest[innerEnd] != '.' {
			innerEnd++
		}
		rest = rest[innerEnd:]
		if len(rest) > 0 && rest[0] == '<' {
			r, ok := scanArgs(rest)
			if !ok {
				return "", false
			}
			rest = r
		}
	}
	if len(rest) == 0 || rest[0] != ';' {
		return "", false
	}
	return rest[1:], true
}

// TypeVarRefsInFieldSig returns the type-variable names referenced in a single field type signature,
// in first-seen order (with duplicates preserved; the caller dedups). E.g. "Ljava/util/List<TV;>;"
// -> ["V"]; "TK;" -> ["K"].
func TypeVarRefsInFieldSig(sig string) []string {
	var out []string
	if _, ok := scanTypeVarRefs(sig, &out); !ok {
		return nil
	}
	return out
}

// TypeVarRefsInMethodSig returns the type-variable names referenced in a METHOD signature's parameter
// types, return type and throws clause, EXCLUDING the method's own leading formal type parameters
// (`<E:...>`). E.g. "(LRef<TK;TV;>;)TT;" -> ["K","V","T"]; "<E:Ljava/lang/Object;>(TE;)V" -> [] (E is
// the method's own formal, filtered out). Used to recover enclosing type variables a flattened inner
// class references ONLY in its method signatures (not fields/supertype), e.g. spring-core
// ConcurrentReferenceHashMap$Task<T>.execute(...Reference<K,V>...), so they can be raw-erased at render.
func TypeVarRefsInMethodSig(sig string) []string {
	rest := sig
	own := map[string]bool{}
	if len(rest) > 0 && rest[0] == '<' {
		// Collect the method's own formal type-parameter names so we can exclude them, then skip the
		// whole `<...>` formal section.
		for _, n := range ClassFormalTypeParamNames(sig) {
			own[n] = true
		}
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil
		}
		rest = r
	}
	if len(rest) == 0 || rest[0] != '(' {
		return nil
	}
	rest = rest[1:] // skip '('
	var raw []string
	// Parameter types until ')'.
	for len(rest) > 0 && rest[0] != ')' {
		remaining, ok := scanTypeVarRefs(rest, &raw)
		if !ok {
			return dedupExcluding(raw, own)
		}
		rest = remaining
	}
	if len(rest) > 0 && rest[0] == ')' {
		rest = rest[1:] // skip ')'
	}
	// Return type, then any throws (each introduced by '^').
	for len(rest) > 0 {
		if rest[0] == '^' {
			rest = rest[1:]
			continue
		}
		remaining, ok := scanTypeVarRefs(rest, &raw)
		if !ok {
			break
		}
		rest = remaining
	}
	return dedupExcluding(raw, own)
}

// TypeVarRefsInMethodParams is TypeVarRefsInMethodSig restricted to the PARAMETER types (it ignores
// the return type and throws clause), excluding the method's own leading formal type parameters. It is
// used to raw-erase enclosing type variables that a flattened inner class references only in its method
// PARAMETER positions, which must match the (also-erased) overridden/parent method's parameter erasure
// (e.g. the ConcurrentReferenceHashMap$Task subclass family $1..$5 overriding execute(Reference<K,V>...)).
func TypeVarRefsInMethodParams(sig string) []string {
	rest := sig
	own := map[string]bool{}
	if len(rest) > 0 && rest[0] == '<' {
		for _, n := range ClassFormalTypeParamNames(sig) {
			own[n] = true
		}
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil
		}
		rest = r
	}
	if len(rest) == 0 || rest[0] != '(' {
		return nil
	}
	rest = rest[1:] // skip '('
	var raw []string
	for len(rest) > 0 && rest[0] != ')' {
		remaining, ok := scanTypeVarRefs(rest, &raw)
		if !ok {
			break
		}
		rest = remaining
	}
	return dedupExcluding(raw, own)
}

// dedupExcluding returns the first-seen-ordered unique names of raw that are not in exclude.
func dedupExcluding(raw []string, exclude map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range raw {
		if n == "" || exclude[n] || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// FreeTypeVarRefsInClassSig returns the type-variable names referenced in the SUPERTYPE portion of a
// class signature (the superclass + interfaces that follow the formal type parameter section), in
// first-seen order. Subtracting ClassFormalTypeParamNames from these yields the "free" variables a
// nested/inner/anonymous class inherits from an enclosing scope.
func FreeTypeVarRefsInClassSig(sig string) []string {
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil
		}
		rest = r
	}
	var out []string
	for len(rest) > 0 {
		remaining, ok := scanTypeVarRefs(rest, &out)
		if !ok {
			break
		}
		rest = remaining
	}
	return out
}

// ParseMethodSignatureTypeParams extracts formal type parameters from a method
// signature, e.g. "<E:Ljava/lang/Object;>(LList<TE;>;)TE;" returns "<E>".
// Returns "" if the method has no type parameters or parsing fails.
func ParseMethodSignatureTypeParams(sig string) string {
	return parseFormalTypeParams(sig, &class_context.ClassContext{})
}

// ParseMethodSignatureTypeParamsCtx is ParseMethodSignatureTypeParams but renders bound types with
// the given render context so that bound classes in other packages register an import and resolve at
// recompile time (the empty-context form only ever produces the bare name).
func ParseMethodSignatureTypeParamsCtx(sig string, funcCtx *class_context.ClassContext) string {
	return parseFormalTypeParams(sig, funcCtx)
}

// parseFormalTypeParams renders a leading formal type-parameter section ("<T:...>") to Java source
// ("<T>", "<T extends Comparable<T>>", "<K, V>"). A sole `extends Object`/`extends java.lang.Object`
// bound is dropped because it is always redundant (every type variable implicitly extends Object) and
// the bare `<T>` form matches what javac and mature decompilers emit. Returns "" when there is no
// leading section or parsing fails.
func parseFormalTypeParams(sig string, funcCtx *class_context.ClassContext) string {
	if len(sig) == 0 || sig[0] != '<' {
		return ""
	}
	if funcCtx == nil {
		funcCtx = &class_context.ClassContext{}
	}
	rest := sig[1:]
	var params []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return ""
		}
		typeParamName := rest[:colonIdx]
		rest = rest[colonIdx:]
		var bounds []string
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return ""
			}
			rest = remaining
			rendered := boundType.String(funcCtx)
			if rendered == "Object" || rendered == "java.lang.Object" {
				// A `<T extends Object>` bound is always redundant; drop it for the canonical `<T>`.
				continue
			}
			bounds = append(bounds, rendered)
		}
		if len(bounds) > 0 {
			params = append(params, fmt.Sprintf("%s extends %s", typeParamName, strings.Join(bounds, " & ")))
		} else {
			params = append(params, typeParamName)
		}
	}
	if len(rest) == 0 || rest[0] != '>' {
		return ""
	}
	return "<" + strings.Join(params, ", ") + ">"
}

// ParseMethodSignatureFull parses a method Signature attribute that MAY begin with a formal
// type-parameter section ("<T:...>"), which ParseMethodSignature rejects outright. It returns the
// rendered type-parameter string (e.g. "<T>", "<K, V>"; "" when there are none), the parameter types
// and the return type - the latter two with type variables preserved (TT; -> JavaClass{Name:"T"}).
// Returns ("", nil, nil) on any parse failure. For a signature WITHOUT a leading "<...>" section it is
// exactly ParseMethodSignature, so non-generic methods are unaffected. Bound types in the type-param
// string are rendered with funcCtx so other-package bounds register an import.
func ParseMethodSignatureFull(sig string, funcCtx *class_context.ClassContext) (string, []JavaType, JavaType) {
	typeParams := ""
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		typeParams = parseFormalTypeParams(rest, funcCtx)
		r, ok := skipAngleSection(rest)
		if !ok {
			return "", nil, nil
		}
		rest = r
	}
	params, ret := ParseMethodSignature(rest)
	if ret == nil {
		return "", nil, nil
	}
	return typeParams, params, ret
}

// FormalTypeParamBounds parses the leading formal type-parameter section of a class or method Signature
// ("<C::Ljava/util/Collection<-TE;>;M::L.../Multimap<TK;TV;>;>...") and returns, for each type variable,
// the FIRST bound that is a PARAMETERIZED type (has type arguments). Type variables whose bounds are all
// bare classes / Object are omitted (a bare bound carries no receiver type args to recover). Used to
// recover a receiver whose static type is a bare type variable (`C var1`) but whose bound is a
// parameterized container (`Collection<? super E>`), so downstream receiver/param resolution can see the
// element type (guava FluentIterable.copyInto, Multimaps.invertFrom). Returns nil when there is no
// leading section, on any parse failure, or when no type variable has a parameterized bound.
func FormalTypeParamBounds(sig string) map[string]JavaType {
	if len(sig) == 0 || sig[0] != '<' {
		return nil
	}
	rest := sig[1:]
	out := map[string]JavaType{}
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return nil
		}
		name := rest[:colonIdx]
		rest = rest[colonIdx:]
		var firstParam JavaType
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			// An empty class-bound (`::`, or `:` immediately before `>`) means the class bound is Object;
			// skip it and keep reading the interface bounds.
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return nil
			}
			rest = remaining
			// Keep parsing every bound (to advance `rest` correctly) but capture only the FIRST that is
			// parameterized -- that is the container whose type args we can substitute at the call site.
			if firstParam == nil {
				if _, isPt := AsParameterizedType(boundType); isPt {
					firstParam = boundType
				}
			}
		}
		if firstParam != nil {
			out[name] = firstParam
		}
	}
	if len(rest) == 0 || rest[0] != '>' {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MethodFormalTypeParamNames returns the bare names declared in a method signature's leading formal
// type-parameter section, e.g. "<T:Ljava/lang/Object;>(TT;)TT;" -> ["T"]. Returns nil when there is
// no leading "<...>" section. The grammar of a method's formal type-parameter section is identical to
// a class's, so this delegates to ClassFormalTypeParamNames.
func MethodFormalTypeParamNames(sig string) []string {
	return ClassFormalTypeParamNames(sig)
}

// SubstituteTypeVars returns a copy of t with every type-variable reference replaced per the
// substitution sigma (e.g. {K: String, V: Integer}). It is the generic-substitution primitive that
// lets a callee's parsed signature -- written in terms of the DECLARING class's type variables -- be
// instantiated at a call site with the receiver's actual type arguments (the root-cause analogue of
// the hard-coded JDK table). A type variable is modeled as a bare-named *JavaClass (parseSigType emits
// `TK;` as JavaClass{Name:"K"}); a name absent from sigma is left untouched (so concrete class names,
// which never appear as sigma keys, pass through unchanged). Parameterized type args and array element
// types are rewritten recursively. Wildcards are returned as-is (the resolver skips wildcard receivers
// upstream). Pure and allocation-light; nil-safe.
func SubstituteTypeVars(t JavaType, sigma map[string]JavaType) JavaType {
	if t == nil || len(sigma) == 0 {
		return t
	}
	// A bare wildcard does not implement RawType safely (see isWildcardType); leave it untouched.
	if _, ok := t.(*JavaWildcardType); ok {
		return t
	}
	raw := t.RawType()
	switch rt := raw.(type) {
	case *JavaClass:
		if repl, ok := sigma[rt.Name]; ok && repl != nil {
			return repl
		}
		return t
	case *JavaParameterizedType:
		if len(rt.TypeArgs) == 0 {
			return t
		}
		newArgs := make([]JavaType, len(rt.TypeArgs))
		changed := false
		for i, ta := range rt.TypeArgs {
			sub := SubstituteTypeVars(ta, sigma)
			newArgs[i] = sub
			if sub != ta {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return NewParameterizedType(rt.RawClassName, newArgs)
	case *JavaArrayType:
		elem := rt.JavaType
		sub := SubstituteTypeVars(elem, sigma)
		if sub == elem {
			return t
		}
		return newJavaTypeWrap(&JavaArrayType{JavaType: sub, Dimension: rt.Dimension})
	default:
		return t
	}
}

// ClassSigProvider yields a jar-internal class's raw generic signature info by binary internal name
// (slash-separated, e.g. "com/google/common/collect/Multiset"). It returns ok=false for JDK/external
// classes whose bytes are not in the jar (those are covered by the InstantiateJDKMethodParam table).
// The dumper builds the concrete closure (it owns the byte resolver + class parser); the resolver walk
// below consumes only strings, so the lower type/class_context packages never import the parser.
type ClassSigProvider func(internalName string) (classSig string, methodSigs map[string]string, ok bool)

func dotToInternal(name string) string {
	return strings.ReplaceAll(name, ".", "/")
}

func internalToDot(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

// ResolveInstantiatedParamType recovers the generic type of a callee method's paramIndex-th formal
// parameter at a call site, by walking the receiver class's generic supertype hierarchy and composing
// the type-argument substitution along each `extends/implements Super<...>` edge. It is the unified
// cross-class generic resolver (the root-cause replacement for the same-class / identity-one-level
// special cases): given a receiver static type `recvRaw<recvArgs...>` it finds the most-derived
// declaration of (method, argc) that carries a generic Signature, applies the composed substitution to
// its paramIndex-th formal, and returns the instantiated type -- which the caller's existing argument
// cast logic then re-emits as the source's erased `(K)`/`(Foo)` cast.
//
// SAFETY: returns nil (caller keeps the erased descriptor parameter) unless the instantiated formal is
// DENOTABLE and CASTABLE at the call site -- i.e. a CLASS-scope type variable of the current class
// (funcCtx.IsTypeParam) or a concrete (dotted) class name. A method-scope `<T>` formal, a leftover
// FOREIGN bare type variable (un-substituted because the receiver was raw, or a supertype formal not
// in scope), a wildcard/capture, or a parameterized/array result all yield nil -- emitting `(T)` for a
// non-in-scope variable would not compile, and the erasure blocker bucket is scalar casts only. JDK /
// external supertypes (provider miss) yield nil and are left to the JDK table. provider==nil disables
// the walk. Gated upstream by JDEC_GENERIC_RESOLVE_OFF.
func ResolveInstantiatedParamType(funcCtx *class_context.ClassContext, provider ClassSigProvider, recvRaw string, recvArgs []JavaType, method string, argc, paramIndex int) JavaType {
	if funcCtx == nil || provider == nil || recvRaw == "" || method == "" || paramIndex < 0 {
		return nil
	}
	// A lower-bounded `? super X` receiver type arg is a CONSUMER position: a callee param that maps to it
	// accepts an X, so the erased `Object` argument the source cast to X (the wildcard's lower bound) can be
	// re-cast (`(E)`). It is therefore allowed through here and resolved to its bound by substituteAndGateParam.
	// An unbounded `?` or upper-bounded `? extends X` arg captures to an unnameable CAP# with NO denotable
	// cast target, so those still bail. Kill-switch JDEC_GENERIC_SUPERWILDCARD_OFF restores the blanket bail.
	superWildcardOff := os.Getenv("JDEC_GENERIC_SUPERWILDCARD_OFF") != ""
	for _, a := range recvArgs {
		if !isWildcardType(a) {
			continue
		}
		if _, ok := lowerBoundedWildcard(a); ok && !superWildcardOff {
			continue
		}
		return nil
	}
	visited := map[string]bool{}
	return resolveParamWalk(funcCtx, provider, dotToInternal(recvRaw), recvArgs, method, argc, paramIndex, visited)
}

// resolveParamWalk performs the depth-first hierarchy walk for ResolveInstantiatedParamType. sigma maps
// the CURRENT node's formal type-parameter names to their actual arguments (in terms of the original
// call site's denotable types). It is rebuilt for each supertype edge by substituting the supertype's
// type arguments through the current sigma.
func resolveParamWalk(funcCtx *class_context.ClassContext, provider ClassSigProvider, internal string, args []JavaType, method string, argc, paramIndex int, visited map[string]bool) JavaType {
	if internal == "" || visited[internal] {
		return nil
	}
	visited[internal] = true
	classSig, methodSigs, ok := provider(internal)
	if !ok {
		return nil // JDK / external: not in jar, covered by the InstantiateJDKMethodParam table
	}
	// sigma: this node's formal type params -> actual args (positional; raw receiver -> empty sigma).
	formals := ClassFormalTypeParamNames(classSig)
	sigma := map[string]JavaType{}
	for i := 0; i < len(formals) && i < len(args); i++ {
		if args[i] != nil {
			sigma[formals[i]] = args[i]
		}
	}
	// Most-derived declaration with a generic Signature wins: if THIS class declares (method, argc)
	// generically, it is the binding declaration -- resolve here and stop (do not let an ancestor's
	// signature shadow an override).
	if methodSigs != nil {
		if msig := methodSigs[class_context.MethodSigKey(method, argc)]; msig != "" {
			_, params, _ := ParseMethodSignatureFull(msig, funcCtx)
			if paramIndex < len(params) && params[paramIndex] != nil {
				if t := substituteAndGateParam(funcCtx, params[paramIndex], sigma, MethodFormalTypeParamNames(msig)); t != nil {
					return t
				}
			}
			return nil
		}
	}
	// Otherwise walk the (parameterized) supertypes, composing sigma along each edge.
	sup, ifaces := ParseClassSignatureSupers(classSig)
	supers := make([]JavaType, 0, len(ifaces)+1)
	if sup != nil {
		supers = append(supers, sup)
	}
	supers = append(supers, ifaces...)
	for _, st := range supers {
		pt, isPT := st.RawType().(*JavaParameterizedType)
		if !isPT || pt.RawClassName == "" {
			continue // raw supertype carries no mapping -> cannot substitute, skip this subtree
		}
		childArgs := make([]JavaType, len(pt.TypeArgs))
		for i, ta := range pt.TypeArgs {
			childArgs[i] = SubstituteTypeVars(ta, sigma)
		}
		if t := resolveParamWalk(funcCtx, provider, dotToInternal(pt.RawClassName), childArgs, method, argc, paramIndex, visited); t != nil {
			return t
		}
	}
	return nil
}

// ResolveInstantiatedReturnType resolves the generic RETURN type of a call `recv.method(...)` where the
// receiver's static type is `recvRaw<recvArgs>`, by walking the receiver's supertype hierarchy (via
// provider) to the most-derived declaration of (method, argc) and substituting the receiver's actual
// type arguments through the composed type-parameter map. Returns nil when the callee is JDK/external
// (not in jar), declares no generic Signature for (method, argc) anywhere in the hierarchy, or the
// receiver args are unresolvable. Companion of ResolveInstantiatedParamType; used to recover a
// covariant/erased return whose descriptor is raw (e.g. `ListMultimap<K,V>.asMap()` -> `Map<K,
// Collection<V>>`, which inherits Multimap's declaration). Unlike the param resolver it applies NO
// denotability gate: callers compare the recovered erasure/args and decide the cast themselves.
func ResolveInstantiatedReturnType(funcCtx *class_context.ClassContext, provider ClassSigProvider, recvRaw string, recvArgs []JavaType, method string, argc int) JavaType {
	if funcCtx == nil || provider == nil || recvRaw == "" || method == "" {
		return nil
	}
	visited := map[string]bool{}
	return resolveReturnWalk(funcCtx, provider, dotToInternal(recvRaw), recvArgs, method, argc, visited)
}

// resolveReturnWalk performs the depth-first hierarchy walk for ResolveInstantiatedReturnType, mirroring
// resolveParamWalk but binding the callee's RETURN type. sigma maps the current node's formal
// type-parameter names to their actual arguments (in call-site-denotable terms), recomposed per edge.
func resolveReturnWalk(funcCtx *class_context.ClassContext, provider ClassSigProvider, internal string, args []JavaType, method string, argc int, visited map[string]bool) JavaType {
	if internal == "" || visited[internal] {
		return nil
	}
	visited[internal] = true
	classSig, methodSigs, ok := provider(internal)
	if !ok {
		return nil // JDK / external: not in jar
	}
	formals := ClassFormalTypeParamNames(classSig)
	sigma := map[string]JavaType{}
	for i := 0; i < len(formals) && i < len(args); i++ {
		if args[i] != nil {
			sigma[formals[i]] = args[i]
		}
	}
	// Most-derived declaration with a generic Signature wins (an override binds here; do not let an
	// ancestor's signature shadow it).
	if methodSigs != nil {
		if msig := methodSigs[class_context.MethodSigKey(method, argc)]; msig != "" {
			if _, _, ret := ParseMethodSignatureFull(msig, funcCtx); ret != nil {
				return SubstituteTypeVars(ret, sigma)
			}
			return nil
		}
	}
	// Otherwise walk the (parameterized) supertypes, composing sigma along each edge.
	sup, ifaces := ParseClassSignatureSupers(classSig)
	supers := make([]JavaType, 0, len(ifaces)+1)
	if sup != nil {
		supers = append(supers, sup)
	}
	supers = append(supers, ifaces...)
	for _, st := range supers {
		pt, isPT := st.RawType().(*JavaParameterizedType)
		if !isPT || pt.RawClassName == "" {
			continue // raw supertype carries no mapping -> cannot substitute, skip this subtree
		}
		childArgs := make([]JavaType, len(pt.TypeArgs))
		for i, ta := range pt.TypeArgs {
			childArgs[i] = SubstituteTypeVars(ta, sigma)
		}
		if t := resolveReturnWalk(funcCtx, provider, dotToInternal(pt.RawClassName), childArgs, method, argc, visited); t != nil {
			return t
		}
	}
	return nil
}

// ResolveInstantiatedSignature resolves BOTH the parameter types and the RETURN type of a call
// `recv.method(...)` at receiver `recvRaw<recvArgs>`, walking the receiver's supertype hierarchy to the
// most-derived declaration of (method, argc) and substituting the receiver's actual type arguments. It
// mirrors ResolveInstantiatedReturnType (no denotability gate) but also returns the formal parameter
// types (substituted), so a caller can detect an UNCHECKED invocation -- a parameterized formal fed a
// raw argument, whose return javac erases to its erasure (JLS 15.12.2.6). Returns (nil, nil) when the
// callee is JDK/external or declares no generic Signature for (method, argc) anywhere in the hierarchy.
func ResolveInstantiatedSignature(funcCtx *class_context.ClassContext, provider ClassSigProvider, recvRaw string, recvArgs []JavaType, method string, argc int) ([]JavaType, JavaType) {
	if funcCtx == nil || provider == nil || recvRaw == "" || method == "" {
		return nil, nil
	}
	visited := map[string]bool{}
	return resolveSignatureWalk(funcCtx, provider, dotToInternal(recvRaw), recvArgs, method, argc, visited)
}

// resolveSignatureWalk performs the depth-first hierarchy walk for ResolveInstantiatedSignature, binding
// both the callee's PARAMETER types and RETURN type. sigma maps the current node's formal
// type-parameter names to their actual arguments (in call-site-denotable terms), recomposed per edge.
func resolveSignatureWalk(funcCtx *class_context.ClassContext, provider ClassSigProvider, internal string, args []JavaType, method string, argc int, visited map[string]bool) ([]JavaType, JavaType) {
	if internal == "" || visited[internal] {
		return nil, nil
	}
	visited[internal] = true
	classSig, methodSigs, ok := provider(internal)
	if !ok {
		return nil, nil // JDK / external: not in jar
	}
	formals := ClassFormalTypeParamNames(classSig)
	sigma := map[string]JavaType{}
	for i := 0; i < len(formals) && i < len(args); i++ {
		if args[i] != nil {
			sigma[formals[i]] = args[i]
		}
	}
	if methodSigs != nil {
		if msig := methodSigs[class_context.MethodSigKey(method, argc)]; msig != "" {
			if _, params, ret := ParseMethodSignatureFull(msig, funcCtx); ret != nil {
				subParams := make([]JavaType, len(params))
				for i, p := range params {
					subParams[i] = SubstituteTypeVars(p, sigma)
				}
				return subParams, SubstituteTypeVars(ret, sigma)
			}
			return nil, nil
		}
	}
	sup, ifaces := ParseClassSignatureSupers(classSig)
	supers := make([]JavaType, 0, len(ifaces)+1)
	if sup != nil {
		supers = append(supers, sup)
	}
	supers = append(supers, ifaces...)
	for _, st := range supers {
		pt, isPT := st.RawType().(*JavaParameterizedType)
		if !isPT || pt.RawClassName == "" {
			continue
		}
		childArgs := make([]JavaType, len(pt.TypeArgs))
		for i, ta := range pt.TypeArgs {
			childArgs[i] = SubstituteTypeVars(ta, sigma)
		}
		if params, ret := resolveSignatureWalk(funcCtx, provider, dotToInternal(pt.RawClassName), childArgs, method, argc, visited); ret != nil {
			return params, ret
		}
	}
	return nil, nil
}

// FieldSigProvider yields a jar-internal class's FIELD generic Signature by binary internal name and
// field name, or ok=false when the class is JDK/external (bytes not in jar) or the field has no generic
// Signature attribute. The dumper builds the concrete closure (it owns the byte resolver + class
// parser); the walk below consumes only strings, so the lower type/class_context packages never import
// the parser (mirrors ClassSigProvider).
type FieldSigProvider func(internalName, fieldName string) (fieldSig string, ok bool)

// ResolveInstantiatedFieldType recovers the generic type of a field accessed on a receiver whose static
// type is `recvRaw<recvArgs>`, by walking the receiver's generic supertype hierarchy (via classProvider)
// and composing the type-argument substitution along each `extends/implements Super<...>` edge until the
// field's declaring class is reached (via fieldProvider). It is the field analogue of
// ResolveInstantiatedParamType, added to recover an INHERITED parameterized field whose Signature lives
// in a superclass -- funcCtx.FieldSignature is CURRENT-class only, so an inherited field degrades to its
// raw descriptor and every downstream receiver/param resolver loses the type arguments. Canonical case:
// guava RegularContiguousSet<C>'s `this.domain` is declared `DiscreteDomain<C>` in the superclass
// ContiguousSet, so `this.domain.distance(this.first(), var1)` could not recover the erased `(C)` cast on
// var1 ("Comparable cannot be converted to C"). Returns nil when the field is not found in any
// generic-signature ancestor, the chain leaves the jar (provider miss), or the parse fails. Applies NO
// denotability gate: the sole caller (receiverParamTypeArgs) only consumes a parameterized result, whose
// type args are the current class's own (in-scope) variables. See resolveFieldWalk.
func ResolveInstantiatedFieldType(funcCtx *class_context.ClassContext, classProvider ClassSigProvider, fieldProvider FieldSigProvider, recvRaw string, recvArgs []JavaType, fieldName string) JavaType {
	if funcCtx == nil || classProvider == nil || fieldProvider == nil || recvRaw == "" || fieldName == "" {
		return nil
	}
	visited := map[string]bool{}
	return resolveFieldWalk(funcCtx, classProvider, fieldProvider, dotToInternal(recvRaw), recvArgs, fieldName, visited)
}

// resolveFieldWalk performs the depth-first hierarchy walk for ResolveInstantiatedFieldType. sigma maps
// the CURRENT node's formal type-parameter names to their actual arguments (in call-site-denotable
// terms), recomposed per supertype edge. The field, if declared at THIS node with a generic Signature,
// binds and is returned substituted; otherwise the parameterized supertypes are ascended.
func resolveFieldWalk(funcCtx *class_context.ClassContext, classProvider ClassSigProvider, fieldProvider FieldSigProvider, internal string, args []JavaType, fieldName string, visited map[string]bool) JavaType {
	if internal == "" || visited[internal] {
		return nil
	}
	visited[internal] = true
	classSig, _, ok := classProvider(internal)
	if !ok {
		return nil // JDK / external: not in jar
	}
	formals := ClassFormalTypeParamNames(classSig)
	sigma := map[string]JavaType{}
	for i := 0; i < len(formals) && i < len(args); i++ {
		if args[i] != nil {
			sigma[formals[i]] = args[i]
		}
	}
	// The field, if declared HERE with a generic Signature, is the binding declaration -- substitute the
	// composed type-argument map through it and return (a subclass never re-declares an inherited field's
	// generic type, so the most-derived declaration on the walk is authoritative).
	if fsig, ok := fieldProvider(internal, fieldName); ok && fsig != "" {
		if ft := ParseSignature(fsig); ft != nil {
			return SubstituteTypeVars(ft, sigma)
		}
	}
	// Otherwise ascend the (parameterized) supertypes, composing sigma along each edge.
	sup, ifaces := ParseClassSignatureSupers(classSig)
	supers := make([]JavaType, 0, len(ifaces)+1)
	if sup != nil {
		supers = append(supers, sup)
	}
	supers = append(supers, ifaces...)
	for _, st := range supers {
		pt, isPT := st.RawType().(*JavaParameterizedType)
		if !isPT || pt.RawClassName == "" {
			continue // raw supertype carries no mapping -> cannot substitute, skip this subtree
		}
		childArgs := make([]JavaType, len(pt.TypeArgs))
		for i, ta := range pt.TypeArgs {
			childArgs[i] = SubstituteTypeVars(ta, sigma)
		}
		if t := resolveFieldWalk(funcCtx, classProvider, fieldProvider, dotToInternal(pt.RawClassName), childArgs, fieldName, visited); t != nil {
			return t
		}
	}
	return nil
}

// substituteAndGateParam applies sigma to a callee formal parameter type and returns it ONLY when the
// result is denotable and castable at the call site (see ResolveInstantiatedParamType SAFETY). param is
// the pre-substitution formal; methodFormals are the callee's own method-scope type variable names.
func substituteAndGateParam(funcCtx *class_context.ClassContext, param JavaType, sigma map[string]JavaType, methodFormals []string) JavaType {
	// A method-scope `<T>` formal is not in scope at the call site: a `(T)` cast there would not compile.
	if jc, ok := param.RawType().(*JavaClass); ok {
		for _, mf := range methodFormals {
			if mf == jc.Name {
				return nil
			}
		}
	}
	res := SubstituteTypeVars(param, sigma)
	// A formal that substitutes to a lower-bounded wildcard `? super X` (consumer position) accepts an X:
	// the source cast the erased `Object` argument to X. Recover X (the lower bound) as the cast target,
	// restricted to an in-scope class type variable -- the guava Predicate<? super E>.apply(E) /
	// Function<? super F,...>.apply(F) / Collections2$FilteredCollection family. A concrete or out-of-scope
	// bound is rarer and riskier, so it stays nil.
	if w, isW := lowerBoundedWildcard(res); isW {
		if bjc, okb := w.Bound.RawType().(*JavaClass); okb && funcCtx.IsTypeParam(bjc.Name) {
			return w.Bound
		}
		return nil
	}
	// Any other wildcard (unbounded `?` or upper-bounded `? extends X`) is not a denotable cast target;
	// asserting it before RawType also avoids the nil-embed panic documented on isWildcardType.
	if isWildcardType(res) {
		return nil
	}
	jc, ok := res.RawType().(*JavaClass)
	if !ok {
		// Parameterized / array / primitive: the erasure cast bucket is scalar `(K)`/`(Foo)` only, and
		// the downstream cast logic ignores non-*JavaClass expect types anyway. Stay focused.
		return nil
	}
	if funcCtx.IsTypeParam(jc.Name) {
		return res // current-class type variable -> `(K)`
	}
	if strings.Contains(jc.Name, ".") {
		return res // concrete (dotted) class -> `(com.foo.Bar)`
	}
	return nil // leftover foreign bare type variable -> not denotable
}
