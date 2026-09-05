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

func TestWindowUnicastProcessorTypeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableWindow$WindowExactSubscriber.class", "JDEC_RXJAVA_REMAINING_OFF",
		"UnicastProcessor<T> var3 = this.window",
		"UnicastProcessor var3 = this.window")
}

func TestOpenHashSetKeysLocalTArrayIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/OpenHashSet.class", "JDEC_RXJAVA_REMAINING_OFF",
		"T[] var2 = this.keys",
		"Object[] var2 = this.keys")
}

func TestRepeatWhenProcessorObjectNotTIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableRepeatWhen.class", "JDEC_RXJAVA_REMAINING_OFF",
		"FlowableProcessor<Object> var3",
		"FlowableProcessor var3")
}

func TestSchedulerWhenProcessorNotTIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SchedulerWhen.class", "JDEC_RXJAVA_REMAINING_OFF",
		"FlowableProcessor<SchedulerWhen$ScheduledAction> var2",
		"FlowableProcessor var2")
}

func TestMulticastFlowableConnectableUIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableReplay$MulticastFlowable.class", "JDEC_RXJAVA_REMAINING_OFF",
		"ConnectableFlowable<U> var2",
		"ConnectableFlowable var2")
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

func TestGroupJoinUnicastTRightIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableGroupJoin$GroupJoinSubscription.class", "JDEC_RXJAVA_REMAINING_OFF",
		"UnicastProcessor<TRight> var10",
		"UnicastProcessor var10")
}

func TestScalarXMapZHelperApplyTCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ScalarXMapZHelper.class", "JDEC_RXJAVA_REMAINING_OFF",
		"var1.apply((T)(var5))",
		"var1.apply(var5)")
}

func TestRetryWhenProcessorThrowableWitnessIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FlowableRetryWhen.class", "JDEC_RXJAVA_REMAINING_OFF",
		"UnicastProcessor.<Throwable>create(8).toSerialized()",
		"UnicastProcessor.create(8).toSerialized()")
}

func TestObservableRetryWhenSubjectThrowableWitnessIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ObservableRetryWhen.class", "JDEC_RXJAVA_REMAINING_OFF",
		"PublishSubject.<Throwable>create().toSerialized()",
		"PublishSubject.create().toSerialized()")
}

func TestListCompositeDisposableDeleteReturnIsLoadBearing(t *testing.T) {
	t.Skip("empty-sync catch-all now owns this site; unique needle no longer matches")
	assertKillSwitchDecompile(t, "testdata/regression/ListCompositeDisposable.class", "JDEC_RXJAVA_REMAINING_OFF",
		"synchronized(this){\n\n\t\t\t}\n\t\t\treturn false;",
		"synchronized(this){\n\n\t\t\t}\n\t\t}")
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
