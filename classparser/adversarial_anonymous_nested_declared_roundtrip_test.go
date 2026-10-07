package javaclassparser

import "testing"

func TestAdversarialAnonymousNestedDeclaredRunKeepsCaptureEffectsAndFailureRoundTrip(t *testing.T) {
	const fixture = `class NestedTrace{static String trace="";static int calls;static boolean fail;static Object seen;static final RuntimeException failure=new RuntimeException("original");}
class NestedOwner{static Runnable make(final Object token){return new Runnable(){public void run(){NestedTrace.trace+="O";new Runnable(){public void run(){NestedTrace.calls++;NestedTrace.trace+="I";if(NestedTrace.fail)throw NestedTrace.failure;NestedTrace.seen=token;}}.run();NestedTrace.trace+="D";}};}}
class NestedDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()})for(boolean fail:new boolean[]{false,true}){Runnable r=NestedOwner.make(token);Object sentinel=new Object();NestedTrace.trace="";NestedTrace.calls=0;NestedTrace.seen=sentinel;NestedTrace.fail=fail;try{r.run();if(fail||NestedTrace.seen!=token)throw new AssertionError("capture identity");}catch(RuntimeException e){if(!fail||e!=NestedTrace.failure||NestedTrace.seen!=sentinel)throw new AssertionError("failure identity",e);}if(NestedTrace.calls!=1||!NestedTrace.trace.equals(fail?"OI":"OID"))throw new AssertionError("nested dispatch once/order");rows++;}System.out.println(rows+":nested:declared:run:identity:effects:failure");}}`
	testSourceTargetReleaseFamilyFixture(t, fixture, "NestedOwner", "NestedDriver", "4:nested:declared:run:identity:effects:failure\n", "8", []int{8, 11, 16})
}
