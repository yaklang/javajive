package javaclassparser

import (
	"strings"
	"testing"
)

// Same nominal owner does not identify the allocation receiver. Both nested
// constructors and the surviving outer result must keep their original NEWs.
func TestAdversarialPrivateRootNestedAllocationOriginsRoundTrip(t *testing.T) {
	for _, profile := range []string{"other owned argument", "same root argument"} {
		fixture := nativeNonstaticPrivateRootSuperFixture
		trace := "PP"
		switch profile {
		case "other owned argument":
			fixture = strings.ReplaceAll(fixture, "class CapturedRootSuper{", "class CapturedRootSuper{static class Payload{final Object token;Payload(Object token){RootSuperEffects.trace+=\"B\";this.token=token;}}")
			fixture = strings.ReplaceAll(fixture, "return new CapturedRootSuper(0L,token);", "return new CapturedRootSuper(0L,new Payload(token).token);")
			trace = "BP"
		case "same root argument":
			fixture = strings.ReplaceAll(fixture, "return new CapturedRootSuper(0L,token);", "return new CapturedRootSuper(0L,new CapturedRootSuper(1L,token).token);")
		}
		fixture = strings.ReplaceAll(fixture, "RootSuperEffects.fail=0;CapturedRootSuper outer=", "RootSuperEffects.fail=0;RootSuperEffects.trace=\"\";CapturedRootSuper outer=")
		fixture = strings.ReplaceAll(fixture, "CapturedRootSuper.outer(outside);RootSuperEffects.trace=", "CapturedRootSuper.outer(outside);if(!RootSuperEffects.trace.equals(\""+trace+"\")||RootSuperEffects.published!=outer||outer.token!=outside)throw new AssertionError(\"nested allocation order/receiver identity\");RootSuperEffects.trace=")
		t.Run(profile, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{"CapturedRootSuper"}, "CapturedRootSuperDriver", "160:nonstatic:private-root:wide:callback:identity:failure\n", nativeLexicalExactSignatures)
		})
	}
}

const nativeNestedPrivateBridgeFixture = `class NestedBridgeEffects{static String trace="";static int count,fail;static Object published;static final RuntimeException error=new RuntimeException("original identity");}
class NestedBridgeRoot{final Object token;private NestedBridgeRoot(Object token){NestedBridgeEffects.trace+="P";NestedBridgeEffects.published=this;if(++NestedBridgeEffects.count==NestedBridgeEffects.fail)throw NestedBridgeEffects.error;this.token=token;}static class Caller{static NestedBridgeRoot make(Object token){NestedBridgeEffects.trace+="S";return new NestedBridgeRoot(new NestedBridgeRoot(token));}}}
class NestedBridgeDriver{public static void main(String[]args){Object token=new Object();int rows=0;for(Object argument:new Object[]{null,token}){NestedBridgeEffects.trace="";NestedBridgeEffects.count=0;NestedBridgeEffects.fail=0;NestedBridgeRoot result=NestedBridgeRoot.Caller.make(argument);if(!(result.token instanceof NestedBridgeRoot)||((NestedBridgeRoot)result.token).token!=argument||result.token==result||NestedBridgeEffects.published!=result||NestedBridgeEffects.count!=2||!NestedBridgeEffects.trace.equals("SPP"))throw new AssertionError("nested origin/order/identity");for(int fail=1;fail<=2;fail++){NestedBridgeEffects.trace="";NestedBridgeEffects.count=0;NestedBridgeEffects.fail=fail;try{NestedBridgeRoot.Caller.make(argument);throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=NestedBridgeEffects.error||NestedBridgeEffects.count!=fail||!NestedBridgeEffects.trace.equals(fail==1?"SP":"SPP")||((NestedBridgeRoot)NestedBridgeEffects.published).token!=null)throw new AssertionError("failure/partial state");}rows++;}}System.out.println(rows+":nested:private-bridge:new-origins:identity:order:failure");}}
`

func TestAdversarialPrivateBridgeNestedSameOwnerAllocationOriginsRoundTrip(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		fixture := nativeNestedPrivateBridgeFixture
		owner := "NestedBridgeRoot"
		driver := "NestedBridgeDriver"
		name := "ordinary"
		if renamed {
			fixture = strings.NewReplacer("NestedBridgeRoot", "IndependentNestedPacket", "NestedBridgeDriver", "IndependentNestedDriver", "Caller", "Factory").Replace(fixture)
			owner = "IndependentNestedPacket"
			driver = "IndependentNestedDriver"
			name = "renamed"
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "4:nested:private-bridge:new-origins:identity:order:failure\n", nativeLexicalExactSignatures)
		})
	}
}
