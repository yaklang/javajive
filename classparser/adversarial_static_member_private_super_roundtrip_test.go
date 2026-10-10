package javaclassparser

import (
	"strings"
	"testing"
)

// A static lexical member still owns a private SUPER bridge. Its marker can
// be the same real anonymous type used elsewhere in the family. Only the
// original initial THIS packet may erase that unused marker; javac regenerates
// both the private delegation and the anonymous captures before the callback.
func TestAdversarialStaticMemberPrivateSuperComposesAnonymousCaptureRoundTrip(t *testing.T) {
	for _, owner := range []string{"StaticSuperOwner", "RenamedStaticSuperOwner"} {
		for _, layout := range []string{"wide-overload", "no-arguments"} {
			t.Run(owner+"/"+layout, func(t *testing.T) {
				fixture := `class StaticSuperOwner {
 int seed;StaticSuperOwner(int n){seed=n;}
 static class Parent{final long wide;final Object token;private Parent(long n,Object t){StaticSuperEffects.trace+="P";wide=n;token=t;if(StaticSuperEffects.fail)throw StaticSuperEffects.failure;}private Parent(Object t,long n){throw new AssertionError("wrong overload");}}
 static final class Child extends Parent{Child(long n,Object t){super(n,t);StaticSuperEffects.trace+="C";}}
 OpaqueSuper make(){return new OpaqueSuper("label",true,7){int value(){return StaticSuperOwner.this.seed;}};}
}
class StaticSuperEffects {static String trace;static boolean fail;static long wide;static Object token;static int observed;static final RuntimeException failure=new RuntimeException("parent identity"),anonymousFailure=new RuntimeException("anonymous identity");}
class OpaqueSuper {OpaqueSuper(String label,boolean flag,int n){if(!label.equals("label")||!flag||n!=7)throw new AssertionError("super parameter words");StaticSuperEffects.trace+="A";StaticSuperEffects.observed=value();if(StaticSuperEffects.fail)throw StaticSuperEffects.anonymousFailure;}int value(){return 0;}}
class StaticSuperDriver {public static void main(String[]args){int rows=0;Object token=new Object();for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object input:new Object[]{null,token})for(boolean fail:new boolean[]{false,true}){
 long wide=((long)n)*0x100000001L;StaticSuperEffects.wide=wide;StaticSuperEffects.token=input;StaticSuperEffects.fail=fail;StaticSuperEffects.trace="";
 try{StaticSuperOwner.Child child=new StaticSuperOwner.Child(wide,input);if(fail||child.wide!=wide||child.token!=input||!StaticSuperEffects.trace.equals("PC"))throw new AssertionError("private super dispatch/words/effects");}catch(RuntimeException e){if(!fail||e!=StaticSuperEffects.failure||!StaticSuperEffects.trace.equals("P"))throw new AssertionError("parent failure identity/order",e);}
 StaticSuperOwner root=new StaticSuperOwner(n);StaticSuperEffects.trace="";StaticSuperEffects.observed=~n;
 try{OpaqueSuper first=root.make();if(fail||first.value()!=n||StaticSuperEffects.observed!=n||!StaticSuperEffects.trace.equals("A"))throw new AssertionError("pre-super capture callback");root.seed+=101;if(first.value()!=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(101)).intValue())throw new AssertionError("live enclosing heap");StaticSuperEffects.trace="";OpaqueSuper second=root.make();if(first==second||second.value()!=root.seed||StaticSuperEffects.observed!=root.seed||!StaticSuperEffects.trace.equals("A"))throw new AssertionError("repeated allocation/callback identity");}catch(RuntimeException e){if(!fail||e!=StaticSuperEffects.anonymousFailure||StaticSuperEffects.observed!=n||!StaticSuperEffects.trace.equals("A"))throw new AssertionError("anonymous capture/failure order",e);}
 rows++;}System.out.println(rows+":static:private-super:capture:effects:identity");}}
`
				if layout == "no-arguments" {
					fixture = strings.Replace(fixture, "private Parent(long n,Object t)", "private Parent()", 1)
					fixture = strings.Replace(fixture, "wide=n;token=t;", "wide=StaticSuperEffects.wide;token=StaticSuperEffects.token;", 1)
					fixture = strings.Replace(fixture, "Child(long n,Object t){super(n,t);", "Child(){super();", 1)
					fixture = strings.Replace(fixture, "new StaticSuperOwner.Child(wide,input)", "new StaticSuperOwner.Child()", 1)
				}
				fixture = strings.ReplaceAll(fixture, "StaticSuperOwner", owner)
				testNativePrivateSetterFixture(t, fixture, owner, "StaticSuperDriver", "20:static:private-super:capture:effects:identity\n")
			})
		}
	}
}
