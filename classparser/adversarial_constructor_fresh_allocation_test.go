package javaclassparser

import "testing"

const constructorFreshAllocationFixture = `
class FreshEffects {String trace="";final RuntimeException failure=new IllegalArgumentException("side");void parent(){trace+="P";}}
class FreshSide {final long value;FreshSide(FreshEffects effects,long n){effects.trace+="A";if(n<0)throw effects.failure;value=n;}}
class FreshParent {final Object side;FreshParent(FreshEffects effects,long n){side=new FreshSide(effects,n);effects.parent();}}
class FreshBeforeParent {final Object side;FreshBeforeParent(Object side){this.side=side;}}
class FreshBefore extends FreshBeforeParent {FreshBefore(FreshEffects effects,long n){super(new FreshSide(effects,n));}}
class FreshOwner {final Object token;FreshOwner(Object x){token=x;}final class Child extends FreshParent {Child(FreshEffects e,long n){super(e,n);}Object capture(){return FreshOwner.this.token;}}FreshParent make(FreshEffects e,long n){return new Child(e,n);}}
class FreshOracle {static void run(){Object token=new Object();int rows=0;for(Object t:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){FreshEffects e=new FreshEffects();try{FreshParent p=new FreshOwner(t).make(e,n);if(n<0||!e.trace.equals("AP")||((FreshSide)p.side).value!=n||((FreshOwner.Child)p).capture()!=t)throw new AssertionError("fresh allocation/capture");}catch(RuntimeException x){if(n>=0||x!=e.failure||!e.trace.equals("A"))throw new AssertionError("failure identity/order",x);}FreshEffects b=new FreshEffects();try{FreshBefore p=new FreshBefore(b,n);if(n<0||!b.trace.equals("A")||((FreshSide)p.side).value!=n)throw new AssertionError("before main init");}catch(RuntimeException x){if(n>=0||x!=b.failure||!b.trace.equals("A"))throw new AssertionError("before failure",x);}rows++;}System.out.println(rows);}}
public class FreshDriver {public static void main(String[]args){FreshOracle.run();}}
`

func TestAdversarialConstructorFreshAllocationKeepsFailureAndCaptureOrder(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "FreshDriver", constructorFreshAllocationFixture, nil, []string{"FreshOwner", "FreshOwner$Child"}, true, Precision, Compatibility, "legacy")
}
