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
	in := "this.logValue(l0,var5);\nHints.touchDataBuffer(l0,var3,this.logger);\nthis.decode(l0,var2,var3,var4);\n"
	os.Unsetenv("JDEC_DATABUFFER_LAMBDA_CAST_OFF")
	on := fixDataBufferLambdaCast(in)
	if !strings.Contains(on, "this.logValue((DataBuffer)(l0),") ||
		!strings.Contains(on, "Hints.touchDataBuffer((DataBuffer)(l0),") ||
		!strings.Contains(on, "this.decode((DataBuffer)(l0),") {
		t.Errorf("fix ON: expected (DataBuffer) casts, got:\n%s", on)
	}
	t.Setenv("JDEC_DATABUFFER_LAMBDA_CAST_OFF", "1")
	off := fixDataBufferLambdaCast(in)
	if strings.Contains(off, "(DataBuffer)(l0)") {
		t.Errorf("fix OFF: expected no cast, got:\n%s", off)
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
