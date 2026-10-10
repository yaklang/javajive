package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedConstructorBridgeNameTypeKeepsOwnerIdentityRoundTrip(t *testing.T) {
	for _, owner := range []string{"SharedBridgeOwner", "RenamedSharedBridgeOwner"} {
		for _, scope := range []string{"instance", "static"} {
			t.Run(owner+"/"+scope, func(t *testing.T) {
				source := `class SharedBridgeOwner<T>{final Object origin=new Object();
 class First<U> extends SharedBridgeBase {final U token;private First(U t,long n){super(n);token=t;SharedBridgeEffects.trace+="A";}Object enclosing(){return SharedBridgeOwner.this.origin;}int kind(){return 1;}}
 class Second<U> extends SharedBridgeBase {final U token;private Second(U t,long n){super(n);token=t;SharedBridgeEffects.trace+="B";}Object enclosing(){return SharedBridgeOwner.this.origin;}int kind(){return 2;}}
 First<T> first(T t,long n){return new First<T>(t,SharedBridgeEffects.argument(n));}Second<T> second(T t,long n){return new Second<T>(t,SharedBridgeEffects.argument(n));}}
class SharedBridgeEffects{static String trace;static int fail;static Object published;static final RuntimeException error=new RuntimeException("bridge identity");static final Object staticOrigin=new Object();static long argument(long n){trace+="F";return n;}}
abstract class SharedBridgeBase{final Object seen;final int tag;final long n;SharedBridgeBase(long x){SharedBridgeEffects.trace+="P";SharedBridgeEffects.published=this;seen=enclosing();tag=kind();n=x*31+7;if(SharedBridgeEffects.fail==tag)throw SharedBridgeEffects.error;}abstract Object enclosing();abstract int kind();}
class SharedBridgeDriver{public static void main(String[]args)throws Exception{int rows=0;SharedBridgeOwner<Object> owner=new SharedBridgeOwner<Object>();Object token=new Object();for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object input:new Object[]{null,token})for(int which:new int[]{1,2})for(int fail:new int[]{0,1,2}){SharedBridgeEffects.trace="";SharedBridgeEffects.fail=fail;SharedBridgeEffects.published=null;try{SharedBridgeBase first=which==1?owner.first(input,n):owner.second(input,n);Object value=which==1?((SharedBridgeOwner.First)first).token:((SharedBridgeOwner.Second)first).token;long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).longValue();if(fail==which||first.seen!=owner.origin||first.tag!=which||first.n!=expected||value!=input||SharedBridgeEffects.published!=first||!SharedBridgeEffects.trace.equals(which==1?"FPA":"FPB"))throw new AssertionError("original owner/callback/overload/effects");SharedBridgeBase second=which==1?owner.first(input,n):owner.second(input,n);if(second==first||second.seen!=first.seen||second.tag!=which||!SharedBridgeEffects.trace.equals(which==1?"FPAFPA":"FPBFPB"))throw new AssertionError("repeat allocation identity");}catch(RuntimeException e){SharedBridgeBase partial=(SharedBridgeBase)SharedBridgeEffects.published;if(fail!=which||e!=SharedBridgeEffects.error||partial==null||partial.seen!=owner.origin||partial.tag!=which||!SharedBridgeEffects.trace.equals("FP")||(which==1?((SharedBridgeOwner.First)partial).token:((SharedBridgeOwner.Second)partial).token)!=null)throw new AssertionError("partial initialized identity",e);}rows++;}for(Class<?> child:new Class<?>[]{SharedBridgeOwner.First.class,SharedBridgeOwner.Second.class}){int privateCount=0,syntheticCount=0;for(java.lang.reflect.Constructor<?> c:child.getDeclaredConstructors()){if(java.lang.reflect.Modifier.isPrivate(c.getModifiers()))privateCount++;if(c.isSynthetic())syntheticCount++;}if(privateCount!=1||syntheticCount!=1)throw new AssertionError("private/bridge shape");}System.out.println(rows+":shared-name-type:owner:effects:identity");}}`
				if scope == "static" {
					source = strings.ReplaceAll(source, " class First<U>", " static class First<U>")
					source = strings.ReplaceAll(source, " class Second<U>", " static class Second<U>")
					source = strings.ReplaceAll(source, "return SharedBridgeOwner.this.origin;", "return SharedBridgeEffects.staticOrigin;")
					source = strings.ReplaceAll(source, "first.seen!=owner.origin", "first.seen!=SharedBridgeEffects.staticOrigin")
					source = strings.ReplaceAll(source, "partial.seen!=owner.origin", "partial.seen!=SharedBridgeEffects.staticOrigin")
				}
				source = strings.ReplaceAll(source, "SharedBridgeOwner", owner)
				testNativePrivateSetterFixture(t, source, owner, "SharedBridgeDriver", "60:shared-name-type:owner:effects:identity\n")
			})
		}
	}
}
