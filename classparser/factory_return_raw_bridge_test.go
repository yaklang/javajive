package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestFactoryReturnRawBridgeIsLoadBearing pins factoryReturnRawBridge. A method returning
// Imm<K,V> with K bounded (K extends Enum<K>) that returns Imm.of() / Imm.of(k,v) needs
// `(Imm<K, V>) (Imm) (...)` — a direct parameterization cast is inconvertible. Kill-switch:
// JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF. Real hit: guava ImmutableEnumMap.asImmutable /
// ImmutableMap.of inference-variable incompatible bounds.
func TestFactoryReturnRawBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/FactoryReturnRawBridgeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "$Imm<K, V>)") || !strings.Contains(on, "$Imm) (") {
		t.Errorf("fix ON: expected raw-erasure bridge (Imm<K, V>) (Imm), got:\n%s", on)
	}

	t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "$Imm<K, V>)") && strings.Contains(off, "$Imm) (") {
		t.Errorf("fix OFF: expected no raw-erasure bridge, got:\n%s", off)
	}
}

func TestFixSelectMethodsAmbiguousIsLoadBearing(t *testing.T) {
	in := "" +
		"\tpublic static Set<Method> selectMethods(Class<?> var0, ReflectionUtils$MethodFilter var1) {\n" +
		"\t\treturn (Set<Method>) (Set) (selectMethods(var0,(l0) -> {\n" +
		"\t\t\treturn (var1.matches(l0)) ? (Boolean.TRUE) : (null);\n" +
		"\t\t}).keySet());\n" +
		"\t}\n"
	os.Unsetenv("JDEC_SELECTMETHODS_CAST_OFF")
	on := fixSelectMethodsAmbiguous(in)
	if !strings.Contains(on, "(MethodIntrospector$MetadataLookup)((l0) ->") {
		t.Errorf("fix ON: expected MetadataLookup cast, got:\n%s", on)
	}
	t.Setenv("JDEC_SELECTMETHODS_CAST_OFF", "1")
	off := fixSelectMethodsAmbiguous(in)
	if strings.Contains(off, "MetadataLookup") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixNeverThrownCNFEIsLoadBearing(t *testing.T) {
	in := "" +
		"\ttry{\n" +
		"\t\treturn ClassUtils.createCompositeInterface(var2,this.classLoader);\n" +
		"\t}catch(ClassNotFoundException var4){\n" +
		"\t\treturn null;\n" +
		"\t}\n"
	os.Unsetenv("JDEC_CNFE_NEVER_THROWN_OFF")
	on := fixNeverThrownCNFE(in)
	if !strings.Contains(on, "Class.forName(\"java.lang.Object\")") {
		t.Errorf("fix ON: expected Class.forName in try, got:\n%s", on)
	}
	t.Setenv("JDEC_CNFE_NEVER_THROWN_OFF", "1")
	off := fixNeverThrownCNFE(in)
	if strings.Contains(off, "Class.forName(\"java.lang.Object\")") {
		t.Errorf("fix OFF: expected no Class.forName inject, got:\n%s", off)
	}
}

func TestImmutableBuilderWitnessIsLoadBearing(t *testing.T) {
	in := "\tprivate static final ImmutableMap<String, Foo> VALUE_PARSERS = ImmutableMap.builder().put(\"a\", new Foo()).build();\n"
	os.Unsetenv("JDEC_IMMUTABLE_BUILDER_WITNESS_OFF")
	on := fixImmutableBuilderWitness(in)
	if !strings.Contains(on, "ImmutableMap.<String, Foo>builder()") {
		t.Errorf("fix ON: expected type witness, got:\n%s", on)
	}
	t.Setenv("JDEC_IMMUTABLE_BUILDER_WITNESS_OFF", "1")
	off := fixImmutableBuilderWitness(in)
	if strings.Contains(off, ".<String, Foo>builder()") {
		t.Errorf("fix OFF: expected no witness, got:\n%s", off)
	}
}

func TestFixAsMapFunctionRawCastIsLoadBearing(t *testing.T) {
	in := "" +
		"\tpublic AnnotationAttributes asAnnotationAttributes(MergedAnnotation$Adapt... var1) {\n" +
		"\t\treturn ((AnnotationAttributes)(this.asMap((l0) -> {\n" +
		"\t\t\treturn new AnnotationAttributes(l0.getType());\n" +
		"\t\t},var1)));\n" +
		"\t}\n"
	os.Unsetenv("JDEC_ASMAP_FUNCTION_CAST_OFF")
	on := fixAsMapFunctionRawCast(in)
	if !strings.Contains(on, "Function<MergedAnnotation<?>, java.util.Map>)((l0) ->") {
		t.Errorf("fix ON: expected Function<MergedAnnotation<?>, Map> cast, got:\n%s", on)
	}
	t.Setenv("JDEC_ASMAP_FUNCTION_CAST_OFF", "1")
	off := fixAsMapFunctionRawCast(in)
	if strings.Contains(off, "MergedAnnotation<?>") {
		t.Errorf("fix OFF: expected no wildcard Function cast, got:\n%s", off)
	}
}

func TestFixThrowClassCastDropsThrowableIsLoadBearing(t *testing.T) {
	in := "\t\tthrow ((Throwable)(var1.cast(var0)));\n"
	os.Unsetenv("JDEC_THROW_CLASS_CAST_DROP_THROWABLE_OFF")
	on := fixThrowClassCastDropsThrowable(in)
	if !strings.Contains(on, "throw var1.cast(var0);") {
		t.Errorf("fix ON: expected unwrapped throw cls.cast, got:\n%s", on)
	}
	if strings.Contains(on, "throw ((Throwable)") {
		t.Errorf("fix ON: (Throwable) wrap must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_THROW_CLASS_CAST_DROP_THROWABLE_OFF", "1")
	off := fixThrowClassCastDropsThrowable(in)
	if !strings.Contains(off, "throw ((Throwable)") {
		t.Errorf("fix OFF: expected (Throwable) wrap kept, got:\n%s", off)
	}
}

func TestFixNeverThrownIOExceptionIsLoadBearing(t *testing.T) {
	in := "" +
		"\ttry{\n" +
		"\t\tthis.appendTo((Appendable)(var1),var2);\n" +
		"\t\treturn var1;\n" +
		"\t}catch(IOException var3){\n" +
		"\t\tthrow new AssertionError(var3);\n" +
		"\t}\n"
	os.Unsetenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF")
	on := fixNeverThrownIOException(in)
	if !strings.Contains(on, "if(false)throw new IOException()") {
		t.Errorf("fix ON: expected if(false)throw IOException, got:\n%s", on)
	}
	t.Setenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF", "1")
	off := fixNeverThrownIOException(in)
	if strings.Contains(off, "if(false)throw new IOException()") {
		t.Errorf("fix OFF: expected no inject, got:\n%s", off)
	}
}

func TestFixAsMapFunctionRawCastRewritesWrongCast(t *testing.T) {
	in := "this.asMap((Function<MergedAnnotation, AnnotationAttributes>)((l0) -> {\nreturn new AnnotationAttributes(l0.getType());\n}),var1)"
	os.Unsetenv("JDEC_ASMAP_FUNCTION_CAST_OFF")
	on := fixAsMapFunctionRawCast(in)
	if !strings.Contains(on, "Function<MergedAnnotation<?>, java.util.Map>") {
		t.Errorf("fix ON: expected rewrite of wrong Function cast, got:\n%s", on)
	}
	if strings.Contains(on, "Function<MergedAnnotation, AnnotationAttributes>") {
		t.Errorf("fix ON: wrong Function<MergedAnnotation, AnnotationAttributes> must be gone, got:\n%s", on)
	}
}

func TestParamFieldRetRawBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ParamFieldRetRawBridgeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_PARAM_FIELD_RET_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "Holder<K,") || !strings.Contains(on, "Holder) (") && !strings.Contains(on, "(Holder)") {
		// Accept either `(Holder<K, Box<V>>) (Holder) this.map` or similar spacing.
		if !strings.Contains(on, "(Holder") || !strings.Contains(on, "this.map") {
			t.Errorf("fix ON: expected raw-erasure field return bridge, got:\n%s", on)
		}
	}
	t.Setenv("JDEC_PARAM_FIELD_RET_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Holder) (this.map)") || strings.Contains(off, "(Holder)(this.map)") {
		t.Errorf("fix OFF: expected no raw field bridge, got:\n%s", off)
	}
}

// TestUnmodifiableListBridgeIsLoadBearing pins factoryReturnRawBridge on
// Collections.unmodifiableList(Arrays.asList(Object[])). Kill-switch:
// JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF. Real hits: guava Ordering.leastOf
// (List<E extends T>) and Striped.bulkGet (Iterable<L>).
func TestUnmodifiableListBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/UnmodifiableListBridgeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "unmodifiableList") {
		t.Fatalf("expected unmodifiableList in decompile, got:\n%s", on)
	}
	hasListBridge := strings.Contains(on, "(List<E>) (List)") ||
		strings.Contains(on, "(List<E>)(List)") ||
		strings.Contains(on, "(java.util.List<E>) (List)") ||
		strings.Contains(on, "(java.util.List<E>)(List)")
	hasIterBridge := strings.Contains(on, "(Iterable<T>) (Iterable)") ||
		strings.Contains(on, "(Iterable<T>)(Iterable)") ||
		strings.Contains(on, "(java.lang.Iterable<T>) (Iterable)") ||
		strings.Contains(on, "(java.lang.Iterable<T>)(Iterable)")
	if !hasListBridge && !hasIterBridge {
		t.Errorf("fix ON: expected raw List or Iterable bridge around unmodifiableList, got:\n%s", on)
	}

	t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(List<E>) (List)") || strings.Contains(off, "(List<E>)(List)") ||
		strings.Contains(off, "(Iterable<T>) (Iterable)") || strings.Contains(off, "(Iterable<T>)(Iterable)") {
		t.Errorf("fix OFF: expected no raw unmodifiableList bridge, got:\n%s", off)
	}
}

// TestOrderingReverseBridgeIsLoadBearing pins factoryReturnRawBridge on
// Ordering.natural().reverse() returned as Comparator<? super E>. Kill-switch:
// JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF. Real hit: guava Sets$DescendingSet.comparator.
func TestOrderingReverseBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/OrderingReverseBridgeSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF")
	on, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "reverse()") {
		t.Fatalf("expected reverse() in decompile, got:\n%s", on)
	}
	if !strings.Contains(on, "(Comparator") {
		t.Errorf("fix ON: expected raw Comparator bridge around reverse(), got:\n%s", on)
	}

	t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "1")
	off, err := DecompileWithResolver(data, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Comparator<? super E>) (Comparator)") ||
		strings.Contains(off, "(Comparator<? super E>)(Comparator)") {
		t.Errorf("fix OFF: expected no raw Comparator reverse bridge, got:\n%s", off)
	}
}

func TestFactoryFromOrderingIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GuavaMinMaxPriorityQueueBuilder.class")
	if err != nil {
		t.Skipf("guava Builder fixture missing: %v", err)
	}
	os.Unsetenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "Ordering.from") {
		t.Fatalf("expected Ordering.from in decompile, got:\n%s", on)
	}
	if !strings.Contains(on, "(Ordering") {
		t.Errorf("fix ON: expected raw Ordering bridge around from(), got:\n%s", on)
	}
	t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Ordering<T>) (Ordering)") || strings.Contains(off, "(Ordering<T>)(Ordering)") {
		t.Errorf("fix OFF: expected no raw Ordering bridge, got:\n%s", off)
	}
}
