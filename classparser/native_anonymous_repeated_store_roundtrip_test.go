package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousRepeatedStores = `class RepeatedInitEffects {static String trace="";static RepeatedInitParent published;static int calls,fail;static final RuntimeException error=new RuntimeException("original");static int number(int value){trace+="N"+value;return value;}static Object item(Object value){trace+="V";if(published.number()!=(calls==0?17:18))throw new AssertionError("callback observes each preceding write");if(++calls==fail)throw error;return value;}}
abstract class RepeatedInitParent {RepeatedInitParent(){RepeatedInitEffects.trace+="P"+number();RepeatedInitEffects.published=this;}abstract int number();abstract Object item();}
class RepeatedInitOwner {RepeatedInitParent make(final Object value){return new RepeatedInitParent(){int stage=RepeatedInitEffects.number(17);Object saved=RepeatedInitEffects.item(value);{stage=RepeatedInitEffects.number(stage+1);saved=RepeatedInitEffects.item(saved);stage=RepeatedInitEffects.number(stage+1);}int number(){return stage;}Object item(){return saved;}};}}
class RepeatedInitDriver {public static void main(String[]args){int rows=0;for(Object value:new Object[]{null,new Object()})for(int fail:new int[]{0,1,2}){RepeatedInitEffects.trace="";RepeatedInitEffects.calls=0;RepeatedInitEffects.fail=fail;RepeatedInitEffects.published=null;try{RepeatedInitParent p=new RepeatedInitOwner().make(value);if(fail!=0||p.number()!=19||p.item()!=value||!RepeatedInitEffects.trace.equals("P0N17VN18VN19"))throw new AssertionError("ordered repeated writes");}catch(RuntimeException e){RepeatedInitParent p=RepeatedInitEffects.published;if(e!=RepeatedInitEffects.error||p==null||p.number()!=(fail==1?17:18)||p.item()!=(fail==1?null:value)||!RepeatedInitEffects.trace.equals(fail==1?"P0N17V":"P0N17VN18V"))throw new AssertionError("partial publication/failure/identity");}rows++;}System.out.println(rows+":repeated:stores:partial:identity:order");}}
`

func TestNativeAnonymousInitializerRepeatedOwnStoresRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousRepeatedStores, "RepeatedInitOwner", "RepeatedInitDriver", "6:repeated:stores:partial:identity:order\n")
}
func TestNativeAnonymousInitializerRepeatedVolatileStoresRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousRepeatedStores, "int stage=", "volatile int stage=", 1)
	f = strings.Replace(f, "Object saved=", "volatile Object saved=", 1)
	testNativePrivateSetterFixture(t, f, "RepeatedInitOwner", "RepeatedInitDriver", "6:repeated:stores:partial:identity:order\n")
}
func TestNativeAnonymousInitializerRepeatedStoresRenamedRoundTrip(t *testing.T) {
	f := strings.NewReplacer("RepeatedInitOwner", "IndependentMutableInitializer", "stage", "phase", "saved", "kept").Replace(nativeAnonymousRepeatedStores)
	testNativePrivateSetterFixture(t, f, "IndependentMutableInitializer", "RepeatedInitDriver", "6:repeated:stores:partial:identity:order\n")
}

func TestNativeAnonymousInitializerRepeatedWideStoresRoundTrip(t *testing.T) {
	const f = `class WideRepeatedEffects {static String trace="";static WideRepeatedParent published;static int calls,fail;static final RuntimeException error=new RuntimeException("original");static long next(long n){trace+="N";if(published.value()!=n)throw new AssertionError("actual current field before next write");if(++calls==fail)throw error;return n+1;}}
abstract class WideRepeatedParent {WideRepeatedParent(){if(value()!=0||mirror()!=0)throw new AssertionError("default fields during parent");WideRepeatedEffects.trace+="P";WideRepeatedEffects.published=this;}abstract long value();abstract long mirror();}
class WideRepeatedOwner {WideRepeatedParent make(final long seed){return new WideRepeatedParent(){volatile long current=seed;long previous;{current=WideRepeatedEffects.next(current);previous=current;current=WideRepeatedEffects.next(current);}long value(){return current;}long mirror(){return previous;}};}}
class WideRepeatedDriver {public static void main(String[]args){int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE})for(int fail:new int[]{0,1,2}){WideRepeatedEffects.trace="";WideRepeatedEffects.calls=0;WideRepeatedEffects.fail=fail;WideRepeatedEffects.published=null;try{WideRepeatedParent p=new WideRepeatedOwner().make(seed);if(fail!=0||p.value()!=seed+2||p.mirror()!=seed+1||!WideRepeatedEffects.trace.equals("PNN"))throw new AssertionError("wide repeated stores");}catch(RuntimeException e){WideRepeatedParent p=WideRepeatedEffects.published;if(e!=WideRepeatedEffects.error||p==null||p.value()!=(fail==1?seed:seed+1)||p.mirror()!=(fail==1?0:seed+1)||!WideRepeatedEffects.trace.equals(fail==1?"PN":"PNN"))throw new AssertionError("wide partial write/order/overflow");}rows++;}System.out.println(rows+":wide:repeated:stores:partial:overflow");}}
`
	testNativePrivateSetterFixture(t, f, "WideRepeatedOwner", "WideRepeatedDriver", "12:wide:repeated:stores:partial:overflow\n")
}
