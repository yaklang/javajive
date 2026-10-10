package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func repeatedClosedBodyFixture(calls int, prefix string) string {
	var body strings.Builder
	for i := 0; i < calls; i++ {
		body.WriteString("step(n,w,input);")
	}
	body.WriteString("ClosedEffects.mark();if(fail)throw ClosedEffects.failure;")
	f := strings.ReplaceAll(closedCalleeFixture, "CALL", body.String())
	f = strings.ReplaceAll(f, "METHOD", "private void step(int n,long w,Object input){count=n;wide=w;this$0=input;reference=input;result++;}")
	f = strings.ReplaceAll(f, "RESULT_CHECK", fmt.Sprintf("p.result!=%d||p.reference!=input", calls))
	return strings.ReplaceAll(f, "Closed", prefix)
}

// Keep the repeatedly called parent and its driver as unchanged classfiles.
// The counter witnesses EVERY original body execution; a proof cache changes
// only analysis work, never the source calls or their storage/effect order.
func TestAdversarialConstructorRepeatedClosedBodiesRetainEveryCallEffect(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "RepeatedBody"} {
				t.Run(prefix, func(t *testing.T) {
					testIndependentFlatClosedCalleeFamily(t, repeatedClosedBodyFixture(calls, prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

func TestNativeConstructorRepeatedClosedBodiesKeepOriginalFamilyShape(t *testing.T) {
	for _, calls := range []int{6, 10, 14} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			for _, prefix := range []string{"Closed", "RepeatedBody"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, repeatedClosedBodyFixture(calls, prefix), []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
