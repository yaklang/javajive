package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func distinctClosedCallContextsFixture(calls int, prefix string) string {
	var body, methods strings.Builder
	for i := 0; i < calls; i++ {
		fmt.Fprintf(&body, "scope%d(n,w,input);", i)
		fmt.Fprintf(&methods, "private void scope%d(int n,long w,Object input){step(n,w,input);}", i)
	}
	body.WriteString("ClosedEffects.mark();if(fail)throw ClosedEffects.failure;")
	methods.WriteString("private void step(int n,long w,Object input){count=n;wide=w;this$0=input;reference=input;result++;}")
	f := strings.ReplaceAll(closedCalleeFixture, "CALL", body.String())
	f = strings.ReplaceAll(f, "METHOD", methods.String())
	f = strings.ReplaceAll(f, "RESULT_CHECK", fmt.Sprintf("p.result!=%d||p.reference!=input", calls))
	return strings.ReplaceAll(f, "Closed", prefix)
}
func TestAdversarialConstructorClosedLeafProofSurvivesIndependentCallContexts(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CallForest"} {
				t.Run(prefix, func(t *testing.T) {
					testIndependentFlatClosedCalleeFamily(t, distinctClosedCallContextsFixture(calls, prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}
func TestNativeConstructorClosedLeafContextsKeepAllOriginalExecutions(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "CallForest"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, distinctClosedCallContextsFixture(calls, prefix), []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
