package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func handlerDeadLocalsFixture(calls int, prefix string) string {
	var body strings.Builder
	for i := 0; i < calls; i++ {
		fmt.Fprintf(&body, "scratch=%d;ClosedEffects.silent(scratch,input);", i)
	}
	method := "private void prepare(int n,long w,Object input,boolean fail){count=n;wide=w;this$0=input;int scratch=0;try{" + body.String() + "if(fail)throw ClosedEffects.failure;}finally{cleanup();}} private void cleanup(){ClosedEffects.mark();count^=0;wide^=0L;this$0=this$0;}"
	f := strings.ReplaceAll(closedCalleeFixture, "CALL", "prepare(n,w,input,fail)")
	f = strings.ReplaceAll(f, "METHOD", method)
	f = strings.ReplaceAll(f, "RESULT_CHECK", "p.result!=0||p.reference!=null")
	f = strings.ReplaceAll(f, `static void mark(){trace+="M";}`, `static void mark(){trace+="M";}static void silent(int word,Object input){}`)
	return strings.ReplaceAll(f, "Closed", prefix)
}

func TestAdversarialConstructorFinallyDeadLocalsDoNotRepeatCleanupProof(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CleanupState"} {
				t.Run(prefix, func(t *testing.T) {
					testIndependentFlatClosedCalleeFamily(t, handlerDeadLocalsFixture(calls, prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

func TestNativeConstructorFinallyDeadLocalsRetainOriginalFamilyShape(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CleanupState"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, handlerDeadLocalsFixture(calls, prefix), []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
