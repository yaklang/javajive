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

// A checked call already supplies the exception witness. Manufacturing a
// reflective call to satisfy javac adds an unrelated class-loading effect.
func TestCheckedClassLoadCatchPreservesOriginalEffects(t *testing.T) {
	const source = `
class ReviewedCheckedLoader {
 final ClassNotFoundException failure=new ClassNotFoundException("sentinel");String trace="";
 Class<?> resolve(int mode)throws ClassNotFoundException{trace+="L";if(mode==1)throw failure;if(mode==2)throw new IllegalStateException("runtime");return mode==3?null:String.class;}
}
public class CheckedLoadEffectsReview {
 final ReviewedCheckedLoader loader=new ReviewedCheckedLoader();Class<?> cached;
 Class<?> read(int mode){if(cached!=null)return cached;try{return cached=loader.resolve(mode);}catch(ClassNotFoundException caught){if(caught!=loader.failure)throw new AssertionError("identity");loader.trace+="C";return Object.class;}}
 static String run(int mode){CheckedLoadEffectsReview c=new CheckedLoadEffectsReview();String out="";try{Class<?> first=c.read(mode);Class<?> second=c.read(0);out=(first==String.class)+":"+(first==Object.class)+":"+(first==null)+":"+(second==first)+":"+(c.cached==second);}catch(RuntimeException e){out=e.getClass().getName()+":"+e.getMessage()+":"+(c.cached==null);}return out+":"+c.loader.trace;}
 public static void main(String[] args){for(int mode=0;mode<4;mode++)System.out.println(run(mode));}
}`
	_, classes := t04CompileRun(t, "8", "CheckedLoadEffectsReview", map[string]string{"CheckedLoadEffectsReview.java": source})
	resolve := func(name string) ([]byte, bool) { raw, ok := classes[name]; return raw, ok }
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_CNFE_NEVER_THROWN_OFF", setting)
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			var generated string
			var err error
			if mode == "legacy" {
				generated, err = DecompileWithResolver(classes["CheckedLoadEffectsReview"], resolve)
			} else {
				var result DecompileResult
				result, err = DecompileWithOptions(classes["CheckedLoadEffectsReview"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
				generated = result.Source
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(generated, "Class.forName(") {
				t.Fatalf("%s/%q gained a reflective effect absent from the authored source:\n%s", mode, setting, generated)
			}
			if !strings.Contains(generated, ".resolve(") {
				t.Fatalf("original checked invocation missing:\n%s", generated)
			}
		}
	}
	roundTripGenericFlow(t, "CheckedLoadEffectsReview", source, Precision, Compatibility, "legacy")
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

func TestFixNeverThrownIOExceptionPairsMatchingTry(t *testing.T) {
	in := "" +
		"\ttry{\n" +
		"\t\ttry{\n" +
		"\t\t\ttry{\n" +
		"\t\t\t\tvar3.close();\n" +
		"\t\t\t}catch(Throwable var6_1){\n" +
		"\t\t\t\tvar4.addSuppressed(var6_1);\n" +
		"\t\t\t}\n" +
		"\t\t}catch(IOException var3_1){\n" +
		"\t\t\tthrow new IllegalStateException(\"Normalization threw an unexpected exception\",(Throwable)(var3_1));\n" +
		"\t\t}\n" +
		"\t}catch(IOException var3_1){\n" +
		"\t\tthrow new IllegalStateException(\"Normalization threw an unexpected exception\",(Throwable)(var3_1));\n" +
		"\t}\n"
	os.Unsetenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF")
	on := fixNeverThrownIOException(in)
	if strings.Count(on, "if(false)throw new IOException();") != 2 {
		t.Fatalf("expected injector in each IOException try, got %d:\n%s", strings.Count(on, "if(false)throw new IOException();"), on)
	}
	closeTry := strings.Index(on, "var3.close();")
	if closeTry < 0 {
		t.Fatal("missing close()")
	}
	before := on[:closeTry]
	lastTry := strings.LastIndex(before, "try{")
	chunk := on[lastTry:closeTry]
	if strings.Contains(chunk, "if(false)throw new IOException()") {
		t.Fatalf("injector landed in innermost Throwable close() try:\n%s", chunk)
	}
	t.Setenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF", "1")
	off := fixNeverThrownIOException(in)
	if strings.Contains(off, "if(false)throw new IOException()") {
		t.Fatalf("fix OFF: expected no inject, got:\n%s", off)
	}
}

func TestFixNeverThrownIOExceptionSkipsPriorCatch(t *testing.T) {
	in := "" +
		"\ttry{\n" +
		"\t\tbreak;\n" +
		"\t}catch(InterruptedIOException var2){\n" +
		"\t\tThread.interrupted();\n" +
		"\t}catch(IOException var2){\n" +
		"\t\treturn;\n" +
		"\t}\n"
	os.Unsetenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF")
	on := fixNeverThrownIOException(in)
	if !strings.Contains(on, "try{\n\t\tif(false)throw new IOException();\n\t\tbreak;") {
		t.Fatalf("injector must be first stmt of try, got:\n%s", on)
	}
	if strings.Contains(on, "InterruptedIOException var2){\n\t\tif(false)throw") {
		t.Fatalf("injector landed in prior catch:\n%s", on)
	}
}

func TestFixNeverThrownFileAlreadyExistsExceptionIsLoadBearing(t *testing.T) {
	in := "" +
		"\tdo{\n" +
		"\t\ttry{\n" +
		"\t\t\tvar4 = getTempFileName(var1,var2,this.nextTempFileCounter.getAndIncrement());\n" +
		"\t\t\tif (this.pendingDeletes.contains(var4)){\n" +
		"\t\t\t\tcontinue;\n" +
		"\t\t\t}else{\n" +
		"\t\t\t\tbreak;\n" +
		"\t\t\t}\n" +
		"\t\t}catch(FileAlreadyExistsException var4_1){\n" +
		"\t\t\tthrow new RuntimeException(var4_1);\n" +
		"\t\t}\n" +
		"\t} while (true);\n"
	os.Unsetenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF")
	on := fixNeverThrownIOException(in)
	if !strings.Contains(on, "if(false)throw new FileAlreadyExistsException(\"\");") {
		t.Fatalf("fix ON: expected FAE injector, got:\n%s", on)
	}
	t.Setenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF", "1")
	off := fixNeverThrownIOException(in)
	if strings.Contains(off, "if(false)throw new FileAlreadyExistsException") {
		t.Fatalf("fix OFF: expected no inject, got:\n%s", off)
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

// Generic return bridges preserve the erased Object[] factory result. The
// production binding planner supplies them even with the old workaround off.
func TestUnmodifiableListBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/UnmodifiableListBridgeSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(source, "unmodifiableList") || !strings.Contains(source, "(List<E>) (List)") || !strings.Contains(source, "(Iterable<T>) (Iterable)") {
			t.Fatalf("erased factory result has no generic return witness:\n%s", source)
		}
	}
}

// Preserve the original Comparator CHECKCAST independently of the legacy
// factory spelling gate. The subtype/no-CHECKCAST case has a separate authored
// six-mode JVM oracle that verifies the general zero-input factory-chain proof.
func TestOrderingReverseBridgeIsLoadBearing(t *testing.T) {

	path := "testdata/regression/OrderingReverseBridgeSeed.class"
	raw, code, _ := reviewedFixtureMethod(t, path, "comparator", "()Ljava/util/Comparator;")
	assertReviewedTypeVarMethod(t, raw, "comparator", "()Ljava/util/Comparator;", "()Ljava/util/Comparator<-TE;>;")
	assertReviewedTypeVarInvoke(t, path, "comparator", "()Ljava/util/Comparator;", 14, 184, "OrderingReverseBridgeSeed$Ord", "natural", "()LOrderingReverseBridgeSeed$Ord;")
	assertReviewedTypeVarInvoke(t, path, "comparator", "()Ljava/util/Comparator;", 17, 182, "OrderingReverseBridgeSeed$Ord", "reverse", "()LOrderingReverseBridgeSeed$Ord;")
	// This fixture's Ord does not implement Comparator. Its original CHECKCAST
	// remains a real runtime check even when the old spelling gate is disabled.
	assertReviewedOpcode(t, code, 20, 192)
	helper, err := os.ReadFile("testdata/regression/OrderingReverseBridgeSeed$Ord.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedClassSignature(t, helper, "<T:Ljava/lang/Object;>Ljava/lang/Object;")
	assertReviewedTypeVarMethod(t, helper, "natural", "()LOrderingReverseBridgeSeed$Ord;", "<C::Ljava/lang/Comparable;>()LOrderingReverseBridgeSeed$Ord<TC;>;")
	assertReviewedTypeVarMethod(t, helper, "reverse", "()LOrderingReverseBridgeSeed$Ord;", "<S:TT;>()LOrderingReverseBridgeSeed$Ord<TS;>;")
	reviewedSeedSources(t, path, "JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", true, func(source string) {
		body := reviewedSourceMethod(t, source, `comparator\(\)`)
		if !strings.Contains(body, "natural().reverse()") || !strings.Contains(compactReviewedGenericSource(body), "(Comparator<?superE>)(Comparator)") {
			t.Fatal("lost original Comparator check or erased return view: " + body)
		}
	})
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
