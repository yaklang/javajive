package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorFinalReceiverClosesUnshadowedMethods(t *testing.T) {
	for _, row := range closedCalleeBanks {
		t.Run(row.name, func(t *testing.T) {
			for _, prefix := range []string{"Closed", "OtherClosed"} {
				t.Run(prefix, func(t *testing.T) {
					method := strings.ReplaceAll(strings.ReplaceAll(row.method, "private ", "public "), "final ", "protected ")
					f := strings.ReplaceAll(closedCalleeFixture, "CALL", row.call)
					f = strings.ReplaceAll(f, "METHOD", method)
					f = strings.ReplaceAll(f, "RESULT_CHECK", row.check)
					f = strings.ReplaceAll(f, "Closed", prefix)
					testIndependentFlatClosedCalleeFamily(t, f, prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

// This holdout adds intermediate classes and methods sharing the name with
// different descriptors, plus short primitive/wide/reference parameter reads.
// The complete parent chain and original driver stay outside the rebuilt set.
func TestAdversarialConstructorFinalReceiverKeepsAncestryAndReadOnlyBindings(t *testing.T) {
	method := `public void prepare(int n,long w,Object input,boolean fail){count=read(n);wide=readWide(w);this$0=readRef(input);ClosedEffects.mark();if(fail)throw ClosedEffects.failure;}
 public int read(int n){return n;}public long readWide(long w){return w;}public Object readRef(Object input){return input;}`
	for _, level := range []string{"Middle", "Further"} {
		t.Run(level, func(t *testing.T) {
			f := strings.ReplaceAll(closedCalleeFixture, "CALL", "prepare(n,w,input,fail)")
			f = strings.ReplaceAll(f, "METHOD", method)
			f = strings.ReplaceAll(f, "RESULT_CHECK", "p.result!=0||p.reference!=null")
			f = strings.ReplaceAll(f, "Child extends ClosedParent", "Child extends Closed"+level)
			f += `class ClosedMiddle extends ClosedParent{ClosedMiddle(int n,long w,Object input,boolean fail){super(n,w,input,fail);}long read(long n){return n^17;}void prepare(double n){throw new AssertionError("wrong overload");}}
class ClosedFurther extends ClosedMiddle{ClosedFurther(int n,long w,Object input,boolean fail){super(n,w,input,fail);}Object read(String n){return n;}}`
			testIndependentFlatClosedCalleeFamily(t, f, "Closed", "320:closed:method:capture:effects\n", []string{"ClosedOwner", "ClosedOwner$Child"})
		})
	}
}

func TestNativeConstructorFinalReceiverRetainsOriginalFamilyShape(t *testing.T) {
	for _, row := range closedCalleeBanks {
		t.Run(row.name, func(t *testing.T) {
			method := strings.ReplaceAll(strings.ReplaceAll(row.method, "private ", "public "), "final ", "protected ")
			f := strings.ReplaceAll(closedCalleeFixture, "CALL", row.call)
			f = strings.ReplaceAll(f, "METHOD", method)
			f = strings.ReplaceAll(f, "RESULT_CHECK", row.check)
			testNativeIndependentFamilyFixture(t, f, []string{"ClosedOwner"}, "ClosedDriver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
		})
	}
}
