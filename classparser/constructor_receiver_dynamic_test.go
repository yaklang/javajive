package javaclassparser

import (
	"testing"
)

func TestNativeConstructorReceiverDynamicCallsKeepFreshReceiverBoundary(t *testing.T) {
	const source = `class DynamicEffectBase{int result;DynamicEffectBase(int n){this(n,()->null);}DynamicEffectBase(int n,java.util.function.Supplier<Object> supplier){result=n;supplier.get();}}
 final class DynamicEffectChild extends DynamicEffectBase{Object capture;DynamicEffectChild(int n){super(n);}}
 class DynamicCapturedBase{int result;DynamicCapturedBase(int n){java.util.function.IntSupplier s=()->n;result=s.getAsInt();}}
 final class DynamicCapturedChild extends DynamicCapturedBase{Object capture;DynamicCapturedChild(int n){super(n);}}
 class DynamicPublishBase{Object capture;static Object saved;DynamicPublishBase(int n){java.util.function.Supplier<Object> s=()->this;saved=s.get();}}
 final class DynamicPublishChild extends DynamicPublishBase{Object capture;DynamicPublishChild(int n){super(n);}}
 class DynamicBoundBase{int result;DynamicBoundBase(int n){java.util.function.IntSupplier s=this::hashCode;result=s.getAsInt();}}
 final class DynamicBoundChild extends DynamicBoundBase{Object capture;DynamicBoundChild(int n){super(n);}}`
	files := nativeCompileClasses(t, source)
	resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	for _, tc := range []struct {
		child, base string
		safe        bool
	}{{"DynamicEffectChild", "DynamicEffectBase", true}, {"DynamicCapturedChild", "DynamicCapturedBase", true}, {"DynamicPublishChild", "DynamicPublishBase", false}, {"DynamicBoundChild", "DynamicBoundBase", false}} {
		t.Run(tc.child, func(t *testing.T) {
			obj, e := Parse(files[tc.child+".class"])
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(obj)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = resolve
			writes := map[string]bool{tc.child + "\x00capture\x00Ljava/lang/Object;": true}
			if got := d.constructorCaptureChainDoesNotObserve(tc.base, "(I)V", writes); got != tc.safe {
				t.Fatalf("receiver-free dynamic effect=%v want=%v", got, tc.safe)
			}
		})
	}
}
