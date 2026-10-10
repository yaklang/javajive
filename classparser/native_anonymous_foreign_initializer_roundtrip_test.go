package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousForeignFieldInitializerFixture = `class ForeignInitTrace {static String trace="";static final RuntimeException failure=new RuntimeException("parent");static Object observe(Object value){trace+="I";return value;}}
class ForeignInitHolder {Object value;volatile int number;ForeignInitHolder next;ForeignInitHolder(Object value,int number){this.value=value;this.number=number;}}
abstract class ForeignInitParent {ForeignInitParent(ForeignInitHolder h){ForeignInitTrace.trace+="P";if(view()!=null)throw new AssertionError("parent observes default");if(h==null)throw ForeignInitTrace.failure;}abstract Object view();abstract int number();}
class ForeignInitOwner {ForeignInitParent make(ForeignInitHolder holder){return new ForeignInitParent(holder){Object kept=ForeignInitTrace.observe(holder.next.value);int read=holder.number;Object view(){return kept;}int number(){return read;}};}}
class ForeignInitDriver {public static void main(String[] args){Object token=new Object();for(Object value:new Object[]{null,token})for(int number:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){ForeignInitHolder holder=new ForeignInitHolder(token,number);holder.next=new ForeignInitHolder(value,0);ForeignInitTrace.trace="";ForeignInitParent p=new ForeignInitOwner().make(holder);if(p.view()!=value||p.number()!=number||!ForeignInitTrace.trace.equals("PI")||!p.getClass().getName().equals("ForeignInitOwner$1"))throw new AssertionError("actual foreign fields/volatile/identity/order");}ForeignInitTrace.trace="";try{new ForeignInitOwner().make(null);throw new AssertionError("missing parent failure");}catch(RuntimeException failure){if(failure!=ForeignInitTrace.failure||!ForeignInitTrace.trace.equals("P"))throw new AssertionError("parent failure order/identity");}ForeignInitTrace.trace="";try{new ForeignInitOwner().make(new ForeignInitHolder(token,0));throw new AssertionError("missing null receiver");}catch(NullPointerException failure){if(!ForeignInitTrace.trace.equals("P"))throw new AssertionError("foreign dereference ahead of observe");}System.out.println("6:foreign:field:identity:volatile:order:null");}}
`

func TestNativeAnonymousInitializerForeignFieldChainRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousForeignFieldInitializerFixture, "ForeignInitOwner", "ForeignInitDriver", "6:foreign:field:identity:volatile:order:null\n")
}
func TestNativeAnonymousInitializerForeignFieldChainRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(strings.ReplaceAll(nativeAnonymousForeignFieldInitializerFixture, "ForeignInitOwner", "IndependentFieldReadScope"), "ForeignInitHolder", "DifferentReadType")
	testNativePrivateSetterFixture(t, f, "IndependentFieldReadScope", "ForeignInitDriver", "6:foreign:field:identity:volatile:order:null\n")
}
func TestNativeAnonymousInitializerForeignFieldParentMutationRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousForeignFieldInitializerFixture, `if(h==null)throw ForeignInitTrace.failure;`, `if(h==null)throw ForeignInitTrace.failure;if(h.next!=null)h.next.value=h.value;`, 1)
	f = strings.Replace(f, `p.view()!=value`, `p.view()!=token`, 1)
	testNativePrivateSetterFixture(t, f, "ForeignInitOwner", "ForeignInitDriver", "6:foreign:field:identity:volatile:order:null\n")
}
