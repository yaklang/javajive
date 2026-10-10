package javaclassparser

import (
	"strings"
	"testing"
)

func testNativeAnonymousNestedInitializerStore(t *testing.T, renamed, volatile bool, collision ...bool) {
	t.Helper()
	f := nativeAnonymousRepeatedStores
	f = strings.Replace(f, "Object saved=RepeatedInitEffects.item(value);{stage=RepeatedInitEffects.number(stage+1);saved=RepeatedInitEffects.item(saved);stage=RepeatedInitEffects.number(stage+1);}", "Object saved=RepeatedInitEffects.item(value);int seen;Object copied;{seen=(stage=RepeatedInitEffects.number(stage+1));copied=(saved=RepeatedInitEffects.item(saved));stage=RepeatedInitEffects.number(stage+1);}", 1)
	f = strings.Replace(f, "main(String[]args){", "main(String[]args)throws Exception{", 1)
	f = strings.Replace(f, "rows++;", `RepeatedInitParent p=RepeatedInitEffects.published;java.lang.reflect.Field seen=p.getClass().getDeclaredField("seen"),copied=p.getClass().getDeclaredField("copied");seen.setAccessible(true);copied.setAccessible(true);if(seen.getInt(p)!=(fail==1?0:18)||copied.get(p)!=(fail==0?value:null))throw new AssertionError("nested assignment result/partial store order");rows++;`, 1)
	owner := "RepeatedInitOwner"
	if volatile {
		f = strings.Replace(f, "int stage=", "volatile int stage=", 1)
		f = strings.Replace(f, "Object saved=", "volatile Object saved=", 1)
	}
	if renamed {
		owner = "NestedIndependentScope"
		f = strings.NewReplacer("RepeatedInitOwner", owner, "stage", "phase", "saved", "kept", "seen", "observed", "copied", "copy").Replace(f)
	}
	if len(collision) > 0 && collision[0] {
		owner = "$jdec$stack2"
		f = strings.NewReplacer("RepeatedInitOwner", owner, "value", "$jdec$stack1", "stage", "$jdec$stack3").Replace(f)
	}
	testNativePrivateSetterFixture(t, f, owner, "RepeatedInitDriver", "6:repeated:stores:partial:identity:order\n")
}
func TestNativeAnonymousInitializerNestedFieldStoreRoundTrip(t *testing.T) {
	testNativeAnonymousNestedInitializerStore(t, false, false)
}
func TestNativeAnonymousInitializerNestedVolatileFieldStoreRoundTrip(t *testing.T) {
	testNativeAnonymousNestedInitializerStore(t, false, true)
}
func TestNativeAnonymousInitializerNestedFieldStoreRenamedRoundTrip(t *testing.T) {
	testNativeAnonymousNestedInitializerStore(t, true, false)
}

func TestNativeAnonymousInitializerNestedStoreAvoidsCaptureClassAndFieldShadowing(t *testing.T) {
	testNativeAnonymousNestedInitializerStore(t, false, false, true)
}

func TestNativeAnonymousInitializerNestedWideStoreRoundTrip(t *testing.T) {
	const f = `class WideRepeatedEffects {static String trace="";static WideRepeatedParent published;static int calls,fail;static final RuntimeException error=new RuntimeException("original");static long next(long n){trace+="N";if(published.value()!=n)throw new AssertionError("actual current field before next write");if(++calls==fail)throw error;return n+1;}}
abstract class WideRepeatedParent {WideRepeatedParent(){if(value()!=0||mirror()!=0)throw new AssertionError("default fields during parent");WideRepeatedEffects.trace+="P";WideRepeatedEffects.published=this;}abstract long value();abstract long mirror();}
class WideRepeatedOwner {WideRepeatedParent make(final long seed){return new WideRepeatedParent(){volatile long current=seed;long previous;{previous=(current=WideRepeatedEffects.next(current));current=WideRepeatedEffects.next(current);}long value(){return current;}long mirror(){return previous;}};}}
class WideRepeatedDriver {public static void main(String[]args){int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE})for(int fail:new int[]{0,1,2}){WideRepeatedEffects.trace="";WideRepeatedEffects.calls=0;WideRepeatedEffects.fail=fail;WideRepeatedEffects.published=null;try{WideRepeatedParent p=new WideRepeatedOwner().make(seed);if(fail!=0||p.value()!=seed+2||p.mirror()!=seed+1||!WideRepeatedEffects.trace.equals("PNN"))throw new AssertionError("wide repeated stores");}catch(RuntimeException e){WideRepeatedParent p=WideRepeatedEffects.published;if(e!=WideRepeatedEffects.error||p==null||p.value()!=(fail==1?seed:seed+1)||p.mirror()!=(fail==1?0:seed+1)||!WideRepeatedEffects.trace.equals(fail==1?"PN":"PNN"))throw new AssertionError("wide partial write/order/overflow");}rows++;}System.out.println(rows+":wide:repeated:stores:partial:overflow");}}
`
	testNativePrivateSetterFixture(t, f, "WideRepeatedOwner", "WideRepeatedDriver", "12:wide:repeated:stores:partial:overflow\n")
}

func TestNativeAnonymousInitializerNestedLiteralStoreRoundTrip(t *testing.T) {
	const f = `abstract class LiteralNestedParent {LiteralNestedParent(){if(number()!=0)throw new AssertionError("default fields before stores");}abstract int number();}
class LiteralNestedOwner {LiteralNestedParent make(){return new LiteralNestedParent(){int left,right;{right=(left=17);left=19;}int number(){return left*100+right;}};}}
class LiteralNestedDriver {public static void main(String[]args){LiteralNestedParent p=new LiteralNestedOwner().make();if(p.number()!=1917)throw new AssertionError("nested literal result and writes");System.out.println("1917:nested:literal");}}`
	testNativePrivateSetterFixture(t, f, "LiteralNestedOwner", "LiteralNestedDriver", "1917:nested:literal\n")
}
