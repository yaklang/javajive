package javaclassparser

import "testing"

const privateMemberSuperRegistrationFixture = `class PrivateSuperEffects {static String trace="";static Object published;static boolean fail;static final RuntimeException error=new RuntimeException("same");static long arg(long n){trace+="A";return n;}}
class PrivateSuperOwner {private final long word;private final Object token;PrivateSuperOwner(long word,Object token){this.word=word;this.token=token;}
 abstract class Base {final Object observed;final long n;private Base(long n){PrivateSuperEffects.trace+="P";PrivateSuperEffects.published=this;observed=observe();this.n=n;if(PrivateSuperEffects.fail)throw PrivateSuperEffects.error;}abstract Object observe();}
 class Child extends Base {final long result;Child(long n){super(PrivateSuperEffects.arg(n));result=word+n;PrivateSuperEffects.trace+="C";}Object observe(){return PrivateSuperOwner.this.token;}long evaluate(long delta){return word+delta;}}
 Child make(long n){return new Child(n);}
}
class PrivateSuperDriver {public static void main(String[]args){Object identity=new Object();int rows=0;for(Object token:new Object[]{null,identity})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){PrivateSuperOwner owner=new PrivateSuperOwner(seed,token);PrivateSuperEffects.trace="";PrivateSuperEffects.published=null;PrivateSuperEffects.fail=fail;try{PrivateSuperOwner.Child child=owner.make(n);long expected=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(n)).longValue();if(fail||child.observed!=token||child.observe()!=token||child.result!=expected||child.evaluate(n)!=expected||child.n!=n||child!=PrivateSuperEffects.published||!PrivateSuperEffects.trace.equals("APC"))throw new AssertionError("private super/accessor/identity/overflow");}catch(RuntimeException error){PrivateSuperOwner.Child child=(PrivateSuperOwner.Child)PrivateSuperEffects.published;if(!fail||error!=PrivateSuperEffects.error||child==null||child.observed!=token||child.result!=0||child.n!=n||!PrivateSuperEffects.trace.equals("AP"))throw new AssertionError("abrupt constructor order",error);}rows++;}System.out.println(rows+":private-super:registration:callback:word:identity");}}
`

func TestAdversarialPrivateMemberSuperAccessorRegistrationRoundTrip(t *testing.T) {
	testSourceTargetOriginalFamilyFixture(t, privateMemberSuperRegistrationFixture, "PrivateSuperOwner", "PrivateSuperDriver", "100:private-super:registration:callback:word:identity\n")
}
