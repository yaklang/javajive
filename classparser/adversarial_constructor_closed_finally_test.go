package javaclassparser

import (
	"strings"
	"testing"
)

// Retain the original parent, exception identities, effect recorder and driver.
// Only the owner and child are rebuilt: the oracle cannot repair its own finally
// paths. Both normal/abrupt exits and a finally exception overriding a pending
// return/exception execute for every independent captured/reference/int/long row.
var closedFinallyBanks = []struct{ name, call, method, check string }{
	{"void cleanup", "prepare(n,w,input,fail)", `private void prepare(int n,long w,Object input,boolean fail){try{count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;}finally{ClosedEffects.finish();}}`, "p.result!=0||p.reference!=null"},
	{"return cleanup", "result=prepare(n,w,input,fail)", `final int prepare(int n,long w,Object input,boolean fail){try{count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;return n<0?-n:n;}finally{ClosedEffects.finish();}}`, "p.result!=(n<0?-n:n)||p.reference!=null"},
	{"nested cleanup", "reference=prepare(n,w,input,fail)", `private Object prepare(int n,long w,Object input,boolean fail){try{try{count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;return input;}finally{ClosedEffects.trace+="A";}}finally{ClosedEffects.finish();}}`, "p.result!=0||p.reference!=input"},
}

func closedFinallyFixture(row struct{ name, call, method, check string }, prefix string) string {
	f := strings.ReplaceAll(closedCalleeFixture, "CALL", row.call)
	f = strings.ReplaceAll(f, "METHOD", row.method)
	f = strings.ReplaceAll(f, "RESULT_CHECK", row.check)
	f = strings.ReplaceAll(f, `static void mark(){trace+="M";}`, `static boolean cleanupFail;static final RuntimeException cleanupFailure=new RuntimeException("cleanup");static void mark(){trace+="M";}static void finish(){trace+="F";if(cleanupFail)throw cleanupFailure;}`)
	f = strings.ReplaceAll(f, `ClosedEffects.trace="";`, `ClosedEffects.trace="";ClosedEffects.cleanupFail=w==Long.MIN_VALUE;`)
	trace := "PMF"
	if row.name == "nested cleanup" {
		trace = "PMAF"
	}
	f = strings.ReplaceAll(f, `if(fail||`, `if(fail||ClosedEffects.cleanupFail||`)
	f = strings.ReplaceAll(f, `"PMQ"`, `"`+trace+`Q"`)
	f = strings.ReplaceAll(f, `if(!fail||ex!=ClosedEffects.failure||!ClosedEffects.trace.equals("PM"))`, `if(!fail&&!ClosedEffects.cleanupFail||ex!=(ClosedEffects.cleanupFail?ClosedEffects.cleanupFailure:ClosedEffects.failure)||!ClosedEffects.trace.equals("`+trace+`"))`)
	return strings.ReplaceAll(f, "Closed", prefix)
}

func TestAdversarialConstructorClosedFinallyCalleesRetainCaptureAndAbruptOrder(t *testing.T) {
	for _, row := range closedFinallyBanks {
		t.Run(row.name, func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CleanupLedger"} {
				t.Run(prefix, func(t *testing.T) {
					testIndependentFlatClosedCalleeFamily(t, closedFinallyFixture(row, prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

func TestNativeConstructorClosedFinallyCalleesRetainOriginalFamilyShape(t *testing.T) {
	for _, row := range closedFinallyBanks {
		t.Run(row.name, func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CleanupLedger"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, closedFinallyFixture(row, prefix), []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
