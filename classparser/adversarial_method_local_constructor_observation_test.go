package javaclassparser

import (
	"strings"
	"testing"
)

// A local declaration must regenerate capture writes before a superclass
// callback/publication/failure. Moving stores after super is numerically wrong.
func TestAdversarialMethodLocalConstructorPreSuperObservation(t *testing.T) {
	for _, owner := range []string{"LocalCtorObservationOwner", "OtherLocalCtorObservationOwner"} {
		t.Run(owner, func(t *testing.T) {
			source := `class LocalCtorEffects {static int trace,fail;static Object published;static final RuntimeException fault=new RuntimeException("same original");}
abstract class LocalCtorParent {final long observed;final Object identity;LocalCtorParent(){LocalCtorEffects.trace=1;LocalCtorEffects.published=this;observed=read();identity=token();if(LocalCtorEffects.fail!=0)throw LocalCtorEffects.fault;LocalCtorEffects.trace=12;}abstract long read();abstract Object token();}
class LocalCtorObservationOwner {private final long bias;LocalCtorObservationOwner(long bias){this.bias=bias;}LocalCtorParent make(final long seed,final Object token){class Entry extends LocalCtorParent {long read(){return LocalCtorObservationOwner.this.bias+seed;}Object token(){return token;}}return new Entry();}}
class LocalCtorObservationDriver {public static void main(String[] args){Object identity=new Object();int count=0;for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(int fail=0;fail<2;fail++){LocalCtorEffects.trace=0;LocalCtorEffects.fail=fail;LocalCtorEffects.published=null;LocalCtorObservationOwner owner=new LocalCtorObservationOwner(bias);LocalCtorParent value=null;try{value=owner.make(seed,token);if(fail!=0)throw new AssertionError("lost failure");}catch(RuntimeException ex){if(fail==0||ex!=LocalCtorEffects.fault)throw new AssertionError("failure identity");value=(LocalCtorParent)LocalCtorEffects.published;}long want=java.math.BigInteger.valueOf(bias).add(java.math.BigInteger.valueOf(seed)).longValue();if(value==null||value!=LocalCtorEffects.published||value.read()!=want||value.observed!=want||value.token()!=token||value.identity!=token||LocalCtorEffects.trace!=(fail==0?12:1))throw new AssertionError("pre-super capture/callback/publication/overflow/identity/order");count++;}System.out.println(count+":pre-super:callback:publication:failure:overflow:identity");}}`
			source = strings.ReplaceAll(source, "LocalCtorObservationOwner", owner)
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "LocalCtorObservationDriver", "100:pre-super:callback:publication:failure:overflow:identity\n")
		})
	}
}
