package javaclassparser

import (
	"strings"
	"testing"
)

// A fresh closure can retain THIS without publishing it: the parent stores the
// closure only in its own field and never invokes it during construction. The
// unchanged binary driver invokes it afterward, so it must still dispatch to
// the child's real capture. Allocation/retention is distinct from an escape.
func TestAdversarialConstructorConfinedCaptureRoundTrip(t *testing.T) {
	f := strings.Replace(constructorUnknownAbruptCaptureFixture, "final int n;", "final int n;final java.util.function.Supplier<Object> action;Object capture(){return null;}", 1)
	f = strings.Replace(f, "this.n=n;trace=trace*31+2;", "this.n=n;action=new java.util.function.Supplier<Object>(){public Object get(){return capture();}};trace=trace*31+2;", 1)
	f = strings.Replace(f, "this.value=value;}\n}", "this.value=value;}Object capture(){return value;}\n}", 1)
	f = strings.Replace(f, "c.value!=value||AbruptParent.trace!=33", "c.value!=value||c.action.get()!=value||AbruptParent.trace!=33", 1)
	for _, prefix := range []string{"Confined", "Deferred"} {
		t.Run(prefix, func(t *testing.T) {
			testConstructorUnknownInputAbruptCaptureRoundTrip(t, strings.ReplaceAll(f, "Abrupt", prefix), prefix)
		})
	}
}
