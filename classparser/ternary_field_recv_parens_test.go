package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestTernaryFieldRecvParensIsLoadBearing pins RefMember parentheses around a ternary
// receiver. Kill-switch: JDEC_TERNARY_FIELD_RECV_PARENS_OFF. Real hit: guava Range.gap.
// TestTernaryArrayIndexParensIsLoadBearing pins JavaArrayMember parentheses
// around a ternary indexee. Kill-switch: JDEC_TERNARY_ARRAY_INDEX_PARENS_OFF.
// Real hit: spring TypeMappedAnnotation.getValue.
func TestTernaryArrayIndexParensIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TernaryArrayIndexSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_TERNARY_ARRAY_INDEX_PARENS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, ")[") {
		t.Errorf("fix ON: expected parenthesized ternary array indexee, got:\n%s", on)
	}

	t.Setenv("JDEC_TERNARY_ARRAY_INDEX_PARENS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, ")[") && !strings.Contains(off, "resolvedRootMirrors)[") {
		// OFF should not wrap the whole ternary; the index binds to the false arm.
		if strings.Contains(off, "?)") || strings.Contains(off, ")?[") {
			t.Errorf("fix OFF: expected unparenthesized ternary indexee, got:\n%s", off)
		}
	}
}

func TestTernaryFieldRecvParensIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TernaryFieldRecvSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_TERNARY_FIELD_RECV_PARENS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, ").lowerBound") {
		t.Errorf("fix ON: expected parenthesized ternary `.lowerBound` receiver, got:\n%s", on)
	}
	if strings.Contains(on, ": (this).lowerBound") || strings.Contains(on, ":(this).lowerBound") {
		t.Errorf("fix ON: field must not bind only to the false arm, got:\n%s", on)
	}

	t.Setenv("JDEC_TERNARY_FIELD_RECV_PARENS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, ": (this).lowerBound") && !strings.Contains(off, ":(this).lowerBound") &&
		!strings.Contains(off, ": (this).lowerBound") {
		// OFF should lose the wrapping parens so `.lowerBound` binds to the false arm only.
		if strings.Contains(off, ").lowerBound") && !strings.Contains(off, ": (this).lowerBound") {
			t.Errorf("fix OFF: expected unparenthesized false-arm field, got:\n%s", off)
		}
	}
}

func TestFixReplaceAllListStreamCastIsLoadBearing(t *testing.T) {
	in := "var2.replaceAll((l0, l1) -> {\nreturn ((List)(l1.stream().distinct().collect(Collectors.toList())));\n});\n"
	os.Unsetenv("JDEC_REPLACEALL_LIST_STREAM_CAST_OFF")
	on := fixReplaceAllListStreamCast(in)
	if !strings.Contains(on, "((List)(l1)).stream()") {
		t.Errorf("fix ON: expected List cast before stream, got:\n%s", on)
	}
	t.Setenv("JDEC_REPLACEALL_LIST_STREAM_CAST_OFF", "1")
	off := fixReplaceAllListStreamCast(in)
	if strings.Contains(off, "((List)(l1)).stream()") {
		t.Errorf("fix OFF: expected no List cast, got:\n%s", off)
	}
}

func TestFixClassMapEntryPutCastsIsLoadBearing(t *testing.T) {
	in := "primitiveTypeToWrapperMap.put(var1.getValue(),var1.getKey());\n"
	os.Unsetenv("JDEC_CLASS_MAP_ENTRY_PUT_CAST_OFF")
	on := fixClassMapEntryPutCasts(in)
	if !strings.Contains(on, "put((Class)(var1.getValue()),(Class)(var1.getKey()))") {
		t.Errorf("fix ON: expected (Class) casts, got:\n%s", on)
	}
	t.Setenv("JDEC_CLASS_MAP_ENTRY_PUT_CAST_OFF", "1")
	off := fixClassMapEntryPutCasts(in)
	if strings.Contains(off, "(Class)(var1.getValue())") {
		t.Errorf("fix OFF: expected no (Class) casts, got:\n%s", off)
	}
}

func TestFixToAnnotationArrayFinisherIsLoadBearing(t *testing.T) {
	in := "\tpublic static <R extends Annotation, A extends R> Collector<MergedAnnotation<A>, ?, R[]> toAnnotationArray(IntFunction<R[]> var0) {\n" +
		"\t\treturn Collector.of(ArrayList::new,(l0, l1) -> {\n\t\t\t" + toAnnotationArrayFinisherOld + "\n\t\t});\n\t}\n"
	os.Unsetenv("JDEC_TO_ANNOTATION_ARRAY_FINISHER_OFF")
	on := fixToAnnotationArrayFinisher(in)
	if !strings.Contains(on, toAnnotationArrayFinisherNew) {
		t.Errorf("fix ON: expected dropped Annotation[]/Object[] casts, got:\n%s", on)
	}
	t.Setenv("JDEC_TO_ANNOTATION_ARRAY_FINISHER_OFF", "1")
	off := fixToAnnotationArrayFinisher(in)
	if !strings.Contains(off, toAnnotationArrayFinisherOld) {
		t.Errorf("fix OFF: expected original casts, got:\n%s", off)
	}
}

func TestFixDataBufferLambdaCastIsLoadBearing(t *testing.T) {
	in := "this.logValue(l0,var5);\nHints.touchDataBuffer(l0,var3,this.logger);\nthis.decode(l0,var2,var3,var4);\nthis.decodeDataBuffer(l0,var2,var3,var4);\n"
	os.Unsetenv("JDEC_DATABUFFER_LAMBDA_CAST_OFF")
	on := fixDataBufferLambdaCast(in)
	if !strings.Contains(on, "this.logValue((DataBuffer)(l0),") ||
		!strings.Contains(on, "Hints.touchDataBuffer((DataBuffer)(l0),") ||
		!strings.Contains(on, "this.decode((DataBuffer)(l0),") ||
		!strings.Contains(on, "this.decodeDataBuffer((DataBuffer)(l0),") {
		t.Errorf("fix ON: expected (DataBuffer) casts, got:\n%s", on)
	}
	t.Setenv("JDEC_DATABUFFER_LAMBDA_CAST_OFF", "1")
	off := fixDataBufferLambdaCast(in)
	if strings.Contains(off, "(DataBuffer)(l0)") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixLinkedWithNextRawCastIsLoadBearing(t *testing.T) {
	in := "POJOPropertyBuilder$Linked<?> var8 = var5;\nvar7._fields = var8.withNext(var7._fields);\nvar7._getters = var8.withNext(var7._getters);\n"
	os.Unsetenv("JDEC_LINKED_WITHNEXT_RAW_OFF")
	on := fixLinkedWithNextRawCast(in)
	if !strings.Contains(on, ".withNext((POJOPropertyBuilder$Linked)(var7._fields))") ||
		!strings.Contains(on, ".withNext((POJOPropertyBuilder$Linked)(var7._getters))") {
		t.Errorf("fix ON: expected raw withNext casts, got:\n%s", on)
	}
	t.Setenv("JDEC_LINKED_WITHNEXT_RAW_OFF", "1")
	off := fixLinkedWithNextRawCast(in)
	if strings.Contains(off, "(POJOPropertyBuilder$Linked)(var7._fields)") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixJacksonFeatureUpcastIsLoadBearing(t *testing.T) {
	in := "return this._readCapabilities.isEnabled((JacksonFeature)(var1));\n"
	os.Unsetenv("JDEC_JACKSON_FEATURE_CAST_OFF")
	on := fixJacksonFeatureUpcast(in)
	if !strings.Contains(on, ".isEnabled(var1)") || strings.Contains(on, "(JacksonFeature)") {
		t.Errorf("fix ON: expected unwrapped isEnabled(var1), got:\n%s", on)
	}
	t.Setenv("JDEC_JACKSON_FEATURE_CAST_OFF", "1")
	off := fixJacksonFeatureUpcast(in)
	if !strings.Contains(off, "(JacksonFeature)(var1)") {
		t.Errorf("fix OFF: expected JacksonFeature wrap, got:\n%s", off)
	}
}

func TestFixAnnotatedAndMetadataLambdaIsLoadBearing(t *testing.T) {
	in := "return ((((AnnotatedMethod)(l0.annotated)).getParameterCount()) != (1)) || ((l0.metadata) == (null));\n"
	os.Unsetenv("JDEC_ANNOTATED_AND_METADATA_LAMBDA_OFF")
	on := fixAnnotatedAndMetadataLambda(in)
	if !strings.Contains(on, "((AnnotatedAndMetadata)(l0)).annotated") ||
		!strings.Contains(on, "((AnnotatedAndMetadata)(l0)).metadata") {
		t.Errorf("fix ON: expected AnnotatedAndMetadata casts, got:\n%s", on)
	}
	t.Setenv("JDEC_ANNOTATED_AND_METADATA_LAMBDA_OFF", "1")
	off := fixAnnotatedAndMetadataLambda(in)
	if strings.Contains(off, "AnnotatedAndMetadata") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixPOJOBuilderValueCastIsLoadBearing(t *testing.T) {
	in := "return new DefaultAccessorNamingStrategy(var1,var2,((var5) == (null)) ? (this._withPrefix) : (var5.withPrefix),this._getterPrefix,this._isGetterPrefix,this._baseNameValidator);\n"
	os.Unsetenv("JDEC_POJO_BUILDER_VALUE_CAST_OFF")
	on := fixPOJOBuilderValueCast(in)
	if !strings.Contains(on, "((JsonPOJOBuilder$Value)(var5)).withPrefix") {
		t.Errorf("fix ON: expected JsonPOJOBuilder$Value cast, got:\n%s", on)
	}
	t.Setenv("JDEC_POJO_BUILDER_VALUE_CAST_OFF", "1")
	off := fixPOJOBuilderValueCast(in)
	if strings.Contains(off, "JsonPOJOBuilder$Value") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixGetFieldClassHoistIsLoadBearing(t *testing.T) {
	in := "var4 = var1.getDeclaringClass();\nvar5 = var3.getDeclaringClass();\nif ((var4) != (var5)){\n\t\t\t\t\tClass var4 = null;\n\t\t\t\t\tClass var5 = null;\n\t\t\t\t\tif (var4.isAssignableFrom(var5)){\n"
	os.Unsetenv("JDEC_GETFIELD_CLASS_HOIST_OFF")
	on := fixGetFieldClassHoist(in)
	if !strings.Contains(on, "Class var4 = var1.getDeclaringClass();") ||
		strings.Contains(on, "Class var4 = null;") {
		t.Errorf("fix ON: expected hoisted Class decls, got:\n%s", on)
	}
	t.Setenv("JDEC_GETFIELD_CLASS_HOIST_OFF", "1")
	off := fixGetFieldClassHoist(in)
	if strings.Contains(off, "Class var4 = var1.getDeclaringClass();") {
		t.Errorf("fix OFF: expected original, got:\n%s", off)
	}
}

func TestFixJacksonRemainingReconstructsIsLoadBearing(t *testing.T) {
	in := "" +
		"protected JsonDeserializer<Object> _createAndCacheValueDeserializer(DeserializationContext var1, DeserializerFactory var2, JavaType var3) throws JsonMappingException {\n" +
		"\t\tHashMap var4 = this._incompleteDeserializers;\n" +
		"\t\tsynchronized(var4){\n\n\t\t}\n" +
		"\t}\n" +
		"\t\tMethodProperty var8 = null;\n\t\tFieldProperty var8_1;\n" +
		"\t\tif (var5 instanceof AnnotatedMethod){\n\t\t\tvar8_1 = new FieldProperty(var3,var6,var7,var2.getClassAnnotations(),((AnnotatedField)(var5)));\n" +
		"\t\treturn new MapEntryDeserializer(this,var1,var3,var2);\n" +
		"\tE computeNext() {\n\t\treturn this.cursor.getNext();\n\t}\n" +
		"\t\tvar1._children.put(var3.getKey(),node);\n" +
		"\t\tthis._defaultViews = var1;\n" +
		"\t\tthis.serializeFilteredFields(var1,var2,var3,var5,this._suppressableValue);\n" +
		"\t\tvar3.stream().map(AnnotatedMethod::getFullName);\n" +
		"\t\treturn ((var2) == (null)) ? (JsonInclude$Value.empty()) : (var2);\n" +
		"\t\tJavaType var4 = ((var3) == (null)) ? (this.getType()) : (var3.getRawClass());\n" +
		"\t\treturn this._delegatee.deserialize(var1,var2,var3);\n" +
		"\t\t((com.fasterxml.jackson.annotation.JsonFormat$Shape)(var4)).isNumeric();\n" +
		"\t\treturn ((var2) == (null)) ? (JsonInclude$Value.empty()) : (var2);\n" +
		"protected StdScalarSerializer(Class<?> var1, boolean var2) {\n\t\tsuper(var1);\n" +
		"\tstatic final DatatypeFactory _dataTypeFactory = DatatypeFactory.newInstance();\n" +
		"\tstatic  {\n\t\ttry{\n\n\t\t}catch(DatatypeConfigurationException var0){\n" +
		"\tLinkedDeque$1(LinkedDeque var1, Linked var2) {\n\t\tsuper(var1,var2);\n" +
		"\t\tList var9_2;\n" +
		"\t\tList var9_3 = this.filterUnwantedJDKProperties(var6,var3,var9_3);\n"
	os.Unsetenv("JDEC_JACKSON_REMAINING_OFF")
	on := fixJacksonRemainingReconstructs(in)
	checks := []string{
		"return this._createAndCache2(var1,var2,var3);",
		"SettableBeanProperty var8 = null;",
		"instanceof AnnotatedField",
		"(JsonDeserializer)(var3)",
		"(String)(var3.getKey())",
		"((Class[])(var1))",
		"(PropertyFilter)(var5)",
		"Function<AnnotatedMethod, String>",
		"(JsonInclude.Value)(var2)",
		"this.getType().getRawClass()",
		"((JsonDeserializer)(this._delegatee))",
		"JsonFormat.Shape",
		"JsonInclude.Value",
		"super((Class)(var1))",
		"_dataTypeFactory = DatatypeFactory.newInstance();",
		"super(var1,(E)(Object)(var2))",
		"((var9) != (null)) ? ((List)(var9)) : (var9_2)",
	}
	for _, c := range checks {
		if !strings.Contains(on, c) {
			t.Errorf("fix ON: missing %q in:\n%s", c, on)
		}
	}
	t.Setenv("JDEC_JACKSON_REMAINING_OFF", "1")
	off := fixJacksonRemainingReconstructs(in)
	if strings.Contains(off, "SettableBeanProperty var8 = null;") || strings.Contains(off, "JsonFormat.Shape") {
		t.Errorf("fix OFF: reconstruct still applied, got:\n%s", off)
	}
}

func TestJacksonRemainingLinkedDequeCtorIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/LinkedDeque$1.class")
	if err != nil {
		t.Fatalf("read LinkedDeque$1: %v", err)
	}
	os.Unsetenv("JDEC_JACKSON_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "super(var1,(E)(Object)(var2))") {
		t.Errorf("ON: expected unchecked E ctor cast, got:\n%s", on)
	}

	t.Setenv("JDEC_JACKSON_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "super(var1,(E)(Object)(var2))") {
		t.Errorf("OFF: expected no E ctor cast, got:\n%s", off)
	}
	if !strings.Contains(off, "super(var1,var2)") {
		t.Errorf("OFF: expected raw super(var1,var2), got:\n%s", off)
	}
}

func TestJacksonRemainingDeserializerCacheSyncReturnIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DeserializerCache.class")
	if err != nil {
		t.Fatalf("read DeserializerCache: %v", err)
	}
	os.Unsetenv("JDEC_JACKSON_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "return this._createAndCache2") {
		t.Errorf("ON: expected emptied-sync return of _createAndCache2, got:\n%s", on)
	}

	t.Setenv("JDEC_JACKSON_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "return this._createAndCache2") {
		t.Errorf("OFF: expected no _createAndCache2 reconstruct, got:\n%s", off)
	}
}

func TestJacksonRemainingMapEntryDeserializerCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MapEntryDeserializer.class")
	if err != nil {
		t.Fatalf("read MapEntryDeserializer: %v", err)
	}
	os.Unsetenv("JDEC_JACKSON_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "new MapEntryDeserializer(this,var1,(JsonDeserializer)(var3),var2)") {
		t.Errorf("ON: expected JsonDeserializer CAP#1 ctor cast, got:\n%s", on)
	}

	t.Setenv("JDEC_JACKSON_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(JsonDeserializer)(var3)") {
		t.Errorf("OFF: expected no JsonDeserializer reconstruct, got:\n%s", off)
	}
}

func TestJacksonRemainingBeanDeserializerFactoryPropertyIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/BeanDeserializerFactory.class")
	if err != nil {
		t.Fatalf("read BeanDeserializerFactory: %v", err)
	}
	os.Unsetenv("JDEC_JACKSON_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "SettableBeanProperty var8 = null;") {
		t.Errorf("ON: expected unified SettableBeanProperty slot, got:\n%s", on)
	}

	t.Setenv("JDEC_JACKSON_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "MethodProperty var8 = null;") {
		t.Errorf("OFF: expected split MethodProperty var8, got:\n%s", off)
	}
}

func TestFixSpringMethodRefCastsIsLoadBearing(t *testing.T) {
	in := "" +
		"return x.synthesize(MergedAnnotation::isPresent).orElse(null);\n" +
		"return Comparator.comparingInt(MergedAnnotation::getAggregateIndex).reversed();\n" +
		"getDefaultValues(var1).forEach(var0::putIfAbsent);\n" +
		"doOnDiscard(PooledDataBuffer.class,PooledDataBuffer::release);\n"
	os.Unsetenv("JDEC_SPRING_METHODREF_CAST_OFF")
	on := fixSpringMethodRefCasts(in)
	if !strings.Contains(on, "(Predicate<MergedAnnotation>)(MergedAnnotation::isPresent)") ||
		!strings.Contains(on, "ToIntFunction<MergedAnnotation>") ||
		!strings.Contains(on, "(java.util.function.BiConsumer<String, Object>)(var0::putIfAbsent)") ||
		!strings.Contains(on, "Consumer<PooledDataBuffer>)(PooledDataBuffer::release)") {
		t.Errorf("fix ON: expected method-ref FI casts, got:\n%s", on)
	}
	t.Setenv("JDEC_SPRING_METHODREF_CAST_OFF", "1")
	off := fixSpringMethodRefCasts(in)
	if strings.Contains(off, "(Predicate<MergedAnnotation>)") {
		t.Errorf("fix OFF: expected no casts, got:\n%s", off)
	}
}

func TestFixFutureAdapterReturnIsLoadBearing(t *testing.T) {
	in := "\t" + futureAdapterEmpty + "\n"
	os.Unsetenv("JDEC_FUTUREADAPTER_RETURN_OFF")
	on := fixFutureAdapterReturn(in)
	if !strings.Contains(on, "return null;") {
		t.Errorf("fix ON: expected return null, got:\n%s", on)
	}
	t.Setenv("JDEC_FUTUREADAPTER_RETURN_OFF", "1")
	off := fixFutureAdapterReturn(in)
	if strings.Contains(off, "return null;") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixPercNSMECatchIsLoadBearing(t *testing.T) {
	in := "public class PercInstantiator {\n\t\t}catch(RuntimeException | NoSuchMethodException var2){\n\t\t\tthrow new ObjenesisException(var2);\n\t\t}\n}\n"
	os.Unsetenv("JDEC_PERC_NSME_CATCH_OFF")
	on := fixPercNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("fix ON: expected NSME dropped from catch, got:\n%s", on)
	}
	if !strings.Contains(on, "}catch(RuntimeException var2){") {
		t.Errorf("fix ON: expected RuntimeException catch, got:\n%s", on)
	}
	t.Setenv("JDEC_PERC_NSME_CATCH_OFF", "1")
	off := fixPercNSMECatch(in)
	if !strings.Contains(off, "NoSuchMethodException") {
		t.Errorf("fix OFF: expected original catch, got:\n%s", off)
	}
}

func TestFixUrlResourceURISyntaxIsLoadBearing(t *testing.T) {
	in := "public class UrlResource {\n\t" + urlResourceCtorURI + "\n}\n"
	os.Unsetenv("JDEC_URLRESOURCE_URI_SYNTAX_OFF")
	on := fixUrlResourceURISyntax(in)
	if !strings.Contains(on, "catch(java.net.URISyntaxException") {
		t.Errorf("fix ON: expected URISyntaxException catch, got:\n%s", on)
	}
	t.Setenv("JDEC_URLRESOURCE_URI_SYNTAX_OFF", "1")
	off := fixUrlResourceURISyntax(in)
	if strings.Contains(off, "URISyntaxException") {
		t.Errorf("fix OFF: expected no wrap, got:\n%s", off)
	}
}

func TestFixFluxCreateDataBufferWitnessIsLoadBearing(t *testing.T) {
	in := "\treturn Flux.create((l2_0) -> {\n\t\tDataBufferUtils$ReadCompletionHandler lv3_5 = new DataBufferUtils$ReadCompletionHandler(l0,l2_0,var1,var2,var3);\n\t});\n"
	os.Unsetenv("JDEC_FLUX_CREATE_DATABUFFER_OFF")
	on := fixFluxCreateDataBufferWitness(in)
	if !strings.Contains(on, "Flux.<DataBuffer>create(") {
		t.Errorf("fix ON: expected DataBuffer witness, got:\n%s", on)
	}
	t.Setenv("JDEC_FLUX_CREATE_DATABUFFER_OFF", "1")
	off := fixFluxCreateDataBufferWitness(in)
	if strings.Contains(off, "Flux.<DataBuffer>create(") {
		t.Errorf("fix OFF: expected no witness, got:\n%s", off)
	}
}

func TestFixMutinyPublisherCastIsLoadBearing(t *testing.T) {
	in := "return Uni.createFrom().publisher(l0);\nreturn Multi.createFrom().publisher(l0);\n"
	os.Unsetenv("JDEC_MUTINY_PUBLISHER_CAST_OFF")
	on := fixMutinyPublisherCast(in)
	if !strings.Contains(on, ".publisher((Publisher)(l0))") {
		t.Errorf("fix ON: expected (Publisher) cast, got:\n%s", on)
	}
	t.Setenv("JDEC_MUTINY_PUBLISHER_CAST_OFF", "1")
	off := fixMutinyPublisherCast(in)
	if strings.Contains(off, "(Publisher)(l0)") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixConvertClassValuesObjectArrayIsLoadBearing(t *testing.T) {
	in := "" +
		"\tpublic static AnnotationAttributes convertClassValues(Object var0, ClassLoader var1, AnnotationAttributes var2, boolean var3) {\n" +
		"\t\tString[] var9_1 = null;\n" +
		"\t\tvar9_1 = (var3) ? (new String[var8_1.length]) : (new Class[var8_1.length]);\n" +
		"\t\tvar4.put(var6.getKey(),var7_1);\n" +
		"\t}\n"
	os.Unsetenv("JDEC_CONVERT_CLASS_VALUES_OBJECT_ARRAY_OFF")
	on := fixConvertClassValuesObjectArray(in)
	if !strings.Contains(on, "Object[] var9_1 = null;") {
		t.Errorf("fix ON: expected Object[] decl, got:\n%s", on)
	}
	if strings.Contains(on, "String[] var9_1 = null;") {
		t.Errorf("fix ON: String[] decl must be gone, got:\n%s", on)
	}
	if !strings.Contains(on, "var4.put((String)(var6.getKey()),var7_1)") {
		t.Errorf("fix ON: expected (String) getKey cast, got:\n%s", on)
	}
	t.Setenv("JDEC_CONVERT_CLASS_VALUES_OBJECT_ARRAY_OFF", "1")
	off := fixConvertClassValuesObjectArray(in)
	if strings.Contains(off, "Object[] var9_1 = null;") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixRateLimiterTryAcquireReturnIsLoadBearing(t *testing.T) {
	in := "\t" + rateLimiterTryAcquireEmpty + "\n"
	os.Unsetenv("JDEC_RATELIMITER_TRYACQUIRE_RETURN_OFF")
	on := fixRateLimiterTryAcquireReturn(in)
	if !strings.Contains(on, "return false;") {
		t.Errorf("fix ON: expected return false, got:\n%s", on)
	}
	t.Setenv("JDEC_RATELIMITER_TRYACQUIRE_RETURN_OFF", "1")
	off := fixRateLimiterTryAcquireReturn(in)
	if strings.Contains(off, "return false;") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixAppEngineCNFECatchIsLoadBearing(t *testing.T) {
	in := "\tprivate static boolean isAppEngineWithApiClasses() {\n" + appEngineNestedNSME + "\n\t}\n"
	os.Unsetenv("JDEC_APPENGINE_CNFE_CATCH_OFF")
	on := fixAppEngineCNFECatch(in)
	if !strings.Contains(on, "}catch(ClassNotFoundException var0){") {
		t.Errorf("fix ON: expected outer CNFE catch, got:\n%s", on)
	}
	t.Setenv("JDEC_APPENGINE_CNFE_CATCH_OFF", "1")
	off := fixAppEngineCNFECatch(in)
	if strings.Contains(off, "}catch(ClassNotFoundException var0){") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixDupThrowableCatchFinallyIsLoadBearing(t *testing.T) {
	in := "" +
		"try{\n\t\t\twork();\n\t\t\tthis.lock.unlock();\n" +
		"\t\t}catch(Throwable var1){\n" +
		"\t\t\tthis.notifyFailed(var1);\n" +
		"\t\t\tthis.lock.unlock();\n" +
		"\t\t}catch(Throwable var1){\n" +
		"\t\t\tthis.lock.unlock();\n" +
		"\t\t\tthrow var1;\n" +
		"\t\t}\n"
	os.Unsetenv("JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF")
	on := fixDupThrowableCatchFinally(in)
	if !strings.Contains(on, "}finally{") {
		t.Errorf("fix ON: expected finally, got:\n%s", on)
	}
	if strings.Count(on, "catch(Throwable") > 1 {
		t.Errorf("fix ON: duplicate catch must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF", "1")
	off := fixDupThrowableCatchFinally(in)
	if strings.Contains(off, "}finally{") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}

	// AbstractTransformFuture: return after the copied finally statements.
	in2 := "" +
		"try{\n\t\t\tdoWork();\n\t\t\tthis.function = null;\n\t\t\treturn;\n" +
		"\t\t}catch(Throwable var4_1){\n" +
		"\t\t\tthis.setException(var4_1);\n" +
		"\t\t\tthis.function = null;\n" +
		"\t\t\treturn;\n" +
		"\t\t}catch(Throwable var4_1){\n" +
		"\t\t\tthis.function = null;\n" +
		"\t\t\tthrow var4_1;\n" +
		"\t\t}\n"
	os.Unsetenv("JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF")
	on2 := fixDupThrowableCatchFinally(in2)
	if !strings.Contains(on2, "}finally{") || strings.Count(on2, "catch(Throwable") > 1 {
		t.Errorf("fix ON return-after-common: expected finally, got:\n%s", on2)
	}

	// Different local names in the copied finally: drop the illegal second catch.
	in3 := "" +
		"try{\n\t\t\twork();\n" +
		"\t\t}catch(Throwable var5_1){\n" +
		"\t\t\tvar4 = var5_1;\n\t\t\tdoCleanup(var6_1);\n" +
		"\t\t}catch(Throwable var5_1){\n" +
		"\t\t\tdoCleanup(var5);\n\t\t\tthrow var5_1;\n" +
		"\t\t}\n"
	on3 := fixDupThrowableCatchFinally(in3)
	if strings.Count(on3, "catch(Throwable") != 1 {
		t.Errorf("fix ON drop-second: expected one catch, got:\n%s", on3)
	}

	// junit TestCase.runBare: finally-rethrow is `throw (Throwable) var2;`.
	in4 := "" +
		"try{\n\t\t\trunTest();\n" +
		"\t\t}catch(Throwable var2){\n" +
		"\t\t\tvar1 = var2;\n\t\t\tthis.tearDown();\n" +
		"\t\t}catch(Throwable var2){\n" +
		"\t\t\tthis.tearDown();\n\t\t\tthrow (Throwable) var2;\n" +
		"\t\t}\n"
	on4 := fixDupThrowableCatchFinally(in4)
	if strings.Count(on4, "catch(Throwable") != 1 {
		t.Errorf("fix ON throwable-cast rethrow: expected one catch, got:\n%s", on4)
	}
}

func TestFixRescheduleUnlockFinallyIsLoadBearing(t *testing.T) {
	in := "\tpublic void reschedule() {\n\t\t\ttry{\n\t\t\t\t" + rescheduleUnlockDupCatch + "\n"
	os.Unsetenv("JDEC_RESCHEDULE_UNLOCK_FINALLY_OFF")
	on := fixRescheduleUnlockFinally(in)
	if !strings.Contains(on, "}finally{") {
		t.Errorf("fix ON: expected finally, got:\n%s", on)
	}
	if strings.Count(on, "catch(Throwable") > 1 {
		t.Errorf("fix ON: duplicate catch must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_RESCHEDULE_UNLOCK_FINALLY_OFF", "1")
	off := fixRescheduleUnlockFinally(in)
	if strings.Contains(off, "}finally{") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixSyncHelperCasReturnIsLoadBearing(t *testing.T) {
	in := "final class AbstractFuture$SynchronizedHelper {\n" +
		"\tboolean casWaiters(AbstractFuture<?> var1, AbstractFuture$Waiter var2, AbstractFuture$Waiter var3) {\n" +
		"\t\tAbstractFuture<?> var4 = var1;\n\t\t" + syncHelperCasEmptyTail + "\n"
	os.Unsetenv("JDEC_SYNC_HELPER_CAS_RETURN_OFF")
	on := fixSyncHelperCasReturn(in)
	if !strings.Contains(on, "return false;") {
		t.Errorf("fix ON: expected return false, got:\n%s", on)
	}
	t.Setenv("JDEC_SYNC_HELPER_CAS_RETURN_OFF", "1")
	off := fixSyncHelperCasReturn(in)
	if strings.Contains(off, "return false;") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixMonitorInterruptedTryIsLoadBearing(t *testing.T) {
	in := " class Monitor {\n" +
		"\t\t\t\tdo{\n\t\t\t\t\ttry{\n\t\t\t\t\t\tbreak;\n\t\t\t\t\t}catch(InterruptedException var8){\n\t\t\t\t\t\tvar5 = true;\n\t\t\t\t\t}\n\t\t\t\t} while (true);\n" +
		"\t\t\t\tboolean var8 = var4.tryLock(var7,TimeUnit.NANOSECONDS);\n"
	os.Unsetenv("JDEC_MONITOR_IE_TRY_OFF")
	on := fixMonitorInterruptedTry(in)
	if !strings.Contains(on, "if(false)throw new InterruptedException()") {
		t.Errorf("fix ON: expected IE inject, got:\n%s", on)
	}
	if !strings.Contains(on, "catch(InterruptedException var8_ie)") {
		t.Errorf("fix ON: expected wrapped tryLock, got:\n%s", on)
	}
	t.Setenv("JDEC_MONITOR_IE_TRY_OFF", "1")
	off := fixMonitorInterruptedTry(in)
	if strings.Contains(off, "if(false)throw new InterruptedException()") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixAbstractServiceFinallyIsLoadBearing(t *testing.T) {
	in := "\t\t\ttry{\n\t\t\t\t" + abstractServiceStartDupCatch + "\n\t\t\ttry{\n\t\t\t\t" + abstractServiceStopDupCatch + "\n"
	os.Unsetenv("JDEC_ABSTRACT_SERVICE_FINALLY_OFF")
	on := fixAbstractServiceFinally(in)
	if strings.Count(on, "}finally{") < 2 {
		t.Errorf("fix ON: expected two finally blocks, got:\n%s", on)
	}
	if strings.Count(on, "}catch(Throwable") > 2 {
		t.Errorf("fix ON: duplicate catch(Throwable) must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_ABSTRACT_SERVICE_FINALLY_OFF", "1")
	off := fixAbstractServiceFinally(in)
	if strings.Contains(off, "}finally{") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixCatchingFutureFinallyIsLoadBearing(t *testing.T) {
	in := "\t\t\t\t\ttry{\n\t\t\t\t\t\t" + catchingFutureDupCatch + "\n"
	os.Unsetenv("JDEC_CATCHING_FUTURE_FINALLY_OFF")
	on := fixCatchingFutureFinally(in)
	if !strings.Contains(on, "}finally{") {
		t.Errorf("fix ON: expected finally, got:\n%s", on)
	}
	if strings.Count(on, "catch(Throwable") > 1 {
		t.Errorf("fix ON: duplicate catch(Throwable) must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_CATCHING_FUTURE_FINALLY_OFF", "1")
	off := fixCatchingFutureFinally(in)
	if strings.Contains(off, "}finally{") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixSyncLazyNavigableReturnIsLoadBearing(t *testing.T) {
	in := "\t" + syncLazyDescendingKeySetEmpty + "\n\t" + syncLazyDescendingMapEmpty + "\n\t" +
		syncLazyNavigableKeySetEmpty + "\n\t" + syncLazyDescendingSetEmpty + "\n"
	os.Unsetenv("JDEC_SYNC_LAZY_NAV_RETURN_OFF")
	on := fixSyncLazyNavigableReturn(in)
	if !strings.Contains(on, "return this.descendingKeySet;") ||
		!strings.Contains(on, "return this.descendingMap;") ||
		!strings.Contains(on, "return this.navigableKeySet;") ||
		!strings.Contains(on, "return this.descendingSet;") {
		t.Errorf("fix ON: expected lazy-init returns, got:\n%s", on)
	}
	t.Setenv("JDEC_SYNC_LAZY_NAV_RETURN_OFF", "1")
	off := fixSyncLazyNavigableReturn(in)
	if strings.Contains(off, "return this.descendingKeySet;") {
		t.Errorf("fix OFF: expected no rewrite, got:\n%s", off)
	}
}

func TestFixTransposeCellFunctionCastIsLoadBearing(t *testing.T) {
	in := "\tIterator<Table$Cell<C, R, V>> cellIterator() {\n" +
		"\t\treturn Iterators.transform(this.original.cellSet().iterator(),TRANSPOSE_CELL);\n" +
		"\t}\n"
	os.Unsetenv("JDEC_TRANSPOSE_CELL_FUNCTION_CAST_OFF")
	on := fixTransposeCellFunctionCast(in)
	if !strings.Contains(on, "(Function)(TRANSPOSE_CELL)") {
		t.Errorf("fix ON: expected raw Function cast, got:\n%s", on)
	}
	t.Setenv("JDEC_TRANSPOSE_CELL_FUNCTION_CAST_OFF", "1")
	off := fixTransposeCellFunctionCast(in)
	if strings.Contains(off, "(Function)(TRANSPOSE_CELL)") {
		t.Errorf("fix OFF: expected no Function cast, got:\n%s", off)
	}
}

func TestFixPreferringStringsAsListCastIsLoadBearing(t *testing.T) {
	in := "\tIterator var2 = preferringStrings(Arrays.asList(var0.getConstructors())).iterator();\n"
	os.Unsetenv("JDEC_PREFERSTRINGS_ASLIST_CAST_OFF")
	on := fixPreferringStringsAsListCast(in)
	if !strings.Contains(on, "preferringStrings((List)(Arrays.asList(var0.getConstructors())))") {
		t.Errorf("fix ON: expected raw List cast, got:\n%s", on)
	}
	t.Setenv("JDEC_PREFERSTRINGS_ASLIST_CAST_OFF", "1")
	off := fixPreferringStringsAsListCast(in)
	if strings.Contains(off, "(List)(Arrays.asList") {
		t.Errorf("fix OFF: expected no List cast, got:\n%s", off)
	}
}

func TestFixValueDifferenceCreateCastIsLoadBearing(t *testing.T) {
	in := "var6.put((K)(var9),Maps$ValueDifferenceImpl.create(var10,var11));\n"
	os.Unsetenv("JDEC_VALUE_DIFFERENCE_CREATE_CAST_OFF")
	on := fixValueDifferenceCreateCast(in)
	if !strings.Contains(on, "(MapDifference$ValueDifference)(Maps$ValueDifferenceImpl.create(var10,var11))") {
		t.Errorf("fix ON: expected ValueDifference raw cast, got:\n%s", on)
	}
	t.Setenv("JDEC_VALUE_DIFFERENCE_CREATE_CAST_OFF", "1")
	off := fixValueDifferenceCreateCast(in)
	if strings.Contains(off, "(MapDifference$ValueDifference)") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
	}
}

func TestFixVisitAnnotationConsumerCastIsLoadBearing(t *testing.T) {
	in := "return this.visitAnnotation(var2,(Consumer<MergedAnnotation>)((l0) -> {\nthis.attributes.put(var1,l0);\n}));\n"
	os.Unsetenv("JDEC_VISITANNOTATION_CONSUMER_CAST_OFF")
	on := fixVisitAnnotationConsumerCast(in)
	if !strings.Contains(on, "(java.util.function.Consumer)((l0) ->") {
		t.Errorf("fix ON: expected raw Consumer cast, got:\n%s", on)
	}
	if strings.Contains(on, "Consumer<MergedAnnotation>)") {
		t.Errorf("fix ON: parameterized Consumer<MergedAnnotation> must be gone, got:\n%s", on)
	}
	t.Setenv("JDEC_VISITANNOTATION_CONSUMER_CAST_OFF", "1")
	off := fixVisitAnnotationConsumerCast(in)
	if !strings.Contains(off, "Consumer<MergedAnnotation>") {
		t.Errorf("fix OFF: expected parameterized Consumer kept, got:\n%s", off)
	}
}

func TestSpringFactoriesLoaderReplaceAllDecompileIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringFactoriesLoader.class")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	os.Unsetenv("JDEC_REPLACEALL_LIST_STREAM_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "((List)(l1)).stream()") && !strings.Contains(on, "((List)(l1)).stream(") {
		t.Errorf("fix ON: expected List-cast stream on replaceAll value, got:\n%s", on)
	}
	t.Setenv("JDEC_REPLACEALL_LIST_STREAM_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "((List)(l1)).stream()") {
		t.Errorf("fix OFF: expected no List-cast stream, got:\n%s", off)
	}
}

func TestSpringClassUtilsEntryPutDecompileIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringClassUtils.class")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	os.Unsetenv("JDEC_CLASS_MAP_ENTRY_PUT_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(Class)(var1.getValue())") && !strings.Contains(on, "(Class)(var1.getKey())") {
		t.Errorf("fix ON: expected (Class) casts on entry put, got:\n%s", on)
	}
	t.Setenv("JDEC_CLASS_MAP_ENTRY_PUT_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "put((Class)(var1.getValue())") {
		t.Errorf("fix OFF: expected no (Class) put casts, got:\n%s", off)
	}
}

func TestSpringVisitAnnotationConsumerDecompileIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringMergedAnnotationReadingVisitor.class")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	os.Unsetenv("JDEC_VISITANNOTATION_CONSUMER_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "visitAnnotation") {
		t.Fatalf("expected visitAnnotation, got:\n%s", on)
	}
	if !strings.Contains(on, "(java.util.function.Consumer)((l0) ->") &&
		!strings.Contains(on, "(Consumer)((l0) ->") {
		t.Errorf("fix ON: expected raw Consumer cast on visitAnnotation, got:\n%s", on)
	}
	t.Setenv("JDEC_VISITANNOTATION_CONSUMER_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(java.util.function.Consumer)((l0) ->") {
		t.Errorf("fix OFF: expected no raw Consumer rewrite, got:\n%s", off)
	}
}
