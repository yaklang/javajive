package javaclassparser

import "testing"

func TestArray2FuncCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Functions$Array2Func.class", "JDEC_RXJAVA_REMAINING_OFF",
		"this.f.apply((T1)(var1[0]),(T2)(var1[1]))",
		"this.f.apply(var1[0],var1[1])")
}

func TestFlowableMapOnNextUCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableMap$MapSubscriber.class", "JDEC_RXJAVA_REMAINING_OFF",
		"this.downstream.onNext((U)(var2))",
		"this.downstream.onNext(var2)")
}

func TestOpenHashSetKeysLocalTArrayIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/OpenHashSet.class", "JDEC_RXJAVA_REMAINING_OFF",
		"T[] var2 = this.keys",
		"Object[] var2 = this.keys")
}

func TestSwitchMapCancelledRawSentinelIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableSwitchMap$SwitchMapSubscriber.class", "JDEC_RXJAVA_REMAINING_OFF",
		"SwitchMapInnerSubscriber CANCELLED",
		"SwitchMapInnerSubscriber<Object, Object> CANCELLED")
}

func TestRxThreadFactoryThreadLubIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/RxThreadFactory.class", "JDEC_RXJAVA_REMAINING_OFF",
		"Thread var3 = (this.nonBlocking)",
		"RxThreadFactory$RxCustomThread var3 = (this.nonBlocking)")
}

func TestToListSingleCallableCastIsLoadBearing(t *testing.T) {
	// This test owns the legacy RxJava source rewrite; the typed constructor
	// binding pass has its own end-to-end test and kill switch.
	t.Setenv("JDEC_THIS_CTOR_OVERLOAD_CAST_OFF", "1")
	assertKillSwitchDecompile(t, "testdata/regression/FlowableToListSingle.class", "JDEC_RXJAVA_REMAINING_OFF",
		"this(var1,(Callable)(ArrayListSupplier.asCallable()))",
		"this(var1,ArrayListSupplier.asCallable())")
}

func TestNotificationLiteAcceptTCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/NotificationLite.class", "JDEC_RXJAVA_REMAINING_OFF",
		"var1.onNext((T)(var0))",
		"var1.onNext(var0)")
}

func TestMapConditionalTryOnNextUCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableMap$MapConditionalSubscriber.class", "JDEC_RXJAVA_REMAINING_OFF",
		"this.downstream.tryOnNext((U)(var2))",
		"this.downstream.tryOnNext(var2)")
}

func TestDematerializeGetValueRCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableDematerialize$DematerializeSubscriber.class", "JDEC_RXJAVA_REMAINING_OFF",
		"this.downstream.onNext((R)(var2.getValue()))",
		"this.downstream.onNext(var2.getValue())")
}

func TestScalarXMapZHelperApplyTCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ScalarXMapZHelper.class", "JDEC_RXJAVA_REMAINING_OFF",
		"var1.apply((T)(var5))",
		"var1.apply(var5)")
}

func TestParallelDispatcherDropsUnreachableClearIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ParallelFromPublisher$ParallelDispatcher.class", "JDEC_RXJAVA_REMAINING_OFF",
		"} while (true);\n\t}",
		"} while (true);\n\t\tvar2.clear();\n\t}")
}

func TestBehaviorProcessorDefaultCtorThisIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BehaviorProcessor.class", "JDEC_RXJAVA_REMAINING_OFF",
		"BehaviorProcessor(T var1) {\n\t\tthis();\n\t\tthis.value.lazySet",
		"BehaviorProcessor(T var1) {\n\t\tthis.value.lazySet")
}

func TestRxRecoveryPreservesDisjointQueueAndCounterIdentities(t *testing.T) {
	body := "package io.reactivex;\nclass FlowablePublish$PublishSubscriber {\n void scan() {Object var14;long var14_1=2;var14 = var5.poll();var14 = null;if (this.checkTerminated(var4,(var14) == (null))) return;} }"
	if got := fixRxjavaRemainingReconstructs(body); got != body {
		t.Fatalf("rewrote queue stores to an unrelated counter:\n%s", got)
	}
}

// A class formal does not establish a local's type argument. In particular,
// a raw subject accepting an erased Iterator.next must not acquire TRight from
// another member of the class. The core binding proof owns declaration views.
func TestRxRemainingKeepsUnprovedLocalArguments(t *testing.T) {
	for _, name := range []string{"GroupJoin$GroupJoinDisposable<TRight>", "SchedulerWhen<T>", "FlowableRepeatWhen<T>", "MulticastFlowable<U>", "Window<T>"} {
		body := "class " + name + " { void run() { UnicastSubject var10 = create(); var10.onNext(values.next()); FlowableProcessor var3 = create(); ConnectableFlowable var2 = create(); } }"
		if got := fixRxjavaRemainingReconstructs(body); got != body {
			t.Fatalf("unproved class-wide substitution for %s:\n%s", name, got)
		}
	}
}
