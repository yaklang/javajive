package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// TestSubstituteTypeVars pins the generic-substitution primitive that instantiates a callee signature
// (written in the declaring class's type variables) at a call site with the receiver's actual type
// arguments. It is the root-cause core shared by the unified cross-class resolver, so a regression here
// would silently break every recovered argument cast.
func TestSubstituteTypeVars(t *testing.T) {
	ctx := &class_context.ClassContext{}
	str := NewJavaClass("java.lang.String")
	integer := NewJavaClass("java.lang.Integer")

	// identity: {K:String}, K -> String; an unmapped var V stays V; a concrete class stays.
	sigma := map[string]JavaType{"K": str, "V": integer}

	cases := []struct {
		name  string
		in    JavaType
		sigma map[string]JavaType
		want  string
	}{
		{"typevar_hit", NewJavaTypeVariable("K"), sigma, "String"},
		{"typevar_other_hit", NewJavaTypeVariable("V"), sigma, "Integer"},
		{"typevar_miss_unchanged", NewJavaTypeVariable("T"), sigma, "T"},
		{"concrete_unchanged", NewJavaClass("java.lang.String"), sigma, "String"},
		{"empty_sigma_unchanged", NewJavaTypeVariable("K"), map[string]JavaType{}, "K"},
		{
			"parameterized_args_substituted",
			NewParameterizedType("java.util.Map", []JavaType{NewJavaTypeVariable("K"), NewJavaTypeVariable("V")}),
			sigma,
			"Map<String, Integer>",
		},
		{
			"nested_parameterized",
			NewParameterizedType("java.util.List", []JavaType{
				NewParameterizedType("java.util.Map", []JavaType{NewJavaTypeVariable("K"), NewJavaTypeVariable("V")}),
			}),
			sigma,
			"List<Map<String, Integer>>",
		},
		{
			"array_of_typevar",
			NewJavaArrayType(NewJavaTypeVariable("K")),
			sigma,
			"String[]",
		},
		{
			"wildcard_bound_substituted",
			&JavaWildcardType{Variant: "super", Bound: NewJavaTypeVariable("K")},
			sigma,
			"? super String",
		},
		{
			"nested_wildcard_bound_substituted",
			NewParameterizedType("java.util.function.Function", []JavaType{
				&JavaWildcardType{Variant: "super", Bound: NewJavaTypeVariable("K")},
				&JavaWildcardType{Variant: "extends", Bound: NewJavaTypeVariable("V")},
			}),
			sigma,
			"Function<? super String, ? extends Integer>",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := SubstituteTypeVars(c.in, c.sigma)
			if got == nil {
				t.Fatalf("%s: got nil", c.name)
			}
			if s := got.String(ctx); s != c.want {
				t.Errorf("%s: got %q want %q", c.name, s, c.want)
			}
		})
	}

	// nil-safe.
	if SubstituteTypeVars(nil, sigma) != nil {
		t.Errorf("nil input should return nil")
	}
}

// A jar-internal generic map implementation can inherit a remapping method from
// ConcurrentMap.  The sibling provider deliberately stops at the JDK boundary,
// but the receiver arguments composed along the jar hierarchy are still enough
// to instantiate the stable JDK functional formal.  The return remains unknown:
// callers that only need a parameter must not invent return metadata.
func TestResolveInstantiatedSignatureRecoversJDKLeafFormal(t *testing.T) {
	ctx := &class_context.ClassContext{}
	provider := func(internal string) (string, map[string]string, bool) {
		if internal != "example/LocalMap" {
			return "", nil, false
		}
		return "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;Ljava/util/concurrent/ConcurrentMap<TK;TV;>;", nil, true
	}
	recvArgs := []JavaType{
		NewJavaTypeVariable("K"),
		NewParameterizedType("java.util.concurrent.CompletableFuture", []JavaType{NewJavaTypeVariable("V")}),
	}
	params, ret := ResolveInstantiatedSignature(ctx, provider, "example.LocalMap", recvArgs, "computeIfPresent", 2)
	if ret != nil {
		t.Fatalf("JDK leaf parameter recovery must not invent a return type: %s", nameOf(ret))
	}
	if len(params) != 2 || params[1] == nil {
		t.Fatalf("missing JDK leaf functional formal: %#v", params)
	}
	want := "BiFunction<? super K, ? super CompletableFuture<V>, ? extends CompletableFuture<V>>"
	if got := params[1].String(ctx); got != want {
		t.Fatalf("JDK leaf formal = %q, want %q", got, want)
	}

	t.Setenv("JDEC_GENERIC_PARAM_INFER_OFF", "1")
	params, _ = ResolveInstantiatedSignature(ctx, provider, "example.LocalMap", recvArgs, "computeIfPresent", 2)
	if len(params) != 0 {
		t.Fatalf("generic-inference kill switch retained JDK leaf formals: %#v", params)
	}
}

func TestResolveInstantiatedSignatureExactSelectsOverloadThroughHierarchy(t *testing.T) {
	ctx := &class_context.ClassContext{}
	wantedDesc := "(Ljava/lang/Object;Ljava/util/function/BiFunction;)Ljava/lang/Object;"
	otherDesc := "(Ljava/lang/Object;Ljava/util/function/Function;)Ljava/lang/Object;"
	provider := func(internal string) (string, map[string]string, bool) {
		switch internal {
		case "example/Child":
			return "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;Lexample/Api<TK;TV;>;", nil, true
		case "example/Api":
			return "<A:Ljava/lang/Object;B:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{
				class_context.MethodDescKey("get", wantedDesc): "(TA;Ljava/util/function/BiFunction<-TA;Ljava/util/concurrent/Executor;Ljava/util/concurrent/CompletableFuture<TB;>;>;)TB;",
				class_context.MethodDescKey("get", otherDesc):  "(TA;Ljava/util/function/Function<-TA;+TB;>;)TB;",
			}, true
		default:
			return "", nil, false
		}
	}
	params, _, methodFormals := ResolveInstantiatedSignatureExact(ctx, provider, "example.Child", []JavaType{
		NewJavaTypeVariable("K"), NewJavaTypeVariable("V"),
	}, "get", wantedDesc, 2)
	if len(methodFormals) != 0 || len(params) != 2 || params[1] == nil {
		t.Fatalf("exact hierarchy resolution failed: params=%#v methodFormals=%v", params, methodFormals)
	}
	want := "BiFunction<? super K, Executor, CompletableFuture<V>>"
	if got := params[1].String(ctx); got != want {
		t.Fatalf("exact overload formal = %q, want %q", got, want)
	}

	// The legacy arity-only index is intentionally absent when overloads
	// collide. Exact resolution must never fall across to the other signature.
	if params, _ := ResolveInstantiatedSignature(ctx, provider, "example.Child", []JavaType{
		NewJavaTypeVariable("K"), NewJavaTypeVariable("V"),
	}, "get", 2); len(params) != 0 {
		t.Fatalf("arity-only resolver guessed across overloads: %#v", params)
	}
}

func TestInstantiateJDKMethodReturnPreservesMapViewChain(t *testing.T) {
	ctx := &class_context.ClassContext{}
	mapArgs := []JavaType{NewJavaTypeVariable("K"), NewJavaTypeVariable("V")}
	entrySet := InstantiateJDKMethodReturn("java.util.Map", "entrySet", 0, mapArgs)
	if entrySet == nil || entrySet.String(ctx) != "Set<Map.Entry<K, V>>" {
		t.Fatalf("entrySet target = %v", entrySet)
	}
	setType, ok := AsParameterizedType(entrySet)
	if !ok {
		t.Fatalf("entrySet result lost type arguments: %v", entrySet)
	}
	spliterator := InstantiateJDKMethodReturn("java.util.Set", "spliterator", 0, setType.TypeArgs)
	if spliterator == nil || spliterator.String(ctx) != "Spliterator<Map.Entry<K, V>>" {
		t.Fatalf("spliterator target = %v", spliterator)
	}

	for method, want := range map[string]string{
		"keySet": "Set<K>",
		"values": "Collection<V>",
	} {
		got := InstantiateJDKMethodReturn("java.util.Map", method, 0, mapArgs)
		if got == nil || got.String(ctx) != want {
			t.Errorf("%s target = %v, want %s", method, got, want)
		}
	}
}

func TestInstantiateJDKMethodReturnUsesOnlyProducerArgument(t *testing.T) {
	ctx := &class_context.ClassContext{}
	typeVar := NewJavaTypeVariable("V")
	upperV := &JavaWildcardType{Variant: "extends", Bound: typeVar}
	lowerK := &JavaWildcardType{Variant: "super", Bound: NewJavaTypeVariable("K")}

	cases := []struct {
		name, raw, method string
		argc              int
		args              []JavaType
		want              string
	}{
		{"map get", "java.util.Map", "get", 1, []JavaType{NewJavaTypeVariable("K"), typeVar}, "V"},
		{"function covariant result", "java.util.function.Function", "apply", 1, []JavaType{lowerK, upperV}, "V"},
		{"bifunction covariant result", "java.util.function.BiFunction", "apply", 2, []JavaType{lowerK, lowerK, upperV}, "V"},
		{"supplier result", "java.util.function.Supplier", "get", 0, []JavaType{typeVar}, "V"},
		{"callable result", "java.util.concurrent.Callable", "call", 0, []JavaType{typeVar}, "V"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InstantiateJDKMethodReturn(tc.raw, tc.method, tc.argc, tc.args)
			if got == nil || got.String(ctx) != tc.want {
				t.Fatalf("return target = %v, want %s", got, tc.want)
			}
		})
	}

	for _, bad := range []JavaType{
		&JavaWildcardType{},
		&JavaWildcardType{Variant: "super", Bound: typeVar},
	} {
		if got := InstantiateJDKMethodReturn("java.util.function.Function", "apply", 1, []JavaType{lowerK, bad}); got != nil {
			t.Fatalf("non-producer wildcard invented return target: %s", got.String(ctx))
		}
	}

	iter := InstantiateJDKMethodReturn("java.lang.Iterable", "iterator", 0, []JavaType{upperV})
	if iter == nil || iter.String(ctx) != "Iterator<? extends V>" {
		t.Fatalf("nested producer wildcard was collapsed unsafely: %v", iter)
	}
}

func TestNestedInvariantMapViewDoesNotInventCaptureType(t *testing.T) {
	args := []JavaType{NewJavaTypeVariable("K"), &JavaWildcardType{Variant: "extends", Bound: NewJavaTypeVariable("V")}}
	if got := InstantiateJDKMethodReturn("java.util.Map", "entrySet", 0, args); got != nil {
		t.Fatalf("Set<Entry<K,CAP>> cannot be widened to Set<Entry<K,? extends V>>: %s", got.String(&class_context.ClassContext{}))
	}
	if got := InstantiateJDKMethodReturn("java.util.Map", "remove", 2, args); got != nil {
		t.Fatal("boolean remove(Object,Object) confused with V remove(Object)")
	}
}

func TestExactMethodSignatureShadowsReceiverTypeVariable(t *testing.T) {
	ctx := &class_context.ClassContext{}
	descriptor := "(Ljava/util/function/Function;)V"
	provider := func(string) (string, map[string]string, bool) {
		return "<T:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{
			class_context.MethodDescKey("use", descriptor): "<T:Ljava/lang/Object;>(Ljava/util/function/Function<TT;TT;>;)V",
		}, true
	}
	params, _, formals := ResolveInstantiatedSignatureExact(ctx, provider, "example.Owner", []JavaType{NewJavaClass("java.lang.String")}, "use", descriptor, 1)
	if len(params) != 1 || params[0].String(ctx) != "Function<T, T>" || len(formals) != 1 || formals[0] != "T" {
		t.Fatalf("method-local T was captured by the receiver substitution: %v/%v", params, formals)
	}
}

func TestSubstitutionRequiresFormalRoleRatherThanMatchingClassSpelling(t *testing.T) {
	ctx := &class_context.ClassContext{}
	sigma := map[string]JavaType{"T": NewJavaClass("java.lang.String"), "Owner$Token": NewJavaClass("java.lang.String")}
	for _, row := range []struct{ sig, want string }{{"TT;", "String"}, {"LT;", "T"}, {"[TT;", "String[]"}, {"[LT;", "T[]"}, {"Ljava/util/Map<LT;TT;>;", "Map<T, String>"}, {"Ljava/util/Map<LOwner$Token;TOwner$Token;>;", "Map<Owner$Token, String>"}, {"Ljava/util/List<+LT;>;", "List<? extends T>"}, {"Ljava/util/List<+TT;>;", "List<? extends String>"}} {
		t.Run(row.sig, func(t *testing.T) {
			original := ParseSignature(row.sig)
			if original == nil {
				t.Fatal("signature")
			}
			before := original.String(ctx)
			substituted := SubstituteTypeVars(original, sigma)
			if got := substituted.String(ctx); got != row.want {
				t.Fatalf("substitution=%q want=%q", got, row.want)
			}
			if original.String(ctx) != before {
				t.Fatal("substitution mutated original")
			}
		})
	}
	literal := NewJavaClass("T")
	if SubstituteTypeVars(literal, sigma) != literal {
		t.Fatal("a concrete class is not a variable reference")
	}
}

func TestInstantiatedSignatureRetainsLiteralClassInsideParameterizedFormal(t *testing.T) {
	ctx := &class_context.ClassContext{}
	ctx.DeclarationSourceName = func(binary string) (string, bool) { return "Owner.Token", binary == "Owner$Token" }
	provider := func(name string) (string, map[string]string, bool) {
		if name != "Owner$Box" {
			return "", nil, false
		}
		return "<Owner$Token:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", "(Ljava/util/List;)Ljava/lang/Object;"): "(Ljava/util/List<LOwner$Token;>;)TOwner$Token;"}, true
	}
	params, result, formals := ResolveInstantiatedSignatureExact(ctx, provider, "Owner$Box", []JavaType{NewJavaClass("java.lang.String")}, "apply", "(Ljava/util/List;)Ljava/lang/Object;", 1)
	if len(params) != 1 || params[0].String(ctx) != "List<Owner.Token>" || result == nil || result.String(ctx) != "String" || len(formals) != 0 {
		t.Fatalf("literal/formal instantiation=%v/%v/%v", params, result, formals)
	}
}
