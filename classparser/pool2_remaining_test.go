package javaclassparser

import (
	"strings"
	"testing"
)

func TestPool2LinkedBlockingDequeThisFirstIsLoadBearing(t *testing.T) {
	// Four original same-owner constructor calls include the collection overload;
	// its lock/iteration effects must remain after delegation.
	assertReviewedConstructorDelegations(t, "testdata/regression/LinkedBlockingDeque.class", "LinkedBlockingDeque", "JDEC_POOL2_REMAINING_OFF", 4,
		"this(2147483647);", "this.lock.lock();")
}

func TestPool2GetGenericTypeSuperclassCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PoolImplUtils.class", "JDEC_POOL2_REMAINING_OFF",
		"getGenericType(var0,(Class)(var1.getSuperclass()))",
		"getGenericType(var0,var1.getSuperclass())")
}

func TestPool2EvictionPolicyNSMECatchIsLoadBearing(t *testing.T) {
	assertReviewedPoolReflectionCatchRegions(t)
}

func TestPool2SecurityManagerPrintlnIsLoadBearing(t *testing.T) {
	raw := reviewedRemainingSAMRaw(t, "SecurityManagerCallStack")
	assertReviewedRemainingSAMTarget(t, raw, "org/apache/commons/pool2/impl/SecurityManagerCallStack", "lambda$printStackTrace$1", "(Ljava/io/PrintWriter;Ljava/lang/ref/WeakReference;)V", "(Ljava/lang/Object;)V", "(Ljava/lang/ref/WeakReference;)V")
	assertReviewedTypeVarInvoke(t, "testdata/regression/SecurityManagerCallStack.class", "lambda$printStackTrace$1", "(Ljava/io/PrintWriter;Ljava/lang/ref/WeakReference;)V", 5, 182, "java/io/PrintWriter", "println", "(Ljava/lang/Object;)V")
	reviewedSeedSources(t, "testdata/regression/SecurityManagerCallStack.class", "JDEC_POOL2_REMAINING_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `boolean\s+printStackTrace\(`)
		carrier := requireReviewedPattern(t, body, `Consumer<WeakReference>\s+(\w+)\s*=\s*\((\w+)\)\s*->`)
		if !strings.Contains(body, ".println("+carrier[2]+".get());") || !strings.Contains(body, ".forEach("+carrier[1]+")") {
			t.Fatal("weak-reference SAM carrier detached from forEach")
		}
	})
}
